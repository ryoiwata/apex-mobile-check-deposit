# Pre-Search Document: Mobile Check Deposit System

**Project:** Mobile Check Deposit System for Apex Fintech Services
**Date:** March 2026
**Time Budget:** 2 hours
**Author:** [Your Name]

---

## Phase 1: Define Your Constraints

### 1. Scale & Load Profile

**How many concurrent users will the system need to support at demo time?**

One. This is a take-home project evaluated by 1-2 reviewers running through demo scripts sequentially. There is no concurrent load requirement for the deliverable. However, architectural decisions should demonstrate awareness of concurrency — the state machine must be safe against race conditions (two goroutines attempting to transition the same transfer), and the settlement batch must handle multiple deposits atomically.

**How many API calls does a single deposit generate internally?**

A single happy-path deposit generates approximately 6-8 internal operations:

1. Deposit submission → write transfer record (Postgres)
2. Vendor Service validation → stub call (in-process or HTTP to localhost)
3. State transition: Requested → Validating → Analyzing (2 Postgres updates)
4. Funding Service: account resolution + business rule checks (1-2 Postgres reads, 1 Redis read for dupe hash)
5. State transition: Analyzing → Approved (1 Postgres update)
6. Ledger posting: create ledger entry + state to FundsPosted (1 Postgres transaction, 2 writes)
7. Settlement: batch query + X9 file generation (1 Postgres read, 1 file write)
8. State transition: FundsPosted → Completed (1 Postgres update)

Total: ~10-12 database operations, 1 vendor call, 1 Redis call, 1 file write per deposit.

**What is the maximum acceptable latency for each operation?**

Per the spec's performance benchmarks:
- Vendor stub validation: < 1 second (trivial — it's a stub returning deterministic responses)
- Ledger posting: < 2 seconds from approval
- Settlement file generation: < 5 seconds for a batch
- State transitions: < 1 second, queryable immediately
- Operator queue: flagged items visible immediately

All of these are easily achievable for a single-user local system. The real constraint is transactional correctness, not speed.

**How large will data grow, and how long must it be retained?**

For the demo: dozens of transfers, dozens of ledger entries, a handful of settlement files. Trivial storage. Schema design should not over-optimize for scale, but should be correct — foreign keys, indexes on status and account_id, append-only ledger entries. No retention policy needed for a demo.

### 2. Budget & Cost Ceiling

**Total budget for infrastructure during development:**

$0. The entire stack runs locally via Docker Compose. No cloud services, no paid APIs, no AI inference costs. PostgreSQL, Redis, Go, and React are all free. The moov-io/imagecashletter library is open source (Apache 2.0).

**What if deployed to cloud?**

The spec says local development is acceptable; cloud is optional (free tier). If deployed:
- AWS free tier: 1 t2.micro EC2, 20GB EBS, 750 hours/month RDS Postgres — covers everything
- Alternatively: Railway, Render, or Fly.io free tiers can host the Docker Compose stack
- Cost: $0 within free tier limits

**Are there any paid dependencies?**

No. All libraries are open-source:
- `gin-gonic/gin` — MIT
- `lib/pq` — MIT
- `redis/go-redis` — BSD-2
- `moov-io/imagecashletter` — Apache 2.0
- `stretchr/testify` — MIT
- React, Vite — MIT

### 3. Time to Ship

**Which component has the most risk?**

The **state machine + ledger posting integration** is the highest-risk component. It touches the most rubric points (25 for core correctness + 10 for return/reversal = 35 points), requires transactional database operations across multiple tables, and must handle both happy and unhappy paths correctly. A bug in state transition validation or ledger math cascades into test failures across every scenario.

Second highest risk: the **vendor stub** (15 points). Not because it's technically hard, but because it must support 7 distinct response scenarios triggered deterministically. If the stub design is awkward (e.g., requires code changes to switch scenarios), it costs points on both stub quality and test coverage.

Third: **settlement file generation** using moov-io/imagecashletter. This is a library I haven't used before. Budget 2-3 hours for learning the API and generating a valid X9 file. Fallback: generate a structured JSON equivalent (the spec explicitly allows this).

**What is the minimum viable feature set for a passing score?**

Mapping to the rubric:
- System design + architecture (20 pts): state machine diagram, service boundary docs, decision log
- Core correctness (25 pts): happy path end-to-end, business rules enforced, ledger postings correct
- Vendor stub (15 pts): 7 deterministic scenarios, configurable via account suffix
- Operator workflow (10 pts): review queue, approve/reject, audit log
- Return/reversal (10 pts): bounced check → reversal + $30 fee → Returned state
- Tests (10 pts): minimum 10 tests covering all paths
- Developer experience (10 pts): one-command setup, README, demo scripts

**If I cut one subsystem, which would it be?**

The **operator UI** (React frontend). The backend API for operator review can still be tested via demo scripts (curl commands), and the rubric only allocates 10 points to it. I'd keep the operator backend handlers and audit logging, but replace the React dashboard with a CLI/script-based review flow. This saves 4-6 hours of React development time that can be redirected to core correctness and test coverage.

Fallback plan: If the React UI is cut, the demo scripts must be comprehensive enough to show the operator workflow via API calls with formatted output.

**Time allocation plan (7 days):**

| Day | Focus | Hours | Deliverable |
|-----|-------|-------|------------|
| 1 (first 2h) | Pre-search | 2 | This document |
| 1 (remaining) | Project scaffold, Docker Compose, DB schema, state machine | 6 | Skeleton compiles, docker compose up works, migrations run |
| 2 | Vendor stub (all 7 scenarios) + Funding Service (business rules) | 8 | Stub returns correct responses, rules enforce limits |
| 3 | Ledger posting + reversal engine + state machine integration | 8 | Happy path end-to-end, reversal with fee works |
| 4 | Settlement engine (moov-io X9) + operator API handlers | 8 | X9 file generates, operator approve/reject works |
| 5 | React operator UI + deposit submission UI | 6 | Functional (not polished) frontend |
| 6 | Tests (10+), demo scripts, integration testing | 8 | All tests pass, all scenarios exercised |
| 7 | Documentation, decision log, README polish, final testing | 6 | Submission-ready repo |

**Total:** ~52 hours. Buffer: days 5-6 can absorb overflow from days 2-4.

### 4. Compliance & Security

**Are we handling user data that requires encryption at rest?**

No. The spec explicitly requires synthetic data only — no real PII, account numbers, or check images. However, building with production-safe habits is important for the interview:
- Mask account numbers in logs (last 4 digits only)
- Store secrets via environment variables, never hardcoded
- Append-only ledger (no delete/update endpoints)
- Structured error responses that never leak internal details

**How will we store credentials securely?**

No third-party API credentials exist (all services are local/stubbed). Database and Redis connection strings go in `.env` (gitignored), with `.env.example` committed. Docker Compose uses `env_file` directive.

**What happens if the server is compromised?**

Not a real concern for a local demo. In a production writeup (decision log), I'd note: credential rotation, encrypted at-rest storage for check images, audit log tamper detection, and PCI-DSS compliance for handling financial instrument images.

### 5. Skill Constraints

**Strongest area:** Backend systems, Go, database design, API architecture.

**Weakest area:** React frontend. Mitigation: keep the UI minimal, use Tailwind for quick styling, focus on functionality over polish. The rubric allocates only 10 points to operator workflow UI — a functional dashboard with working approve/reject beats a beautiful one that doesn't connect to the backend.

**Integration patterns I've worked with:** REST APIs, PostgreSQL, Redis, Docker Compose, state machines. The X9 ICL settlement format and moov-io library are new — budget extra time.

**Fallback if moov-io doesn't work out by hour 12:** Generate settlement files as structured JSON with the same data fields (the spec allows "X9 ICL format or structured JSON equivalent"). Document this in the decision log as a deliberate trade-off.

---

## Phase 2: Architecture Discovery

### 1. Service Architecture

**Will services communicate synchronously or asynchronously?**

Synchronously for the MVP. The deposit flow is a linear pipeline: submit → validate → analyze → approve → post → settle. Each step depends on the result of the previous one. Synchronous request-response keeps the implementation simple, debuggable, and deterministic.

For the decision log (production consideration): Apex runs Kafka and Pub/Sub. A production system would use event-driven state transitions — each state change publishes an event, downstream services consume and process. The settlement engine would be a Kafka consumer that batches FundsPosted events. This decouples services and improves resilience but adds significant infrastructure complexity that is inappropriate for a take-home.

**Monolith or microservices?**

Monolith with clear internal package boundaries. All services live in a single Go binary under `internal/`:

```
internal/
├── vendor/       # Vendor Service stub
├── funding/      # Funding Service business rules
├── ledger/       # Ledger posting + reversals
├── state/        # Transfer state machine
├── settlement/   # X9 ICL generation
├── operator/     # Review queue + audit
├── models/       # Shared domain types
├── middleware/    # Auth, rate limiting
└── db/           # Postgres + Redis setup
```

This gives the evaluator clear separation of concerns (rubric: "clean, readable code with clear separation") while avoiding the operational overhead of running multiple services in Docker Compose. Each package has its own interface, making it testable in isolation.

**How will we handle multi-step workflows?**

The transfer state machine is the orchestrator. Each API handler advances the state machine one step:

1. `POST /deposits` → creates transfer in `Requested`, calls vendor stub, transitions to `Validating`
2. Vendor response triggers transition to `Analyzing` (pass/flagged) or `Rejected` (fail)
3. Funding rules run during `Analyzing` → transitions to `Approved` or `Rejected`
4. Ledger posting runs on `Approved` → transitions to `FundsPosted`
5. Settlement batch queries `FundsPosted` → generates X9 → transitions to `Completed`

The state machine enforces that no step can be skipped. Every transition is validated against the allowed transition table before executing.

**How will we prevent duplicate actions?**

Three layers:
1. **Vendor-level duplicate detection**: Stub checks for matching check serial + routing number (deterministic, triggered by account suffix `*1004`)
2. **Funding-level duplicate detection**: Redis hash of (routing_number + account_number + amount + check_serial) with 90-day TTL. If hash exists, reject.
3. **State machine protection**: A transfer in `Validating` cannot be re-submitted. The state machine rejects any transition that doesn't match the allowed table.

### 2. Data Layer Design

**What data must persist across sessions?**

Everything. Transfers, ledger entries, audit logs, operator actions, and settlement batch records must survive server restarts. PostgreSQL handles all persistent data. Redis handles ephemeral data (rate limit counters, duplicate check hashes with TTL).

**Schema design (core tables):**

```sql
-- Transfers: the central entity
CREATE TABLE transfers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id VARCHAR(50) NOT NULL,
    amount_cents BIGINT NOT NULL CHECK (amount_cents > 0),
    status VARCHAR(20) NOT NULL DEFAULT 'requested'
        CHECK (status IN ('requested','validating','analyzing','approved',
                          'funds_posted','completed','rejected','returned')),
    flagged BOOLEAN DEFAULT FALSE,
    flag_reason VARCHAR(100),
    vendor_transaction_id VARCHAR(100),
    micr_routing VARCHAR(9),
    micr_account VARCHAR(20),
    micr_serial VARCHAR(20),
    micr_confidence DECIMAL(3,2),
    ocr_amount_cents BIGINT,
    declared_amount_cents BIGINT NOT NULL,
    front_image_ref VARCHAR(255),
    back_image_ref VARCHAR(255),
    settlement_batch_id UUID,
    return_reason VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW(),
    updated_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_transfers_status ON transfers(status);
CREATE INDEX idx_transfers_account ON transfers(account_id);
CREATE INDEX idx_transfers_created ON transfers(created_at);

-- Ledger entries: append-only, never update or delete
CREATE TABLE ledger_entries (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    to_account_id VARCHAR(50) NOT NULL,
    from_account_id VARCHAR(50) NOT NULL,
    type VARCHAR(20) NOT NULL DEFAULT 'MOVEMENT',
    sub_type VARCHAR(20) NOT NULL DEFAULT 'DEPOSIT',
    transfer_type VARCHAR(20) NOT NULL DEFAULT 'CHECK',
    currency VARCHAR(3) NOT NULL DEFAULT 'USD',
    amount_cents BIGINT NOT NULL,
    memo VARCHAR(50) DEFAULT 'FREE',
    source_application_id UUID,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_ledger_transfer ON ledger_entries(transfer_id);
CREATE INDEX idx_ledger_to_account ON ledger_entries(to_account_id);

-- State history: every transition recorded
CREATE TABLE state_transitions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    from_state VARCHAR(20) NOT NULL,
    to_state VARCHAR(20) NOT NULL,
    triggered_by VARCHAR(50),  -- 'system', 'operator:OP-001', etc.
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_state_transfer ON state_transitions(transfer_id);

-- Operator audit log
CREATE TABLE audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_id VARCHAR(50) NOT NULL,
    action VARCHAR(20) NOT NULL,  -- 'approve', 'reject', 'override'
    transfer_id UUID NOT NULL REFERENCES transfers(id),
    notes TEXT,
    metadata JSONB,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
CREATE INDEX idx_audit_transfer ON audit_logs(transfer_id);
CREATE INDEX idx_audit_operator ON audit_logs(operator_id);

-- Settlement batches
CREATE TABLE settlement_batches (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_date DATE NOT NULL,
    file_path VARCHAR(255),
    deposit_count INTEGER NOT NULL DEFAULT 0,
    total_amount_cents BIGINT NOT NULL DEFAULT 0,
    status VARCHAR(20) DEFAULT 'pending',  -- pending, submitted, acknowledged
    bank_reference VARCHAR(100),
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Accounts (seed data for demo)
CREATE TABLE accounts (
    id VARCHAR(50) PRIMARY KEY,
    correspondent_id VARCHAR(50) NOT NULL,
    account_type VARCHAR(20) NOT NULL,  -- 'individual', 'retirement', 'joint'
    status VARCHAR(20) NOT NULL DEFAULT 'active',
    created_at TIMESTAMPTZ DEFAULT NOW()
);

-- Correspondents (seed data for demo)
CREATE TABLE correspondents (
    id VARCHAR(50) PRIMARY KEY,
    name VARCHAR(100) NOT NULL,
    omnibus_account_id VARCHAR(50) NOT NULL,
    created_at TIMESTAMPTZ DEFAULT NOW()
);
```

**Why PostgreSQL over SQLite?**

- FK constraints across transfers → ledger_entries → audit_logs enforce referential integrity
- Transactional guarantees: reversal posting (debit original + debit fee + state update) must be atomic
- JSONB columns for flexible metadata without schema changes
- Concurrent read/write support (SQLite's single-writer limitation would block operator queries during deposit processing)
- Apex runs Postgres in production — signals production thinking to evaluators

**Why Redis (and when to cut it)?**

Redis serves two purposes:
1. **Duplicate check hash cache**: Store `SHA256(routing + account + amount + serial)` with 90-day TTL. O(1) lookup on every deposit.
2. **Rate limiting**: Sliding window counter per account for deposit submission (10/minute).

If time is tight, both can be replaced with Postgres queries (duplicate check via SQL, rate limiting via count-with-timestamp query). Redis is a "production polish" addition — include it if ahead of schedule, cut it if behind. The duplicate check is more important than rate limiting.

### 3. Vendor Stub Design

**How will the stub determine which response to return?**

Account suffix mapping — the last 4 digits of the account_id determine the vendor response:

| Suffix | Response | IQA | MICR | Dupe | Amount Match | Next State |
|--------|----------|-----|------|------|-------------|------------|
| `1001` | FAIL | blur | — | — | — | Rejected |
| `1002` | FAIL | glare | — | — | — | Rejected |
| `1003` | FLAGGED | pass | failure | — | — | Analyzing (flagged) |
| `1004` | FAIL | pass | pass | found | — | Rejected |
| `1005` | FLAGGED | pass | pass | clear | mismatch | Analyzing (flagged) |
| `1006` | PASS | pass | pass | clear | match | Analyzing |
| `0000` | PASS | pass (basic) | pass | clear | match | Analyzing |
| other  | PASS | pass | pass | clear | match | Analyzing |

**Why account suffix over request headers or config files?**

- **Deterministic**: same account always produces same result, no setup needed
- **Self-documenting**: test scripts show the account number, making it obvious which scenario is being exercised
- **No code changes**: evaluators can test any scenario by changing the account_id in their request
- **Composable**: tests can run in any order, no shared state between scenarios

**What does the stub return?**

A structured `VendorResponse` with all fields populated (or null for failures):

```go
type VendorResponse struct {
    Status         string      `json:"status"`           // "pass", "fail", "flagged"
    IQAResult      string      `json:"iqa_result"`       // "pass", "fail_blur", "fail_glare"
    MICRData       *MICRData   `json:"micr_data"`        // null on MICR failure
    OCRAmountCents *int64      `json:"ocr_amount_cents"`
    DuplicateCheck string      `json:"duplicate_check"`  // "clear", "duplicate_found"
    AmountMatch    bool        `json:"amount_match"`
    TransactionID  string      `json:"transaction_id"`   // vendor-side reference
    ErrorCode      *string     `json:"error_code"`
    ErrorMessage   *string     `json:"error_message"`
}
```

### 4. Settlement File Strategy

**X9 ICL via moov-io or JSON fallback?**

Start with moov-io/imagecashletter. The library provides Go structs for all X9 record types (FileHeader, CashLetterHeader, BundleHeader, CheckDetail, ImageView, etc.). The core workflow:

1. Query all transfers in `FundsPosted` state with `created_at` before EOD cutoff
2. Create a `CashLetter` with a `Bundle` containing `CheckDetail` records
3. Populate each `CheckDetail` with MICR data from the transfer
4. Add `ImageView` references for front/back images
5. Write the file via `imagecashletter.NewWriter()`

**Fallback**: If moov-io proves too complex within the time budget, generate a structured JSON file with identical data fields. Document this in the decision log as "JSON equivalent per spec allowance, would use X9 ICL in production."

**EOD cutoff logic:**

```go
func isBeforeCutoff(depositTime time.Time) bool {
    ct, _ := time.LoadLocation("America/Chicago")
    depositCT := depositTime.In(ct)
    cutoff := time.Date(depositCT.Year(), depositCT.Month(), depositCT.Day(),
        18, 30, 0, 0, ct) // 6:30 PM CT
    return depositCT.Before(cutoff)
}
```

Deposits after 6:30 PM CT get `batch_date` set to the next business day. Weekend/holiday handling is out of scope for MVP (noted in risks).

### 5. Authentication & Session Model

**What auth model does this project need?**

Simplified for demo. The spec mentions "validate investor session" but does not require full OAuth. Implementation:

- `Authorization: Bearer <session_token>` header on all deposit endpoints
- Token maps to a seeded investor account in the database
- Hardcoded test tokens in `.env.example` (e.g., `TEST_TOKEN_1006=tok_investor_1006`)
- Middleware validates token exists and resolves to an account; returns 401 if missing/invalid

For operator endpoints: `X-Operator-ID` header identifies the operator. No authentication (simplified for demo), but every action is logged with the operator ID for the audit trail.

**Production note for decision log:** Real system would use Apex's existing OAuth/gRPC auth infrastructure, with JWT tokens issued by the correspondent's identity provider and validated against Apex's auth service.

### 6. Frontend Approach

**What gets built?**

Minimal React app with 4 views:

1. **Deposit Form**: file upload (front/back images), amount input, account selector → POST to API
2. **Transfer Status**: real-time status display for a single transfer, showing state history
3. **Operator Review Queue**: table of flagged deposits with images, MICR data, approve/reject buttons
4. **Ledger View**: account balance + list of ledger entries

**Technical decisions:**
- Vite for build tooling (fast, zero-config)
- Tailwind for styling (utility classes, no CSS files)
- Single `api.js` module for all backend calls
- No state management library (useState is sufficient for this scope)
- No routing library (tab-based navigation within a single page)

**Time budget:** 6 hours max. If behind schedule, replace with comprehensive demo scripts (curl + jq).

---

## Phase 3: Post-Stack Refinement

### 1. Failure Mode Analysis

| Component | Failure Mode | User Experience | Mitigation |
|-----------|-------------|----------------|------------|
| PostgreSQL down | All operations fail | 503 with "service unavailable" message | Health check endpoint; Docker restart policy |
| Redis down | Dupe detection skipped; rate limiting disabled | Graceful degradation — log warning, continue without Redis | Fallback to Postgres query for dupe check |
| Vendor stub error | Unexpected response format | 502 with "vendor service error" | Wrap stub call in try/catch; return structured error |
| State machine conflict | Two requests transition same transfer | 409 with "state conflict" | Database-level optimistic locking (WHERE status = expected_status) |
| Settlement file write fails | X9 file not generated | Log error; settlement batch marked "failed" | Retry logic; manual trigger endpoint |
| Invalid deposit amount | Negative or zero amount submitted | 400 with specific validation error | Input validation in handler before any processing |
| Image too large | >10MB upload | 413 with size limit message | Gin middleware to enforce request body size |

### 2. Security Considerations

**Can a malicious request cause unintended state transitions?**

No. The state machine validates every transition against the allowed table. A request to move a `Rejected` transfer to `Approved` returns 409. The only way to change state is through the validated transition function.

**Can one account access another's transfers?**

For the MVP, transfers are not scoped to sessions (simplified auth). A production system would add `account_id` to the auth context and filter all queries. Noted in risks.

**Are secrets ever logged?**

No. The logging middleware redacts the `Authorization` header. Database connection strings are loaded from `.env` and never appear in log output. Account numbers are masked to last 4 digits in all log entries.

### 3. Testing Strategy

**How will we validate the system?**

Three layers:

1. **Unit tests** (per package): state machine transitions, business rules, fee calculation, vendor stub response routing. Table-driven tests with testify. ~10-12 tests.
2. **Integration tests** (cross-package): happy path end-to-end, each vendor scenario end-to-end, reversal flow. Use test database in Docker. ~5 tests.
3. **Demo scripts** (shell): curl-based scripts that exercise every path against a running system. Human-readable output. Serve as both validation and demo.

**What about the frontend?**

Manual testing only. The rubric doesn't score frontend tests, and the UI is simple enough to validate by clicking through the 4 views. Time is better spent on backend test coverage.

### 4. Observability

**What will we log for each deposit?**

Structured JSON logs with:
- `transfer_id`: UUID for correlation across all log entries
- `state`: current transfer state
- `action`: what happened (e.g., "vendor_validation", "rule_check", "ledger_post")
- `result`: outcome (e.g., "pass", "fail_blur", "over_limit")
- `duration_ms`: time for the operation
- `operator_id`: if an operator action
- `account_id_masked`: last 4 digits only

Every state transition, business rule evaluation, and operator action is logged. This creates the "per-deposit decision trace" required by the spec.

### 5. Deployment Plan

**Local only (primary):**

```bash
docker compose up --build
```

Starts: Go backend (:8080), React frontend (:5173), PostgreSQL (:5432), Redis (:6379). Migrations run on backend startup. Seed data inserted on first run.

**Cloud (optional, if time allows):**

- AWS free tier: EC2 t2.micro running Docker Compose
- Or: Railway/Render deploying from GitHub repo
- CORS configured for deployed frontend URL

**Rollback strategy:** `docker compose down -v && docker compose up --build` destroys all data and starts fresh. For a demo, this is sufficient. Noted in risks that production would need proper migration versioning and zero-downtime deploys.

---

## Key Decisions Summary

| Decision | Choice | Primary Alternative | Rationale |
|----------|--------|-------------------|-----------|
| Language | Go | Java + Spring Boot | Apex is Go-first; lighter footprint; faster iteration |
| HTTP framework | Gin | Chi, Echo | Largest community; strong middleware; evaluator familiarity |
| Database | PostgreSQL | SQLite | FK constraints; ACID transactions for reversals; Apex's stack |
| Cache | Redis (optional) | In-memory / Postgres | Dupe hash TTL; rate limiting; Apex runs Redis; cut if behind |
| Settlement format | moov-io X9 ICL | Structured JSON | Purpose-built library; shows domain awareness; JSON as fallback |
| Vendor stub trigger | Account suffix | Config file, headers | Deterministic; self-documenting; no code changes needed |
| Architecture | Monolith w/ packages | Microservices | Clear separation without operational overhead; appropriate for scope |
| State management | Explicit state machine | Direct DB updates | Enforces valid transitions; audit-friendly; prevents impossible states |
| Money representation | int64 cents | float64 | Eliminates rounding; standard financial systems practice |
| Frontend | React (minimal) | HTMX, CLI-only | Matches Apex stack; reactive operator queue; cut to CLI if behind |
| Async/events | Synchronous | Kafka, Pub/Sub | Simpler; debuggable; event-driven noted for production decision log |

---

## Risk Register

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| moov-io/imagecashletter learning curve exceeds budget | Medium | Medium | Fallback to JSON settlement format (spec allows) |
| React UI consumes too much time | Medium | Low | Cut to CLI/demo scripts; backend API still testable |
| State machine race conditions in tests | Low | High | Optimistic locking with `WHERE status = expected`; test with concurrent goroutines |
| Postgres schema migrations break on rebuild | Low | Medium | Idempotent migrations with `IF NOT EXISTS`; `docker compose down -v` as reset |
| Reversal fee math is wrong | Low | High | Dedicated unit test for fee calculation; amounts in cents eliminate rounding |
| EOD cutoff timezone handling | Medium | Low | Use `America/Chicago` timezone explicitly; skip weekends/holidays for MVP |
| Evaluator's Docker environment differs | Low | Medium | Pin all image versions; test on clean machine before submission |

---

## Commit Checkpoint

Pre-search complete. Timer stopped. Committing to repo and beginning implementation.

**First implementation task:** Project scaffold — `go mod init`, Docker Compose file, database migrations, state machine package with transition table and tests.
