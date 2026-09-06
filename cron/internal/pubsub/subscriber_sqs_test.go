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
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/service/sqs"
	"github.com/aws/smithy-go"
	"gocloud.dev/pubsub"
	"gocloud.dev/pubsub/mempubsub"
	"google.golang.org/protobuf/encoding/protojson"
	"google.golang.org/protobuf/proto"

	"github.com/ossf/scorecard-infra/cron/data"
)

// Short enough that the suite does not wait out production intervals, long
// enough that a loaded CI runner still ticks several times inside settleTime.
const (
	testExtension = 40 * time.Millisecond
	testGrace     = 30 * time.Millisecond
	testBackoff   = 5 * time.Millisecond
	settleTime    = 250 * time.Millisecond
)

// countingVisibilityChanger records every renewal so tests can assert both
// that the heartbeat runs and that it stops.
type countingVisibilityChanger struct {
	err      error
	timeouts []int32
	handles  []string
	mu       sync.Mutex
}

func (c *countingVisibilityChanger) ChangeMessageVisibility(
	_ context.Context,
	params *sqs.ChangeMessageVisibilityInput,
	_ ...func(*sqs.Options),
) (*sqs.ChangeMessageVisibilityOutput, error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.timeouts = append(c.timeouts, params.VisibilityTimeout)
	if params.ReceiptHandle != nil {
		c.handles = append(c.handles, *params.ReceiptHandle)
	}
	return &sqs.ChangeMessageVisibilityOutput{}, c.err
}

func (c *countingVisibilityChanger) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.timeouts)
}

type receiveResult struct {
	msg *pubsub.Message
	err error
}

// scriptedReceiver plays back a fixed sequence of Receive outcomes.
type scriptedReceiver struct {
	shutdownErr error
	results     []receiveResult
	calls       int
	mu          sync.Mutex
}

func (r *scriptedReceiver) Receive(_ context.Context) (*pubsub.Message, error) {
	r.mu.Lock()
	defer r.mu.Unlock()
	if r.calls >= len(r.results) {
		return nil, errors.New("Receive called more times than the test scripted")
	}
	res := r.results[r.calls]
	r.calls++
	return res.msg, res.err
}

func (r *scriptedReceiver) Shutdown(_ context.Context) error {
	return r.shutdownErr
}

func (r *scriptedReceiver) receiveCalls() int {
	r.mu.Lock()
	defer r.mu.Unlock()
	return r.calls
}

type stubAPIError struct {
	code string
}

func (e *stubAPIError) Error() string                 { return e.code }
func (e *stubAPIError) ErrorCode() string             { return e.code }
func (e *stubAPIError) ErrorMessage() string          { return e.code }
func (e *stubAPIError) ErrorFault() smithy.ErrorFault { return smithy.FaultServer }

// newAckableMessage returns a real gocloud message. pubsub.Message's ack hooks
// are unexported, so a zero value panics on Ack; the in-memory driver is the
// supported way to get one that can be acked and nacked without a cloud.
func newAckableMessage(t *testing.T, body []byte) *pubsub.Message {
	t.Helper()
	ctx := t.Context()

	topic := mempubsub.NewTopic()
	t.Cleanup(func() {
		if err := topic.Shutdown(context.Background()); err != nil {
			t.Logf("mempubsub topic.Shutdown: %v", err)
		}
	})
	// An ack deadline far longer than any test, so mempubsub never redelivers
	// underneath us and muddies what is being asserted.
	sub := mempubsub.NewSubscription(topic, time.Hour)
	t.Cleanup(func() {
		if err := sub.Shutdown(context.Background()); err != nil {
			t.Logf("mempubsub sub.Shutdown: %v", err)
		}
	})

	if err := topic.Send(ctx, &pubsub.Message{Body: body}); err != nil {
		t.Fatalf("mempubsub topic.Send: %v", err)
	}
	msg, err := sub.Receive(ctx)
	if err != nil {
		t.Fatalf("mempubsub sub.Receive: %v", err)
	}
	return msg
}

func testRequestBody(t *testing.T) ([]byte, *data.ScorecardBatchRequest) {
	t.Helper()
	url := "repo1"
	req := &data.ScorecardBatchRequest{Repos: []*data.Repo{{Url: &url}}}
	body, err := protojson.Marshal(req)
	if err != nil {
		t.Fatalf("protojson.Marshal: %v", err)
	}
	return body, req
}

// newTestSubscriber wires a subscriber over scripted results with a fixed
// receipt handle, bypassing As() so tests need no driver-backed message.
func newTestSubscriber(
	ctx context.Context,
	recv receiver,
	changer visibilityChanger,
	handle string,
) *sqsSubscriber {
	return &sqsSubscriber{
		ctx:          ctx,
		subscription: recv,
		client:       changer,
		queueURL:     "https://sqs.us-east-1.amazonaws.com/000000000000/test-queue",
		receiptHandle: func(*pubsub.Message) (string, bool) {
			return handle, handle != ""
		},
		extension:  testExtension,
		grace:      testGrace,
		minBackoff: testBackoff,
		maxBackoff: testBackoff,
	}
}

// TestSQSSubscriberHeartbeatOutlivesVisibilityWindow is the test E3 exists
// for: a consumer slower than one visibility window must not lose its message.
func TestSQSSubscriberHeartbeatOutlivesVisibilityWindow(t *testing.T) {
	t.Parallel()

	body, want := testRequestBody(t)
	changer := &countingVisibilityChanger{}
	recv := &scriptedReceiver{results: []receiveResult{{msg: newAckableMessage(t, body)}}}
	subscriber := newTestSubscriber(t.Context(), recv, changer, "handle-1")

	got, err := subscriber.SynchronousPull()
	if err != nil {
		t.Fatalf("SynchronousPull: %v", err)
	}
	if !proto.Equal(got, want) {
		t.Errorf("request: got %v, want %v", got, want)
	}

	// Hold the message well past several renewal intervals, as a slow scan does.
	time.Sleep(settleTime)

	renewals := changer.count()
	if renewals < 2 {
		t.Fatalf("heartbeat renewals: got %d, want at least 2 -- the message would have expired mid-scan", renewals)
	}

	subscriber.Ack()

	for _, timeout := range changer.timeouts {
		if timeout != int32(testExtension.Seconds()) {
			t.Errorf("renewal timeout: got %d, want %d", timeout, int32(testExtension.Seconds()))
		}
	}
	for _, handle := range changer.handles {
		if handle != "handle-1" {
			t.Errorf("renewal receipt handle: got %q, want %q", handle, "handle-1")
		}
	}
}

// TestSQSSubscriberHeartbeatStops covers every way the caller can finish with
// a message. A renewal landing after the message is gone is a wasted API call
// at best and an error log at worst.
func TestSQSSubscriberHeartbeatStops(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		finish func(*sqsSubscriber)
		name   string
	}{
		{name: "Ack", finish: func(s *sqsSubscriber) { s.Ack() }},
		{name: "Nack", finish: func(s *sqsSubscriber) { s.Nack() }},
		{name: "Close", finish: func(s *sqsSubscriber) { _ = s.Close() }},
		{name: "CloseAfterAck", finish: func(s *sqsSubscriber) { s.Ack(); _ = s.Close() }},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			body, _ := testRequestBody(t)
			changer := &countingVisibilityChanger{}
			recv := &scriptedReceiver{results: []receiveResult{{msg: newAckableMessage(t, body)}}}
			subscriber := newTestSubscriber(t.Context(), recv, changer, "handle-1")

			if _, err := subscriber.SynchronousPull(); err != nil {
				t.Fatalf("SynchronousPull: %v", err)
			}
			time.Sleep(settleTime)

			// Must not panic, however the caller finishes -- including Close
			// after Ack, which closes the stop channel a second time.
			testcase.finish(subscriber)

			atFinish := changer.count()
			time.Sleep(settleTime)
			if after := changer.count(); after != atFinish {
				t.Errorf("renewals continued after %s: %d at finish, %d later", testcase.name, atFinish, after)
			}
		})
	}
}

func TestSQSSubscriberHeartbeatStopsOnContextCancel(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	body, _ := testRequestBody(t)
	changer := &countingVisibilityChanger{}
	recv := &scriptedReceiver{results: []receiveResult{{msg: newAckableMessage(t, body)}}}
	subscriber := newTestSubscriber(ctx, recv, changer, "handle-1")

	if _, err := subscriber.SynchronousPull(); err != nil {
		t.Fatalf("SynchronousPull: %v", err)
	}
	time.Sleep(settleTime)
	cancel()
	time.Sleep(settleTime)

	atCancel := changer.count()
	time.Sleep(settleTime)
	if after := changer.count(); after != atCancel {
		t.Errorf("renewals continued after context cancel: %d then %d", atCancel, after)
	}
}

// TestSQSSubscriberRenewalFailureIsNotFatal guards the deliberate divergence
// from subscriber_gcs.go, which calls log.Fatal inside its heartbeat.
func TestSQSSubscriberRenewalFailureIsNotFatal(t *testing.T) {
	t.Parallel()

	body, _ := testRequestBody(t)
	changer := &countingVisibilityChanger{err: errors.New("throttled")}
	recv := &scriptedReceiver{results: []receiveResult{{msg: newAckableMessage(t, body)}}}
	subscriber := newTestSubscriber(t.Context(), recv, changer, "handle-1")

	if _, err := subscriber.SynchronousPull(); err != nil {
		t.Fatalf("SynchronousPull: %v", err)
	}
	time.Sleep(settleTime)

	if renewals := changer.count(); renewals < 2 {
		t.Errorf("heartbeat stopped after a failed renewal: got %d renewals, want at least 2", renewals)
	}
	subscriber.Ack()
}

func TestSQSSubscriberReceive(t *testing.T) {
	t.Parallel()

	body, want := testRequestBody(t)

	testcases := []struct {
		wantErr      error
		name         string
		results      []receiveResult
		wantCalls    int
		wantRequest  bool
		wantAnyError bool
	}{
		{
			name:        "Basic",
			results:     []receiveResult{{msg: newAckableMessage(t, body)}},
			wantCalls:   1,
			wantRequest: true,
		},
		{
			name: "RetriesTransientError",
			results: []receiveResult{
				{err: errors.New("connection reset")},
				{err: &stubAPIError{code: "ServiceUnavailable"}},
				{msg: newAckableMessage(t, body)},
			},
			wantCalls:   3,
			wantRequest: true,
		},
		{
			name:         "ReturnsOnAccessDenied",
			results:      []receiveResult{{err: &stubAPIError{code: "AccessDenied"}}},
			wantCalls:    1,
			wantAnyError: true,
		},
		{
			name:         "ReturnsOnMissingQueue",
			results:      []receiveResult{{err: &stubAPIError{code: "QueueDoesNotExist"}}},
			wantCalls:    1,
			wantAnyError: true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			recv := &scriptedReceiver{results: testcase.results}
			subscriber := newTestSubscriber(t.Context(), recv, &countingVisibilityChanger{}, "handle-1")

			got, err := subscriber.SynchronousPull()
			switch {
			case testcase.wantAnyError && err == nil:
				t.Errorf("expected an error, got nil")
			case !testcase.wantAnyError && err != nil:
				t.Errorf("unexpected error: %v", err)
			}
			if testcase.wantRequest && !proto.Equal(got, want) {
				t.Errorf("request: got %v, want %v", got, want)
			}
			if calls := recv.receiveCalls(); calls != testcase.wantCalls {
				t.Errorf("Receive calls: got %d, want %d", calls, testcase.wantCalls)
			}
			if testcase.wantRequest {
				subscriber.Ack()
			}
		})
	}
}

// TestSQSSubscriberSkipsUnparseableMessage covers the poison-message case: a
// body that is not a ScorecardBatchRequest must cost one nack, not the worker
// process. Every error out of SynchronousPull is fatal to cron/worker, so
// returning one here would panic the pod and leave the same body to crash the
// next worker that received it.
func TestSQSSubscriberSkipsUnparseableMessage(t *testing.T) {
	t.Parallel()

	body, want := testRequestBody(t)
	recv := &scriptedReceiver{results: []receiveResult{
		{msg: newAckableMessage(t, []byte("{not a ScorecardBatchRequest"))},
		{msg: newAckableMessage(t, body)},
	}}
	subscriber := newTestSubscriber(t.Context(), recv, &countingVisibilityChanger{}, "handle-1")

	got, err := subscriber.SynchronousPull()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !proto.Equal(got, want) {
		t.Errorf("request: got %v, want %v", got, want)
	}
	if calls := recv.receiveCalls(); calls != 2 {
		t.Errorf("Receive calls: got %d, want 2 (bad message skipped, good one returned)", calls)
	}
	subscriber.Ack()
}

// TestSQSSubscriberMissingReceiptHandle covers a driver that stops handing
// back SQS messages. Running on without a heartbeat would look healthy and
// produce duplicate scans much later, so this fails loudly instead.
func TestSQSSubscriberMissingReceiptHandle(t *testing.T) {
	t.Parallel()

	body, _ := testRequestBody(t)
	changer := &countingVisibilityChanger{}
	recv := &scriptedReceiver{results: []receiveResult{{msg: newAckableMessage(t, body)}}}
	subscriber := newTestSubscriber(t.Context(), recv, changer, "")

	got, err := subscriber.SynchronousPull()
	if !errors.Is(err, errNoSQSMessage) {
		t.Errorf("error: got %v, want %v", err, errNoSQSMessage)
	}
	if got != nil {
		t.Errorf("request: got %v, want nil", got)
	}

	time.Sleep(settleTime)
	if renewals := changer.count(); renewals != 0 {
		t.Errorf("heartbeat started without a receipt handle: %d renewals", renewals)
	}
}

// ctxAwareReceiver fails Shutdown on a dead context, the way gocloud's real
// subscription does.
type ctxAwareReceiver struct {
	scriptedReceiver
}

func (r *ctxAwareReceiver) Shutdown(ctx context.Context) error {
	return ctx.Err()
}

// TestSQSSubscriberCloseSurvivesCancelledContext pins the one route
// cron/worker can actually take to Close. SynchronousPull returns a nil
// message only after the context is cancelled, so if Close reused that
// context every graceful shutdown would fail and the pending ack batch would
// never flush, re-scanning the last message on every pod termination.
func TestSQSSubscriberCloseSurvivesCancelledContext(t *testing.T) {
	t.Parallel()

	ctx, cancel := context.WithCancel(t.Context())
	subscriber := newTestSubscriber(ctx, &ctxAwareReceiver{}, &countingVisibilityChanger{}, "handle-1")
	cancel()

	if err := subscriber.Close(); err != nil {
		t.Errorf("Close after context cancel: got %v, want nil", err)
	}
}

func TestSQSSubscriberCloseReturnsShutdownError(t *testing.T) {
	t.Parallel()

	wantErr := errors.New("mock shutdown failure")
	recv := &scriptedReceiver{shutdownErr: wantErr}
	subscriber := newTestSubscriber(t.Context(), recv, &countingVisibilityChanger{}, "handle-1")

	if err := subscriber.Close(); !errors.Is(err, wantErr) {
		t.Errorf("Close: got %v, want %v", err, wantErr)
	}
}

func TestIsPermanentReceiveError(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		err  error
		name string
		want bool
	}{
		{name: "AccessDenied", err: &stubAPIError{code: "AccessDenied"}, want: true},
		{name: "QueueDoesNotExist", err: &stubAPIError{code: "QueueDoesNotExist"}, want: true},
		{
			name: "NonExistentQueue",
			err:  &stubAPIError{code: "AWS.SimpleQueueService.NonExistentQueue"},
			want: true,
		},
		{name: "Throttled", err: &stubAPIError{code: "RequestThrottled"}, want: false},
		{name: "ServiceUnavailable", err: &stubAPIError{code: "ServiceUnavailable"}, want: false},
		{name: "PlainError", err: errors.New("connection reset"), want: false},
		{
			name: "WrappedAccessDenied",
			err:  fmt.Errorf("during receive: %w", &stubAPIError{code: "AccessDenied"}),
			want: true,
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()
			if got := isPermanentReceiveError(testcase.err); got != testcase.want {
				t.Errorf("isPermanentReceiveError(%v): got %v, want %v", testcase.err, got, testcase.want)
			}
		})
	}
}
