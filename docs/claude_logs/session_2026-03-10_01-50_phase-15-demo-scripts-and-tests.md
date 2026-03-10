# Session Log: Demo Scripts and Remaining Go Tests (Phases 14 & 15)

**Date:** 2026-03-10 01:50 UTC
**Duration:** ~60 minutes
**Focus:** Implement Phase 14 (missing Go test files) and Phase 15 (4 evaluator-facing demo scripts)

---

## What Got Done

### Phase 14 — Go Tests

- **Created** `backend/internal/operator/service_test.go` — 3 integration tests requiring Postgres:
  - `TestApprove_MovesToFundsPosted` — inserts flagged+analyzing transfer, calls `Approve()`, asserts `funds_posted` and 1 DEPOSIT ledger entry
  - `TestApprove_WritesAuditLog` — verifies `audit_logs` row with `action=approve` and correct `operator_id`
  - `TestReject_MovesToRejected` — inserts flagged+analyzing transfer, calls `Reject()`, asserts `rejected` state, 0 ledger entries, audit log with `action=reject`
- **Verified** `backend/internal/settlement/generator_test.go` already existed from a prior phase with 4 tests: `TestCutoffTime_CorrectUTCConversion`, `TestCutoffTime_DST_Summer`, `TestSettlement_ExcludesRejected`, `TestSettlement_ExcludesAlreadyBatched`
- **Ran full Go test suite** via Docker (`golang:1.22-alpine` container with `--network apex-mobile-check-deposit_default`): **all 29 tests pass** across 6 packages (funding, ledger, operator, settlement, state, vendor)
- **Saved** test output to `reports/go-test-results.txt`

### Phase 15 — Demo Scripts

- **Created** `scripts/demo-happy-path.sh` — 9 assertions covering full lifecycle:
  - Flush Redis → submit clean-pass deposit (ACC-SOFI-1006, random amount) → verify `funds_posted` → check state_history → trigger settlement → verify `completed` + `settlement_batch_id` → check ledger DEPOSIT entry with correct amount
- **Created** `scripts/demo-all-scenarios.sh` — 13 assertions covering all 7 vendor stub scenarios + over-limit:
  - ACC-SOFI-1001 (IQA blur → rejected), 1002 (IQA glare → rejected), 1003 (MICR failure → analyzing + flagged + `micr_failure`), 1004 (duplicate → rejected), 1005 (amount mismatch → analyzing + flagged + `amount_mismatch`), 1006 (clean pass → funds_posted), 0000 (basic pass → funds_posted), 600000 cents (over-limit → HTTP 422 + `DEPOSIT_OVER_LIMIT`)
- **Created** `scripts/demo-return.sh` — 9 assertions covering return/reversal flow:
  - Flush Redis → submit deposit → settlement → `completed` → POST return → assert `returned` + `amount_cents` matches → verify ledger has 3 entries (DEPOSIT + REVERSAL + RETURN_FEE) with correct amounts
- **Created** `scripts/trigger-settlement.sh` — utility script that triggers EOD settlement for a given date (defaults to today UTC), prints `batch_id`, `deposit_count`, `total_amount_cents`, `file_path`, `status`
- **Made all scripts executable** (`chmod +x`)
- **Saved** demo output to `reports/demo-happy-path-results.txt`, `reports/demo-all-scenarios-results.txt`, `reports/demo-return-results.txt`
- **Committed** all files to branch `phase-15/demo-scripts-and-tests` with message `feat(scripts): add demo scripts and remaining Go tests for full scenario coverage`

---

## Issues & Troubleshooting

### Problem 1: Docker daemon not accessible via default socket

- **Problem:** Initial `docker compose up` command failed with "Cannot connect to the Docker daemon at `unix:///home/riwata/.docker/desktop/docker.sock`"
- **Cause:** The system uses the standard Docker socket path `unix:///var/run/docker.sock`, not the Docker Desktop path
- **Fix:** All Docker commands prefixed with `DOCKER_HOST=unix:///var/run/docker.sock`

### Problem 2: First `docker compose up` attempt was user-interrupted

- **Problem:** User interrupted the `docker compose up --build` command before it executed
- **Cause:** User re-issued the full task prompt rather than approving the tool call
- **Fix:** Re-ran after user confirmed intent; all files created previously were still on disk so no rework needed

### Problem 3: Settlement batch returned 0 deposits (happy path demo)

- **Problem:** `demo-happy-path.sh` step 3 failed — `deposit_count=0` even though a deposit had just been submitted and reached `funds_posted`
- **Cause:** The script used `TODAY=$(date +%Y-%m-%d)` which returned the local date "2026-03-09". The `CutoffTime` for March 9 CST = 00:30 UTC March 10. The deposit was created at 01:40 UTC March 10, which is **after** the cutoff for the March 9 batch — so it was excluded
- **Fix:** Changed all scripts to use `TODAY=$(date -u +%Y-%m-%d)` (UTC date). UTC date was "2026-03-10", cutoff for March 10 CST = 00:30 UTC March 11. Deposit at 01:40 UTC March 10 is before that cutoff → included ✓

### Problem 4: jq parse error in happy path script

- **Problem:** `demo-happy-path.sh` produced `jq: parse error: Unfinished JSON term at EOF at line 2, column 0` when trying to extract the DEPOSIT entry
- **Cause:** Used `jq '...' | head -1` piped into a second `jq -r '.amount_cents'` call. When jq outputs a JSON object followed by nothing, `head -1` produces a partial line that fails the second jq parse
- **Fix:** Rewrote to use a single jq expression: `jq -r '[.data.entries[] | select(...)] | .[0].amount_cents // "0"'`

### Problem 5: `bc` not available in the demo environment

- **Problem:** The scripts used `$(echo "scale=2; $AMOUNT/100" | bc)` to display a dollar amount, which would fail if `bc` is not installed
- **Cause:** `bc` is not guaranteed to be present in minimal environments (Alpine, CI)
- **Fix:** Removed the `bc` display entirely — scripts just echo the raw cent amount

### Problem 6: Return handler response missing `original_amount_cents`/`return_fee_cents`

- **Problem:** `demo-return.sh` step 3 failed asserting `original_amount_cents` and `return_fee_cents` on the return API response (both were `null`)
- **Cause:** The `Return` handler in `deposit/handler.go` returns the raw `models.Transfer` object, which has no `original_amount_cents` or `return_fee_cents` fields — those figures exist only in the ledger entries
- **Fix:** Changed step 3 assertions to check `amount_cents` on the returned transfer (which equals the original deposit amount). Left the fee verification to step 4 (ledger entries), which already correctly asserted `RETURN_FEE` amount = 3000

---

## Decisions Made

- **UTC date for batch_date in demo scripts** — Using `date -u +%Y-%m-%d` ensures the settlement cutoff is always in the future relative to deposits just submitted, regardless of what time of day the demo is run. This is more robust than local date, which can produce a cutoff that's already passed when running in the evening (CST).

- **Random amounts for clean-pass deposits** — All demo scripts use `RANDOM` to pick non-overlapping amount ranges for clean-pass accounts. This avoids Redis duplicate-hash collisions when the same script is run multiple times in succession, since the vendor stub always returns the same routing/account/serial for clean-pass accounts.

- **Redis flush at script start** — All demo scripts attempt to flush Redis before running. Two fallback paths are tried (`DOCKER_HOST=unix:///var/run/docker.sock` first, then plain `docker exec`). If both fail, a warning is printed but the script continues — the random amounts make collisions unlikely even without a flush.

- **No `bc` dependency** — Removed human-readable dollar formatting to avoid dependency on `bc`. The scripts are correctness-focused; exact cent values in output are sufficient.

- **Operator service test uses `funding.NewAccountResolver` directly** — `NewService` for operator requires a `*funding.Service`, but the test only needs account resolution (for `Approve`). Passed `nil` for the funding service since operator's `Approve` only calls `s.resolver.Resolve()`, which is initialized from the separate `funding.NewAccountResolver(db)` call inside `NewService`. This avoids needing a full funding service wired up in the test.

---

## Current State

**Branches:** `phase-15/demo-scripts-and-tests` (committed, not yet merged to main)

**Go tests:** 29 tests across 6 packages — all pass
| Package | Tests | Status |
|---|---|---|
| `internal/vendor` | 8 | ✓ all pass |
| `internal/state` | 7 | ✓ all pass |
| `internal/funding` | 10 | ✓ all pass |
| `internal/ledger` | 6 | ✓ all pass |
| `internal/operator` | 3 | ✓ all pass (new) |
| `internal/settlement` | 4 | ✓ all pass |

**Demo scripts:** All passing against live Docker Compose stack
| Script | Result |
|---|---|
| `demo-happy-path.sh` | 9/9 ✓ |
| `demo-all-scenarios.sh` | 13/13 ✓ |
| `demo-return.sh` | 9/9 ✓ |
| `trigger-settlement.sh` | runs cleanly ✓ |

**Completed phases:** 1–15 (scaffold, DB, models, state machine, vendor stub, funding, ledger, deposit pipeline, middleware, operator service, settlement engine, main.go wiring, React frontend, tests, demo scripts)

**Reports directory:** `reports/` contains `go-test-results.txt`, `demo-happy-path-results.txt`, `demo-all-scenarios-results.txt`, `demo-return-results.txt`

---

## Next Steps

1. **Phase 16** — Finalize Dockerfiles (multi-stage backend, Node frontend) — verify builds cleanly in CI-like environment
2. **Phase 18** — Documentation:
   - `docs/architecture.md` — text-based system diagram, service boundaries, happy-path and return/reversal data flow
   - `docs/decision_log.md` — complete all locked decisions including synchronous pipeline, in-process vendor stub, WHERE-status optimistic locking, JSON settlement fallback
   - `docs/risks.md` — production gaps, known limitations
   - `reports/scenario-coverage.md` — table mapping each vendor stub scenario to its test and demo script result
3. **Merge to main** — PR from `phase-15/demo-scripts-and-tests` once documentation is done
4. **Final acceptance test run** — fresh `docker compose down -v && docker compose up --build -d` followed by full demo script suite to confirm idempotency
