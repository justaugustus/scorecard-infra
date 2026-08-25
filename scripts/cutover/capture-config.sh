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
# Captures the results API's deployment configuration before it is changed
# (design W11 / task 6.0).
#
# None of this configuration lives in git. It cannot be restored with
# `git revert`, and it stops being readable the moment project access lapses --
# which, given the migration this supports, is a scheduled event rather than a
# hypothetical. Run it early; running it late is the same as not running it.
#
#   ./capture-config.sh <output-dir> [gcp-project]
#
# Best-effort by design: each capture is independent, and a section that fails
# for lack of permission or a wrong resource name records the failure and the
# run continues. A partial capture is useful; an aborted one is not.
#
#     !!  UNVERIFIED  !!
# These commands have not been executed against the real project -- the
# authoring environment has no gcloud and no credentials. Treat the resource
# names as informed guesses from api/openapi.yaml, api/Dockerfile, and
# docs/dns.md, and expect to correct some. The FAILED markers in the summary are
# where to start.
#
# Secrets: this script captures configuration *shape*, not secret values. It
# redacts anything that looks like a credential in environment blocks. Read the
# output before storing it anywhere shared, and never commit it -- see the
# .gitignore entry for scripts/cutover/out/.

set -uo pipefail   # deliberately not -e: see "best-effort" above

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
# Default somewhere gitignored and timestamped. Passing "." writes a dozen loose
# files into whatever directory you happened to be standing in -- which is how
# the first real run littered the repository root.
OUT="${1:-${SCRIPT_DIR}/out/$(date -u +%Y%m%dT%H%M%SZ)}"
PROJECT="${2:-openssf}"
REGION="${REGION:-us-central1}"

command -v gcloud >/dev/null || { echo "error: gcloud is required" >&2; exit 1; }

# Refuse to scatter output across a git working tree. The capture is a dozen
# files per run and none of it belongs in version control.
if [ -d "${OUT}/.git" ] || { [ "$(cd "${OUT}" 2>/dev/null && pwd)" = "$(git -C "${OUT}" rev-parse --show-toplevel 2>/dev/null)" ] && [ -n "$(git -C "${OUT}" rev-parse --show-toplevel 2>/dev/null)" ]; }; then
  cat >&2 <<EOF
error: refusing to write into a repository root: ${OUT}

  The capture writes ~12 loose files. Use the default, which is gitignored:

      $0

  or name a directory outside the repository.
EOF
  exit 2
fi

# Fail fast on credentials. Every capture below authenticates the same way, so
# an expired token is one problem, not fifteen -- and reporting it fifteen times
# buries the single instruction that fixes it.
if ! gcloud auth print-access-token >/dev/null 2>&1; then
  cat >&2 <<EOF
error: gcloud has no usable credentials for this session.

  Every capture would fail identically. Authenticate first:

      gcloud auth login
      gcloud config set project ${PROJECT}

  If you are running this from a non-interactive context (an agent session, CI),
  the login has to happen in a real terminal -- gcloud cannot prompt.
EOF
  exit 1
fi

mkdir -p "${OUT}"
SUMMARY="${OUT}/SUMMARY.txt"
: > "${SUMMARY}"

note() { echo "$*" | tee -a "${SUMMARY}"; }

# capture <slug> <description> <command...>
capture() {
  local slug="$1" desc="$2"
  shift 2
  local out="${OUT}/${slug}.json"
  if "$@" > "${out}" 2> "${OUT}/${slug}.err"; then
    if [ -s "${out}" ]; then
      note "ok       ${slug}  -- ${desc}"
      rm -f "${OUT}/${slug}.err"
    else
      note "EMPTY    ${slug}  -- ${desc} (command succeeded, returned nothing)"
    fi
  else
    note "FAILED   ${slug}  -- ${desc}"
    note "         $(head -1 "${OUT}/${slug}.err" 2>/dev/null)"
  fi
}

note "Results API deployment capture"
note "project: ${PROJECT}   region: ${REGION}   date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
note ""

# --- Build ---------------------------------------------------------------
# The trigger is what has to be repointed, or decommissioned if the build moves
# to GitHub Actions. Capture its full definition, not just its name: the
# substitutions and included/excluded file globs are the parts nobody remembers.
capture build-triggers "Cloud Build triggers" \
  gcloud builds triggers list --project="${PROJECT}" --format=json

# --- Run -----------------------------------------------------------------
# The service definition carries the image digest currently serving traffic,
# the env vars the container reads (FASTLY_PURGE_TOKEN, API_BASE_URL,
# STORAGE_EMULATOR_HOST), the service account, and the scaling settings.
# The revision list is the rollback target: "shift traffic back" needs a name.
capture run-services "Cloud Run services" \
  gcloud run services list --project="${PROJECT}" --region="${REGION}" --format=json
capture run-service-detail "Cloud Run service detail (scorecard-api-prod)" \
  gcloud run services describe scorecard-api-prod \
    --project="${PROJECT}" --region="${REGION}" --format=json
capture run-revisions "Cloud Run revisions (the rollback targets)" \
  gcloud run revisions list --project="${PROJECT}" --region="${REGION}" \
    --service=scorecard-api-prod --format=json
capture run-domain-mappings "Cloud Run domain mappings" \
  gcloud beta run domain-mappings list --project="${PROJECT}" --region="${REGION}" --format=json

# --- Endpoints / ESPv2 ---------------------------------------------------
# api/openapi.yaml is deployed here, so the live service config is the ground
# truth for what the gateway actually enforces -- which may have drifted from
# the checked-in contract. If the gateway is being dropped, this is the record
# of what stops being enforced.
capture endpoints-services "Cloud Endpoints services" \
  gcloud endpoints services list --project="${PROJECT}" --format=json
capture endpoints-config "Deployed Endpoints config (api.securityscorecards.dev)" \
  gcloud endpoints configs list --service=api.securityscorecards.dev \
    --project="${PROJECT}" --format=json

# --- DNS -----------------------------------------------------------------
# Capture the records before the repoint, so the rollback is a known value
# rather than a reconstruction. Note the TTLs: they set how long a bad cutover
# takes to undo.
capture dns-zones "Cloud DNS zones" \
  gcloud dns managed-zones list --project="${PROJECT}" --format=json
for zone in scorecard-dev securityscorecards-dev; do
  capture "dns-records-${zone}" "DNS records in ${zone}" \
    gcloud dns record-sets list --zone="${zone}" --project="${PROJECT}" --format=json
done

# --- IAM -----------------------------------------------------------------
# Which identity the service runs as, and what it can reach. The new deployment
# needs an equivalent, and "equivalent" is only checkable against this.
capture iam-policy "Project IAM policy" \
  gcloud projects get-iam-policy "${PROJECT}" --format=json
capture service-accounts "Service accounts" \
  gcloud iam service-accounts list --project="${PROJECT}" --format=json

# --- Storage -------------------------------------------------------------
# Bucket metadata only. The API reads gs://ossf-scorecard-results and falls back
# to gs://ossf-scorecard-cron-results; both need an equivalent on the far side,
# and their IAM is how the new deployment gets read access.
for bucket in ossf-scorecard-results ossf-scorecard-cron-results; do
  capture "bucket-${bucket}" "Bucket metadata: ${bucket}" \
    gcloud storage buckets describe "gs://${bucket}" --project="${PROJECT}" --format=json
done

# --- Secrets: names only -------------------------------------------------
# Never the values. What matters for the handoff is knowing which secrets exist
# and where they are referenced, so equivalents can be provisioned.
capture secrets "Secret Manager entries (names and metadata only)" \
  gcloud secrets list --project="${PROJECT}" --format=json

# --- Redaction pass ------------------------------------------------------
# gcloud does not return secret payloads, but env blocks can carry inline values
# for things like FASTLY_PURGE_TOKEN. Redact anything that looks like one before
# this directory is stored or shared.
note ""
note "Redacting probable inline credentials..."
for f in "${OUT}"/*.json; do
  [ -f "${f}" ] || continue
  if command -v jq >/dev/null && jq -e . "${f}" >/dev/null 2>&1; then
    if jq 'walk(
             if type == "object" and has("name") and has("value")
                and (.name | test("(?i)token|secret|key|password|credential"))
             then .value = "<redacted>"
             else . end
           )' "${f}" > "${f}.tmp" 2>/dev/null; then
      mv "${f}.tmp" "${f}"
    else
      # Leave the original in place; a failed redaction must not silently
      # truncate a capture that may be unrepeatable.
      rm -f "${f}.tmp"
      note "WARNING  redaction pass failed for $(basename "${f}") -- review it by hand"
    fi
  fi
done
note "done -- read the output before sharing it regardless."

# --- Things this script cannot capture -----------------------------------
note ""
note "NOT captured here; capture these by hand:"
note "  * Fastly service configuration, VCL, and surrogate-key setup."
note "    Needs a Fastly API token; the API purges via FASTLY_PURGE_TOKEN and"
note "    API_BASE_URL, and purge behavior is per-hostname -- see"
note "    scripts/api-conformance/README.md for why that matters."
note "  * The OSS-Fuzz project: google/oss-fuzz projects/scorecard-web/"
note "    (project.yaml and build.sh name the source repository). Public git;"
note "    no credentials needed."
note "  * Registry contents and image retention policy for the images the"
note "    Cloud Run service has run."
note "  * Whatever monitoring, alerting, or uptime checks point at the API."
note ""
note "Summary written to ${SUMMARY}"
