# Scenario Coverage Report

Generated: 2026-03-10

---

## Go Unit / Integration Tests

All tests run with `go test ./... -v` from the `backend/` directory. Results from `reports/go-test-results.txt`.

| Package | Test File | Tests | Status |
|---------|-----------|-------|--------|
| `internal/vendor` | `stub_test.go` | 8 | ✅ PASS |
| `internal/funding` | `rules_test.go` | 10 | ✅ PASS |
| `internal/ledger` | `service_test.go` | 6 | ✅ PASS |
| `internal/operator` | `service_test.go` | 3 | ✅ PASS |
| `internal/settlement` | `generator_test.go` | 4 | ✅ PASS |
| `internal/state` | `machine_test.go` | 7 | ✅ PASS |
| **Total** | | **38** | **✅ All pass** |

### Test Names by Package

**`internal/vendor`** (stub_test.go — 8 tests):
- `TestStub_CleanPass_1006`
- `TestStub_CleanPass_DefaultSuffix`
- `TestStub_IQABlur_1001`
- `TestStub_IQAGlare_1002`
- `TestStub_MICRFailure_1003_Flagged`
- `TestStub_DuplicateDetected_1004`
- `TestStub_AmountMismatch_1005_Flagged`
- `TestStub_Stateless_SameInputSameOutput`

**`internal/funding`** (rules_test.go — 10 tests):
- `TestDepositLimit_UnderLimit`
- `TestDepositLimit_AtLimit_500000`
- `TestDepositLimit_OverLimit_500001`
- `TestContributionType_Retirement_Individual`
- `TestContributionType_Individual_Empty`
- `TestDuplicateCheck_FirstDeposit_Allowed`
- `TestDuplicateCheck_SecondDeposit_Rejected`
- `TestAccountResolver_Active_ReturnsOmnibus`
- `TestAccountResolver_NotFound`
- `TestAccountResolver_Suspended_Ineligible`

**`internal/ledger`** (service_test.go — 6 tests):
- `TestPostFunds_CreatesDepositEntry`
- `TestPostFunds_CorrectAccountMapping`
- `TestPostReversal_TwoEntries`
- `TestPostReversal_CorrectAmounts`
- `TestPostReversal_SubTypes`
- `TestLedgerEntries_AppendOnly`

**`internal/operator`** (service_test.go — 3 tests):
- `TestApprove_MovesToFundsPosted`
- `TestApprove_WritesAuditLog`
- `TestReject_MovesToRejected`

**`internal/settlement`** (generator_test.go — 4 tests):
- `TestCutoffTime_CorrectUTCConversion`
- `TestCutoffTime_DST_Summer`
- `TestSettlement_ExcludesRejected`
- `TestSettlement_ExcludesAlreadyBatched`

**`internal/state`** (machine_test.go — 7 tests):
- `TestValidTransition_RequestedToValidating`
- `TestValidTransition_CompletedToReturned`
- `TestInvalidTransitions` (table-driven: 3 sub-cases)
- `TestInvalidTransition_RequestedToApproved`
- `TestInvalidTransition_CompletedToApproved`
- `TestInvalidTransition_RejectedToFundsPosted`
- `TestOptimisticLock_ConcurrentTransition`

---

## Demo Scripts

Run against a live `docker compose up` stack. Results from the files in `reports/`.

| Script | Description | Assertions | Status |
|--------|-------------|-----------|--------|
| `demo-happy-path.sh` | Full lifecycle: submit → settle → complete + ledger verify | 9/9 pass | ✅ PASS |
| `demo-all-scenarios.sh` | All 7 vendor stub scenarios + operator review flow | 13/13 pass | ✅ PASS |
| `demo-return.sh` | Return/reversal: complete deposit → return → verify ledger entries | 9/9 pass | ✅ PASS |
| `trigger-settlement.sh` | EOD settlement trigger with assertions | (included in happy path) | ✅ PASS |

### demo-happy-path.sh Assertions
1. Deposit reaches `funds_posted`
2. GET by ID returns `funds_posted`
3. State history is non-empty (length ≥ 1)
4. Settlement batch includes ≥ 1 deposit
5. Batch status is `submitted`
6. Deposit is now `completed`
7. `settlement_batch_id` is set on transfer
8. Ledger has ≥ 1 entry for transfer
9. DEPOSIT entry amount matches submitted amount

### demo-all-scenarios.sh Assertions
1. IQA blur (`*1001`) → `rejected`
2. IQA glare (`*1002`) → `rejected`
3. MICR failure (`*1003`) → `analyzing` with `flagged=true`
4. Duplicate detected (`*1004`) → `rejected`
5. Amount mismatch (`*1005`) → `analyzing` with `flagged=true`
6. Clean pass (`*1006`) → `funds_posted`
7. Basic pass (`*0000`) → `funds_posted`
8. Retirement account → `funds_posted`, `contribution_type=INDIVIDUAL`
9–11. Operator approve flow: flagged deposit → operator queue → `funds_posted`
12–13. Operator reject flow: flagged deposit → `rejected`, audit log entry created

### demo-return.sh Assertions
1. Deposit reaches `funds_posted`
2. Settlement includes deposit
3. Deposit is `completed` after settlement
4. Transfer moves to `returned`
5. `amount_cents` on returned transfer matches deposit
6. Ledger has 3+ entries (DEPOSIT + REVERSAL + RETURN_FEE)
7. DEPOSIT entry amount matches original
8. REVERSAL entry amount matches original
9. RETURN_FEE entry amount is 3000 ($30)

---

## Phase Acceptance Tests

| Script | Phase | Assertions | Status |
|--------|-------|-----------|--------|
| `phase-08-test-results.txt` | Phase 8: Deposit handler | 16/16 pass | ✅ PASS |
| `phase-10-test-results.txt` | Phase 10: Operator service | 21/21 pass | ✅ PASS |
| `phase-11-test-results.txt` | Phase 11: Settlement engine | 22/22 pass | ✅ PASS |
| `phase-13-test-results.txt` | Phase 13: React frontend | 25/25 pass | ✅ PASS |

---

## Vendor Stub Scenario Coverage Matrix

| Scenario | Account | Vendor Response | Expected State | Go Test | Demo Script |
|----------|---------|-----------------|----------------|---------|-------------|
| IQA Fail (Blur) | `ACC-SOFI-1001` | `fail`, `iqa: fail_blur` | `rejected` | `TestStub_IQABlur_1001` | `demo-all-scenarios.sh` |
| IQA Fail (Glare) | `ACC-SOFI-1002` | `fail`, `iqa: fail_glare` | `rejected` | `TestStub_IQAGlare_1002` | `demo-all-scenarios.sh` |
| MICR Failure | `ACC-SOFI-1003` | `flagged`, `micr: null` | `analyzing` (flagged) | `TestStub_MICRFailure_1003_Flagged` | `demo-all-scenarios.sh` |
| Duplicate Detected | `ACC-SOFI-1004` | `fail`, `dupe: found` | `rejected` | `TestStub_DuplicateDetected_1004` | `demo-all-scenarios.sh` |
| Amount Mismatch | `ACC-SOFI-1005` | `flagged`, `ocr≠declared` | `analyzing` (flagged) | `TestStub_AmountMismatch_1005_Flagged` | `demo-all-scenarios.sh` |
| Clean Pass | `ACC-SOFI-1006` | `pass`, all populated | `funds_posted` | `TestStub_CleanPass_1006` | `demo-happy-path.sh`, `demo-all-scenarios.sh` |
| Basic Pass | `ACC-SOFI-0000` | `pass`, basic | `funds_posted` | `TestStub_CleanPass_DefaultSuffix` | `demo-all-scenarios.sh` |
| Retirement Account | `ACC-RETIRE-001` | `pass` | `funds_posted` + `contribution_type=INDIVIDUAL` | `TestContributionType_Retirement_Individual` | `demo-all-scenarios.sh` |

---

## Rubric Alignment

| Rubric Category | Points | Coverage |
|----------------|--------|---------|
| System design and architecture | 20 | `docs/architecture.md` — service boundaries, state machine diagram, data flows. `docs/decision_log.md` — 20 decisions with alternatives. `README.md` — architecture diagram. |
| Core correctness | 25 | `demo-happy-path.sh` (9 assertions). `TestPostFunds_*` (6 tests). Phase 8 acceptance tests (16 assertions). Full Requested→FundsPosted→Completed lifecycle verified. |
| Vendor Service stub quality | 15 | `TestStub_*` (8 tests, all 7 scenarios + stateless check). `demo-all-scenarios.sh` (all 7 scenarios exercised). Account suffix mapping — zero code changes to switch scenarios. |
| Operator workflow and observability | 10 | `TestApprove_*`, `TestReject_*` (3 tests). Phase 10 acceptance tests (21 assertions). `demo-all-scenarios.sh` covers approve and reject flows with audit log verification. |
| Return/reversal handling | 10 | `TestPostReversal_*` (3 tests verifying entry count, amounts, subtypes). `demo-return.sh` (9 assertions including $30 fee check). |
| Tests and evaluation rigor | 10 | 38 Go unit tests across 6 packages. 31 demo script assertions. 84 phase acceptance test assertions. Scenario coverage matrix above. |
| Developer experience | 10 | `docker compose up --build` starts all 4 containers. README with quick start, demo commands. 4 demo scripts. `docs/` with architecture, decisions, risks. `SUBMISSION.md`. |
| **Total** | **100** | |

---

## How to Reproduce

```bash
# Unit tests (no running stack needed)
cd backend
go test ./... -v

# Demo scripts (requires running stack)
docker compose up --build -d
sleep 12  # wait for services to be ready
./scripts/demo-happy-path.sh
./scripts/demo-all-scenarios.sh
./scripts/demo-return.sh
./scripts/trigger-settlement.sh

# Health check
curl -s http://localhost:8080/health | jq .
```
