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
	"errors"
	"testing"
)

// TestCreateSubscriberScheme covers the dispatch itself. The gcppubsub cases
// are absent on purpose: both GCP constructors reach for real credentials, and
// a test that needs a GCP project proves nothing about routing. That path is
// a verbatim move into createGCPSubscriber and stays covered by
// TestSubscriber in subscriber_gocloud_test.go.
func TestCreateSubscriberScheme(t *testing.T) {
	t.Parallel()

	testcases := []struct {
		wantErr error
		name    string
		url     string
		wantSQS bool
	}{
		{
			name: "SQS",
			// region is supplied so config resolution stays offline.
			url:     "awssqs://sqs.us-east-1.amazonaws.com/000000000000/test-queue?region=us-east-1",
			wantSQS: true,
		},
		{
			name:    "UnknownScheme",
			url:     "kafka://broker/topic",
			wantErr: errUnsupportedScheme,
		},
		{
			name:    "NoScheme",
			url:     "scorecard-batch-requests",
			wantErr: errUnsupportedScheme,
		},
		{
			name:    "MalformedURL",
			url:     "awssqs://host\x7f/queue",
			wantErr: nil, // url.Parse rejects it; only the error text differs.
		},
	}

	for _, testcase := range testcases {
		t.Run(testcase.name, func(t *testing.T) {
			t.Parallel()

			subscriber, err := CreateSubscriber(t.Context(), testcase.url)

			if testcase.wantSQS {
				if err != nil {
					t.Fatalf("CreateSubscriber: unexpected error: %v", err)
				}
				if _, ok := subscriber.(*sqsSubscriber); !ok {
					t.Errorf("subscriber type: got %T, want *sqsSubscriber", subscriber)
				}
				if err := subscriber.Close(); err != nil {
					t.Errorf("Close: %v", err)
				}
				return
			}

			if err == nil {
				t.Fatalf("CreateSubscriber(%q): expected an error, got nil", testcase.url)
			}
			if testcase.wantErr != nil && !errors.Is(err, testcase.wantErr) {
				t.Errorf("error: got %v, want %v", err, testcase.wantErr)
			}
		})
	}
}

// TestCreateSQSSubscriberDefaults pins the wiring the heartbeat depends on:
// the queue URL the driver will read, and renewal timings matching
// subscriber_gcs.go rather than drifting into a second set of numbers.
func TestCreateSQSSubscriberDefaults(t *testing.T) {
	t.Parallel()

	const queueURL = "https://sqs.us-east-1.amazonaws.com/000000000000/test-queue"

	subscriber, err := createSQSSubscriber(
		t.Context(),
		"awssqs://sqs.us-east-1.amazonaws.com/000000000000/test-queue?region=us-east-1",
	)
	if err != nil {
		t.Fatalf("createSQSSubscriber: %v", err)
	}
	// Closed inline rather than in t.Cleanup: cleanup runs after t.Context is
	// cancelled, so Shutdown would always fail there and the failure would
	// mean nothing.
	defer func() {
		if err := subscriber.Close(); err != nil {
			t.Errorf("Close: %v", err)
		}
	}()

	sqsSub, ok := subscriber.(*sqsSubscriber)
	if !ok {
		t.Fatalf("subscriber type: got %T, want *sqsSubscriber", subscriber)
	}
	if sqsSub.queueURL != queueURL {
		t.Errorf("queueURL: got %q, want %q", sqsSub.queueURL, queueURL)
	}
	if sqsSub.extension != visibilityExtension {
		t.Errorf("extension: got %v, want %v", sqsSub.extension, visibilityExtension)
	}
	if sqsSub.grace != visibilityGracePeriod {
		t.Errorf("grace: got %v, want %v", sqsSub.grace, visibilityGracePeriod)
	}
	if sqsSub.grace >= sqsSub.extension {
		t.Errorf("grace %v must be shorter than extension %v, or renewal never fires early enough",
			sqsSub.grace, sqsSub.extension)
	}
}
