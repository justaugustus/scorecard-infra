#!/usr/bin/env bash

OLD_NS="ns-cloud-b1.googledomains.com"
NEW_NS="dns1.p01.nsone.net"
DOMAIN=

# List of domains to verify
DOMAINS=(
  "scorecard.dev"
  "securityscorecard.dev"
)

# Subdomains/prefixes to check ("" represents root/apex @)
SUBDOMAINS=("" "api" "api-staging" "www")

# Record types to inspect
RECORD_TYPES=("A" "AAAA" "CNAME" "TXT" "NS")

MISMATCH_COUNT=0
TOTAL_CHECKS=0

echo "=========================================================="
echo " Starting Multi-Domain DNS Diff"
echo " OLD NS: $OLD_NS"
echo " NEW NS: $NEW_NS"
echo "=========================================================="

for DOMAIN in "${DOMAINS[@]}"; do
  echo ""
  echo "=========================================================="
  echo " 🌐 Checking Domain: $DOMAIN"
  echo "=========================================================="

  for sub in "${SUBDOMAINS[@]}"; do
    NAME="${sub:+${sub}.}${DOMAIN}"

    for type in "${RECORD_TYPES[@]}"; do
      ((TOTAL_CHECKS++))

      # Query each nameserver directly and normalize output
      OLD_RES=$(dig @"$OLD_NS" "$NAME" "$type" +short +time=2 +tries=2 | sed 's/"//g' | sort)
      NEW_RES=$(dig @"$NEW_NS" "$NAME" "$type" +short +time=2 +tries=2 | sed 's/"//g' | sort)

      # Skip if neither nameserver has a record
      if [[ -z "$OLD_RES" && -z "$NEW_RES" ]]; then
        continue
      fi

      if [[ "$OLD_RES" != "$NEW_RES" ]]; then
        ((MISMATCH_COUNT++))
        echo "❌ [MISMATCH] $NAME ($type)"
        echo "   ├── OLD ($OLD_NS):"
        echo "${OLD_RES:-       <no record>}" | sed 's/^/   │   /'
        echo "   └── NEW ($NEW_NS):"
        echo "${NEW_RES:-       <no record>}" | sed 's/^/       /'
        echo "----------------------------------------------------------"
      else
        echo "✅ [MATCH] $NAME ($type)"
      fi
    done
  done
done

echo ""
echo "=========================================================="
echo " Scan Complete: $TOTAL_CHECKS checks executed."
if [[ $MISMATCH_COUNT -eq 0 ]]; then
  echo "🎉 All records match perfectly across all domains!"
else
  echo "⚠️  Found $MISMATCH_COUNT mismatch(es). Review differences before delegating."
fi
echo "=========================================================="
