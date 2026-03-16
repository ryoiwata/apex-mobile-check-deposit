# Session Log: Align App to System Flow Diagram

**Date:** 2026-03-15, 22:00
**Duration:** ~2 hours
**Focus:** Update backend and frontend to match every branching path, loop-back, and decision node defined in `docs/mobile-check-deposit-flow.jsx`

---

## What Got Done

### Backend — Funding Service (Collect-All)
- `funding/rules.go` — Refactored duplicate detection into two separate operations:
  - `checkDuplicateExists()` — non-destructive Redis EXISTS (used during rule evaluation, so a rejected deposit never marks a check as "deposited")
  - `markCheckDeposited()` — Redis SETNX, called only after all rules pass
  - Added `applyContributionCap()` — new Rule 2 for retirement accounts ($6,000 per-transaction cap, `MaxRetirementContributionCents`)
  - Added `dupeRedisKey()` shared helper; kept `applyDuplicateCheck()` for backward compatibility with existing tests
- `funding/service.go` — Full collect-all rewrite of `ApplyRules()`:
  - Added `RuleViolation` struct (`Code`, `Rule`, `Message`)
  - Added `CollectAllError` type wrapping `[]RuleViolation`, implements `error`
  - All three rules (deposit limit, contribution cap, duplicate check) now evaluated unconditionally; every violation collected before returning
  - Account eligibility kept as a hard-fail prerequisite (omnibus lookup needed downstream)

### Backend — Deposit Handler (Collect-All Response)
- `deposit/service.go` — Added `errors` import; `ApplyRules` `CollectAllError` is now passed through instead of swallowed
- `deposit/handler.go` — `Submit` handler now detects `CollectAllError` via `errors.As` and returns HTTP 422 with `{"error": "business_rules_failed", "violations": [...]}` array instead of a generic error string

### Backend — Vendor Service (IQA Retake Guidance)
- `vendor/models.go` — Added `RetakeGuidance string` field to `Response`
- `vendor/stub.go` — `iqaFailBlur` and `iqaFailGlare` now populate `RetakeGuidance` with specific, actionable instructions
- `models/transfer.go` — Added transient fields `RetakeGuidance *string` and `VendorErrorCode *string` (not stored in DB, populated in-memory from vendor response)
- `deposit/service.go` — IQA failure path now copies `RetakeGuidance` and `VendorErrorCode` onto the returned transfer so the handler can surface them to the client

### Backend — Return Handler (Notification Payload)
- `deposit/handler.go` — `Return` handler response enriched with: `original_amount_cents`, `fee_cents`, `total_debit_cents`, human-readable `message` with fee amount, and `"action": "new_deposit"` to signal the loop-back to the investor flow

### Backend — Settlement (After-Cutoff Rollover)
- `settlement/service.go` — `Batch` struct extended with `DepositsRolledToNextDay int` and `NextSettlementDate *string`
- Added `countDepositsAfterCutoff()` — counts `funds_posted` transfers created after the cutoff with no batch assigned
- Added `nextBusinessDay()` — skips Saturday/Sunday
- `RunSettlement()` now computes rolled count when current time is after cutoff; returns `status: "rolled_to_next_day"` with next date when no eligible deposits exist but post-cutoff ones are queued; always attaches rolled count to successful batch responses

### Backend — Operator (Contribution Type Override)
- `operator/service.go` — Added `OverrideContributionType()` method: validates transfer is in `Analyzing+flagged` state, updates `contribution_type` in a transaction, writes an `"override"` audit log entry with before/after values in metadata JSONB
- `operator/handler.go` — Added `OverrideContributionType` HTTP handler for `PATCH /api/v1/operator/deposits/:id/contribution-type`
- `main.go` — Registered the new PATCH route; added `PATCH` to CORS `Access-Control-Allow-Methods`

### Frontend — DepositForm (IQA Retake Loop + Collect-All Display)
- Full component rewrite with new state: `iqaError`, `violations`, `needsReauth`
- IQA failures (detected via `transfer.retake_guidance` on a rejected response) now replace the form with a retake prompt showing the specific guidance message; "Retake Photo" button resets only images — amount and account ID are preserved across retakes
- 422 responses with `violations` array render a list of all violations with rule name and message
- 401 responses show a re-authentication prompt
- Accepts `initialAccountId` prop so "Start New Deposit" from returned state can pre-select the account

### Frontend — ReviewQueue (Queue Cycling Loop)
- Added `actionCount` state to distinguish "never had items" from "worked through the queue"
- Queue count badge (`{n} pending`) shown in the header when items are present
- Empty state shows "All caught up!" when `actionCount > 0`, plain "No flagged deposits" otherwise
- `onAction` now calls `fetchQueue()` immediately (was already the case) with an explicit comment
- Settlement result panel updated to show `rolled_to_next_day` status in yellow with next date and queued count

### Frontend — TransferStatus (Return Notification + New Deposit Button)
- Added return notification panel that renders when `transfer.status === 'returned'`: shows return reason, $30 fee notice, and guidance to submit a new deposit
- "Start New Deposit" button calls `onStartNewDeposit(transfer.account_id)` prop if provided

### Frontend — App.jsx (Wiring)
- Added `returnAccountId` state and `handleStartNewDeposit()` callback
- `TransferStatus` receives `onStartNewDeposit={handleStartNewDeposit}`
- `DepositForm` receives `initialAccountId={returnAccountId}` so the account is pre-selected after a return

### Frontend — api.js
- Added `overrideContributionType(id, contributionType)` — `PATCH` to `/api/v1/operator/deposits/:id/contribution-type`

### Tests Added
- `vendor/stub_test.go` — `TestVendorFlow_IQABlur_RetakeGuidance`, `TestVendorFlow_IQAGlare_RetakeGuidance`, `TestVendorFlow_CleanPass_NoRetakeGuidance`
- `funding/rules_test.go` — `TestContributionCap_Retirement_UnderCap`, `TestContributionCap_Retirement_OverCap`, `TestContributionCap_Individual_AlwaysPasses`, `TestFundingFlow_CollectAll_DepositLimitAndDuplicate`, `TestFundingFlow_CollectAll_SingleViolation_OverLimit`, `TestFundingFlow_CollectAll_AllPass_NoViolations`
- Existing `rules_test.go` fixed: `dupeKey` helper updated to call `dupeRedisKey`, unused `crypto/sha256` import removed, `vendor` package import added

---

## Issues & Troubleshooting

- **Problem:** `applyDuplicateCheck` used `SetNX` — in the collect-all pattern, if a deposit is over-limit AND first-time, calling `SetNX` would mark the check as "deposited" even though the transfer gets rejected, permanently blocking the investor from resubmitting.
  - **Cause:** Original implementation combined "check if duplicate" and "mark as deposited" in one atomic operation, which was fine for the short-circuit pattern but breaks collect-all.
  - **Fix:** Split into `checkDuplicateExists()` (pure EXISTS, non-destructive) used during rule evaluation, and `markCheckDeposited()` (SETNX with TTL) called only after all rules pass. `applyDuplicateCheck` kept unchanged for backward-compatible tests.

- **Problem:** `funding/rules_test.go` had `import "crypto/sha256"` and a local `dupeKey` helper that duplicated hash logic.
  - **Cause:** The original test computed the Redis key inline; when `dupeRedisKey` was extracted as a shared helper, the test's local computation became redundant and the `sha256` import became unused.
  - **Fix:** Changed `dupeKey` in the test to simply call `dupeRedisKey(...)`, removed the `crypto/sha256` import.

- **Problem:** The `deposit/handler.go` originally imported only `errors`, `models`, `gin`, and `uuid` — the `CollectAllError` check required `funding` package and the enriched return response required `fmt`.
  - **Cause:** Adding `errors.As(err, &cae)` where `cae` is `*funding.CollectAllError` requires the `funding` import; `fmt.Sprintf` for the fee message requires `fmt`.
  - **Fix:** Added both imports.

- **Problem:** `ReviewQueue.jsx` initially needed a closing `</div>` adjustment when the queue header was restructured to nest the title/badge in a child `<div>`.
  - **Cause:** The original header had a flat two-child structure; adding the badge wrapper div changed the nesting depth.
  - **Fix:** The extra closing tag was added correctly on the second edit attempt.

---

## Decisions Made

- **Collect-all prerequisite exception:** Account eligibility remains a hard fail (not part of collect-all) because the omnibus account ID returned by `Resolve` is needed by ledger posting downstream. Without a valid account, there's nothing meaningful to report for contribution cap either. All other three rules (deposit limit, contribution cap, duplicate) run unconditionally.

- **Contribution cap threshold:** Set to `$6,000` (`MaxRetirementContributionCents = 600000`) for retirement accounts. This is distinct from the $5,000 deposit limit so the two rules can fire independently for amounts between $5,001–$6,000. For non-retirement accounts the cap never fires.

- **Duplicate check split (not rename):** Kept `applyDuplicateCheck` (SetNX) alongside the new `checkDuplicateExists` + `markCheckDeposited` to avoid breaking the 2 existing Redis integration tests (`TestDuplicateCheck_FirstDeposit_Allowed`, `TestDuplicateCheck_SecondDeposit_Rejected`) which test it directly.

- **RetakeGuidance as transient Transfer field:** Rather than a new API response wrapper type, `RetakeGuidance` and `VendorErrorCode` were added directly to `models.Transfer` as non-DB fields (`db:"-"`). The `scanTransfer` function only scans explicit columns so these stay nil after DB reads and are populated only in the vendor failure code path.

- **Rollover count only when after cutoff:** `countDepositsAfterCutoff` is only called when `time.Now()` is after the cutoff, avoiding an unnecessary DB query for midday settlement triggers.

- **`onStartNewDeposit` as prop, not context/global state:** Kept the callback threading explicit through `App → TransferStatus → DepositForm` rather than using React context, consistent with the existing prop-passing pattern in the codebase.

---

## Current State

**What's working (code complete, not yet runtime-verified — Go not installed in this environment):**
- Funding service evaluates all three rules every time and returns every violation at once
- IQA failures carry specific retake guidance through the full pipeline to the UI
- `DepositForm` handles three distinct rejection paths: IQA retake loop, collect-all violations list, session expiry re-auth
- Return handler response includes investor-facing fee message and `"action": "new_deposit"` signal
- `TransferStatus` shows a return notification panel with "Start New Deposit" button that navigates back to the deposit form with account pre-selected
- `ReviewQueue` shows queue depth, "All caught up" after working through items, and rolled-to-next-day settlement status
- `PATCH /api/v1/operator/deposits/:id/contribution-type` endpoint wired, audited, and accessible via `api.overrideContributionType()`
- Settlement response always includes how many deposits are queued for the next business day

**Not addressed this session (out of scope or deferred):**
- Bank ACK retry loop (item 7 in the original prompt) — requires a DB migration adding `retry_count` and `last_retry_at` to `settlement_batches`, a `POST /settlement/retry/:batch_id` endpoint, and escalation logic; deferred
- The `PATCH /contribution-type` UI in `ReviewQueue` — the API and backend are wired but the ReviewQueue card UI doesn't yet expose a dropdown/button for it
- Demo scripts not updated to exercise collect-all paths

---

## Next Steps

1. **Runtime verify** — Run `docker compose up --build` and exercise the collect-all path (submit over-limit + pre-seeded duplicate) to confirm the 422 violations array flows end-to-end.
2. **Bank ACK retry loop** — Add `retry_count INT DEFAULT 0` and `last_retry_at TIMESTAMPTZ` columns to `settlement_batches` (migration `003_settlement_retry.sql`); add `POST /api/v1/operator/settlement/retry/:batch_id` endpoint; add escalation to `"escalated"` status after `MAX_SETTLEMENT_RETRIES` (default 3).
3. **Contribution type override UI** — Add an override dropdown to `CheckCard` in `ReviewQueue.jsx` that calls `api.overrideContributionType()` and refreshes the card; show updated `contribution_type` alongside the approve button.
4. **Update demo scripts** — Extend `demo-all-scenarios.sh` to submit a deposit that triggers 2 violations (over-limit + duplicate) and assert the response contains a `violations` array with 2 elements.
5. **Add flow-coverage tests** — Implement the remaining tests listed in the original prompt: `TestOperatorFlow_ContributionOverride_BeforeApproval`, `TestSettlementFlow_AfterCutoff_RolledToNextDay`, `TestSettlementFlow_BankNoAck_RetryLoop`.
6. **Wire `onStartNewDeposit` to reset scenario** — Currently the DepositForm preserves the last-used vendor scenario when navigated to via "Start New Deposit". Consider resetting it to `CLEAN_PASS` on a return-triggered navigation.
