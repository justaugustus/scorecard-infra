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
# Captures the Fastly configuration fronting the results API (task 6.0, W11).
#
# Why this is a separate script from capture-config.sh, and why it matters more
# than its late arrival suggests: DNS shows api.scorecard.dev and
# api.securityscorecards.dev are both CNAMEs to x.sni.global.fastly.net. Fastly
# is the front door, the origin is configured *inside Fastly*, and no Cloud Run
# domain mapping is in the request path. So the cutover -- "shift traffic to the
# new deployment" -- is a change to a Fastly backend, and this configuration is
# the thing that has to be understood before it, and restored to roll back.
#
# gcloud cannot reach any of it.
#
#   FASTLY_API_TOKEN=... ./capture-fastly.sh [output-dir]
#
# Token scope: needs read access to service configuration (`global:read` is
# enough). NOTE that this is probably NOT the same token as the API's
# FASTLY_PURGE_TOKEN in Secret Manager -- purge tokens are commonly scoped to
# purging alone and will 403 here. Expect to mint a read token.
#
#     !!  UNVERIFIED  !!
# Written without a token to test against. The endpoint paths follow Fastly's
# documented API, but treat a failure as "the call is wrong" before concluding
# anything about the service.
#
# Output may contain credentials. VCL and snippets are free-form and can embed
# tokens or shared secrets; no pattern-based redaction is trustworthy against
# them, so this script does not pretend to redact. Read before sharing. The
# output directory is gitignored.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUT="${1:-${SCRIPT_DIR}/out/fastly-$(date -u +%Y%m%dT%H%M%SZ)}"
API="https://api.fastly.com"

command -v curl >/dev/null || { echo "error: curl is required" >&2; exit 1; }
command -v jq   >/dev/null || { echo "error: jq is required" >&2; exit 1; }

if [ -z "${FASTLY_API_TOKEN:-}" ]; then
  cat >&2 <<'EOF'
error: FASTLY_API_TOKEN is not set.

  Mint a read-scoped token (global:read) in the Fastly UI under
  Account > API tokens, then:

      FASTLY_API_TOKEN=... ./capture-fastly.sh

  The API's FASTLY_PURGE_TOKEN is probably purge-scoped and will not work.
EOF
  exit 1
fi

mkdir -p "${OUT}"
SUMMARY="${OUT}/SUMMARY.txt"
: > "${SUMMARY}"
note() { echo "$*" | tee -a "${SUMMARY}"; }

# fapi <path> -> stdout JSON, non-zero on HTTP error
fapi() {
  local path="$1" code
  code=$(curl -sS -o "${OUT}/.tmp" -w '%{http_code}' \
    -H "Fastly-Key: ${FASTLY_API_TOKEN}" \
    -H "Accept: application/json" \
    "${API}${path}") || return 1
  case "${code}" in
    2*) cat "${OUT}/.tmp"; return 0 ;;
    *)  echo "HTTP ${code}: $(head -c 200 "${OUT}/.tmp")" >&2; return 1 ;;
  esac
}

# capture <slug> <description> <api-path>
capture() {
  local slug="$1" desc="$2" path="$3"
  if fapi "${path}" > "${OUT}/${slug}.json" 2> "${OUT}/${slug}.err"; then
    note "ok       ${slug}  -- ${desc}"
    rm -f "${OUT}/${slug}.err"
  else
    note "FAILED   ${slug}  -- ${desc}"
    note "         $(head -1 "${OUT}/${slug}.err" 2>/dev/null)"
  fi
}

# Fail fast on the token, for the same reason capture-config.sh does: every
# call below shares it, so a bad token is one problem reported once.
if ! fapi /current_user > "${OUT}/current_user.json" 2>"${OUT}/current_user.err"; then
  note "error: token rejected by ${API}/current_user"
  note "       $(head -1 "${OUT}/current_user.err")"
  note "       A purge-only token will fail here; this needs global:read."
  exit 1
fi

note "Fastly capture   date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
note "user: $(jq -r '.login // .name // "?"' "${OUT}/current_user.json")"
note ""

capture services "Services visible to this token" /service

# Find the services serving the API hostnames, rather than assuming one.
# Not `mapfile`: macOS ships bash 3.2, where it does not exist, and this script
# is most likely to be run from a maintainer's laptop.
SERVICE_IDS=""
while IFS= read -r line; do
  [ -n "${line}" ] && SERVICE_IDS="${SERVICE_IDS} ${line}"
done < <(jq -r '.[].id' "${OUT}/services.json" 2>/dev/null)
if [ -z "${SERVICE_IDS// /}" ]; then
  note "no services returned -- nothing further to capture"
  exit 1
fi

for sid in ${SERVICE_IDS}; do
  name=$(jq -r --arg id "${sid}" '.[] | select(.id==$id) | .name' "${OUT}/services.json")
  # The active version is the one serving traffic; earlier versions are the
  # rollback targets, exactly like Cloud Run revisions.
  ver=$(fapi "/service/${sid}" 2>/dev/null | jq -r '.versions[]? | select(.active==true) | .number' | head -1)
  if [ -z "${ver}" ]; then
    note "SKIP     ${name} (${sid}) -- no active version found"
    continue
  fi
  note "service  ${name} (${sid}) active version ${ver}"
  capture "svc-${sid}-details"  "  service detail + version list"      "/service/${sid}/details"
  # Backends are the cutover: this is where the origin is named.
  capture "svc-${sid}-backends" "  backends (the origins)"             "/service/${sid}/version/${ver}/backend"
  capture "svc-${sid}-domains"  "  domains served"                     "/service/${sid}/version/${ver}/domain"
  capture "svc-${sid}-vcl"      "  custom VCL"                         "/service/${sid}/version/${ver}/vcl"
  capture "svc-${sid}-snippets" "  VCL snippets"                       "/service/${sid}/version/${ver}/snippet"
  capture "svc-${sid}-conds"    "  conditions (host routing lives here)" "/service/${sid}/version/${ver}/condition"
  capture "svc-${sid}-cache"    "  cache settings"                     "/service/${sid}/version/${ver}/cache_settings"
  capture "svc-${sid}-headers"  "  header rules"                       "/service/${sid}/version/${ver}/header"
  capture "svc-${sid}-reqset"   "  request settings"                   "/service/${sid}/version/${ver}/request_settings"
done

rm -f "${OUT}/.tmp"

note ""
note "Read these first:"
note "  * svc-*-backends.json  -- what the origin is today. Changing it is the"
note "    cutover; restoring it is the rollback."
note "  * svc-*-domains.json   -- confirms which service serves which hostname,"
note "    and whether both API hostnames share one service. They cache"
note "    separately either way, keyed by Host."
note "  * svc-*-conds.json     -- host-based routing. Only"
note "    api.securityscorecards.dev has an Endpoints gateway in front of it, so"
note "    if the two hostnames reach different origins, it is expressed here."
note ""
note "NOT redacted: VCL and snippets are free-form and can embed credentials."
note "Read before sharing. Output is gitignored."
note ""
note "Summary written to ${SUMMARY}"
