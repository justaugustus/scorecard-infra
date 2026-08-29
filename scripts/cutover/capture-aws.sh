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
# Captures what the AWS account already contains, before anything is
# provisioned into it (provision-aws, task 1.1, A6).
#
# The counterpart to capture-config.sh (GCP) and capture-fastly.sh. Those two
# exist because the running topology turned out to differ from the assumed one
# in four material ways -- three Cloud Run services rather than one, a gateway
# enforcing a three-year-old config, an Endpoints service for only one of the
# two hostnames, and DNS zone names that did not match their domains. There is
# no reason to expect better odds against an account this repository has never
# inspected, and one where an EC2 instance is already running for an initial
# API test.
#
#   ./capture-aws.sh [output-dir]
#
# This script answers one question the rest of the work is blocked on: WHICH
# REGION. The target region is defined as wherever the result buckets already
# are, not chosen -- a cluster in a different region from its buckets forfeits
# the S3 gateway endpoint and pays NAT egress on every object read, on a
# service whose entire workload is object reads (A5). So the bucket sweep runs
# first and the regions it finds drive every regional section below.
#
# Override that inference with AWS_REGIONS="us-east-1 us-west-2" if the account
# holds buckets that are not ours.
#
# Authentication: the standard AWS credential chain (SSO profile, environment,
# instance role). AWS_PROFILE and AWS_REGION are honoured.
#
# RUN THIS OFF ANY TLS-INTERCEPTING NETWORK. A corporate VPN that re-signs
# certificates makes the AWS CLI fail with "SSL validation failed ...
# self-signed certificate in certificate chain". That reads exactly like a
# permissions problem in the summary and is not one -- in the first real run it
# took out every call against one bucket while its neighbours succeeded, which
# is the most misleading shape this failure has. If a section fails on SSL,
# nothing has been learned about the account: re-run it, or set AWS_CA_BUNDLE.
#
# Status: first real run 2026-08-29, 20 of 20 sections against the live
# account. Three defects it exposed, all fixed here: the AWS CLI emits a blank
# line before its error so `head -1` reported seven failures with no reason
# beside any of them; GetBucketLifecycleConfiguration and GetBucketPolicy raise
# an error to mean "not configured", so every bucket was marked FAILED for
# having no lifecycle rule; and a successful call returning an empty list was
# reported as plain 'ok', which made an account containing no compute look
# identical to one full of it. Sections now carry item counts.
#
# Secret VALUES are never fetched. Secrets Manager is captured with
# list-secrets, which returns names, ARNs, and rotation metadata and no
# secret material. Do not add get-secret-value to this script. Output is
# gitignored regardless, because ARNs carry the account ID.

set -uo pipefail

SCRIPT_DIR="$(cd -- "$(dirname -- "${BASH_SOURCE[0]}")" && pwd)"
OUT="${1:-${SCRIPT_DIR}/out/aws-$(date -u +%Y%m%dT%H%M%SZ)}"

# The AWS CLI pages to a terminal by default, which hangs a non-interactive run.
export AWS_PAGER=""

command -v aws >/dev/null || {
  cat >&2 <<'EOF'
error: the AWS CLI is required.

  Install: https://docs.aws.amazon.com/cli/latest/userguide/getting-started-install.html
      brew install awscli

EOF
  exit 1
}
command -v jq >/dev/null || { echo "error: jq is required" >&2; exit 1; }

# Refuse a repository root. Passing '.' to capture-config.sh scattered a dozen
# loose files into a working tree, which is why that script grew this check and
# why this one has it before its first write rather than after.
if [ -e "${OUT}/.git" ]; then
  echo "error: refusing to write into a repository root: ${OUT}" >&2
  echo "       pass a subdirectory, or omit the argument for a timestamped one." >&2
  exit 1
fi

mkdir -p "${OUT}" || exit 1
SUMMARY="${OUT}/SUMMARY.txt"
: > "${SUMMARY}"
note() { echo "$*" | tee -a "${SUMMARY}"; }

# The AWS CLI prints a leading blank line before its error, so `head -1` on a
# .err file yields an empty string -- which is how the first real run reported
# seven FAILED sections with no reason beside any of them. Take the first line
# that actually has content.
first_error() {
  grep -m1 '[^[:space:]]' "$1" 2>/dev/null
}

# Several S3 "get" calls raise an error to mean "not configured" rather than
# returning an empty document. Reporting those as FAILED marked every bucket in
# the first run as broken and buried the one failure that was real.
NOT_CONFIGURED='NoSuchLifecycleConfiguration|NoSuchBucketPolicy|NoSuchTagSet'
NOT_CONFIGURED="${NOT_CONFIGURED}|NoSuchPublicAccessBlockConfiguration"
NOT_CONFIGURED="${NOT_CONFIGURED}|ServerSideEncryptionConfigurationNotFoundError"
NOT_CONFIGURED="${NOT_CONFIGURED}|NoSuchCORSConfiguration|ReplicationConfigurationNotFoundError"

# Count the items in whichever array an AWS list response wraps them in, so the
# summary distinguishes "returned a list of three" from "returned an empty
# list". Both are HTTP 200 and the first run reported both as plain 'ok', which
# made an account with no compute in it look identical to one full of it.
item_count() {
  jq -r '
    if type == "array" then length
    elif type == "object" then
      ([to_entries[] | select(.value | type == "array") | .value | length]
        | if length > 0 then .[0] else empty end)
    else empty end
  ' "$1" 2>/dev/null
}

# capture <slug> <description> <aws args...>
# Records stdout and never aborts the run: one section failing for lack of a
# permission should not cost the other twenty.
capture() {
  local slug="$1" desc="$2"
  shift 2
  if aws "$@" > "${OUT}/${slug}.json" 2> "${OUT}/${slug}.err"; then
    local n
    n="$(item_count "${OUT}/${slug}.json")"
    if [ ! -s "${OUT}/${slug}.json" ] ||
       [ "$(tr -d '[:space:]' < "${OUT}/${slug}.json")" = "null" ] ||
       [ "$(tr -d '[:space:]' < "${OUT}/${slug}.json")" = "{}" ]; then
      note "none     ${slug}  -- ${desc} (200, nothing configured)"
    elif [ "${n}" = "0" ]; then
      note "empty    ${slug}  -- ${desc} (200, zero items)"
    elif [ -n "${n}" ]; then
      note "ok  [${n}]  ${slug}  -- ${desc}"
    else
      note "ok       ${slug}  -- ${desc}"
    fi
    rm -f "${OUT}/${slug}.err"
  else
    local err
    err="$(first_error "${OUT}/${slug}.err")"
    if echo "${err}" | grep -qE "${NOT_CONFIGURED}"; then
      note "none     ${slug}  -- ${desc} (not configured)"
      rm -f "${OUT}/${slug}.err"
      return
    fi
    note "FAILED   ${slug}  -- ${desc}"
    note "         ${err}"
  fi
}

# Fail fast on credentials, for the reason both sibling scripts do: every
# command below authenticates identically, so a missing credential is one
# problem reported once rather than twenty times over the instruction that
# fixes it. capture-config.sh's first real run reported the same auth error
# fifteen times and buried the fix.
if ! aws sts get-caller-identity > "${OUT}/caller-identity.json" \
     2> "${OUT}/caller-identity.err"; then
  note "error: the AWS CLI has no usable credentials."
  note "       $(head -3 "${OUT}/caller-identity.err" | tr '\n' ' ')"
  note ""
  note "  Authenticate:        aws sso login --profile <profile>"
  note "  or select one:       AWS_PROFILE=<profile> $0"
  note ""
  note "  An SSL certificate error here is a trust-store problem, not a"
  note "  credential one -- set AWS_CA_BUNDLE to the CA bundle in use."
  exit 1
fi

note "AWS capture      date: $(date -u +%Y-%m-%dT%H:%M:%SZ)"
note "account:  $(jq -r '.Account // "unknown"' "${OUT}/caller-identity.json")"
note "identity: $(jq -r '.Arn // "unknown"' "${OUT}/caller-identity.json")"
note ""

# ---------------------------------------------------------------------------
# Global sections
# ---------------------------------------------------------------------------

capture account-aliases "Account alias" \
  iam list-account-aliases
capture regions-enabled "Regions enabled for this account" \
  account list-regions --output json
capture buckets "S3 buckets (account-wide; this drives the region inference)" \
  s3api list-buckets
capture iam-roles "IAM roles" \
  iam list-roles
capture iam-oidc-providers "IAM OIDC providers (GitHub Actions federation)" \
  iam list-open-id-connect-providers
capture route53-zones "Route 53 hosted zones (expected empty: zones are on Netlify DNS)" \
  route53 list-hosted-zones

# ---------------------------------------------------------------------------
# Per-bucket detail, and the region inference (A5)
# ---------------------------------------------------------------------------

# Not `mapfile` -- macOS ships bash 3.2, where it does not exist, and this runs
# from a maintainer's laptop. Same reason capture-fastly.sh reads this way.
BUCKETS=""
while IFS= read -r line; do
  [ -n "${line}" ] && BUCKETS="${BUCKETS} ${line}"
done < <(jq -r '.Buckets[]?.Name // empty' "${OUT}/buckets.json" 2>/dev/null)

BUCKET_REGIONS=""
if [ -n "${BUCKETS// /}" ]; then
  note ""
  note "buckets:"
  for b in ${BUCKETS}; do
    # get-bucket-location returns null for us-east-1, which is the one case
    # where "no answer" is an answer rather than a failure.
    loc=$(aws s3api get-bucket-location --bucket "${b}" \
            --query 'LocationConstraint' --output text 2>/dev/null)
    case "${loc}" in
      None|null|"") loc="us-east-1" ;;
    esac
    note "  ${b}  [${loc}]"
    case " ${BUCKET_REGIONS} " in
      *" ${loc} "*) ;;
      *) BUCKET_REGIONS="${BUCKET_REGIONS} ${loc}" ;;
    esac

    slug="bucket-${b}"
    capture "${slug}-versioning" "  ${b} versioning" \
      s3api get-bucket-versioning --bucket "${b}"
    capture "${slug}-encryption" "  ${b} encryption" \
      s3api get-bucket-encryption --bucket "${b}"
    capture "${slug}-lifecycle" "  ${b} lifecycle" \
      s3api get-bucket-lifecycle-configuration --bucket "${b}"
    capture "${slug}-public-access" "  ${b} public access block" \
      s3api get-public-access-block --bucket "${b}"
    capture "${slug}-policy" "  ${b} policy" \
      s3api get-bucket-policy --bucket "${b}"
  done
fi

# AWS_REGIONS overrides the inference, for an account holding buckets that are
# not ours. Falling back to the configured default is a last resort and is
# called out in the summary, because a single-region sweep that silently missed
# the real region would read as "nothing there".
REGIONS="${AWS_REGIONS:-${BUCKET_REGIONS}}"
if [ -z "${REGIONS// /}" ]; then
  REGIONS="$(aws configure get region 2>/dev/null)"
  note ""
  note "WARNING: no buckets found, so the region could not be inferred."
  note "         Falling back to the configured default: ${REGIONS:-none}."
  note "         Re-run with AWS_REGIONS=\"...\" once the real region is known;"
  note "         do not let this fallback stand in for an answer."
fi

if [ -z "${REGIONS// /}" ]; then
  note ""
  note "error: no region to sweep. Set AWS_REGIONS and re-run."
  note "Summary written to ${SUMMARY}"
  exit 1
fi

note ""
note "regional sweep over:${REGIONS}"

# ---------------------------------------------------------------------------
# Regional sections
# ---------------------------------------------------------------------------

for r in ${REGIONS}; do
  note ""
  note "region ${r}"

  capture "vpcs-${r}"        "  VPCs" \
    ec2 describe-vpcs --region "${r}"
  capture "subnets-${r}"     "  subnets" \
    ec2 describe-subnets --region "${r}"
  capture "natgw-${r}"       "  NAT gateways" \
    ec2 describe-nat-gateways --region "${r}"
  capture "eips-${r}"        "  Elastic IPs (static egress addresses)" \
    ec2 describe-addresses --region "${r}"
  capture "vpc-endpoints-${r}" "  VPC endpoints (is there an S3 gateway endpoint?)" \
    ec2 describe-vpc-endpoints --region "${r}"

  # An instance for the initial API test is known to exist. It is a useful test
  # bed and not a valid Fastly origin -- Fastly must verify a certificate for a
  # hostname, which a bare IP cannot present (A10).
  capture "ec2-instances-${r}" "  EC2 instances (an API test instance is expected)" \
    ec2 describe-instances --region "${r}"

  capture "eks-clusters-${r}" "  EKS clusters" \
    eks list-clusters --region "${r}"
  capture "sqs-queues-${r}"   "  SQS queues" \
    sqs list-queues --region "${r}"
  capture "load-balancers-${r}" "  load balancers" \
    elbv2 describe-load-balancers --region "${r}"
  capture "target-groups-${r}"  "  target groups" \
    elbv2 describe-target-groups --region "${r}"
  capture "acm-certs-${r}"      "  ACM certificates (the origin's cert lives here)" \
    acm list-certificates --region "${r}"

  # Names, ARNs and rotation metadata only. No values, deliberately -- see the
  # header. Three secrets are ours (github, gitlab, fastly); three in the GKE
  # cluster belong to the separate criticality-score service and are not.
  capture "secrets-${r}"        "  Secrets Manager entries (names only, no values)" \
    secretsmanager list-secrets --region "${r}"

  capture "quota-eip-${r}"      "  Elastic IP quota" \
    service-quotas list-service-quotas --service-code ec2 --region "${r}" \
      --query "Quotas[?contains(QuotaName, 'Elastic IP')]"
  capture "quota-eks-${r}"      "  EKS quotas" \
    service-quotas list-service-quotas --service-code eks --region "${r}"
done

note ""
note "Read these first:"
note "  * buckets.json and the [region] tags above -- this is the answer to"
note "    'which region', and it is an observation, not a choice. Everything"
note "    else goes wherever the result buckets already are (A5)."
note "  * ec2-instances-*.json -- the API test instance. Useful as a test bed,"
note "    not usable as a Fastly origin: Fastly must verify a certificate for a"
note "    hostname, and a bare IP cannot present one (A10)."
note "  * vpcs-*.json and vpc-endpoints-*.json -- is there already a VPC to"
note "    adopt, and does it have an S3 gateway endpoint? Adopting beats"
note "    duplicating, and duplicating is harder to unwind (task 1.4)."
note "  * secrets-*.json -- which of github, gitlab and fastly already exist."
note "    Three further secrets in the GKE cluster belong to criticality-score"
note "    and are not ours to move."
note "  * FAILED markers above -- on a first run these are more likely to be"
note "    defects in this script than facts about the account. That is how the"
note "    GCP capture went, twice."
note ""
note "Contains account identifiers (ARNs). Output is gitignored; read before"
note "sharing. No secret values are fetched."
note ""
note "Summary written to ${SUMMARY}"
