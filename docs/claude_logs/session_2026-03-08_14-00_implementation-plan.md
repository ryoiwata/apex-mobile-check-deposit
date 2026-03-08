# Session Log: Implementation Plan Creation via Structured Interview

**Date:** 2026-03-08 ~14:00 CT
**Duration:** ~90 minutes
**Focus:** Interview-driven resolution of all architectural ambiguities and production of a complete, file-level implementation plan

---

## What Got Done

- Read all project context: `CLAUDE.md`, `docs/PRD.md`, `docs/presearch.md`, and all four `.claude/rules/` files (`code-style.md`, `testing.md`, `prompts.md`, `security.md`)
- Identified ~20 implementation ambiguities not resolved by existing documentation
- Conducted 5 structured interview rounds using `AskUserQuestion`, covering every layer of the system
- Resolved all major open questions (see Decisions Made below)
- Wrote `docs/implementation-plan.md` — 18 build phases with specific file paths, function signatures, data structures, and acceptance criteria
- Created `memory/MEMORY.md` with locked decisions for future session continuity

---

## Issues & Troubleshooting

- **Problem:** Round 5 of questions was rejected by the user mid-tool-call.
  - **Cause:** User re-submitted the original prompt, which reset the session context.
  - **Fix:** Identified which round-5 questions had already been answered (account resolution, seed data, module name/volumes) from the re-run, consolidated into a single final round, and proceeded to write the plan.

- **Problem:** The EOD cutoff question in Round 2 returned only 3 answers instead of 4 (4th question about in-flight EOD handling was not captured in the response).
  - **Cause:** Answer aggregation in the multi-question tool response dropped the last question.
  - **Fix:** Re-asked the EOD cutoff question as the first item in Round 3. Answer confirmed: `created_at <= cutoff AND settlement_batch_id IS NULL`.

---

## Decisions Made

### Pipeline Architecture
- **Decision:** Fully synchronous deposit pipeline — POST /deposits runs Requested→FundsPosted in a single HTTP request.
- **Reasoning:** Single-user demo doesn't need async; synchronous is simpler to test, debug, and demo. Settlement (Completed) is a separate manual-trigger batch.

### Vendor Stub
- **Decision:** In-process function call with `VendorService` interface.
- **Reasoning:** Avoids Docker Compose complexity, keeps tests fast, same interface boundary means production would just swap in an HTTP client.

### State Machine Locking
- **Decision:** `UPDATE transfers SET status=$1 WHERE id=$2 AND status=$3` — check `rows affected = 0` for conflict.
- **Reasoning:** No lock held during validation logic; better concurrency than SELECT FOR UPDATE. Simpler code.

### Transition Logging
- **Decision:** `machine.Transition()` takes `*sql.Tx` and atomically writes to both `transfers` and `state_transitions` tables.
- **Reasoning:** Impossible to change state without an audit record. Callers can't forget to log.

### Operator Approval Flow
- **Decision:** Approve handler runs Analyzing→Approved→FundsPosted + ledger posting + audit log all in one Postgres transaction.
- **Reasoning:** Consistent with synchronous pipeline design. Operator sees deposit at FundsPosted in the approve response — no second trigger needed.

### Contribution Type
- **Decision:** Lives in the `transfers` table as `contribution_type VARCHAR(20)`. Set during Analyzing by funding rules (retirement accounts → INDIVIDUAL). Operator can override via the approve body.
- **Reasoning:** Audit trail is cleaner at the transfer level; doesn't require passing it all the way through to the ledger entry.

### Reversal Accounting
- **Decision:** Both reversal ledger entries are `FromAccountID=investor, ToAccountID=omnibus`. Entry 1: `SubType=REVERSAL`, amount=original deposit. Entry 2: `SubType=RETURN_FEE`, amount=3000.
- **Reasoning:** Matches PRD description of "debit investor for original + debit investor for fee." Decision log notes production would use a dedicated fee revenue account.

### EOD Cutoff Basis
- **Decision:** `created_at <= cutoff` (investor submission time, not processing time).
- **Reasoning:** Avoids race condition where a deposit submitted before cutoff might finish processing after. Matches how real bank cutoffs work.

### Image Handling
- **Decision:** Accept real file uploads, save to `/data/images/{transfer_id}/front.png` and `back.png`. Serve via `GET /api/v1/deposits/:id/images/:side`. Include synthetic placeholder PNGs in `scripts/fixtures/` for demo scripts.
- **Reasoning:** Makes operator UI functional with real image display. X9 ICL file generation reads real bytes. JSON settlement fallback if moov-io blocks progress.

### Auth Model
- **Decision:** Hardcoded tokens in `.env.example`. Token validates role only. `account_id` comes from request body — one investor token works for all 7 vendor stub scenarios.
- **Reasoning:** Mirrors Apex B2B model. Evaluator doesn't need 7 tokens to test 7 scenarios. Decision log notes production would use OAuth/JWT.

### Migrations
- **Decision:** `RunMigrations()` called in `main.go` at startup. Raw SQL with `CREATE TABLE IF NOT EXISTS`. Seed data with `ON CONFLICT DO NOTHING`. No external migration library.
- **Reasoning:** No extra dependency needed. `docker compose down -v && docker compose up` for clean reset.

### UI Polling
- **Decision:** Operator queue polls every 5s via `setInterval`. Transfer status polls every 2s while status is non-terminal. Cleanup on unmount.
- **Reasoning:** Simple, reliable for demo. Decision log notes production would use WebSockets or SSE.

### Settlement Trigger
- **Decision:** Manual API only — `POST /api/v1/settlement/trigger`. No background cron.
- **Reasoning:** Evaluator controls when batching happens. Deterministic for demo. Decision log notes production would use a scheduled job.

### Seed Data
- **Decision:** 3 correspondents (SoFi/OMNI-SOFI-001, Webull/OMNI-WBL-001, CashApp/OMNI-CASH-001) and 8 investor accounts: ACC-SOFI-1001 through 1006 (all 7 stub scenarios), ACC-SOFI-0000 (basic pass), ACC-RETIRE-001 (retirement type, Webull correspondent).
- **Reasoning:** Demonstrates multi-tenant omnibus mapping. Self-documenting account IDs with scenario suffixes. Retirement account on a different correspondent shows FK chain works.

### Module Name
- **Decision:** `github.com/apex/mcd` — short for clean imports across all packages.
- **Reasoning:** GitHub repo name stays `mobile-check-deposit`. Module name is internal convention.

### Docker Volumes
- **Decision:** Named volumes: `mcd-images`, `mcd-settlement`, `postgres-data`, `redis-data`. Backend creates directories at startup with `os.MkdirAll`.
- **Reasoning:** Persists across container restarts. `docker compose down -v` for clean reset.

---

## Current State

- **No code written yet.** This was a pure planning session.
- `docs/implementation-plan.md` exists with 18 phases covering every layer of the system.
- `docs/PRD.md` and `docs/presearch.md` exist as prior context (written in earlier sessions).
- `memory/MEMORY.md` has locked decisions for session continuity.
- All architectural ambiguities are resolved. The plan is ready to execute.

### What the plan covers
- Phase 1: Project scaffold, go.mod, Docker Compose, Makefile
- Phase 2: DB schema (7 tables) + seed data SQL
- Phase 3: Domain models (Transfer, Account, errors)
- Phase 4: State machine with optimistic locking + 6 tests
- Phase 5: Vendor stub with all 7 deterministic scenarios + 7 tests
- Phase 6: Funding service (deposit limit, dupe detection, account resolver, contribution type) + 10 tests
- Phase 7: Ledger service (append-only posting, reversal with fee) + 6 tests
- Phase 8: Deposit handler/service (full synchronous pipeline) + return handler
- Phase 9: Middleware (auth + Redis rate limiting)
- Phase 10: Operator service (approve/reject/audit) + 3 tests
- Phase 11: Settlement engine (X9 ICL generation, EOD cutoff) + 6 tests
- Phase 12: Main server DI wiring and router setup
- Phase 13: React frontend (4 components: DepositForm, ReviewQueue, TransferStatus, LedgerView)
- Phase 14: Full test suite (29 total tests across 6 test files)
- Phase 15: Demo scripts (happy path, all scenarios, return/reversal, settlement trigger)
- Phase 16: Dockerfiles (multi-stage backend, Vite frontend)
- Phase 17: Health check endpoint
- Phase 18: Documentation (architecture.md, decision_log.md, risks.md, scenario-coverage.md)

---

## Next Steps

1. **Start Phase 1** — Initialize repo structure, `go mod init github.com/apex/mcd`, write `docker-compose.yml`, `backend/.env.example`, `Makefile`. Verify `docker compose up` starts containers.
2. **Phase 2** — Write migration SQL (`001_create_tables.sql`, `002_seed_data.sql`) and `internal/db/migrations.go`. Verify idempotent startup.
3. **Phase 3 + 4** — Write models and state machine. Write the 6 state machine tests first (TDD). All tests pass before moving on.
4. **Phase 5** — Vendor stub. Write stub_test.go first, then implement `stub.go`. Confirm all 7 scenarios deterministic.
5. **Phase 6** — Funding service. `rules_test.go` first, then implement. Confirm deposit limit, dupe check (Redis), contribution type, account resolver.
6. **Phase 7 + 8** — Ledger service, then deposit handler/service. At this point the happy path should be end-to-end testable via curl.
7. **Checkpoint** — Run `./scripts/demo-happy-path.sh` (even if script is minimal at this stage). If it passes, core correctness is proven.
8. **Phase 10 + 11** — Operator service and settlement engine. Full flow testable.
9. **Phase 13** — React frontend. Aim for functional, not polished.
10. **Phase 14** — Complete remaining tests to hit 29 total.
11. **Phase 15** — Demo scripts with real fixture images.
12. **Phase 18** — Documentation pass. Rubric alignment check.
