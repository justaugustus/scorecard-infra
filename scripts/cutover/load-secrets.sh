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
# Loads the batch plane's credentials out of capture-gke.sh's output and into
# the AWS Secrets Manager containers deploy/cron/secrets declares (migrate-api
# 6.0a). Apply that root first -- this script writes values, it does not
# create secrets, matching the containers-then-values-out-of-band split the
# infra-as-code spec requires.
#
#   ./load-secrets.sh <gke-capture-dir> [cluster] [namespace]
#
# Defaults to the openssf/default namespace, which is where 6.0a found the
# github and gitlab secrets that matter. criticality-score's credentials are a
# different hosted service and are deliberately not handled here.
#
# One AWS Secrets Manager secret per Kubernetes Secret, value as a JSON object
# of its keys -- github's app_id/app_key/installation_id/token all move
# together, same as they exist today. Kubernetes Secret data is base64; this
# decodes it before upload, since Secrets Manager should hold the credential
# itself, not another layer of encoding on top of it.
#
# fastly is not handled here even though it lives in the same namespace: it
# collapses into the serving plane's existing scorecard/{staging,production}/fastly
# secret rather than getting a batch-side copy (6.0a's decision -- two live
# copies of one purge token is a rotation footgun, not redundancy).

set -uo pipefail

CAPTURE_DIR="${1:?usage: $0 <gke-capture-dir> [cluster] [namespace]}"
CLUSTER="${2:-openssf}"
NAMESPACE="${3:-default}"
NAME_PREFIX="scorecard/batch"

# The secrets this script is willing to load. Add to this list deliberately,
# not by making the script take whatever the capture happens to contain --
# criticality-score's github and the staging/enumerate variants exist in the
# same shape and are explicitly out of scope (6.0a).
SECRET_NAMES=(github gitlab)

command -v aws >/dev/null || { echo "error: aws CLI is required" >&2; exit 1; }
command -v jq  >/dev/null || { echo "error: jq is required" >&2; exit 1; }

SECRETS_FILE="${CAPTURE_DIR}/${CLUSTER}-${NAMESPACE}-secrets.json"
[ -f "${SECRETS_FILE}" ] || {
  echo "error: ${SECRETS_FILE} not found." >&2
  echo "  Expected capture-gke.sh output for cluster=${CLUSTER} namespace=${NAMESPACE}." >&2
  exit 1
}

# Fail fast, once, rather than once per secret -- every call below
# authenticates the same way.
if ! aws sts get-caller-identity >/dev/null 2>&1; then
  echo "error: AWS CLI has no usable credentials for this session." >&2
  exit 1
fi

DRY_RUN="${DRY_RUN:-0}"
STATUS=0

for NAME in "${SECRET_NAMES[@]}"; do
  SECRET_ID="${NAME_PREFIX}/${NAME}"

  DATA_JSON="$(jq -c --arg n "${NAME}" \
    '.items[]? | select(.metadata.name == $n) | .data // {}' \
    "${SECRETS_FILE}")"

  if [ -z "${DATA_JSON}" ] || [ "${DATA_JSON}" = "{}" ]; then
    echo "SKIP     ${NAME}  -- not found in ${SECRETS_FILE}" >&2
    STATUS=1
    continue
  fi

  # Decode every base64 value. Base64 is Kubernetes' wire encoding, not a
  # secret-worthy transform -- Secrets Manager should hold the credential,
  # not an encoded copy of it.
  VALUE_JSON="$(echo "${DATA_JSON}" | jq -c 'with_entries(.value |= @base64d)')"

  if ! aws secretsmanager describe-secret --secret-id "${SECRET_ID}" >/dev/null 2>&1; then
    echo "FAILED   ${SECRET_ID}  -- container does not exist yet." >&2
    echo "         Apply deploy/cron/secrets first (tofu apply)." >&2
    STATUS=1
    continue
  fi

  if [ "${DRY_RUN}" = "1" ]; then
    echo "DRY-RUN  ${SECRET_ID}  -- would load $(echo "${VALUE_JSON}" | jq -r 'keys | join(",")')"
    continue
  fi

  if aws secretsmanager put-secret-value \
      --secret-id "${SECRET_ID}" \
      --secret-string "${VALUE_JSON}" >/dev/null; then
    echo "ok       ${SECRET_ID}  -- loaded $(echo "${VALUE_JSON}" | jq -r 'keys | join(",")')"
  else
    echo "FAILED   ${SECRET_ID}  -- put-secret-value failed" >&2
    STATUS=1
  fi

  # Value never touches a shell variable printed elsewhere, and unset it here
  # rather than letting it sit in the process environment until the loop ends.
  unset VALUE_JSON DATA_JSON
done

if [ "${STATUS}" -eq 0 ] && [ "${DRY_RUN}" != "1" ]; then
  echo ""
  echo "Loaded. Delete ${CAPTURE_DIR} once you've confirmed the values read back:"
  echo "  aws secretsmanager get-secret-value --secret-id ${NAME_PREFIX}/github --query SecretString"
fi

exit "${STATUS}"
