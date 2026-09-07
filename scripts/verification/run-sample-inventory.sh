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
# provision-cron-aws task 9.1 (verification), formalized. Runs the real
# scorecard-batch-controller against a small, explicit repository list
# instead of the two inventory files baked into its image, so a shard
# publishes and a real worker picks it up -- without ever touching the
# ~1.3M-repo production inventory.
#
# Since task 9.7 the results land in the PRODUCTION corpus, not the -test
# buckets this script was written against: it writes wherever the deployed
# scorecard-config ConfigMap points, and that is now the real thing. For
# data2 and rawdata that is harmless, since both are keyed by run and only
# ever add a prefix. In cron-results it overwrites <repo>/results.json and
# <repo>/raw.json, the latest-result pointers the live API serves, so every
# repository in the CSV becomes visible to users at whatever this run
# scores it. Versioning is on with a three-run retention window, and the
# CDN is purged per repository, so pick the list deliberately.
#
#   ./run-sample-inventory.sh [path/to/projects.csv]
#
# Defaults to testdata/sample-projects.csv (the same 3 repos used to find
# and verify the fix for the controller's missing rawdata IAM grant:
# github.com/ossf/scorecard, github.com/ossf/scorecard-infra,
# github.com/octocat/Hello-World).
#
# Clones the live CronJob's pod spec rather than duplicating it here, so
# this script tracks whatever cron/k8s/controller.yaml actually has deployed
# instead of a copy that can drift. Drops the worker-update sidecar (a
# rollout restart of all 14 workers is not needed for a 3-repo run) and
# repoints args at a ConfigMap-mounted copy of the CSV.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
CSV="${1:-${SCRIPT_DIR}/testdata/sample-projects.csv}"
JOB_NAME="scorecard-batch-controller-verify-$(date -u +%Y%m%dT%H%M%SZ | tr '[:upper:]' '[:lower:]')"
CONFIGMAP_NAME="scorecard-batch-sample-inventory"

command -v kubectl >/dev/null || { echo "error: kubectl is required" >&2; exit 1; }
command -v python3 >/dev/null || { echo "error: python3 is required" >&2; exit 1; }
[ -f "${CSV}" ] || { echo "error: no such file: ${CSV}" >&2; exit 1; }

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

# Read back from the deployed ConfigMap rather than hardcoded, so the hint
# printed at the end names whichever corpus this run actually wrote. The
# previous hardcoded -test name survived task 9.7 and was wrong for a day.
RESULT_BUCKET="$(kubectl get cm scorecard-config -o jsonpath='{.data.config\.yaml}' 2>/dev/null \
  | python3 -c "
import sys, yaml
print((yaml.safe_load(sys.stdin).get('result-data-bucket-url') or '').removeprefix('s3://') or '<result-data-bucket>')
" 2>/dev/null)"
: "${RESULT_BUCKET:=<result-data-bucket>}"

echo "== creating ConfigMap ${CONFIGMAP_NAME} from ${CSV} ==" >&2
kubectl create configmap "${CONFIGMAP_NAME}" \
  --from-file="projects-sample.csv=${CSV}" \
  --dry-run=client -o yaml | kubectl apply -f -

echo "== cloning scorecard-batch-controller CronJob's pod spec ==" >&2
kubectl get cronjob scorecard-batch-controller -o json > /tmp/scorecard-verify-cronjob.json

python3 - "${JOB_NAME}" "${CONFIGMAP_NAME}" <<'PYEOF'
import json
import sys

job_name, configmap_name = sys.argv[1], sys.argv[2]

with open('/tmp/scorecard-verify-cronjob.json') as f:
    cj = json.load(f)

pod_spec = cj['spec']['jobTemplate']['spec']['template']['spec']

# Drop the worker-update sidecar: this run doesn't touch the worker fleet.
pod_spec['containers'] = [c for c in pod_spec['containers'] if c['name'] == 'controller']

controller = pod_spec['containers'][0]
controller['args'] = [
    '--config=/etc/scorecard/config.yaml',
    '/sample/projects-sample.csv',
]
controller['volumeMounts'].append({
    'name': 'sample-inventory',
    'mountPath': '/sample',
    'readOnly': True,
})
pod_spec['volumes'].append({
    'name': 'sample-inventory',
    'configMap': {'name': configmap_name},
})

job = {
    'apiVersion': 'batch/v1',
    'kind': 'Job',
    'metadata': {
        'name': job_name,
        'labels': {'scorecard.dev/purpose': 'group-9-verification'},
    },
    'spec': {
        'template': {'spec': pod_spec},
        'backoffLimit': 0,
    },
}

with open('/tmp/scorecard-verify-job.json', 'w') as f:
    json.dump(job, f, indent=2)
PYEOF

echo "== applying Job ${JOB_NAME} ==" >&2
kubectl apply -f /tmp/scorecard-verify-job.json

echo "== waiting for completion (up to 60s) ==" >&2
if kubectl wait --for=condition=complete "job/${JOB_NAME}" --timeout=60s 2>/dev/null; then
  echo "controller completed cleanly" >&2
else
  echo "controller did not report Complete within 60s -- check its logs:" >&2
  echo "  kubectl logs job/${JOB_NAME}" >&2
fi

kubectl logs "job/${JOB_NAME}" 2>&1 || true

echo
echo "Job: ${JOB_NAME}"
echo "Inspect results with, e.g.:"
echo "  aws s3 ls s3://${RESULT_BUCKET}/\$(date -u +%Y.%m.%d)/"
echo "  ./check-bucket-consistency.sh <the printed HHMMSS prefix>"
