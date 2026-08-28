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
# Verifies that the new nameservers serve the same zone as the old ones, before
# delegation is changed (task 6.3d).
#
#   ./dnsdiff.sh [--list] [capture-dir]
#
#     --list   print the expected record matrix and exit; no DNS queries, so it
#              works anywhere and shows exactly what coverage you are getting.
#
# THE CHECK LIST COMES FROM THE CAPTURE, NOT FROM A LIST IN THIS FILE
#
# The first version of this script hardcoded the domains and subdomains to
# check. Both of its blind spots came from that, and neither was visible in its
# output:
#
#   * It queried `securityscorecard.dev`. The zone is `securityscorecards.dev`
#     -- plural. Every lookup for it returned empty from both nameservers, and
#     a "skip if neither has a record" guard turned 20 silent failures into a
#     clean-looking section with no rows. api.securityscorecards.dev is one of
#     the two published API hostnames and it went unverified while the report
#     read as though the domain had passed.
#   * Its subdomain list was ("" api api-staging www), which omits the four
#     `_acme-challenge` CNAMEs. Those are the records whose loss is invisible
#     for weeks and then breaks Fastly TLS renewal -- exactly the ones a
#     pre-delegation check exists to catch.
#
# So the expected set is read from capture-config.sh's `dns-records-*.json`,
# which is what Cloud DNS actually served. A name cannot be missed by being
# forgotten, only by being absent from the capture. Silence is never success:
# a record in the capture that the new nameserver does not answer for is a
# hard failure, not a skipped row.
#
# Requires `dig` and `jq`, and network access to both nameservers.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"

OLD_NS="${OLD_NS:-ns-cloud-b1.googledomains.com}"
NEW_NS="${NEW_NS:-dns1.p01.nsone.net}"

LIST_ONLY=0
if [ "${1:-}" = "--list" ]; then LIST_ONLY=1; shift; fi

command -v jq >/dev/null || { echo "error: jq is required" >&2; exit 2; }
[ "${LIST_ONLY}" -eq 1 ] || command -v dig >/dev/null || \
  { echo "error: dig is required" >&2; exit 2; }

has_records() {
  local f
  for f in "$1"/dns-records-*.json; do [ -e "${f}" ] && return 0; done
  return 1
}

# Newest capture directory that actually holds DNS records.
find_capture() {
  local d
  find "${SCRIPT_DIR}/out" -mindepth 1 -maxdepth 1 -type d 2>/dev/null | sort |
    while IFS= read -r d; do has_records "${d}" && echo "${d}"; done | tail -1
}
CAPTURE="${1:-$(find_capture)}"

if [ -z "${CAPTURE}" ] || ! has_records "${CAPTURE}"; then
  cat >&2 <<EOF
error: no DNS capture found.

  Expected dns-records-*.json under a capture directory. Produce one with:

      ./capture-config.sh

  or pass the directory explicitly:

      $0 /path/to/capture-dir
EOF
  exit 2
fi

# --- Build the expected set ----------------------------------------------
# SOA is excluded: its serial changes on every edit, so comparing it across a
# migration reports a difference that is always there and never means anything.
# Apex NS is kept but classified separately -- it *must* differ during a
# delegation move, because it is the move. Counting it as a mismatch inflates
# the total and teaches people to skim the output.
EXPECTED="$(mktemp)"
trap 'rm -f "${EXPECTED}"' EXIT

for f in "${CAPTURE}"/dns-records-*.json; do
  jq -r '.[] | select(.type != "SOA")
         | [.name, .type, (.rrdatas | join(" "))] | @tsv' "${f}" 2>/dev/null
done | sort -u > "${EXPECTED}"

if [ ! -s "${EXPECTED}" ]; then
  echo "error: capture parsed but yielded no records: ${CAPTURE}" >&2
  exit 2
fi

if [ "${LIST_ONLY}" -eq 1 ]; then
  echo "Expected records from: ${CAPTURE}"
  echo "$(wc -l < "${EXPECTED}" | tr -d ' ') records"
  echo
  printf '%-52s %-6s %s\n' "NAME" "TYPE" "VALUE"
  while IFS="$(printf '\t')" read -r name type vals; do
    printf '%-52s %-6s %s\n' "${name}" "${type}" "${vals}"
  done < "${EXPECTED}"
  exit 0
fi

# Normalize for comparison: one token per line, quotes stripped, lowercased,
# sorted. Values containing spaces (a multi-string TXT record) would be split
# here; none of these zones has one, and the --list output is where you would
# notice if that changed.
norm() { tr ' ' '\n' | tr -d '"' | tr '[:upper:]' '[:lower:]' | sed '/^$/d' | sort; }

MATCH=0; MISMATCH=0; MISSING=0; EXPECTED_DIFF=0; DRIFT=0

echo "=========================================================="
echo " Pre-delegation DNS diff"
echo " capture: ${CAPTURE}"
echo " OLD NS:  ${OLD_NS}"
echo " NEW NS:  ${NEW_NS}"
echo "=========================================================="
echo

while IFS="$(printf '\t')" read -r NAME TYPE WANT; do
  [ -n "${NAME}" ] || continue

  WANT_N="$(printf '%s' "${WANT}" | norm)"
  OLD_N="$(dig @"${OLD_NS}" "${NAME}" "${TYPE}" +short +time=2 +tries=2 2>/dev/null | norm)"
  NEW_N="$(dig @"${NEW_NS}" "${NAME}" "${TYPE}" +short +time=2 +tries=2 2>/dev/null | norm)"

  # The delegation itself.
  if [ "${TYPE}" = "NS" ]; then
    EXPECTED_DIFF=$((EXPECTED_DIFF + 1))
    echo "ℹ️  [EXPECTED] ${NAME} (NS) -- delegation change, not a discrepancy"
    echo "      old: $(echo "${OLD_N}" | tr '\n' ' ')"
    echo "      new: $(echo "${NEW_N}" | tr '\n' ' ')"
    continue
  fi

  # The failure the old script could not report: the new zone simply does not
  # have the record. Empty is never a pass.
  if [ -z "${NEW_N}" ]; then
    MISSING=$((MISSING + 1))
    echo "❌ [MISSING]  ${NAME} (${TYPE}) -- absent from ${NEW_NS}"
    echo "      expected: $(echo "${WANT_N}" | tr '\n' ' ')"
    continue
  fi

  if [ "${NEW_N}" != "${WANT_N}" ]; then
    MISMATCH=$((MISMATCH + 1))
    echo "❌ [MISMATCH] ${NAME} (${TYPE})"
    echo "      captured: $(echo "${WANT_N}" | tr '\n' ' ')"
    echo "      new:      $(echo "${NEW_N}" | tr '\n' ' ')"
    continue
  fi

  # New matches the capture. If the *old* server has since moved, the capture
  # is stale as a rebuild source -- worth knowing before trusting it again.
  if [ -n "${OLD_N}" ] && [ "${OLD_N}" != "${WANT_N}" ]; then
    DRIFT=$((DRIFT + 1))
    echo "⚠️  [DRIFT]    ${NAME} (${TYPE}) -- live old zone differs from capture"
    echo "      captured: $(echo "${WANT_N}" | tr '\n' ' ')"
    echo "      old:      $(echo "${OLD_N}" | tr '\n' ' ')"
    continue
  fi

  MATCH=$((MATCH + 1))
  echo "✅ [MATCH]    ${NAME} (${TYPE})"
done < "${EXPECTED}"

echo
echo "=========================================================="
echo " match ${MATCH}   mismatch ${MISMATCH}   missing ${MISSING}   drift ${DRIFT}   expected-diff ${EXPECTED_DIFF}"
echo
echo " Not covered: records present in the new zone but absent from the"
echo " capture. Listing those needs a zone export from the new provider;"
echo " this script can only verify that nothing was dropped."

if [ $((MISMATCH + MISSING)) -gt 0 ]; then
  echo "⚠️  Do not delegate until these are resolved."
  echo "=========================================================="
  exit 1
fi
echo "🎉 Every captured record is served correctly by ${NEW_NS}."
echo "=========================================================="
