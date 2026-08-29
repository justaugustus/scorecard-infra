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
# Results-API conformance harness (design W11, step 3).
#
#   ./conformance.sh capture  <base-url> <out-dir>
#   ./conformance.sh compare  <base-url-a> <base-url-b> [out-dir]
#
# Base URLs must include an explicit scheme (https:// or http://). A bare
# hostname's default scheme is not something to guess at: against an
# ALB-fronted origin, plain HTTP hits the HTTP->HTTPS redirect listener before
# reaching the application, so a bare host silently compares two redirect
# pages instead of the app.
#
# `compare` queries both deployments and reports any request whose observable
# behavior differs. It is the cutover gate: the migrated service must answer
# identically to the one it replaces.
#
# Why compare live-against-live rather than against a committed fixture: result
# bodies change whenever a repository is rescanned, so a stored baseline is
# stale within a week and every later run drowns in false differences. Querying
# both endpoints in the same run removes time as a variable. `capture` exists
# for the record, not for the gate.
#
# What counts as a difference:
#   status code, Content-Type, Location, and the caching headers the CDN and
#   browsers act on (Cache-Control, Surrogate-Control, Surrogate-Key), plus the
#   response body -- normalized, see below.
#
# What is ignored, because it differs between any two deployments without the
# behavior differing: Date, Server, Alt-Svc, Via, X-Cloud-Trace-Context,
# X-Request-Id, Content-Length (recomputed after normalization), and any
# Set-Cookie.
#
# Body normalization: Scorecard results embed a `date` (the scan date) and the
# engine `version`/`commit`. Two deployments reading the same bucket return the
# same values, so these are NOT normalized by default -- a difference in them is
# a real finding (it means the two are reading different data). Pass
# --ignore-scan-metadata to relax that when comparing across a deliberate data
# migration.

set -euo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
REQUESTS="${REQUESTS:-${SCRIPT_DIR}/requests.tsv}"
IGNORE_SCAN_METADATA=0

# Headers that must match for the deployments to be considered equivalent.
SIGNIFICANT_HEADERS='^(content-type|location|cache-control|surrogate-control|surrogate-key|access-control-allow-origin|access-control-allow-methods|access-control-allow-headers|www-authenticate|retry-after):'

die() { echo "error: $*" >&2; exit 1; }

# A bare hostname's default scheme is not a safe assumption: curl's default
# varies by version, and a plain-HTTP request to an ALB-fronted origin hits
# its HTTP->HTTPS redirect listener before ever reaching the application --
# silently producing a comparison of two redirect pages instead of the app.
# Require the scheme explicitly rather than let that happen again.
require_scheme() {
  case "$1" in
    http://*|https://*) ;;
    *) die "'$1' has no scheme -- pass https://$1 (or http://), not a bare hostname. A bare host's default scheme is not guaranteed, and against an ALB origin it silently compares HTTP-redirect pages instead of the application." ;;
  esac
}

usage() {
  sed -n '17,24p' "${BASH_SOURCE[0]}" | sed 's/^# \{0,1\}//'
  exit 2
}

# fetch <base-url> <path> <out-prefix> <extra-curl-args...>
# Writes <prefix>.status, <prefix>.headers, <prefix>.body.
fetch() {
  local base="$1" path="$2" prefix="$3"
  shift 3
  local url="${base%/}${path}"

  # --globoff: paths may contain [] or {}; curl would treat them as globs.
  # No -L: a 302 from the badge endpoint is the behavior under test, not a step
  # on the way to it.
  local status
  status=$(curl -sS --globoff --max-time 30 \
    -o "${prefix}.body.raw" \
    -D "${prefix}.headers.raw" \
    -w '%{http_code}' \
    "$@" "${url}" || echo "000")

  echo "${status}" > "${prefix}.status"

  # Normalize headers: lowercase names, drop volatile ones, sort for stable diff.
  tr -d '\r' < "${prefix}.headers.raw" \
    | awk -F': ' 'NF>1 {printf "%s: %s\n", tolower($1), substr($0, index($0,": ")+2)}' \
    | grep -Ei "${SIGNIFICANT_HEADERS}" \
    | sort > "${prefix}.headers" || true

  # Cache diagnostics: recorded, never compared. When a body differs, the first
  # question is "are these two different results, or the same result at
  # different cache vintages" -- and that is answerable from age/x-cache without
  # a second round of investigation.
  tr -d '\r' < "${prefix}.headers.raw" \
    | awk -F': ' 'NF>1 {printf "%s: %s\n", tolower($1), substr($0, index($0,": ")+2)}' \
    | grep -Ei '^(age|x-cache|x-served-by|via):' \
    | sort > "${prefix}.info" || true

  normalize_body "${prefix}.body.raw" > "${prefix}.body"
  rm -f "${prefix}.body.raw" "${prefix}.headers.raw"
}

# Pretty-print JSON so formatting differences do not read as behavior
# differences; pass anything else through unchanged.
normalize_body() {
  local f="$1"
  if jq -e . "${f}" >/dev/null 2>&1; then
    if [ "${IGNORE_SCAN_METADATA}" -eq 1 ]; then
      jq -S 'if type == "object" then (.date? |= "<ignored>") | (.scorecard? |= "<ignored>") else . end' "${f}"
    else
      jq -S . "${f}"
    fi
  else
    cat "${f}"
  fi
}

# run_set <base-url> <out-dir>
run_set() {
  local base="$1" dir="$2"
  mkdir -p "${dir}"
  local name path extra
  while IFS=$'\t' read -r name path extra; do
    [ -z "${name:-}" ] && continue
    case "${name}" in \#*) continue ;; esac
    [ -z "${path:-}" ] && die "request '${name}' has no path"
    # extra holds raw curl args (quoted headers); word-splitting is intended.
    # shellcheck disable=SC2086
    eval "fetch \"\${base}\" \"\${path}\" \"\${dir}/\${name}\" ${extra:-}"
    printf '  %-24s %s\n' "${name}" "$(cat "${dir}/${name}.status")"
  done < "${REQUESTS}"
}

cmd_capture() {
  local base="${1:-}" dir="${2:-}"
  [ -n "${base}" ] && [ -n "${dir}" ] || usage
  require_scheme "${base}"
  echo "Capturing ${base} -> ${dir}"
  run_set "${base}" "${dir}"
  echo "Captured $(find "${dir}" -name '*.status' | wc -l | tr -d ' ') requests."
}

cmd_compare() {
  local a="${1:-}" b="${2:-}" dir="${3:-}"
  [ -n "${a}" ] && [ -n "${b}" ] || usage
  require_scheme "${a}"
  require_scheme "${b}"
  if [ -z "${dir}" ]; then
    dir="$(mktemp -d)"
    # Double-quoted so ${dir} is substituted now, while it's a local in this
    # function's scope -- the trap only fires after the script's last command,
    # by which point cmd_compare has returned and a deferred lookup of ${dir}
    # would be unbound under `set -u`.
    # shellcheck disable=SC2064
    trap "rm -rf '${dir}'" EXIT
  fi

  echo "A: ${a}"
  run_set "${a}" "${dir}/a"
  echo "B: ${b}"
  run_set "${b}" "${dir}/b"

  echo
  local differences=0 checked=0 name
  while IFS= read -r f; do
    name="$(basename "${f}" .status)"
    checked=$((checked + 1))
    local report=""
    for part in status headers body; do
      if ! diff -q "${dir}/a/${name}.${part}" "${dir}/b/${name}.${part}" >/dev/null 2>&1; then
        # `|| true` is load-bearing: diff exits non-zero when files differ, and
        # under `set -e` that would abort the run on the first difference --
        # silently, mid-loop, which is the worst possible behavior for a gate.
        local detail
        detail="$(diff -u \
          --label "A/${name}.${part}" "${dir}/a/${name}.${part}" \
          --label "B/${name}.${part}" "${dir}/b/${name}.${part}" 2>&1 | head -40 || true)"
        report="${report}$(printf '\n    --- %s ---\n%s' "${part}" "${detail}")"
      fi
    done
    if [ -n "${report}" ]; then
      differences=$((differences + 1))
      printf 'DIFF  %s%s\n' "${name}" "${report}"
      # Cache vintage is the most common innocent explanation for a body
      # difference between two CDN-fronted endpoints, so show it unprompted.
      printf '\n    --- cache diagnostics (not compared) ---\n'
      printf '    A: %s\n' "$(tr '\n' ' ' < "${dir}/a/${name}.info")"
      printf '    B: %s\n\n' "$(tr '\n' ' ' < "${dir}/b/${name}.info")"
    else
      printf 'same  %s\n' "${name}"
    fi
  done < <(find "${dir}/a" -name '*.status' | sort)

  echo
  if [ "${differences}" -eq 0 ]; then
    echo "PASS: ${checked} requests, no observable differences."
    return 0
  fi
  echo "FAIL: ${differences} of ${checked} requests differ."
  echo
  echo "A difference is not automatically a defect -- but it is a question that"
  echo "must be answered before traffic moves, not after. Re-run with"
  echo "--ignore-scan-metadata only if the two deployments are knowingly reading"
  echo "different data."
  return 1
}

main() {
  local args=()
  for arg in "$@"; do
    case "${arg}" in
      --ignore-scan-metadata) IGNORE_SCAN_METADATA=1 ;;
      -h|--help) usage ;;
      *) args+=("${arg}") ;;
    esac
  done
  set -- "${args[@]+"${args[@]}"}"

  command -v curl >/dev/null || die "curl is required"
  command -v jq   >/dev/null || die "jq is required"
  [ -f "${REQUESTS}" ] || die "request set not found: ${REQUESTS}"

  local sub="${1:-}"
  shift || true
  case "${sub}" in
    capture) cmd_capture "$@" ;;
    compare) cmd_compare "$@" ;;
    *) usage ;;
  esac
}

main "$@"
