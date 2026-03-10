#!/bin/bash
# demo-return.sh — Return/reversal flow demo:
# Submit clean deposit → settlement → return → verify reversal + fee ledger entries.
# Runs against a running docker compose stack.
# Usage: ./scripts/demo-return.sh

set -euo pipefail

BASE="http://localhost:8080"
TOKEN="tok_investor_test"
OP="OP-001"
FRONT="scripts/fixtures/check-front.png"
BACK="scripts/fixtures/check-back.png"
TODAY=$(date -u +%Y-%m-%d)

PASS=0
FAIL=0

pass() { echo "  ✓ PASS: $1"; PASS=$((PASS+1)); }
fail() { echo "  ✗ FAIL: $1"; FAIL=$((FAIL+1)); }

assert_eq() {
    local label="$1" got="$2" want="$3"
    if [ "$got" = "$want" ]; then
        pass "$label (got: $got)"
    else
        fail "$label (want: $want, got: $got)"
    fi
}

assert_gte() {
    local label="$1" got="$2" want="$3"
    if [ "$got" -ge "$want" ]; then
        pass "$label (got: $got, want >= $want)"
    else
        fail "$label (want >= $want, got: $got)"
    fi
}

echo "=============================="
echo " DEMO: Return / Reversal Flow"
echo " $(date -u '+%Y-%m-%dT%H:%M:%SZ')"
echo "=============================="
echo ""

# ── Setup: Flush Redis to avoid stale duplicate hashes ───────────────────────
echo "Setup: Flushing Redis (clears stale duplicate-check hashes)"
FLUSH_OK=0
if DOCKER_HOST=unix:///var/run/docker.sock docker exec apex-mobile-check-deposit-redis-1 redis-cli FLUSHALL >/dev/null 2>&1; then
    echo "  Redis flushed (DOCKER_HOST=unix:///var/run/docker.sock)"
    FLUSH_OK=1
elif docker exec apex-mobile-check-deposit-redis-1 redis-cli FLUSHALL >/dev/null 2>&1; then
    echo "  Redis flushed (default docker)"
    FLUSH_OK=1
fi
if [ "$FLUSH_OK" -eq 0 ]; then
    echo "  WARNING: Redis flush failed — stale hashes may cause false duplicate rejections"
fi
echo ""

# Use a random amount to avoid duplicate hash collisions on re-run.
AMOUNT=$((10000 + RANDOM % 90000))
echo "Using deposit amount: ${AMOUNT} cents"
echo "Return fee: 3000 cents (\$30.00)"
echo ""

# ── Step 1: Submit clean-pass deposit ────────────────────────────────────────
echo "Step 1: Submit clean-pass deposit (ACC-SOFI-1006)"
RESP=$(curl -s -X POST "$BASE/api/v1/deposits" \
    -H "Authorization: Bearer $TOKEN" \
    -F "account_id=ACC-SOFI-1006" \
    -F "amount_cents=$AMOUNT" \
    -F "front_image=@$FRONT" \
    -F "back_image=@$BACK")

TRANSFER_ID=$(echo "$RESP" | jq -r '.data.transfer_id')
STATUS=$(echo "$RESP" | jq -r '.data.status')
echo "  Transfer ID : $TRANSFER_ID"
echo "  Status      : $STATUS"
assert_eq "deposit reaches funds_posted" "$STATUS" "funds_posted"
echo ""

# ── Step 2: Trigger settlement → completed ────────────────────────────────────
echo "Step 2: Trigger EOD settlement for $TODAY"
SETTLE=$(curl -s -X POST "$BASE/api/v1/operator/settlement/trigger" \
    -H "X-Operator-ID: $OP" \
    -H "Content-Type: application/json" \
    -d "{\"batch_date\": \"$TODAY\"}")

SETTLE_COUNT=$(echo "$SETTLE" | jq -r '.data.deposit_count')
echo "  Deposits settled: $SETTLE_COUNT"
assert_gte "settlement includes at least 1 deposit" "$SETTLE_COUNT" "1"

# Verify transfer is now completed.
COMPLETED=$(curl -s "$BASE/api/v1/deposits/$TRANSFER_ID" \
    -H "Authorization: Bearer $TOKEN")
COMPLETED_STATUS=$(echo "$COMPLETED" | jq -r '.data.status')
assert_eq "deposit is completed after settlement" "$COMPLETED_STATUS" "completed"
echo ""

# ── Step 3: POST return ───────────────────────────────────────────────────────
echo "Step 3: POST /operator/deposits/$TRANSFER_ID/return"
RETURN_RESP=$(curl -s -X POST "$BASE/api/v1/operator/deposits/$TRANSFER_ID/return" \
    -H "X-Operator-ID: $OP" \
    -H "Content-Type: application/json" \
    -d '{"return_reason":"insufficient_funds","bank_reference":"RET-DEMO-001"}')

RETURN_STATUS=$(echo "$RETURN_RESP" | jq -r '.data.status')
RETURN_AMOUNT=$(echo "$RETURN_RESP" | jq -r '.data.amount_cents')
echo "  Return status   : $RETURN_STATUS"
echo "  Transfer amount : $RETURN_AMOUNT"
assert_eq "transfer moves to returned" "$RETURN_STATUS" "returned"
assert_eq "amount_cents on returned transfer matches deposit" "$RETURN_AMOUNT" "$AMOUNT"
echo ""

# ── Step 4: Verify ledger entries ────────────────────────────────────────────
echo "Step 4: GET /ledger/ACC-SOFI-1006 — verify 3 entries (DEPOSIT + REVERSAL + RETURN_FEE)"
LEDGER=$(curl -s "$BASE/api/v1/ledger/ACC-SOFI-1006" \
    -H "Authorization: Bearer $TOKEN")

TRANSFER_ENTRIES=$(echo "$LEDGER" | jq '[.data.entries[] | select(.transfer_id == "'"$TRANSFER_ID"'")]')
ENTRY_COUNT=$(echo "$TRANSFER_ENTRIES" | jq 'length')

echo "  Ledger entries for transfer: $ENTRY_COUNT"
if [ "$ENTRY_COUNT" -ge 3 ]; then
    pass "ledger has 3+ entries for transfer (DEPOSIT + REVERSAL + RETURN_FEE)"
else
    fail "ledger has ${ENTRY_COUNT} entries, want >= 3 (DEPOSIT + REVERSAL + RETURN_FEE)"
fi

# Check DEPOSIT entry amount.
DEPOSIT_AMOUNT=$(echo "$TRANSFER_ENTRIES" | jq -r '[.[] | select(.sub_type == "DEPOSIT")] | .[0].amount_cents // "0"')
assert_eq "DEPOSIT entry amount matches original" "$DEPOSIT_AMOUNT" "$AMOUNT"

# Check REVERSAL entry amount.
REVERSAL_AMOUNT=$(echo "$TRANSFER_ENTRIES" | jq -r '[.[] | select(.sub_type == "REVERSAL")] | .[0].amount_cents // "0"')
assert_eq "REVERSAL entry amount matches original" "$REVERSAL_AMOUNT" "$AMOUNT"

# Check RETURN_FEE entry amount.
FEE_AMOUNT=$(echo "$TRANSFER_ENTRIES" | jq -r '[.[] | select(.sub_type == "RETURN_FEE")] | .[0].amount_cents // "0"')
assert_eq "RETURN_FEE entry amount is 3000 (\$30)" "$FEE_AMOUNT" "3000"
echo ""

# ── Summary ───────────────────────────────────────────────────────────────────
TOTAL=$((PASS+FAIL))
echo "=============================="
echo " Results: $PASS/$TOTAL tests passed"
echo "=============================="

if [ "$FAIL" -gt "0" ]; then
    exit 1
fi
