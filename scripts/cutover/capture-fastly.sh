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
# Why this matters more than its late arrival suggests: DNS shows
# api.scorecard.dev and api.securityscorecards.dev are both CNAMEs to
# x.sni.global.fastly.net. Fastly is the front door, the origin is configured
# *inside Fastly*, and no Cloud Run domain mapping is in the request path. So
# the cutover -- "shift traffic to the new deployment" -- is a change to a
# Fastly backend, and this configuration is what has to be understood before it
# and restored to roll back. gcloud cannot reach any of it.
#
#   ./capture-fastly.sh [output-dir]
#
# Uses the Fastly CLI (https://www.fastly.com/documentation/reference/cli/),
# which resolves 'active' to the serving version itself -- so this script does
# not have to fetch a version list and pick the active entry out of it, which is
# the step most likely to be wrong.
#
# Authentication, in the CLI's own order of preference:
#   * a token stored with `fastly auth` (run `fastly auth login` once), or
#   * the FASTLY_API_TOKEN environment variable, or
#   * --token, via FASTLY_TOKEN_FLAG below.
# The token needs read access to service configuration. The API's
# FASTLY_PURGE_TOKEN is very likely purge-scoped and will fail here.
#
# Status: run clean against the real account on 2026-08-25, all 23 sections, at
# CLI v16.0.0. Three things it got wrong first and now does not: the service
# list keys on `ServiceID`, not `id`; `vcl condition list` rejects `--json`; and
# domains live in the Domains v1 API, so `service domain list` returns [] for
# these services -- which reads as "no domains" rather than "wrong API".
#
# Output may contain credentials: VCL and snippets are free-form and can embed
# tokens. No pattern-based redaction is trustworthy against them, so this
# script does not pretend to redact. Read before sharing; output is gitignored.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUT="${1:-${SCRIPT_DIR}/out/fastly-$(date -u +%Y%m%dT%H%M%SZ)}"

# Passed to every invocation. Empty unless FASTLY_API_TOKEN is set, in which
# case the CLI picks it up from the environment anyway -- this exists so a
# stored auth profile and an env token both work without branching.
FASTLY_TOKEN_FLAG=()

command -v fastly >/dev/null || {
  cat >&2 <<'EOF'
error: the Fastly CLI is required.

  Install: https://www.fastly.com/documentation/reference/cli/
      brew install fastly/tap/fastly

EOF
  exit 1
}
command -v jq >/dev/null || { echo "error: jq is required" >&2; exit 1; }

mkdir -p "${OUT}"
SUMMARY="${OUT}/SUMMARY.txt"
: > "${SUMMARY}"
note() { echo "$*" | tee -a "${SUMMARY}"; }

# capture_ext <extension> <slug> <description> <fastly args...>
# Runs a CLI subcommand, records stdout, and never aborts the run: one section
# failing for lack of a permission should not cost the other fifteen.
capture_ext() {
  local ext="$1" slug="$2" desc="$3"
  shift 3
  if fastly "$@" "${FASTLY_TOKEN_FLAG[@]+"${FASTLY_TOKEN_FLAG[@]}"}" \
       > "${OUT}/${slug}.${ext}" 2> "${OUT}/${slug}.err"; then
    if [ -s "${OUT}/${slug}.${ext}" ]; then
      note "ok       ${slug}  -- ${desc}"
      rm -f "${OUT}/${slug}.err"
    else
      note "EMPTY    ${slug}  -- ${desc} (succeeded, returned nothing)"
    fi
  else
    note "FAILED   ${slug}  -- ${desc}"
    note "         $(head -1 "${OUT}/${slug}.err" 2>/dev/null)"
  fi
}

# Most subcommands support --json. `vcl condition list` does not -- passing it
# is rejected with a bare USAGE block, which is how this was found -- so that
# one is captured as text.
capture() { capture_ext json "$@"; }
capture_text() { capture_ext txt "$@"; }

# Fail fast on credentials, for the reason the gcloud capture does: every
# command below authenticates the same way, so a bad token is one problem
# reported once rather than fifteen times over the instruction that fixes it.
if ! fastly whoami "${FASTLY_TOKEN_FLAG[@]+"${FASTLY_TOKEN_FLAG[@]}"}" \
      > "${OUT}/whoami.txt" 2> "${OUT}/whoami.err"; then
  note "error: the Fastly CLI has no usable credentials."
  note "       $(head -2 "${OUT}/whoami.err" | tr '\n' ' ')"
  note ""
  note "  Authenticate once:      fastly auth login"
  note "  or pass a token:        FASTLY_API_TOKEN=... $0"
  note ""
  note "  A purge-scoped token will not work; this needs read access to"
  note "  service configuration."
  exit 1
fi

note "Fastly capture   date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
note "authenticated as: $(head -1 "${OUT}/whoami.txt")"
note ""

capture services "Services visible to this token" service list --json

# Domains live in the Domains v1 API here, NOT on the service version. Every
# `service domain list` returns [] for these services, which reads as "no
# domains" rather than "wrong API" -- so this is captured explicitly, and it is
# the file that answers which hostname reaches which service.
capture domains "Domain -> service mapping (Domains v1 API)" domain list --json

# Iterate the services rather than assuming which one fronts the API: the two
# API hostnames may be one service or two, and that is part of what we are here
# to find out. Not `mapfile` -- macOS ships bash 3.2, where it does not exist,
# and this runs from a maintainer's laptop.
SERVICE_IDS=""
while IFS= read -r line; do
  [ -n "${line}" ] && SERVICE_IDS="${SERVICE_IDS} ${line}"
done < <(jq -r '.[].ServiceID // empty' "${OUT}/services.json" 2>/dev/null)

if [ -z "${SERVICE_IDS// /}" ]; then
  note ""
  note "no services parsed from services.json -- inspect it by hand."
  note "If the CLI's JSON shape differs from what this expects, the service"
  note "loop below is what needs fixing, not your token."
  exit 1
fi

for sid in ${SERVICE_IDS}; do
  name=$(jq -r --arg id "${sid}" \
    '.[] | select(.ServiceID == $id) | .Name' \
    "${OUT}/services.json" 2>/dev/null)
  note ""
  note "service  ${name:-unknown} (${sid})"

  capture "svc-${sid}-service"  "  service detail" \
    service describe --service-id="${sid}" --json
  capture "svc-${sid}-versions" "  version list (earlier versions are rollback targets)" \
    service version list --service-id="${sid}" --json

  # 'active' is resolved by the CLI to whichever version is serving traffic.
  # Backends are the cutover: this is where the origin is named.
  capture "svc-${sid}-backends" "  backends (the origin -- what a cutover changes)" \
    service backend list --service-id="${sid}" --version=active --json
  capture_text "svc-${sid}-conditions" "  conditions (host-based routing; text, no --json)" \
    service vcl condition list --service-id="${sid}" --version=active
  capture "svc-${sid}-vcl-custom" "  custom VCL" \
    service vcl custom list --service-id="${sid}" --version=active --json
  capture "svc-${sid}-vcl-snippets" "  VCL snippets" \
    service vcl snippet list --service-id="${sid}" --version=active --json

  # The generated VCL is the effective configuration: cache settings, header
  # rules and request settings are compiled into it. The CLI exposes no
  # per-resource command for those, and this is a better record anyway -- it is
  # what Fastly actually runs, rather than the pieces it was assembled from.
  capture "svc-${sid}-vcl-generated" "  generated VCL (the effective config)" \
    service vcl describe --service-id="${sid}" --version=active --json
done

note ""
note "Read these first:"
note "  * svc-*-backends.json   -- the origin today. Changing it is the cutover;"
note "    restoring it is the rollback."
note "  * domains.json          -- which hostname reaches which service."
note "    Both production hostnames map to one service; Fastly still caches"
note "    them separately, keyed by Host, which is how they came to serve"
note "    results a week apart (task 6.3a)."
note "  * svc-*-conditions.txt and svc-*-vcl-generated.json -- host-based"
note "    routing. Only api.securityscorecards.dev has an Endpoints gateway in"
note "    front of it, so if the two hostnames reach different origins, that is"
note "    where it is expressed."
note ""
note "NOT redacted: VCL and snippets are free-form and can embed credentials."
note "Read before sharing. Output is gitignored."
note ""
note "Summary written to ${SUMMARY}"
