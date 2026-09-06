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
# provision-cron-aws task 9.3: kill a worker mid-scan, confirm the message
# becomes visible again and a second worker completes it.
#
#   ./kill-worker-mid-scan.sh
#
# Force-deletes the pod (--grace-period=0 --force), not a graceful delete.
# cron/internal/pubsub/subscriber_sqs.go's Close() was fixed (task 6.7) so a
# graceful SIGTERM flushes the pending ack before the pod exits -- exactly
# the case this task is NOT testing. Only an ungraceful kill leaves the
# heartbeat with no chance to run its own shutdown path, which is what
# "killed mid-scan" means here.
#
# Runs run-sample-inventory.sh to get a real shard in flight, then races to
# delete whichever pod is holding it before the scan finishes. The sample
# repos are small and scan in seconds, so this is timing-sensitive by
# nature -- if it misses the window, the message is already Acked and the
# script says so rather than reporting a false pass.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

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

queue_depth() {
  aws sqs get-queue-attributes --queue-url "${QUEUE_URL}" \
    --attribute-names ApproximateNumberOfMessages ApproximateNumberOfMessagesNotVisible \
    --output json
}

echo "== publishing a fresh shard ==" >&2
"${SCRIPT_DIR}/run-sample-inventory.sh" >&2

echo "== looking for the worker pod that picked it up ==" >&2
POD=""
for _ in $(seq 1 20); do
  DEPTH="$(queue_depth)"
  IN_FLIGHT="$(echo "${DEPTH}" | python3 -c 'import json,sys; print(json.load(sys.stdin)["Attributes"]["ApproximateNumberOfMessagesNotVisible"])')"
  if [ "${IN_FLIGHT}" != "0" ]; then
    # Whichever pod logged a receipt most recently is almost certainly the
    # one holding it -- there is no exact "which pod has this message" API,
    # so this is a best-effort identification, not a guarantee.
    POD="$(kubectl get pods -l app=scorecard-batch-worker -o name 2>/dev/null | while read -r p; do
      if kubectl logs "${p#pod/}" --since=15s 2>/dev/null | grep -q "Received message"; then
        echo "${p#pod/}"
      fi
    done | tail -1)"
    [ -n "${POD}" ] && break
  fi
  sleep 1
done

if [ -z "${POD}" ]; then
  echo "error: could not identify a worker pod holding the message in time -- the scan" >&2
  echo "may have already completed. Re-run, or lower the sample repo count so the" >&2
  echo "window is wider." >&2
  exit 1
fi

echo "== force-deleting ${POD} mid-scan ==" >&2
kubectl delete pod "${POD}" --grace-period=0 --force

echo "== polling queue depth for redelivery ==" >&2
for i in $(seq 1 30); do
  DEPTH="$(queue_depth)"
  echo "  t+${i}s: ${DEPTH}" | python3 -c 'import json,sys; l=sys.stdin.read(); print(l.split(":",1)[0], json.loads(l.split(":",1)[1])["Attributes"])'
  sleep 1
done

echo
echo "Expect: ApproximateNumberOfMessagesNotVisible drops to 0 once the" \
     "deleted pod's un-renewed visibility window expires, then rises to 1" \
     "again once a second pod picks up the redelivery, then both drop to 0" \
     "once that pod completes it. Confirm the final result with" \
     "check-bucket-consistency.sh against the shard prefix" \
     "run-sample-inventory.sh printed above."
