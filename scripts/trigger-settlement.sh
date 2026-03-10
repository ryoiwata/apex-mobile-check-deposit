#!/bin/bash
# trigger-settlement.sh — Trigger EOD settlement for today's date.
# Outputs batch_id, deposit_count, total_amount, file_path, status.
# Runs against a running docker compose stack.
# Usage: ./scripts/trigger-settlement.sh [YYYY-MM-DD]

set -euo pipefail

BASE="http://localhost:8080"
OP="OP-001"
BATCH_DATE="${1:-$(date +%Y-%m-%d)}"

echo "=============================="
echo " Settlement Trigger"
echo " Date: $BATCH_DATE"
echo " $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "=============================="
echo ""

RESP=$(curl -s -X POST "$BASE/api/v1/operator/settlement/trigger" \
    -H "X-Operator-ID: $OP" \
    -H "Content-Type: application/json" \
    -d "{\"batch_date\": \"$BATCH_DATE\"}")

echo "Response:"
echo "$RESP" | jq .
echo ""

BATCH_ID=$(echo "$RESP" | jq -r '.data.batch_id // "n/a"')
DEPOSIT_COUNT=$(echo "$RESP" | jq -r '.data.deposit_count // "n/a"')
TOTAL_AMOUNT=$(echo "$RESP" | jq -r '.data.total_amount_cents // "n/a"')
FILE_PATH=$(echo "$RESP" | jq -r '.data.file_path // "n/a"')
STATUS=$(echo "$RESP" | jq -r '.data.status // "n/a"')

echo "Batch ID        : $BATCH_ID"
echo "Deposit count   : $DEPOSIT_COUNT"
echo "Total (cents)   : $TOTAL_AMOUNT"
echo "File path       : $FILE_PATH"
echo "Status          : $STATUS"
echo ""

ERROR=$(echo "$RESP" | jq -r '.error // empty')
if [ -n "$ERROR" ]; then
    echo "ERROR: $ERROR"
    exit 1
fi

echo "Settlement triggered successfully."
exit 0
