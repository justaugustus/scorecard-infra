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
# provision-cron-aws task 9.4: confirm no inconsistent output landed in the
# test buckets from a killed/retried attempt -- no partial write, no
# duplicate, nothing tagged with the wrong commit.
#
#   ./check-bucket-consistency.sh <date> <time-prefix>
#   ./check-bucket-consistency.sh 2026.09.06 064901
#
# What "consistent" means here, from reading cron/internal/worker/main.go
# directly (not assumed): the raw bucket's shard file is written BEFORE the
# data2 shard file, and data2's presence is what worker.go's resultExists
# check gates re-processing on -- so a crash between those two writes is
# self-healing (a retry rewrites both, same key, no duplicate). What this
# script can actually catch is the shard-level pair (data2 vs rawdata) and
# each per-repo cron-results export existing for every repo the shard
# result lists -- it cannot detect a wrong-commit tag without a second,
# independent scan to compare against, so that check is a manual read of
# the printed output, not an automated pass/fail.

set -uo pipefail

DATE="${1:?usage: check-bucket-consistency.sh <YYYY.MM.DD> <HHMMSS-prefix>}"
PREFIX="${2:?usage: check-bucket-consistency.sh <YYYY.MM.DD> <HHMMSS-prefix>}"

command -v aws >/dev/null || { echo "error: the AWS CLI is required" >&2; exit 1; }

echo "== data2-test/${DATE}/${PREFIX}/ =="
aws s3 ls "s3://ossf-scorecard-data2-test/${DATE}/${PREFIX}/"
echo
echo "== rawdata-test/${DATE}/${PREFIX}/ =="
aws s3 ls "s3://ossf-scorecard-rawdata-test/${DATE}/${PREFIX}/"

DATA2_SHARDS="$(aws s3 ls "s3://ossf-scorecard-data2-test/${DATE}/${PREFIX}/" | awk '{print $4}' | grep '^shard-' || true)"
RAW_SHARDS="$(aws s3 ls "s3://ossf-scorecard-rawdata-test/${DATE}/${PREFIX}/" | awk '{print $4}' | grep '^shard-' || true)"

if [ "${DATA2_SHARDS}" != "${RAW_SHARDS}" ]; then
  echo
  echo "MISMATCH: data2 and rawdata don't have the same shard files for this prefix:"
  echo "  data2:   ${DATA2_SHARDS}"
  echo "  rawdata: ${RAW_SHARDS}"
  exit 1
fi

if [ -z "${DATA2_SHARDS}" ]; then
  echo
  echo "No shard files found for ${DATE}/${PREFIX} in either bucket yet -- too early, or wrong prefix."
  exit 1
fi

echo
echo "== per-repo cron-results-test exports, one row per repo in the shard =="
for SHARD in ${DATA2_SHARDS}; do
  aws s3 cp "s3://ossf-scorecard-data2-test/${DATE}/${PREFIX}/${SHARD}" - | python3 -c "
import json, subprocess, sys

missing = []
for line in sys.stdin:
    line = line.strip()
    if not line:
        continue
    d = json.loads(line)
    repo = d['repo']['name']
    commit = d.get('repo', {}).get('commit', '')
    for key in (f'{repo}/results.json', f'{repo}/{commit}/results.json'):
        out = subprocess.run(
            ['aws', 's3api', 'head-object', '--bucket', 'ossf-scorecard-cron-results-test', '--key', key],
            capture_output=True,
        )
        status = 'OK' if out.returncode == 0 else 'MISSING'
        print(f'{status}: {key}')
        if out.returncode != 0:
            missing.append(key)

if missing:
    sys.exit(1)
"
done

echo
echo "If everything above says OK: 9.4 passes for this shard. A commit-SHA" \
     "mismatch (the wrong content under the right key) needs a manual diff" \
     "against a known-good re-scan -- this script only checks presence."
