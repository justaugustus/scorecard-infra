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
# provision-cron-aws task 9.9: verify denial at runtime, not merely absent
# grants, for the five boundaries in tasks.md's (corrected 2026-09-06) 9.9.
#
#   ./verify-iam-denials.sh
#
# None of this repo's own images (controller/worker/cii/github-server) carry
# a shell or the AWS CLI -- they're distroless Go binaries -- so this runs a
# throwaway public.ecr.aws/aws-cli/aws-cli pod per boundary, under the same
# ServiceAccount the real workload uses. EKS Pod Identity binds the IAM role
# to the ServiceAccount, not the image, so this exercises the exact same
# role a real controller/worker/cii pod would.
#
# Run this BEFORE task 9.7 repoints anything at production: 9.7 legitimately
# widens the worker's grant to the six adopted buckets, and boundary 1 below
# stops being meaningful once that happens.

set -uo pipefail

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

FAIL=0

run_check() {
  local name="$1" sa="$2" cmd="$3"
  local pod
  pod="verify-denial-${name}-$(date -u +%s)"

  cat <<EOF | kubectl apply -f - >/dev/null
apiVersion: v1
kind: Pod
metadata:
  name: ${pod}
  labels:
    scorecard.dev/purpose: group-9-verification
spec:
  serviceAccountName: ${sa}
  restartPolicy: Never
  nodeSelector:
    scorecard.dev/pool: system
  containers:
    - name: aws-cli
      image: public.ecr.aws/aws-cli/aws-cli:latest
      command: ["/bin/sh", "-c"]
      args: ["${cmd}"]
EOF

  kubectl wait --for=jsonpath='{.status.phase}'=Succeeded "pod/${pod}" --timeout=60s >/dev/null 2>&1
  kubectl wait --for=jsonpath='{.status.phase}'=Failed "pod/${pod}" --timeout=60s >/dev/null 2>&1

  local out
  out="$(kubectl logs "${pod}" 2>&1)"

  if echo "${out}" | grep -qi "AccessDenied"; then
    echo "PASS  ${name}: AccessDenied as expected"
  else
    echo "FAIL  ${name}: expected AccessDenied, got:"
    echo "        ${out//$'\n'/$'\n'        }"
    FAIL=1
  fi

  kubectl delete pod "${pod}" --wait=false >/dev/null 2>&1
}

echo "== 1/5: worker writing an adopted production bucket =="
run_check "worker-writes-prod-bucket" "scorecard-batch-worker" \
  "echo denial-check | aws s3 cp - s3://ossf-scorecard-data2/verify-9-9-denial-check.txt"

echo "== 2/5: worker calling ReceiveMessage on the DLQ =="
DLQ_URL="$(aws sqs get-queue-attributes \
  --queue-url "$(kubectl get configmap scorecard-queue -o jsonpath='{.data.request-topic-url}' | \
    sed -E 's#^awssqs://sqs\.([a-z0-9-]+)\.amazonaws\.com#https://sqs.\1.amazonaws.com#; s/\?region=.*$//')" \
  --attribute-names RedrivePolicy --query 'Attributes.RedrivePolicy' --output text | \
  python3 -c 'import json,sys; print(json.load(sys.stdin)["deadLetterTargetArn"])' | \
  xargs -I{} aws sqs get-queue-url --queue-name "$(basename {})" --query QueueUrl --output text)"
run_check "worker-reads-dlq" "scorecard-batch-worker" \
  "aws sqs receive-message --queue-url ${DLQ_URL}"

echo "== 3/5: CII worker touching a bucket that isn't cii-data-test =="
run_check "cii-writes-other-bucket" "scorecard-cii-worker" \
  "echo denial-check | aws s3 cp - s3://ossf-scorecard-data2-test/verify-9-9-denial-check.txt"

echo "== 4/5: controller writing cron-results-test =="
run_check "controller-writes-cron-results" "scorecard-batch-controller" \
  "echo denial-check | aws s3 cp - s3://ossf-scorecard-cron-results-test/verify-9-9-denial-check.txt"

echo "== 5/5: a deploy/api secret, read from the worker's role =="
run_check "worker-reads-api-secret" "scorecard-batch-worker" \
  "aws secretsmanager get-secret-value --secret-id scorecard/production/fastly"

echo
if [ "${FAIL}" -ne 0 ]; then
  echo "one or more boundaries did NOT deny as expected -- see FAIL lines above." >&2
  exit 1
fi
echo "all five boundaries denied as expected."
