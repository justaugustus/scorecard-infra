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

package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// awsOverlayPath is the config the EKS workloads mount as the
// `scorecard-config` ConfigMap, in place of this package's config.yaml.
const awsOverlayPath = "../../deploy/cron/config-aws.yaml"

func readAWSOverlay(t *testing.T) []byte {
	t.Helper()
	body, err := os.ReadFile(filepath.Clean(awsOverlayPath))
	if err != nil {
		t.Fatalf("reading %s: %v", awsOverlayPath, err)
	}
	return body
}

// TestAWSOverlayParses reads the overlay through this package's own parser.
// A malformed or drifted overlay is otherwise only discovered by a pod that
// will not start, which is a slow and unhelpful way to find a typo.
func TestAWSOverlayParses(t *testing.T) {
	t.Parallel()

	if _, err := getParsedConfigFromFile(readAWSOverlay(t)); err != nil {
		t.Fatalf("getParsedConfigFromFile: %v", err)
	}
}

// TestAWSOverlayBucketsAreTheProductionCorpus replaces the -test suffix check
// this test carried until task 9.7. That check existed so no verification run
// could reach a production bucket; 9.7 is the task that deliberately repoints
// the plane, so the invariant worth keeping is no longer "never production"
// but "exactly these four, spelled correctly".
//
// Naming them explicitly rather than pattern-matching is the point. A typo
// creates a bucket-shaped string that IAM will simply deny, and the failure
// surfaces as a stalled run rather than a config error; a silent revert to a
// -test bucket surfaces as a production corpus that quietly stops updating.
func TestAWSOverlayBucketsAreTheProductionCorpus(t *testing.T) {
	t.Parallel()

	body := readAWSOverlay(t)
	parsed, err := getParsedConfigFromFile(body)
	if err != nil {
		t.Fatalf("getParsedConfigFromFile: %v", err)
	}

	if strings.Contains(string(body), "gs://") {
		t.Error("overlay still names a gs:// bucket; every bucket should be s3:// on EKS")
	}

	scorecardParams := parsed.AdditionalParams["scorecard"]
	want := map[string]string{
		"result-data-bucket-url":     "s3://ossf-scorecard-data2",
		"api-results-bucket-url":     "s3://ossf-scorecard-cron-results",
		"cii-data-bucket-url":        "s3://ossf-scorecard-cii-data",
		"raw-result-data-bucket-url": "s3://ossf-scorecard-rawdata",
	}
	got := map[string]string{
		"result-data-bucket-url":     parsed.ResultDataBucketURL,
		"api-results-bucket-url":     scorecardParams["api-results-bucket-url"],
		"cii-data-bucket-url":        scorecardParams["cii-data-bucket-url"],
		"raw-result-data-bucket-url": scorecardParams["raw-result-data-bucket-url"],
	}
	for name, wantURL := range want {
		if got[name] != wantURL {
			t.Errorf("%s: got %q, want %q", name, got[name], wantURL)
		}
	}
}

// TestAWSOverlayQueueURLsComeFromEnv pins the decision to keep the SQS URL out
// of a public repository. If a queue URL is ever committed here, the account
// ID comes with it.
func TestAWSOverlayQueueURLsComeFromEnv(t *testing.T) {
	t.Parallel()

	parsed, err := getParsedConfigFromFile(readAWSOverlay(t))
	if err != nil {
		t.Fatalf("getParsedConfigFromFile: %v", err)
	}

	if parsed.RequestTopicURL != "" {
		t.Errorf("request-topic-url is set to %q; it is supplied as "+
			"SCORECARD_REQUEST_TOPIC_URL at deploy time", parsed.RequestTopicURL)
	}
	if parsed.RequestSubscriptionURL != "" {
		t.Errorf("request-subscription-url is set to %q; it is supplied as "+
			"SCORECARD_REQUEST_SUBSCRIPTION_URL at deploy time", parsed.RequestSubscriptionURL)
	}
}

// TestAWSOverlayMetricExporterIsSupported catches the blank value that would
// take every worker down. cron/monitoring's GetExporter returns
// ErrEmptyConfigValue for an empty exporter and cron/internal/worker calls it
// during start-up, so "unset" is a crash loop rather than metrics turned off.
func TestAWSOverlayMetricExporterIsSupported(t *testing.T) {
	t.Parallel()

	parsed, err := getParsedConfigFromFile(readAWSOverlay(t))
	if err != nil {
		t.Fatalf("getParsedConfigFromFile: %v", err)
	}

	// Mirrors the switch in cron/monitoring/exporter.go's GetExporter.
	switch parsed.MetricExporter {
	case "printer", "stackdriver":
	case "":
		t.Error("metric-exporter is empty; GetExporter treats that as an error and the worker will not start")
	default:
		t.Errorf("metric-exporter %q is not a type cron/monitoring supports", parsed.MetricExporter)
	}
}
