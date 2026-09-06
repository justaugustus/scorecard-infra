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
# provision-cron-aws task 9.6: confirm the CII worker completes one cycle
# against ossf-scorecard-cii-data-test.
#
#   ./run-cii-cycle.sh
#
# cron/internal/cii/main.go takes no positional args and touches no
# queue -- it paginates https://www.bestpractices.dev/projects.json at 1
# request/second until a page comes back empty, writing
# {cii-data-bucket-url}/{repo}/result.json per project. That's a full,
# unthrottleable cycle (no sample-size knob), so this is a real run, not a
# scaled-down one -- expect it to take a while. Every write is an
# unconditional overwrite keyed by repo URL, so re-running this script is
# always safe.
#
# scorecard-cii-worker also has its own real schedule (cron/k8s/cii.yaml:
# "0 20 * * 0", weekly) and is not suspended, so this task may already be
# closed by that natural run by the time this script would be needed --
# check `kubectl get cronjob scorecard-cii-worker` for a recent
# lastScheduleTime/lastSuccessfulTime before triggering a redundant one.

set -uo pipefail

JOB_NAME="scorecard-cii-worker-verify-$(date -u +%Y%m%dT%H%M%SZ | tr '[:upper:]' '[:lower:]')"

command -v kubectl >/dev/null || { echo "error: kubectl is required" >&2; exit 1; }

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

echo "== scorecard-cii-worker's own schedule, for reference ==" >&2
kubectl get cronjob scorecard-cii-worker -o custom-columns=SCHEDULE:.spec.schedule,LAST_SCHEDULE:.status.lastScheduleTime,SUSPEND:.spec.suspend

echo "== triggering a one-off run as ${JOB_NAME} ==" >&2
kubectl create job "${JOB_NAME}" --from=cronjob/scorecard-cii-worker

echo "== this can take a while (full bestpractices.dev pagination, 1 req/s) ==" >&2
echo "   tail progress with: kubectl logs -f job/${JOB_NAME}" >&2

if kubectl wait --for=condition=complete "job/${JOB_NAME}" --timeout=1800s; then
  echo "CII worker completed one cycle" >&2
else
  echo "did not report Complete within 30 minutes -- check:" >&2
  echo "  kubectl logs job/${JOB_NAME}" >&2
  exit 1
fi

echo
echo "Spot-check the bucket, e.g.:"
echo "  aws s3 ls s3://ossf-scorecard-cii-data-test/github.com/ossf/ --recursive"
