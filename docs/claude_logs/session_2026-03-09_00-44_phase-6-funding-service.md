# Session Log: Phase 6 — Funding Service Implementation

**Date:** 2026-03-09 00:44
**Duration:** ~30 minutes
**Focus:** Implement the business rules engine, account resolver, and Redis duplicate detection for the funding service

---

## What Got Done

- Created `backend/internal/funding/accounts.go`:
  - `AccountResolver` struct backed by `*sql.DB`
  - `NewAccountResolver(db *sql.DB) *AccountResolver` constructor
  - `Resolve(ctx, accountID) (*models.AccountWithCorrespondent, error)` — JOINs `accounts` and `correspondents` tables to fetch the omnibus account ID; returns `ErrAccountNotFound` for missing accounts and `ErrAccountIneligible` for non-active accounts

- Created `backend/internal/funding/rules.go`:
  - Constants: `MaxDepositAmountCents = int64(500000)`, `DupeTTL = 90 * 24 * time.Hour`
  - `applyDepositLimit(amountCents int64) error` — rejects anything over $5,000
  - `applyDuplicateCheck(ctx, rdb, routing, account, serial, amountCents) error` — SHA256-hashes `routing:account:amount:serial`, uses Redis SETNX with 90-day TTL; returns `ErrDuplicateDeposit` if key already set; gracefully degrades (logs warning, returns nil) if Redis is unreachable
  - `applyContributionType(accountType string) string` — returns `"INDIVIDUAL"` for retirement accounts, `""` for all others

- Created `backend/internal/funding/service.go`:
  - `RuleResult` struct: `OmnibusAccountID`, `ContributionType`, `RulesApplied []string`, `RulesPassed bool`, `FailReason string`
  - `Service` struct holding `*sql.DB`, `*redis.Client`, and `*AccountResolver`
  - `NewService(db, rdb) *Service` constructor
  - `ApplyRules(ctx, transfer, vendorResp) (*RuleResult, error)` — applies 4 rules in order: deposit limit → account eligibility + omnibus lookup → contribution type default → duplicate check (skipped if `vendorResp.MICRData == nil`)

- Created `backend/internal/funding/rules_test.go`:
  - 10 tests total, all passing
  - `getTestDB(t)` helper — reads `DATABASE_URL` env var, skips test if Postgres unreachable
  - `getTestRedis(t)` helper — reads `REDIS_URL` env var, skips test if Redis unreachable
  - `dupeKey(routing, account, serial, amountCents)` helper — mirrors `applyDuplicateCheck` hash logic for test cleanup via `t.Cleanup`

- Compiled the full backend with `golang:1.22-alpine` Docker container — zero errors

- Ran all 10 funding tests against live Postgres and Redis via the compose network — all passed in 0.138s

- Committed: `feat(funding): add business rules engine, account resolver, and duplicate detection`

---

## Issues & Troubleshooting

- **Problem:** First attempt at running `docker run` failed immediately with "Cannot connect to the Docker daemon."
  - **Cause:** The command was run from a working directory that wasn't the project root; the Docker socket was unreachable in that context (the user rejected the tool call).
  - **Fix:** User re-submitted the full prompt; subsequent Docker calls succeeded from the correct working directory.

- **Problem:** First draft of `rules_test.go` included a broken `sha256Hash` helper function with nonsensical inner function syntax, used for cleanup in `TestDuplicateCheck_FirstDeposit_Allowed`.
  - **Cause:** Attempted to write an inline cleanup helper without importing `crypto/sha256` in the test file, resulting in invalid Go.
  - **Fix:** Rewrote the test file with a proper `dupeKey(routing, account, serial, amountCents string) string` helper that imports `crypto/sha256` directly and mirrors the production hash logic. Used `t.Cleanup` to delete the Redis key after each test.

---

## Decisions Made

- **Unique routing number per test run for Redis tests:** Used `fmt.Sprintf("TEST-%d", time.Now().UnixNano())` as the routing number to guarantee no cross-test key collisions in Redis. This avoids flakiness when tests are run repeatedly against a shared Redis instance without a flush between runs.

- **Dynamic account ID for suspended account test:** Instead of relying on a pre-seeded suspended account (which doesn't exist in seed data), `TestAccountResolver_Suspended_Ineligible` inserts a uniquely-named account (`ACC-TEST-SUSPENDED-<nanos>`) directly, then cleans it up with `t.Cleanup`. This keeps the test self-contained and idempotent.

- **Graceful Redis degradation in `applyDuplicateCheck`:** Per the implementation plan, if Redis is down, the duplicate check logs a warning and returns `nil` (allows the deposit). This is an intentional availability-over-consistency trade-off documented in the plan.

- **`dupeKey` helper in test file:** Rather than exporting the hash computation, we replicated the same `crypto/sha256` logic in the test helper. This is acceptable because the test file is in the same package (`package funding`), and the helper is only used for cleanup — not for asserting behavior.

---

## Current State

**Phases complete:** 1–6 (scaffold, DB schema, models, state machine, vendor stub, funding service)

**Passing tests:**
- `internal/state/` — 6 tests (state machine transitions + optimistic locking)
- `internal/vendor/` — 8 tests (all 7 stub scenarios + stateless assertion)
- `internal/funding/` — 10 tests (5 pure unit + 5 integration with Postgres/Redis)

**Working infrastructure:** Docker Compose stack (Postgres, Redis, backend, frontend) is healthy and running locally.

**Not yet implemented:** Phases 7–18 (ledger, deposit handler, middleware, operator service, settlement, main server wiring, React frontend, scripts, integration tests).

---

## Next Steps

1. **Phase 7: Ledger Service** — `internal/ledger/models.go`, `repository.go`, `service.go`; implement `PostFundsTx` (DEPOSIT entry) and `PostReversal` (REVERSAL + RETURN_FEE entries); write 6 tests covering correct account mapping, amounts, and append-only constraint
2. **Phase 8: Deposit Handler & Service** — pipeline orchestrator; `POST /deposits` runs full Requested→FundsPosted pipeline synchronously; image saving; `GET /deposits/:id` with state history
3. **Phase 9: Middleware** — `InvestorAuth` (Bearer token), `OperatorAuth` (X-Operator-ID header), `RateLimit` (Redis INCR, 10/min per account)
4. **Phase 10: Operator Service** — review queue, approve/reject, audit log
5. **Phase 11: Settlement Engine** — X9 ICL file via moov-io/imagecashletter, EOD cutoff, batch tracking
6. **Phase 12: Main Server Wiring** — DI in `cmd/server/main.go`, health check endpoint, route registration
