# Session Log: Phase 7 — Ledger Service Implementation

**Date:** 2026-03-09 09:22
**Duration:** ~20 minutes
**Focus:** Implement the append-only ledger posting service with deposit and reversal logic

## What Got Done

- Created `backend/internal/ledger/models.go` — `Entry` struct mapping to `ledger_entries` table with all fields: ID, TransferID, ToAccountID, FromAccountID, Type, SubType, TransferType, Currency, AmountCents (int64), Memo, SourceApplicationID, CreatedAt
- Created `backend/internal/ledger/repository.go` — `Repository` struct with three methods:
  - `PostEntryTx` — only write method; inserts a ledger entry within a caller-provided `*sql.Tx`
  - `GetByTransferID` — SELECT all entries for a transfer, ordered by `created_at ASC`
  - `GetByAccountID` — SELECT entries where `to_account_id OR from_account_id` matches, with optional `from`/`to` date range filter using dynamic query building
- Created `backend/internal/ledger/service.go` — `Service` wrapping the repository with three domain methods:
  - `PostFundsTx` — creates a single DEPOSIT entry (investor as ToAccountID, omnibus as FromAccountID)
  - `PostReversal` — creates two entries atomically: REVERSAL (original amount, investor debited) + RETURN_FEE ($30, investor debited), both investor→omnibus direction
  - `GetByTransferID` / `GetByAccountID` — delegates to repository
- Created `backend/internal/ledger/service_test.go` — 6 tests all passing:
  - `TestPostFunds_CreatesDepositEntry` — asserts exactly 1 DEPOSIT row in `ledger_entries`
  - `TestPostFunds_CorrectAccountMapping` — asserts `to_account_id`=investor, `from_account_id`=omnibus
  - `TestPostReversal_TwoEntries` — asserts exactly 2 rows created on reversal
  - `TestPostReversal_CorrectAmounts` — asserts entry 1 = original amount, entry 2 = 3000 (fee)
  - `TestPostReversal_SubTypes` — asserts entry 1 = "REVERSAL", entry 2 = "RETURN_FEE" in order
  - `TestLedgerEntries_AppendOnly` — compile-time interface assertion that `Repository` only exposes the three allowed methods (no Update/Delete)
- Compiled cleanly with `docker run ... golang:1.22-alpine go build ./...`
- All 6 tests passed against live Postgres via Docker Compose network
- Committed: `feat(ledger): add append-only ledger posting and reversal with fee deduction`
- Updated `MEMORY.md` to reflect Phase 7 complete

## Issues & Troubleshooting

No issues encountered this session. Compilation and tests passed on the first attempt.

## Decisions Made

- **`GetByAccountID` filters on both `to_account_id` AND `from_account_id`** — the plan specified "entries where to_account_id matches" but the implementation covers both directions so that reversal entries (where investor is `from_account_id`) are also returned for the ledger view endpoint. This matches the intent of the ledger view API spec.
- **Dynamic query building for date filter** — rather than four separate query variants, used `argIdx` to append `AND created_at >= $N` / `AND created_at <= $N` clauses only when `from`/`to` pointers are non-nil. Keeps the method to one code path.
- **`TestLedgerEntries_AppendOnly` as interface assertion** — instead of reflection or string matching on method names, defined a local `appendOnlyLedger` interface and did a compile-time `var _ appendOnlyLedger = NewRepository(db)` assertion. This is the idiomatic Go approach and fails at compile time if the interface isn't satisfied (not a runtime assertion).
- **Test cleanup order** — `cleanupTransfer` deletes `ledger_entries` before `transfers` to respect the FK constraint (`ledger_entries.transfer_id REFERENCES transfers(id)`). Also deletes `state_transitions` for completeness even though ledger tests don't create them.
- **Followed the same test helper pattern as Phase 4** — `getTestDB` skips (not fails) if Postgres is unreachable, keeping unit test runs clean in environments without a DB.

## Current State

- **Phases 1–7 complete** and committed on branch `phase-7/ledger-service`
- All ledger tests pass (6/6)
- Cumulative test count across all packages: state (6+), vendor (8), funding (10), ledger (6) = 30+ tests
- Backend compiles cleanly
- Docker Compose with Postgres + Redis is running locally
- No frontend work has been done yet

**What's working:**
- Full DB schema and seed data (correspondents, accounts)
- State machine with optimistic locking and transition audit trail
- Vendor stub with 7 deterministic scenarios keyed by account suffix
- Funding service: deposit limits, account eligibility, contribution type, Redis duplicate detection
- Ledger service: append-only DEPOSIT posting, two-entry REVERSAL + RETURN_FEE

**What's not yet built:**
- Phase 8: Deposit handler & service (pipeline orchestrator — the main POST /deposits flow)
- Phase 9: Middleware (auth, rate limiting)
- Phase 10: Operator service (review queue, approve/reject, audit log)
- Phase 11: Settlement engine (X9 ICL generation, EOD cutoff)
- Phase 12: Main server DI wiring (main.go)
- Phase 13: React frontend
- Phase 14–18: Demo scripts, integration tests, documentation

## Next Steps

1. **Phase 8 — Deposit Handler & Service** (highest priority — wires everything together)
   - `internal/deposit/service.go` — `Submit()` orchestrates the full Requested→FundsPosted pipeline synchronously
   - `internal/deposit/handler.go` — POST /deposits (multipart), GET /deposits/:id, GET /deposits, image serving, POST /deposits/:id/return
   - `approveAndPost()` helper used by both deposit service and operator service
2. **Phase 9 — Middleware** (can do alongside Phase 8)
   - `InvestorAuth` — Bearer token validation
   - `OperatorAuth` — X-Operator-ID header
   - `RateLimit` — Redis INCR per account per minute
3. **Phase 12 — Main server wiring** — after Phase 8 + 9 have their interfaces defined
4. **Phase 10 — Operator service** — review queue, approve/reject in one tx with audit log
5. **Phase 11 — Settlement engine** — X9 ICL via moov-io, EOD cutoff logic
6. **Phase 13 — React frontend** — only if time permits; demo scripts cover all scenarios via curl
