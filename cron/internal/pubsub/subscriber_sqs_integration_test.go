// Copyright 2026 OpenSSF Scorecard Authors
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//      http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

//go:build integration

// This file needs a real SQS queue and AWS credentials, so it is tagged out of
// the default build. The unit tests cover the heartbeat's logic against mocks;
// what they structurally cannot cover is whether the gocloud driver hands back
// the shapes this package expects. As() conversions either match the driver at
// runtime or they do not, and a mock always agrees with itself.
//
//	SCORECARD_SQS_TEST_QUEUE_URL='awssqs://sqs.<region>.amazonaws.com/<account>/<queue>?region=<region>' \
//	  go test -tags=integration ./cron/internal/pubsub/ -run TestSQSRoundTrip -v
package pubsub

import (
	"context"
	"fmt"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"

	"github.com/ossf/scorecard-infra/cron/data"
)

const testQueueURLEnv = "SCORECARD_SQS_TEST_QUEUE_URL"

// Short enough to observe several renewals inside the test, long enough that
// a slow round trip to SQS does not starve them.
const (
	integrationExtension = 6 * time.Second
	integrationGrace     = 4 * time.Second
	integrationHold      = 7 * time.Second
)

// recordingVisibilityChanger delegates to the real SQS client and records what
// came back, so the test can assert SQS actually accepted the renewals rather
// than that the code merely attempted them. extendVisibility only logs errors,
// by design, so without this the renewals would be invisible to the test.
type recordingVisibilityChanger struct {
	inner visibilityChanger
	errs  []error
	calls int
	mu    sync.Mutex
}

func (r *recordingVisibilityChanger) ChangeMessageVisibility(
	ctx context.Context,
	params *sqs.ChangeMessageVisibilityInput,
	optFns ...func(*sqs.Options),
) (*sqs.ChangeMessageVisibilityOutput, error) {
	out, err := r.inner.ChangeMessageVisibility(ctx, params, optFns...)
	r.mu.Lock()
	defer r.mu.Unlock()
	r.calls++
	if err != nil {
		r.errs = append(r.errs, err)
	}
	return out, err
}

func (r *recordingVisibilityChanger) result() (int, []error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls, append([]error(nil), r.errs...)
}

func queueDepth(ctx context.Context, client *sqs.Client, queueURL string) (visible, inFlight string, err error) {
	out, err := client.GetQueueAttributes(ctx, &sqs.GetQueueAttributesInput{
		QueueUrl: aws.String(queueURL),
		AttributeNames: []sqstypes.QueueAttributeName{
			sqstypes.QueueAttributeNameApproximateNumberOfMessages,
			sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible,
		},
	})
	if err != nil {
		return "", "", fmt.Errorf("GetQueueAttributes: %w", err)
	}
	visible = out.Attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessages)]
	inFlight = out.Attributes[string(sqstypes.QueueAttributeNameApproximateNumberOfMessagesNotVisible)]
	return visible, inFlight, nil
}

// TestSQSRoundTrip publishes one message, receives it, holds it past several
// renewal windows, and acks it. Everything it asserts is something a mock
// cannot answer: that the driver's URL form is the one we build, that As()
// yields a client and a receipt handle, that SQS accepts our renewals, and
// that Ack actually deletes.
//
// It deliberately does not call t.Parallel: it drives one shared live queue
// and asserts on that queue's depth, so anything else running against the same
// queue would make the assertions meaningless.
//
//nolint:paralleltest // see above
func TestSQSRoundTrip(t *testing.T) {
	queueURL := os.Getenv(testQueueURLEnv)
	if queueURL == "" {
		t.Skipf("set %s to an awssqs:// URL to run this test", testQueueURLEnv)
	}

	// Not t.Context(): the subscriber holds this for the life of the test and
	// the cleanup below still needs it after the test body returns.
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	marker := fmt.Sprintf("https://example.invalid/sqs-round-trip/%d", time.Now().UnixNano())
	want := &data.ScorecardBatchRequest{Repos: []*data.Repo{{Url: &marker}}}

	// --- publish, through the same path cron/internal/controller uses -------
	publisher, err := CreatePublisher(ctx, queueURL)
	if err != nil {
		t.Fatalf("CreatePublisher: %v", err)
	}
	if err := publisher.Publish(want); err != nil {
		t.Fatalf("Publish: %v", err)
	}
	// Close blocks until the async send completes and surfaces its errors.
	if err := publisher.Close(); err != nil {
		t.Fatalf("publisher.Close: %v", err)
	}
	t.Logf("published marker %s", marker)

	// --- subscribe, through the same path cron/worker uses ------------------
	subscriber, err := CreateSubscriber(ctx, queueURL)
	if err != nil {
		t.Fatalf("CreateSubscriber: %v", err)
	}

	sqsSub, ok := subscriber.(*sqsSubscriber)
	if !ok {
		t.Fatalf("subscriber type: got %T, want *sqsSubscriber", subscriber)
	}

	// Subscription.As gave us a real client, or CreateSubscriber would have
	// failed. Keep it for the depth check, and wrap it so renewals are visible.
	realClient, ok := sqsSub.client.(*sqs.Client)
	if !ok {
		t.Fatalf("subscription client: got %T, want *sqs.Client", sqsSub.client)
	}
	recorder := &recordingVisibilityChanger{inner: realClient}
	sqsSub.client = recorder
	sqsSub.extension = integrationExtension
	sqsSub.grace = integrationGrace

	// defer, not t.Cleanup: cleanup callbacks run after the test function's
	// deferred calls, so the cancel above would already have fired. Registering
	// this later means LIFO closes the subscriber first, then cancels.
	defer func() {
		if err := subscriber.Close(); err != nil {
			t.Errorf("subscriber.Close: %v", err)
		}
	}()

	got, err := subscriber.SynchronousPull()
	if err != nil {
		t.Fatalf("SynchronousPull: %v", err)
	}
	if got == nil {
		t.Fatal("SynchronousPull returned no message; the queue had nothing to deliver")
	}
	if len(got.GetRepos()) != 1 || got.GetRepos()[0].GetUrl() != marker {
		subscriber.Nack()
		t.Fatalf("received a message this test did not publish (%v). "+
			"The queue is not empty; drain it before re-running.", got.GetRepos())
	}

	// --- the assertion mocks cannot make ------------------------------------
	handle, ok := receiptHandleFromMessage(sqsSub.msg)
	if !ok || handle == "" {
		t.Fatalf("receiptHandleFromMessage failed against a real driver message. "+
			"The heartbeat has nothing to extend, so every worker would die on "+
			"its first message. got handle=%q ok=%v", handle, ok)
	}
	t.Logf("recovered receipt handle via As(), %d bytes", len(handle))

	// --- hold it, as a slow scan does ---------------------------------------
	time.Sleep(integrationHold)

	calls, errs := recorder.result()
	if calls == 0 {
		t.Errorf("no visibility renewals in %v with a %v window", integrationHold, integrationExtension)
	}
	for _, err := range errs {
		t.Errorf("SQS rejected a visibility renewal: %v", err)
	}
	t.Logf("SQS accepted %d visibility renewals", calls-len(errs))

	// --- ack, and confirm the message is really gone ------------------------
	subscriber.Ack()

	callsAtAck, _ := recorder.result()
	time.Sleep(integrationExtension)
	if after, _ := recorder.result(); after != callsAtAck {
		t.Errorf("renewals continued after Ack: %d at ack, %d after", callsAtAck, after)
	}

	// Ack deletes asynchronously through gocloud's ack batcher; give it a beat
	// before reading the queue's own view.
	time.Sleep(5 * time.Second)
	visible, inFlight, err := queueDepth(ctx, realClient, sqsSub.queueURL)
	if err != nil {
		t.Fatalf("queueDepth: %v", err)
	}
	if visible != "0" || inFlight != "0" {
		t.Errorf("queue not drained after Ack: %s visible, %s in flight "+
			"(a message left in flight returns after the queue's visibility timeout)",
			visible, inFlight)
	}
}
