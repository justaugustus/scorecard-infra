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
# Proves that a result written by this plane actually reaches users, rather
# than landing in the bucket behind a CDN entry nobody invalidated.
#
#   ./verify-cdn-purge.sh [--yes] [repo-uri]
#
# Why this needs its own script. Every failure mode here is silent. The
# worker's purge errors are logged at Info and swallowed, so a run whose
# every purge failed still reports success; and when purging is switched
# off entirely, `getPurger` returns a no-op client and the run does not
# even log an error. In both cases the writes succeed, the bucket is
# correct, and the served results are stale. The only way to tell is to
# compare what the CDN serves against what the origin serves.
#
# That comparison is the assertion. A cache-busting query string reaches
# the origin, the bare URL is served by the CDN, and the two agreeing is
# what "the purge worked" means. Watching the `age` header is weaker:
# api.scorecard.dev sends `max-age: 600` but Fastly has been observed
# serving entries more than 22 hours old, so `age` says more about POP
# routing than about invalidation.
#
# Choosing a probe repository. `getResults` serves a self-published Action
# result ahead of the cron one, so a repository that publishes its own
# scores never exercises this path at all -- its API response does not come
# from the bucket this plane writes. Cron records are date-only
# (`r.Date.Format("2006-01-02")`) and Action records carry a full RFC 3339
# timestamp, which is how the preflight below tells them apart. The default
# probe is github.com/octocat/Hello-World: cron-served, already in the
# sample inventory, and not owned by us.
#
# This writes to the production corpus. One repository's results.json and
# raw.json are overwritten -- the same keys Monday's run overwrites anyway,
# on a bucket with versioning and a three-run retention window. It still
# asks first unless --yes is passed.
#
# Requires AWS_CA_BUNDLE to be exported; see the runbook in
# deploy/cron/README.md.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
API_HOST="https://api.scorecard.dev"
ASSUME_YES=0

while [ $# -gt 0 ]; do
  case "$1" in
    --yes|-y) ASSUME_YES=1; shift ;;
    -*) echo "error: unknown flag: $1" >&2; exit 2 ;;
    *) break ;;
  esac
done

REPO="${1:-github.com/octocat/Hello-World}"

for tool in kubectl aws curl python3; do
  command -v "${tool}" >/dev/null || { echo "error: ${tool} is required" >&2; exit 1; }
done

# The EKS context name is the full ARN, which embeds the account ID -- matched
# by suffix here rather than hardcoded whole, since this repository is public.
CTX="$(kubectl config current-context 2>/dev/null)"
case "${CTX}" in
  */scorecard-batch) ;;
  *)
    echo "error: kubectl context is '${CTX}', not the scorecard-batch cluster." >&2
    exit 1
    ;;
esac

fail() { echo "FAIL: $*" >&2; FAILED=1; }
FAILED=0

# --- Preflight: is purging even wired up? -------------------------------
#
# Checked before the scan rather than inferred from the result afterwards,
# so a misconfiguration reports itself instead of looking like a purge that
# did not take effect.

echo "== preflight ==" >&2

# 1. No env-level override. An empty SCORECARD_API_BASE_URL on the worker
#    short-circuits getPurger to a no-op before it ever reads the token, and
#    a non-empty one silently overrides the mounted config.
OVERRIDE="$(kubectl get deploy scorecard-batch-worker -o json 2>/dev/null | python3 -c "
import json, sys
d = json.load(sys.stdin)
for c in d['spec']['template']['spec']['containers']:
    for e in c.get('env', []):
        if e['name'] == 'SCORECARD_API_BASE_URL':
            print(repr(e.get('value', '')))
")"
if [ -n "${OVERRIDE}" ]; then
  fail "worker sets SCORECARD_API_BASE_URL=${OVERRIDE}; it overrides the" \
       "mounted config. Purging is off if empty. Remove it from cron/k8s/worker.yaml."
else
  echo "  ok: no SCORECARD_API_BASE_URL override on the worker" >&2
fi

# 2. The mounted config names a base URL, and the results bucket to watch.
CFG="$(kubectl get cm scorecard-config -o jsonpath='{.data.config\.yaml}' 2>/dev/null)"
read -r BASE_URL RESULTS_BUCKET <<<"$(printf '%s' "${CFG}" | python3 -c "
import sys, yaml
p = yaml.safe_load(sys.stdin).get('additional-params', {}).get('scorecard', {})
url = (p.get('api-results-bucket-url') or '').removeprefix('s3://')
print(p.get('api-base-url') or '-', url or '-')
")"
if [ "${BASE_URL}" = "-" ]; then
  fail "api-base-url is empty in the scorecard-config ConfigMap; getPurger returns a no-op client."
else
  echo "  ok: api-base-url is ${BASE_URL}" >&2
fi
[ "${RESULTS_BUCKET}" = "-" ] && { echo "error: no api-results-bucket-url in the ConfigMap" >&2; exit 1; }
echo "  ok: results bucket is ${RESULTS_BUCKET}" >&2

# 3. The token exists. getPurger checks this after the base URL, so an
#    absent secret is the other way purging silently becomes a no-op.
if [ -z "$(kubectl get secret fastly -o jsonpath='{.data.purge_token}' 2>/dev/null)" ]; then
  fail "the 'fastly' secret has no purge_token; getPurger returns a no-op client."
else
  echo "  ok: fastly/purge_token is present" >&2
fi

[ "${FAILED}" -eq 1 ] && { echo; echo "preflight failed -- not running a scan." >&2; exit 1; }

# --- Baseline ------------------------------------------------------------

api_date() {
  # $1: full URL. Prints the record's date field, or '-'.
  curl -sf "$1" 2>/dev/null | python3 -c "
import json, sys
try:
    print(json.load(sys.stdin).get('date') or '-')
except Exception:
    print('-')
" 2>/dev/null || echo '-'
}

s3_mtime() {
  aws s3api head-object --bucket "${RESULTS_BUCKET}" \
    --key "${REPO}/results.json" --query 'LastModified' --output text 2>/dev/null || echo '-'
}

echo >&2
echo "== baseline for ${REPO} ==" >&2

CDN_BEFORE="$(api_date "${API_HOST}/projects/${REPO}")"
ORIGIN_BEFORE="$(api_date "${API_HOST}/projects/${REPO}?cb=$$${RANDOM}")"
S3_BEFORE="$(s3_mtime)"

echo "  cdn:    ${CDN_BEFORE}" >&2
echo "  origin: ${ORIGIN_BEFORE}" >&2
echo "  s3:     ${S3_BEFORE}" >&2

case "${ORIGIN_BEFORE}" in
  *T*)
    echo >&2
    echo "error: ${REPO} is served from a self-published Action result, not the" >&2
    echo "cron bucket -- its date carries a full timestamp. This probe cannot" >&2
    echo "observe a cron purge. Pick a repository that does not publish its own" >&2
    echo "scores." >&2
    exit 1
    ;;
  -)
    echo "error: no API result for ${REPO}" >&2
    exit 1
    ;;
esac

if [ "${CDN_BEFORE}" != "${ORIGIN_BEFORE}" ]; then
  echo "  note: CDN is already stale against origin (${CDN_BEFORE} vs ${ORIGIN_BEFORE})," >&2
  echo "        which is the condition this run should clear." >&2
fi

if [ "${ASSUME_YES}" -ne 1 ]; then
  echo >&2
  printf 'This overwrites %s/{results,raw}.json in %s. Continue? [y/N] ' "${REPO}" "${RESULTS_BUCKET}" >&2
  read -r reply
  case "${reply}" in [yY]*) ;; *) echo "aborted." >&2; exit 1 ;; esac
fi

# --- Run a one-repository scan ------------------------------------------

CSV="$(mktemp -t cdn-purge-probe.XXXXXX)"
trap 'rm -f "${CSV}"' EXIT
printf 'repo,metadata\n%s,\n' "${REPO}" > "${CSV}"

echo >&2
echo "== scanning ${REPO} through the real controller and worker ==" >&2
"${SCRIPT_DIR}/run-sample-inventory.sh" "${CSV}" || {
  echo "error: the sample inventory run failed; see its output above." >&2
  exit 1
}

# --- Wait for the write, then check the purge ---------------------------
#
# The controller returning is only the shard being published. Waiting on the
# object's LastModified is what confirms a worker actually processed it.

echo >&2
echo "== waiting for a worker to rewrite ${REPO}/results.json (up to 15m) ==" >&2
DEADLINE=$(( $(date +%s) + 900 ))
S3_AFTER="${S3_BEFORE}"
while [ "$(date +%s)" -lt "${DEADLINE}" ]; do
  S3_AFTER="$(s3_mtime)"
  [ "${S3_AFTER}" != "${S3_BEFORE}" ] && break
  sleep 15
done

if [ "${S3_AFTER}" = "${S3_BEFORE}" ]; then
  fail "the object was never rewritten (still ${S3_BEFORE}); the scan did not" \
       "complete, so the purge was never exercised."
else
  echo "  ok: rewritten at ${S3_AFTER}" >&2
fi

# Fastly purges are fast but not synchronous with the write.
sleep 10

echo >&2
echo "== comparing CDN against origin ==" >&2
CDN_AFTER="$(api_date "${API_HOST}/projects/${REPO}")"
ORIGIN_AFTER="$(api_date "${API_HOST}/projects/${REPO}?cb=$$${RANDOM}")"
echo "  cdn:    ${CDN_AFTER}" >&2
echo "  origin: ${ORIGIN_AFTER}" >&2

if [ "${CDN_AFTER}" != "${ORIGIN_AFTER}" ]; then
  fail "the CDN still serves ${CDN_AFTER} while the origin serves ${ORIGIN_AFTER}." \
       "The result was written but never invalidated -- this is the failure the" \
       "script exists to catch."
else
  echo "  ok: CDN matches origin" >&2
fi

# --- What the workers said ----------------------------------------------
#
# Corroborates the observation above, and distinguishes "purging was off"
# from "purges were attempted and rejected" -- the token being wrong looks
# identical from outside.

echo >&2
echo "== worker purge log lines ==" >&2
LOGS="$(kubectl logs -l app.kubernetes.io/name=worker --tail=2000 --since=30m 2>/dev/null \
  | grep -iE 'purg' | sort | uniq -c | sort -rn | head -20)"
if [ -z "${LOGS}" ]; then
  echo "  (none -- the fleet may have rolled since the scan)" >&2
else
  printf '%s\n' "${LOGS}" >&2
fi
if printf '%s' "${LOGS}" | grep -qi 'purging disabled'; then
  fail "a worker logged 'purging disabled'."
fi
if printf '%s' "${LOGS}" | grep -qi 'failed to purge'; then
  fail "a worker logged 'failed to purge' -- purging is wired up but Fastly rejected it."
fi

echo >&2
if [ "${FAILED}" -eq 0 ]; then
  echo "PASS: ${REPO} was rewritten and the CDN now serves it." >&2
  exit 0
fi
echo "FAILED -- see the messages above." >&2
exit 1
