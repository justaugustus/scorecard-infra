#!/usr/bin/env bash
#
# Copyright 2026 OpenSSF Scorecard Authors
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#      http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.
#
# provision-cron-aws task 9.5: confirm the DLQ receives a message that
# permanently fails, after maxReceiveCount retries, rather than looping
# forever.
#
#   ./trigger-dlq.sh
#
# Publishes one well-formed ScorecardBatchRequest whose repo URL can never
# resolve (an example.invalid host), directly to the real queue via
# `aws sqs send-message` -- NOT malformed JSON. A JSON parse failure happens
# inside SynchronousPull (cron/internal/pubsub/subscriber_sqs.go), before
# cron/worker/worker.go's ack/nack logic ever runs, so it returns an error
# that crashes the whole worker process rather than Nacking one message --
# that would crash-loop real worker pods and could take up to
# maxReceiveCount * the queue's hour-long visibility timeout to reach the
# DLQ, since nothing ever calls Nack. An unscannable-but-valid repo URL
# fails inside processRequest instead, which cron/worker/worker.go's
# announceError already Nacks immediately -- confirmed empirically this
# session: two such messages, published by accident while developing this
# verification, reached the DLQ within seconds because real worker pods
# picked them up, failed fast, and Nacked. This script does deliberately
# and cleans up what that accident did unintentionally.
#
# Real worker contention (the reason the equivalent Go-test approach was
# dropped for 9.3) is not a problem here: any of the 14 workers failing this
# message repeatedly is the real path, not a test artifact to route around.

set -uo pipefail

command -v aws >/dev/null || { echo "error: the AWS CLI is required" >&2; exit 1; }
command -v kubectl >/dev/null || { echo "error: kubectl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "error: python3 is required" >&2; exit 1; }

# The EKS context name is the full ARN (arn:aws:eks:<region>:<account>:cluster/scorecard-batch),
# which embeds the account ID -- matched by suffix here rather than hardcoded whole, since this
# repository is public and that ID has no business in a tracked file.
CTX="$(kubectl config current-context 2>/dev/null)"
case "${CTX}" in
  */scorecard-batch) ;;
  *)
    echo "error: kubectl context is '${CTX}', not the scorecard-batch cluster." >&2
    exit 1
    ;;
esac

RAW_URL="$(kubectl get configmap scorecard-queue -o jsonpath='{.data.request-topic-url}')"
QUEUE_URL="$(echo "${RAW_URL}" | sed -E 's#^awssqs://sqs\.([a-z0-9-]+)\.amazonaws\.com#https://sqs.\1.amazonaws.com#; s/\?region=.*$//')"

MARKER="https://example.invalid/verify-9-5/$(date -u +%s%N)"
BODY="{\"repos\":[{\"url\":\"${MARKER}\"}]}"

echo "== publishing permanently-unscannable marker ==" >&2
echo "  ${MARKER}" >&2
aws sqs send-message --queue-url "${QUEUE_URL}" --message-body "${BODY}" >/dev/null

echo "== resolving this queue's DLQ and maxReceiveCount ==" >&2
POLICY_JSON="$(aws sqs get-queue-attributes --queue-url "${QUEUE_URL}" --attribute-names RedrivePolicy \
  --query 'Attributes.RedrivePolicy' --output text)"
DLQ_ARN="$(echo "${POLICY_JSON}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["deadLetterTargetArn"])')"
MAX_RECEIVE_COUNT="$(echo "${POLICY_JSON}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["maxReceiveCount"])')"
DLQ_NAME="${DLQ_ARN##*:}"
DLQ_URL="$(aws sqs get-queue-url --queue-name "${DLQ_NAME}" --query QueueUrl --output text)"
echo "  DLQ: ${DLQ_NAME}, maxReceiveCount: ${MAX_RECEIVE_COUNT}" >&2

echo "== waiting for a real worker to fail it ${MAX_RECEIVE_COUNT} times (up to ~2 min) ==" >&2
FOUND=""
for _ in $(seq 1 24); do
  sleep 5
  RESULT="$(aws sqs receive-message --queue-url "${DLQ_URL}" --max-number-of-messages 10 \
    --wait-time-seconds 5 --visibility-timeout 30 --output json 2>/dev/null)"
  MATCH="$(echo "${RESULT}" | python3 -c "
import json, sys
d = json.load(sys.stdin)
for m in d.get('Messages', []):
    if '${MARKER}' in m['Body']:
        print(m['ReceiptHandle'])
        break
")"
  if [ -n "${MATCH}" ]; then
    FOUND="${MATCH}"
    break
  fi
done

if [ -z "${FOUND}" ]; then
  echo "error: marker did not reach the DLQ within the wait window. Check queue depth" >&2
  echo "and worker logs -- it may still be cycling, or a worker pod may have" >&2
  echo "crash-looped instead of Nacking (see this script's header)." >&2
  exit 1
fi

echo "confirmed: marker reached the DLQ after ${MAX_RECEIVE_COUNT} deliveries" >&2

echo "== cleaning up: deleting the marker from the DLQ ==" >&2
aws sqs delete-message --queue-url "${DLQ_URL}" --receipt-handle "${FOUND}"
echo "done" >&2
