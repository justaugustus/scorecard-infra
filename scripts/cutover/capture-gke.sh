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
# Captures Kubernetes Secrets and ConfigMaps from the project's GKE clusters
# before the account is shut down (design W11 / task 6.0).
#
#   ./capture-gke.sh [output-dir] [gcp-project]
#
# WHY THIS EXISTS SEPARATELY FROM capture-config.sh
#
# capture-config.sh enumerates Cloud Run, Cloud Build, Endpoints, DNS, IAM,
# buckets, and Secret Manager. It has no GKE section, so a whole class of
# configuration was invisible to it: Secret Manager in this project holds
# exactly one entry (fastly_purge_token), while the GitHub App credentials the
# scanning side depends on are Kubernetes Secrets living inside a cluster.
# Nothing was wrong with that capture; it was looking at a different layer.
#
# THIS SCRIPT WRITES CREDENTIALS TO DISK. THE OTHER TWO DO NOT.
#
# capture-config.sh runs a redaction pass over anything resembling a secret.
# That is exactly the wrong behavior here -- the values *are* the artifact,
# because a credential that cannot be replayed into the new environment has not
# been migrated. So:
#
#   * The output directory is created with mode 0700 and the run sets
#     `umask 077`. Read it, move what you need into a real secret store, and
#     delete the directory. It is gitignored, which stops an accident, not a
#     habit.
#   * Secret values are base64 as Kubernetes returns them. Base64 is an
#     encoding, not encryption. Treat this directory exactly as you would treat
#     the plaintext.
#   * Prefer re-issuing a credential over transplanting it where the issuer
#     makes that cheap. A GitHub App private key can be rotated; the point of
#     capturing it is to avoid an outage if rotation has to wait, not to make
#     rotation unnecessary.
#
# Best-effort per section, like its siblings: a namespace that denies access
# records the failure and the run continues. A partial capture is useful; an
# aborted one is not.
#
# Nothing is filtered out. Service-account token Secrets are cluster-bound and
# worthless once the cluster is gone, but they are captured anyway and flagged
# in INDEX.tsv rather than dropped -- silently omitting rows from a capture that
# cannot be repeated is the more expensive mistake.

set -uo pipefail   # deliberately not -e: see "best-effort" above

umask 077

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUT="${1:-${SCRIPT_DIR}/out/gke-$(date -u +%Y%m%dT%H%M%SZ)}"
PROJECT="${2:-openssf}"

command -v gcloud  >/dev/null || { echo "error: gcloud is required" >&2; exit 1; }
command -v kubectl >/dev/null || { echo "error: kubectl is required" >&2; exit 1; }

# GKE dropped in-tree auth: kubectl cannot talk to a cluster without this
# plugin, and the error it produces otherwise names neither the plugin nor the
# install command.
if ! command -v gke-gcloud-auth-plugin >/dev/null; then
  cat >&2 <<EOF
error: gke-gcloud-auth-plugin is not on PATH.

  kubectl cannot authenticate to GKE without it:

      gcloud components install gke-gcloud-auth-plugin

EOF
  exit 1
fi

# Refuse to scatter output across a git working tree -- capture-config.sh's
# first real run littered the repository root, and this one writes credentials.
if [ -d "${OUT}/.git" ] || { [ "$(cd "${OUT}" 2>/dev/null && pwd)" = "$(git -C "${OUT}" rev-parse --show-toplevel 2>/dev/null)" ] && [ -n "$(git -C "${OUT}" rev-parse --show-toplevel 2>/dev/null)" ]; }; then
  cat >&2 <<EOF
error: refusing to write into a repository root: ${OUT}

  This capture contains credentials. Use the default, which is gitignored:

      $0

  or name a directory outside the repository.
EOF
  exit 2
fi

# Fail fast on credentials. Every call below authenticates the same way, so an
# expired token is one problem reported once, not one per namespace.
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
chmod 700 "${OUT}"
SUMMARY="${OUT}/SUMMARY.txt"
INDEX="${OUT}/INDEX.tsv"
: > "${SUMMARY}"
printf 'cluster\tnamespace\tkind\tname\ttype\tkeys\n' > "${INDEX}"

# Keep the operator's real kubeconfig out of this. get-credentials rewrites
# ~/.kube/config and switches the current context as a side effect, which is a
# rude thing to do to someone mid-incident.
export KUBECONFIG="${OUT}/kubeconfig"
: > "${KUBECONFIG}"

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

note "GKE Secrets and ConfigMaps capture"
note "project: ${PROJECT}   date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
note ""
note "This directory contains credential material. Move what you need into a"
note "real secret store and delete it; do not leave it sitting in a checkout."
note ""

# --- Clusters ------------------------------------------------------------
capture clusters "GKE clusters in ${PROJECT}" \
  gcloud container clusters list --project="${PROJECT}" --format=json

if [ ! -s "${OUT}/clusters.json" ]; then
  note ""
  note "No clusters listed -- nothing further to capture. If you expected some,"
  note "check the project and that the account has container.clusters.list."
  exit 1
fi

# bash 3.2 on macOS has no mapfile. Read from a file rather than a pipe: a
# piped `while` runs in a subshell, so any tally of failed clusters would be
# discarded at the end of the loop -- which for a capture whose whole job is
# telling you what it could not get would be the wrong thing to lose.
CLUSTER_LIST="${OUT}/.clusters.tsv"
FAILED_CLUSTERS="${OUT}/.failed-clusters"
jq -r '.[] | "\(.name)\t\(.location)"' "${OUT}/clusters.json" 2>/dev/null \
  > "${CLUSTER_LIST}"
: > "${FAILED_CLUSTERS}"

if [ ! -s "${CLUSTER_LIST}" ]; then
  note "FAILED   could not parse cluster list -- inspect clusters.json by hand"
  exit 1
fi

while IFS="$(printf '\t')" read -r CLUSTER LOCATION; do
  [ -n "${CLUSTER}" ] || continue
  note ""
  note "=== cluster ${CLUSTER} (${LOCATION}) ==="

  if ! gcloud container clusters get-credentials "${CLUSTER}" \
        --location="${LOCATION}" --project="${PROJECT}" \
        >"${OUT}/creds-${CLUSTER}.err" 2>&1; then
    note "FAILED   credentials for ${CLUSTER}"
    note "         $(head -1 "${OUT}/creds-${CLUSTER}.err")"
    echo "${CLUSTER}" >> "${FAILED_CLUSTERS}"
    continue
  fi
  rm -f "${OUT}/creds-${CLUSTER}.err"

  # Keep the error. `get-credentials` only writes a kubeconfig -- it never
  # contacts the cluster -- so this is the first call that can actually fail,
  # and it fails differently for a private endpoint, a missing RBAC binding,
  # and a cluster that is not running. Discarding stderr here reported "could
  # not list namespaces" with no way to tell which, on the one cluster that
  # mattered.
  NS_ERR="${OUT}/ns-${CLUSTER}.err"
  NAMESPACES="$(kubectl get namespaces -o jsonpath='{.items[*].metadata.name}' 2>"${NS_ERR}")"
  if [ -z "${NAMESPACES}" ]; then
    note "FAILED   could not list namespaces in ${CLUSTER}"
    note "         $(tr '\n' ' ' < "${NS_ERR}" | cut -c1-300)"
    echo "${CLUSTER}" >> "${FAILED_CLUSTERS}"
    continue
  fi
  rm -f "${NS_ERR}"
  note "namespaces: ${NAMESPACES}"

  for NS in ${NAMESPACES}; do
    # The two the action item actually asked for.
    capture "${CLUSTER}-${NS}-secrets" "Secrets in ${NS} (VALUES INCLUDED)" \
      kubectl get secrets -n "${NS}" -o json
    capture "${CLUSTER}-${NS}-configmaps" "ConfigMaps in ${NS}" \
      kubectl get configmaps -n "${NS}" -o json

    # Workload manifests. Not requested, and cheap: these are how the scanning
    # side is actually deployed, they name every image and every env var that
    # references the Secrets above, and they become unreadable at the same
    # moment. Delete this block if the capture should stay narrow.
    capture "${CLUSTER}-${NS}-workloads" "Workloads in ${NS}" \
      kubectl get deployments,statefulsets,daemonsets,cronjobs,jobs \
        -n "${NS}" -o json

    # One row per object, so the whole inventory is greppable without opening
    # a dozen JSON files. Keys only -- values stay in the JSON.
    for KIND in secrets configmaps; do
      F="${OUT}/${CLUSTER}-${NS}-${KIND}.json"
      [ -s "${F}" ] || continue
      jq -r --arg c "${CLUSTER}" --arg n "${NS}" --arg k "${KIND}" \
        '.items[]? | [$c, $n, $k, .metadata.name, (.type // "-"),
                      ((.data // {}) | keys | join(","))] | @tsv' \
        "${F}" >> "${INDEX}" 2>/dev/null
    done
  done
done < "${CLUSTER_LIST}"

# Before the inventory, not after it. A cluster that could not be read is the
# most important thing this run has to say, and twenty "ok" lines above one
# "FAILED" reads as success -- the capture is only as good as its worst gap.
if [ -s "${FAILED_CLUSTERS}" ]; then
  note ""
  note "=========================================================="
  note "INCOMPLETE -- $(wc -l < "${FAILED_CLUSTERS}" | tr -d ' ') cluster(s) not captured:"
  while IFS= read -r c; do [ -n "${c}" ] && note "  * ${c}"; done < "${FAILED_CLUSTERS}"
  note ""
  note "Whatever lives in them is NOT in this capture. Read the reason above"
  note "before assuming the credentials you were looking for are elsewhere."
  note "=========================================================="
fi

note ""
note "Inventory: ${INDEX}"
if [ -s "${INDEX}" ]; then
  note "  $(( $(wc -l < "${INDEX}") - 1 )) objects"
  note ""
  note "Service-account token Secrets are included and are cluster-bound --"
  note "they are not worth migrating. Filter them when reading the index:"
  note "  grep -v kubernetes.io/service-account-token ${INDEX}"
fi

note ""
note "NOT captured here:"
note "  * Anything outside GKE -- run ./capture-config.sh for Cloud Run, Cloud"
note "    Build, Endpoints, DNS, IAM, buckets, and Secret Manager, and"
note "    ./capture-fastly.sh for the CDN, which is the cutover control plane."
note "  * Credentials held outside this project entirely: GitHub organization"
note "    and repository Actions secrets cannot be read back through any API"
note "    and have to be re-issued rather than exported."
note "  * Workload Identity bindings, which tie these service accounts to GCP"
note "    identities that will not exist after the move. capture-config.sh's"
note "    iam-policy.json has the GCP half."
note ""
note "Next: move these into a secret store, then delete this directory."
note "Summary written to ${SUMMARY}"

# Exit nonzero on a missed cluster, so "the capture ran" and "the capture is
# complete" cannot be confused by a caller that only checks the status.
[ -s "${FAILED_CLUSTERS}" ] && exit 1
exit 0
