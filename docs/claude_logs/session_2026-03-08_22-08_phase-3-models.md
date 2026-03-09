# Session Log: Phase 3 — Domain Model Types

**Date:** 2026-03-08 22:08
**Duration:** ~15 minutes
**Focus:** Create shared domain types (Transfer, Account, errors) with zero business logic

## What Got Done

- Created `backend/internal/models/transfer.go`:
  - `TransferStatus` type (string alias) with 8 status constants: `StatusRequested`, `StatusValidating`, `StatusAnalyzing`, `StatusApproved`, `StatusFundsPosted`, `StatusCompleted`, `StatusRejected`, `StatusReturned`
  - `Transfer` struct mapping 1:1 to the `transfers` table — all fields with `json` and `db` struct tags; nullable fields as pointer types (`*string`, `*int64`, `*float64`, `*uuid.UUID`)
  - `StateTransition` struct for the `state_transitions` audit table
- Created `backend/internal/models/account.go`:
  - `Account` struct with ID, CorrespondentID, AccountType, Status, CreatedAt
  - `Correspondent` struct with ID, Name, OmnibusAccountID, CreatedAt
  - `AccountWithCorrespondent` embedding `Account` with additional `OmnibusAccountID` field (used by funding service account resolution)
- Created `backend/internal/models/errors.go`:
  - 8 sentinel errors: `ErrInvalidStateTransition`, `ErrTransferNotFound`, `ErrAccountNotFound`, `ErrAccountIneligible`, `ErrDepositOverLimit`, `ErrDuplicateDeposit`, `ErrTransferNotReturnable`, `ErrTransferNotReviewable`
- Verified `go build ./...` passes with zero errors and zero circular imports
- Committed: `feat(models): add transfer, account, and error domain types`

## Issues & Troubleshooting

- **Problem:** `docker compose run --rm backend go build ./...` failed with `exec: "go": executable file not found in $PATH`
- **Cause:** The backend Docker image uses a multi-stage Dockerfile. The runtime image is `alpine:3.19`, which does not include Go — only the compiled binary is copied over from the `golang:1.22-alpine` builder stage. Running `go` commands inside the final container therefore fails.
- **Fix:** Ran the build command directly against the `golang:1.22-alpine` image with the backend source mounted as a volume: `docker run --rm -v "$(pwd)/backend:/app" -w /app golang:1.22-alpine go build ./...`

## Decisions Made

- **No imports from internal packages in models** — the `models` package imports only stdlib (`time`, `errors`) and `github.com/google/uuid`. This is intentional to avoid circular imports; downstream packages (`state`, `funding`, `ledger`, etc.) all import `models`, so `models` must not import any of them.
- **Pointer types for nullable fields** — all DB-nullable columns use pointer types (`*string`, `*int64`, `*float64`, `*uuid.UUID`) so that Go's zero values don't silently overwrite NULL in the database.
- **`map[string]any` for `StateTransition.Metadata`** — matches the JSONB column type in Postgres; flexible enough for varying transition metadata without a rigid schema.

## Current State

- **Phases complete:** 1 (scaffold), 2 (DB migrations + seed data), 3 (domain models)
- **Compiles cleanly:** `go build ./...` passes against `golang:1.22-alpine`
- **Running infrastructure:** Postgres and Redis containers are up (from Phase 2)
- **Models package:** fully defined, no business logic, no circular imports
- **Not yet implemented:** state machine, vendor stub, funding service, ledger service, deposit handler, operator service, settlement engine, middleware, main server wiring, React frontend

## Next Steps

1. **Phase 4 — State Machine** (`internal/state/`): implement `states.go` (allowed transition table, `IsValid`, `IsTerminal`) and `machine.go` (`Transition` with optimistic locking via `WHERE status = expected`, atomic audit log insert). Write 6 required tests including concurrent transition test.
2. **Phase 5 — Vendor Stub** (`internal/vendor/`): implement `models.go` (request/response types, `Service` interface) and `stub.go` (deterministic responses keyed by account suffix). Write 8 stub tests.
3. **Phase 9 — Middleware** (can be done alongside Phase 5): `auth.go` (InvestorAuth, OperatorAuth) and `ratelimit.go` (Redis-backed INCR per account per minute).
4. **Confirm Go command pattern for this repo** — use `docker run --rm -v "$(pwd)/backend:/app" -w /app golang:1.22-alpine <go command>` for all future Go operations, not `docker compose run --rm backend`.
