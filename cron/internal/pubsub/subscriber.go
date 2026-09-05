// Copyright 2021 OpenSSF Scorecard Authors
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
	"net/url"
	"os"

	"gocloud.dev/pubsub/awssnssqs"
	"gocloud.dev/pubsub/gcppubsub"
	"google.golang.org/protobuf/encoding/protojson"

	"github.com/ossf/scorecard-infra/cron/data"
)

// errUnsupportedScheme means the configured URL names a backend this binary
// has no subscriber for. Adding one is a code change, not a config change.
var errUnsupportedScheme = errors.New("unsupported subscription URL scheme")

// Subscriber interface is used pull messages from PubSub.
type Subscriber interface {
	SynchronousPull() (*data.ScorecardBatchRequest, error)
	Ack()
	Nack()
	Close() error
}

// CreateSubscriber returns an implementation of Subscriber interface, chosen
// by the URL's scheme so one binary can run against either backend during the
// GCP-to-AWS transition. The scheme constants come from the gocloud drivers
// themselves rather than being spelled out here, so a URL this accepts is a
// URL those drivers accept.
func CreateSubscriber(ctx context.Context, subscriptionURL string) (Subscriber, error) {
	parsed, err := url.Parse(subscriptionURL)
	if err != nil {
		return nil, fmt.Errorf("error parsing subscription URL: %w", err)
	}

	switch parsed.Scheme {
	case awssnssqs.SQSScheme:
		return createSQSSubscriber(ctx, subscriptionURL)
	case gcppubsub.Scheme:
		return createGCPSubscriber(ctx, subscriptionURL)
	default:
		return nil, fmt.Errorf("%w: %q", errUnsupportedScheme, parsed.Scheme)
	}
}

// createGCPSubscriber preserves the pre-existing selection exactly: the gocloud
// clients respect PUBSUB_EMULATOR_HOST, but our custom GCS subscriber does not.
// This check moved out of CreateSubscriber when selection became scheme-based;
// it did not change.
func createGCPSubscriber(ctx context.Context, subscriptionURL string) (Subscriber, error) {
	if os.Getenv("PUBSUB_EMULATOR_HOST") != "" {
		return createGocloudSubscriber(ctx, subscriptionURL)
	}
	return createGCSSubscriber(ctx, subscriptionURL)
}

func parseJSONToRequest(jsonData []byte) (*data.ScorecardBatchRequest, error) {
	ret := &data.ScorecardBatchRequest{}
	if err := protojson.Unmarshal(jsonData, ret); err != nil {
		return nil, fmt.Errorf("protojson.Unmarshal: %w", err)
	}
	return ret, nil
}
