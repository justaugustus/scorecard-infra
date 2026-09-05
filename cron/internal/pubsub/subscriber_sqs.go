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

package pubsub

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/url"
	"path"
	"sync"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/sqs"
	sqstypes "github.com/aws/aws-sdk-go-v2/service/sqs/types"
	"github.com/aws/smithy-go"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/awssnssqs"
	"gocloud.dev/pubsub/batcher"

	"github.com/ossf/scorecard-infra/cron/data"
)

const (
	// visibilityExtension and visibilityGracePeriod mirror subscriber_gcs.go's
	// ackDeadlineExtensionInSec and gracePeriodInSec. Those values already run
	// this workload against GCP, so reusing them keeps one number to reason
	// about rather than introducing a second, differently-tuned one for AWS.
	visibilityExtension   = 600 * time.Second
	visibilityGracePeriod = 60 * time.Second

	// longPollWaitTime is SQS's maximum. Workers poll continuously, so the
	// longest wait is also the cheapest -- it is what deploy/cron's
	// receive_wait_time_seconds is set to for the same reason.
	longPollWaitTime = 20 * time.Second

	// Bounds on the retry delay after a transient Receive failure.
	minReceiveBackoff = 1 * time.Second
	maxReceiveBackoff = 30 * time.Second

	// How long Shutdown gets to flush pending acks once the subscriber's own
	// context is gone. Comfortably inside Kubernetes' 30s default termination
	// grace period, so the pod is not killed mid-flush.
	shutdownGracePeriod = 10 * time.Second
)

var (
	// errNoSQSClient means the subscription is not backed by the SQS driver.
	// Treated as fatal rather than "carry on without a heartbeat": a
	// subscriber that looks healthy and silently drops visibility extension
	// would surface as duplicate scans much later, and much less legibly.
	errNoSQSClient = errors.New("subscription did not expose an SQS client")

	// errNoSQSMessage means a received message is not an SQS message, so
	// there is no receipt handle to extend visibility against.
	errNoSQSMessage = errors.New("message did not expose an SQS message")
)

// sqsURLOpener is a configured instance of the driver's own URL opener rather
// than the one it registers on pubsub.DefaultURLMux, because the default
// receive batcher fetches up to 10 messages at a time (MaxBatchSize 10,
// MaxHandlers 100). cron/worker processes strictly one message at a time, so
// the other nine would sit in gocloud's internal buffer with nothing renewing
// their visibility. Those options are deliberately not exposed as URL query
// parameters -- the opener accepts only raw, nacklazy and waittime -- so an
// explicitly constructed URLOpener is the supported way to reach them.
//
// One message may still be buffered ahead of the one being processed. That is
// safe: the driver never sets VisibilityTimeout on ReceiveMessage, so a
// buffered message inherits the queue's own default, which deploy/cron sets to
// an hour.
var sqsURLOpener = &awssnssqs.URLOpener{
	SubscriptionOptions: awssnssqs.SubscriptionOptions{
		WaitTime: longPollWaitTime,
		ReceiveBatcherOptions: batcher.Options{
			MaxBatchSize: 1,
			MaxHandlers:  1,
		},
	},
}

// visibilityChanger is the slice of the SQS API the heartbeat needs. Narrow so
// tests can supply it, the same way receiver and sender are narrowed.
type visibilityChanger interface {
	ChangeMessageVisibility(
		ctx context.Context,
		params *sqs.ChangeMessageVisibilityInput,
		optFns ...func(*sqs.Options),
	) (*sqs.ChangeMessageVisibilityOutput, error)
}

type sqsSubscriber struct {
	ctx          context.Context
	subscription receiver
	client       visibilityChanger

	// receiptHandle is a field rather than a direct call to As() so tests can
	// supply a handle without constructing a driver-backed pubsub.Message.
	receiptHandle func(*pubsub.Message) (string, bool)

	msg      *pubsub.Message
	stop     chan struct{}
	stopOnce *sync.Once

	queueURL string

	// Renewal and retry timings, overridden in tests to run at millisecond
	// scale rather than making the suite wait out production intervals.
	extension  time.Duration
	grace      time.Duration
	minBackoff time.Duration
	maxBackoff time.Duration

	wg sync.WaitGroup
}

// receiptHandleFromMessage recovers the SQS receipt handle through As(),
// gocloud's documented escape hatch for provider data the portable API does
// not carry. Visibility extension is exactly the kind of provider-specific
// operation As() exists for.
func receiptHandleFromMessage(msg *pubsub.Message) (string, bool) {
	var sqsMsg sqstypes.Message
	if !msg.As(&sqsMsg) {
		return "", false
	}
	handle := aws.ToString(sqsMsg.ReceiptHandle)
	return handle, handle != ""
}

func createSQSSubscriber(ctx context.Context, subscriptionURL string) (Subscriber, error) {
	parsed, err := url.Parse(subscriptionURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing subscription URL: %w", err)
	}

	subscription, err := sqsURLOpener.OpenSubscriptionURL(ctx, parsed)
	if err != nil {
		return nil, fmt.Errorf("error during OpenSubscriptionURL: %w", err)
	}

	var client *sqs.Client
	if !subscription.As(&client) {
		if shutdownErr := subscription.Shutdown(ctx); shutdownErr != nil {
			log.Printf("error shutting down unusable subscription: %v", shutdownErr)
		}
		return nil, errNoSQSClient
	}

	return &sqsSubscriber{
		ctx:          ctx,
		subscription: subscription,
		client:       client,
		// Same reconstruction the driver itself performs on the URL, so the
		// heartbeat addresses the queue the subscription is reading.
		queueURL:      "https://" + path.Join(parsed.Host, parsed.Path),
		receiptHandle: receiptHandleFromMessage,
		extension:     visibilityExtension,
		grace:         visibilityGracePeriod,
		minBackoff:    minReceiveBackoff,
		maxBackoff:    maxReceiveBackoff,
	}, nil
}

// SynchronousPull blocks until a message arrives, then starts renewing its
// visibility timeout for as long as the caller holds it.
func (subscriber *sqsSubscriber) SynchronousPull() (*data.ScorecardBatchRequest, error) {
	msg, err := subscriber.receive()
	if err != nil || msg == nil {
		return nil, err
	}

	handle, ok := subscriber.receiptHandle(msg)
	if !ok {
		msg.Nack()
		return nil, errNoSQSMessage
	}

	subscriber.msg = msg
	subscriber.stop = make(chan struct{})
	subscriber.stopOnce = &sync.Once{}
	subscriber.wg.Add(1)
	go subscriber.extendVisibility(handle, subscriber.stop)

	return parseJSONToRequest(msg.Body)
}

// receive retries transient failures instead of surfacing them. Returning
// (nil, nil) the way the GCP subscribers do makes cron/worker break its loop
// and exit 0, so a momentary SQS error would restart every replica at once and
// report it as a clean shutdown. Permanent failures still return, because
// retrying a revoked credential or a deleted queue forever is its own way to
// stop scanning without anyone noticing.
func (subscriber *sqsSubscriber) receive() (*pubsub.Message, error) {
	backoff := subscriber.minBackoff
	for {
		msg, err := subscriber.subscription.Receive(subscriber.ctx)
		if err == nil {
			return msg, nil
		}
		if subscriber.ctx.Err() != nil {
			// Shutting down. Matches the other subscribers' contract with
			// cron/worker: a nil message ends the loop cleanly.
			return nil, nil //nolint:nilerr,nilnil // nil,nil is the Subscriber contract for "stop"
		}
		if isPermanentReceiveError(err) {
			return nil, fmt.Errorf("error during Receive: %w", err)
		}
		log.Printf("transient error during Receive, retrying in %v: %v", backoff, err)
		select {
		case <-subscriber.ctx.Done():
			return nil, nil //nolint:nilnil // as above
		case <-time.After(backoff):
		}
		backoff = min(backoff*2, subscriber.maxBackoff)
	}
}

// isPermanentReceiveError reports whether retrying could ever succeed.
func isPermanentReceiveError(err error) bool {
	var apiErr smithy.APIError
	if !errors.As(err, &apiErr) {
		return false
	}
	switch apiErr.ErrorCode() {
	case "AccessDenied",
		"AccessDeniedException",
		"AWS.SimpleQueueService.NonExistentQueue",
		"InvalidClientTokenId",
		"QueueDoesNotExist",
		"UnrecognizedClientException":
		return true
	default:
		return false
	}
}

// extendVisibility is the reason this subscriber exists rather than the
// gocloud one: gocloud manages visibility only at Ack and Nack, never while a
// message is being processed, and a scan can outlive its timeout.
func (subscriber *sqsSubscriber) extendVisibility(handle string, stop <-chan struct{}) {
	defer subscriber.wg.Done()
	for {
		select {
		case <-subscriber.ctx.Done():
			return
		case <-stop:
			return
		case <-time.After(subscriber.extension - subscriber.grace):
			input := &sqs.ChangeMessageVisibilityInput{
				QueueUrl:          aws.String(subscriber.queueURL),
				ReceiptHandle:     aws.String(handle),
				VisibilityTimeout: int32(subscriber.extension.Seconds()),
			}
			if _, err := subscriber.client.ChangeMessageVisibility(subscriber.ctx, input); err != nil {
				// Logged and retried on the next tick, deliberately not fatal.
				// A missed renewal costs at most one redelivery, which the
				// pipeline already tolerates; killing the process costs every
				// scan in flight on this worker.
				log.Printf("error extending SQS visibility timeout: %v", err)
			}
		}
	}
}

// stopHeartbeat is idempotent and waits for the goroutine to leave, so no
// renewal can land after the message has been acked or the subscriber closed.
func (subscriber *sqsSubscriber) stopHeartbeat() {
	if subscriber.stopOnce == nil {
		return
	}
	subscriber.stopOnce.Do(func() { close(subscriber.stop) })
	subscriber.wg.Wait()
}

func (subscriber *sqsSubscriber) Ack() {
	subscriber.stopHeartbeat()
	if subscriber.msg != nil {
		subscriber.msg.Ack()
	}
}

// Nack relies on the driver, which issues ChangeMessageVisibility with a
// timeout of zero so the message is redelivered immediately.
func (subscriber *sqsSubscriber) Nack() {
	subscriber.stopHeartbeat()
	if subscriber.msg != nil {
		subscriber.msg.Nack()
	}
}

// Close detaches from the subscriber's own context on purpose. SynchronousPull
// returns a nil message only once that context is cancelled, so cron/worker can
// reach Close by no other route than a cancelled context -- handing it that
// context would fail every graceful shutdown by construction. Worse, Shutdown
// is what flushes gocloud's pending ack batch, so a failed one redelivers the
// last message and re-scans it. A short window of its own avoids both.
func (subscriber *sqsSubscriber) Close() error {
	subscriber.stopHeartbeat()

	ctx, cancel := context.WithTimeout(context.WithoutCancel(subscriber.ctx), shutdownGracePeriod)
	defer cancel()

	if err := subscriber.subscription.Shutdown(ctx); err != nil {
		return fmt.Errorf("error during subscription.Shutdown: %w", err)
	}
	return nil
}
