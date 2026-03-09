# Session Log: Phase 2 — Database Schema, Migrations, and Connections

**Date:** 2026-03-08 ~21:40 ET
**Duration:** ~15 minutes
**Focus:** Implement Phase 2 of the implementation plan — database schema migrations, seed data, Postgres/Redis connection helpers, and health endpoint wiring

---

## What Got Done

- Created `backend/internal/db/migrations/001_create_tables.sql` with all 8 tables:
  - `schema_migrations` (migration tracking)
  - `correspondents` (broker-dealers)
  - `accounts` (investor accounts)
  - `transfers` (central domain entity with full state machine status CHECK constraint)
  - `ledger_entries` (append-only financial postings)
  - `state_transitions` (audit trail for every state change)
  - `audit_logs` (operator review actions)
  - `settlement_batches` (EOD X9 ICL batch tracking)
  - All tables include `CREATE TABLE IF NOT EXISTS`, appropriate `CHECK` constraints, foreign keys, and `CREATE INDEX IF NOT EXISTS` for query columns
- Created `backend/internal/db/migrations/002_seed_data.sql` with:
  - 3 correspondents: `CORR-SOFI` (SoFi/OMNI-SOFI-001), `CORR-WBL` (Webull/OMNI-WBL-001), `CORR-CASH` (CashApp/OMNI-CASH-001)
  - 8 investor accounts mapped to vendor stub scenarios: blur (1001), glare (1002), MICR failure (1003), duplicate (1004), amount mismatch (1005), clean pass (1006), basic pass (0000), retirement (ACC-RETIRE-001)
  - All INSERTs use `ON CONFLICT (id) DO NOTHING` for idempotency
- Created `backend/internal/db/postgres.go` — `Connect()` function:
  - Opens pool with `sql.Open("postgres", url)` + `_ "github.com/lib/pq"` side-effect import
  - Sets `MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5m`
  - Pings to verify connectivity before returning
- Created `backend/internal/db/redis.go` — `NewRedisClient()` function:
  - Parses URL with `redis.ParseURL()`
  - Creates client, pings to verify, returns or errors with context
- Created `backend/internal/db/migrations.go` — `RunMigrations()` function:
  - Uses `//go:embed migrations/*.sql` to embed SQL files into the binary
  - Creates tracking table if not exists
  - Sorts files alphabetically (001 before 002)
  - Skips already-applied versions (checks `schema_migrations`)
  - Wraps each migration in a `BEGIN`/`COMMIT` transaction for partial-failure safety
  - Records applied version in `schema_migrations` with `ON CONFLICT DO NOTHING`
- Updated `backend/cmd/server/main.go`:
  - Fails fast (`log.Fatal`) if `DATABASE_URL` or `REDIS_URL` is missing
  - Calls `db.Connect()`, `db.NewRedisClient()`, `db.RunMigrations()` in order; fatal on any failure
  - Logs "Migrations completed successfully" and "Starting server on :PORT"
  - Updated `/health` endpoint to actually ping Postgres and Redis via `PingContext` (2s timeout)
  - Returns `status: "ok"` + HTTP 200 if both connected; `status: "degraded"` + HTTP 503 if either down
  - Includes `postgres`, `redis`, and `timestamp` fields in health response
- Committed as: `feat(db): add schema migrations, seed data, postgres and redis connections`

---

## Issues & Troubleshooting

No issues encountered. The build and verification ran cleanly on the first attempt.

---

## Decisions Made

- **Embed SQL files into binary** — Used `//go:embed migrations/*.sql` so the final Docker image is self-contained. No need to copy SQL files separately or mount volumes for migrations.
- **Transaction per migration file** — Each `.sql` file runs inside a `BEGIN`/`COMMIT`. If a migration partially fails the transaction rolls back, leaving `schema_migrations` clean so a retry is safe.
- **`ON CONFLICT DO NOTHING` in seed data** — Seeds are idempotent. Restarting the backend never fails due to pre-existing seed rows.
- **Health returns 503 on degraded** — Consistent with the API spec in `.claude/rules/prompts.md`; downstream load balancers can use HTTP status to remove unhealthy backends.
- **Connection pool settings** — `MaxOpenConns=25`, `MaxIdleConns=5`, `ConnMaxLifetime=5m` chosen as reasonable defaults matching the implementation plan spec; not tuned for production load.

---

## Current State

**What's working:**
- `docker compose up --build` starts all 4 containers (postgres, redis, backend, frontend)
- Migrations run automatically at backend startup and are idempotent
- All 8 database tables created with constraints and indexes
- All 3 correspondents and 8 investor accounts seeded
- `/health` returns `{"status":"ok","postgres":"connected","redis":"connected","timestamp":"..."}` with HTTP 200
- Backend restart does not fail — migrations skip already-applied versions

**Branch:** `phase-2/database-migrations`
**Last commit:** `c132a0c feat(db): add schema migrations, seed data, postgres and redis connections`

**Phases complete:** 1 (scaffold), 2 (database migrations)
**Phases remaining:** 3–18

---

## Next Steps

In priority order for the next session:

1. **Phase 3 — Models** (`internal/models/`): define `Transfer`, `Account`, `Correspondent`, `AccountWithCorrespondent`, `StateTransition`, and all domain error vars (`ErrInvalidStateTransition`, `ErrDepositOverLimit`, etc.)
2. **Phase 4 — State Machine** (`internal/state/`): implement `states.go` (allowed transition table, `IsValid`, `IsTerminal`) and `machine.go` (`Transition` with optimistic locking, `BeginAndTransition`); add `machine_test.go` with 6 required tests
3. **Phase 5 — Vendor Stub** (`internal/vendor/`): `models.go` (Request/Response/Service interface), `stub.go` (7 deterministic scenarios keyed by account suffix), `stub_test.go`
4. **Phase 6 — Funding Service** (`internal/funding/`): account resolver, deposit limit, Redis duplicate check, contribution type defaulting, `rules_test.go`
5. **Phase 7 — Ledger Service** (`internal/ledger/`): append-only entry posting, reversal with return fee, repository queries
