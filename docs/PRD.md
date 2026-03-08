# Product Requirements Document: Mobile Check Deposit System

**Version:** 1.0
**Date:** March 2026
**Author:** [Your Name]
**Status:** Draft
**Company:** Apex Fintech Services — Week 4 Technical Assessment

---

## 1. Executive Summary

This document defines the requirements for a minimal end-to-end mobile check deposit system that enables investors to deposit checks into brokerage accounts via a mobile application. The system handles the full deposit lifecycle — image capture, vendor validation, business rule enforcement, ledger posting, operator review, settlement, and return/reversal — with a stubbed vendor integration that supports deterministic scenario testing.

The system is built for Apex Fintech Services, a B2B API-driven fintech that provides trading, clearing, and custody infrastructure to broker-dealers including SoFi, Webull, Coinbase, and CashApp. The technical assessment evaluates system design, correctness, and production-oriented thinking against a 100-point rubric.

---

## 2. Problem Statement

Apex's correspondent broker-dealers need to offer mobile check deposit to their end investors. Today, check deposits rely on manual or mail-based processes that are slow, error-prone, and create poor investor experiences. A modern mobile check deposit system must capture check images, validate them against banking standards (IQA, MICR, OCR), enforce business rules (deposit limits, duplicate detection, account eligibility), route flagged items to human operators, settle approved deposits with a bank via industry-standard X9 ICL files, and handle the inevitable bounced checks with correct reversal accounting.

The core challenge is not any single component — it is orchestrating the full lifecycle correctly, ensuring no deposit posts to the ledger without passing validation and business rules, no settlement file includes rejected deposits, and every reversal includes the correct fee deduction.

---

## 3. Goals and Non-Goals

### Goals

- Demonstrate a working end-to-end deposit flow from submission to settlement
- Enforce business rules with zero bypass (no deposit posts without passing validation + rules)
- Provide a configurable vendor stub that exercises all validation paths without code changes
- Implement correct reversal accounting with fee deduction for bounced checks
- Deliver an operator review workflow with a complete audit trail
- Generate X9 ICL settlement files with correct deposit data and EOD cutoff enforcement
- Achieve one-command setup (`docker compose up`) and clear documentation

### Non-Goals

- Real check image processing, OCR, or MICR reading (vendor is fully stubbed)
- Production authentication or authorization (simplified session model for demo)
- Horizontal scaling, distributed locking, or high-availability architecture
- Real bank integration or payment processing
- Mobile-native application (React web UI simulates the mobile experience)
- Compliance certification or regulatory claims
- AI/ML-based fraud detection or risk scoring

---

## 4. Target Users

| User | Role | Primary Actions |
|------|------|----------------|
| **Investor** | End user of a correspondent broker-dealer | Photograph check, submit deposit, track status, receive notifications |
| **Operator** | Apex internal operations staff | Review flagged deposits, approve/reject with rationale, view audit history |
| **System (automated)** | Settlement engine, business rules | Batch deposits for settlement, enforce limits, generate X9 files, process returns |
| **Evaluator** | Apex engineering interviewer | Run demo scripts, review code, test edge cases, assess architecture |

---

## 5. Technical Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Language | Go 1.22+ | Apex's primary language; concurrency primitives; fast compile times |
| HTTP Framework | Gin | Largest Go web framework community; strong middleware support |
| Database | PostgreSQL | FK constraints; ACID transactions for reversals; matches Apex's stack |
| Cache | Redis | Duplicate check hash TTL; rate limiting; Apex runs Redis in production |
| Frontend | React (Vite) | Apex uses React; reactive operator queue; Tailwind for styling |
| Settlement | moov-io/imagecashletter | Purpose-built Go library for X9 ICL format |
| Testing | go test + testify | Standard Go testing with assertion library |
| Infrastructure | Docker Compose | One-command setup for Go backend + Postgres + Redis + React |

### Alternatives Considered

| Decision | Rejected Alternative | Why Rejected |
|----------|---------------------|-------------|
| Go | Java + Spring Boot | Apex leans Go-first; lighter footprint; faster iteration for this scope |
| PostgreSQL | SQLite | No concurrent writes; no FK constraints; signals toy project to evaluators |
| REST | gRPC | Spec requires REST endpoints; gRPC adds proto/codegen complexity; noted for production |
| Synchronous | Kafka/Pub/Sub | Massive infrastructure overhead for a take-home; event-driven noted in decision log |
| React | HTMX | Apex uses React; operator queue benefits from reactive updates |

---

## 6. System Architecture

### 6.1 Service Boundaries

The system is a single Go binary with clearly separated internal packages:

```
┌─────────────────────────────────────────────────────────┐
│                      API Layer (Gin)                     │
│            Auth Middleware · Rate Limiting · Routing      │
├──────────┬──────────┬──────────┬──────────┬─────────────┤
│  Vendor  │ Funding  │  Ledger  │ Operator │ Settlement  │
│  Stub    │ Service  │ Service  │ Service  │ Engine      │
│          │          │          │          │             │
│ IQA/MICR │ Rules    │ Posting  │ Review   │ X9 ICL Gen  │
│ OCR/Dupe │ Limits   │ Reversal │ Audit    │ EOD Batch   │
│          │ Accounts │ Fees     │ Queue    │ Bank ACK    │
├──────────┴──────────┴──────────┴──────────┴─────────────┤
│              State Machine (Transfer Lifecycle)           │
├─────────────────────────┬───────────────────────────────┤
│      PostgreSQL         │          Redis                 │
│  Transfers · Ledger     │   Dupe Hashes · Rate Limits   │
│  Audit · Accounts       │                               │
└─────────────────────────┴───────────────────────────────┘
```

### 6.2 Transfer State Machine

```
Requested ──→ Validating ──→ Analyzing ──→ Approved ──→ FundsPosted ──→ Completed
                  │               │                                          │
                  ▼               ▼                                          ▼
               Rejected       Rejected                                    Returned
```

| State | Description | Entry Condition |
|-------|-------------|-----------------|
| Requested | Deposit submitted by investor | POST /deposits accepted |
| Validating | Sent to Vendor Service for IQA/MICR/OCR | Submission validated, vendor called |
| Analyzing | Business rules being applied by Funding Service | Vendor returned pass or flagged |
| Approved | Passed all checks; awaiting ledger posting | Rules passed or operator approved |
| FundsPosted | Provisional credit posted to investor account | Ledger entry created |
| Completed | Settlement confirmed by Settlement Bank | X9 batch acknowledged |
| Rejected | Failed validation, business rules, or operator review | Any failure or operator reject |
| Returned | Check bounced after settlement; reversal posted | Return notification received |

**Valid Transitions (exhaustive — all others are rejected with 409):**

| From | To | Trigger |
|------|----|---------|
| Requested | Validating | Deposit submitted to vendor |
| Validating | Analyzing | Vendor returned pass or flagged |
| Validating | Rejected | Vendor returned fail (IQA, duplicate) |
| Analyzing | Approved | Business rules passed |
| Analyzing | Rejected | Business rules failed or operator rejected |
| Approved | FundsPosted | Ledger entry created |
| FundsPosted | Completed | Settlement batch acknowledged |
| Completed | Returned | Check bounced post-settlement |

### 6.3 Data Flow: Happy Path

1. **Investor** submits check images + amount + account ID via React UI or API
2. **API Gateway** validates input, checks rate limits, authenticates session
3. **Transfer** created in `Requested` state → Postgres
4. **Vendor Stub** performs IQA, MICR extraction, OCR, duplicate check → returns `pass`
5. **State** transitions: Requested → Validating → Analyzing
6. **Funding Service** resolves account, checks deposit limit ($5,000), checks for duplicates in Redis, validates account eligibility
7. **State** transitions: Analyzing → Approved
8. **Ledger Service** creates transfer record (Type: MOVEMENT, SubType: DEPOSIT, TransferType: CHECK) with omnibus account mapping
9. **State** transitions: Approved → FundsPosted
10. **Settlement Engine** batches deposit into X9 ICL file at EOD (6:30 PM CT cutoff)
11. **State** transitions: FundsPosted → Completed

### 6.4 Data Flow: Return/Reversal

1. **Return notification** received (simulated via API)
2. **Ledger Service** creates two reversal entries: debit original amount + debit $30 return fee
3. **State** transitions: Completed → Returned
4. **Investor** notified of returned check and fee

---

## 7. Functional Requirements

### 7.1 Deposit Submission (P0)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| DEP-01 | Accept check deposit via API or UI | POST /api/v1/deposits accepts front_image, back_image, amount_cents, account_id |
| DEP-02 | Validate input before processing | Reject: missing fields (400), negative/zero amount (400), amount > $5,000 (422), invalid account (422) |
| DEP-03 | Create transfer in Requested state | Transfer record persisted to Postgres with UUID, timestamps |
| DEP-04 | Support re-submission on IQA failure | Error response includes actionable message; investor can retry with new images |
| DEP-05 | Rate limit submissions | Max 10 deposits/minute per account via Redis counter |

### 7.2 Vendor Service Stub (P0)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| VND-01 | Return deterministic responses by account suffix | Account `*1001` → blur, `*1002` → glare, `*1003` → MICR failure, `*1004` → duplicate, `*1005` → amount mismatch, `*1006` → clean pass, `*0000` → basic pass |
| VND-02 | No code changes to switch scenarios | Different account IDs trigger different responses automatically |
| VND-03 | Return structured response for all scenarios | Every response includes status, iqa_result, micr_data (or null), ocr_amount, duplicate_check, amount_match, transaction_id, error_code |
| VND-04 | Support minimum 7 response types | IQA pass, IQA fail (blur), IQA fail (glare), MICR failure, duplicate detected, amount mismatch, clean pass |

### 7.3 Funding Service (P0)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| FND-01 | Enforce $5,000 deposit limit | Deposits > 500000 cents rejected; transfer moves to Rejected |
| FND-02 | Resolve account to internal IDs | Account identifier maps to internal account + correspondent omnibus account |
| FND-03 | Detect duplicate deposits | Redis hash of (routing + account + amount + serial) with 90-day TTL; reject if exists |
| FND-04 | Default contribution type for retirement accounts | Retirement-type accounts default to INDIVIDUAL contribution type |
| FND-05 | Validate account eligibility | Account must exist and be in `active` status |

### 7.4 Ledger Posting (P0)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| LED-01 | Create transfer record with correct fields | ToAccountId, FromAccountId (omnibus), Type: MOVEMENT, SubType: DEPOSIT, TransferType: CHECK, Currency: USD, Amount in cents |
| LED-02 | Ledger entries are append-only | No update or delete endpoints for ledger entries; corrections are new reversal entries |
| LED-03 | Post within single transaction | Ledger entry creation + state transition to FundsPosted in one Postgres transaction |
| LED-04 | No posting without validation | Invariant: zero ledger entries exist for transfers that haven't passed both vendor validation and funding rules |

### 7.5 Operator Review (P1)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| OPR-01 | Review queue shows flagged deposits | Flagged deposits visible with check images, MICR data, confidence scores, amount comparison |
| OPR-02 | Approve/reject with mandatory logging | Every action recorded with operator_id, action, timestamp, notes, transfer_id |
| OPR-03 | Filter and search | Filter by date range, status, account_id, amount range |
| OPR-04 | Override contribution type | Operator can change contribution type default during approval |
| OPR-05 | Audit log viewable | All operator actions queryable with full history per transfer |

### 7.6 Settlement (P0)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| SET-01 | Generate X9 ICL file (or JSON equivalent) | File contains MICR data, image references, amounts, batch metadata for all FundsPosted deposits |
| SET-02 | Enforce EOD cutoff | Deposits after 6:30 PM CT excluded from current batch; rolled to next business day |
| SET-03 | Track batch status | Settlement batch record with deposit count, total amount, file path, bank acknowledgment status |
| SET-04 | Exclude rejected deposits | Invariant: no settlement file includes deposits in Rejected or Returned state |

### 7.7 Return/Reversal Handling (P0)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| RET-01 | Accept return notifications | POST /api/v1/deposits/:id/return with reason and bank reference |
| RET-02 | Create reversal entries | Two ledger entries: debit original amount + debit $30 fee from investor account |
| RET-03 | Transition to Returned | Transfer state moves from Completed to Returned |
| RET-04 | Only completed deposits can be returned | Return request on non-Completed transfer returns 409 |
| RET-05 | Fee amount is configurable | Return fee set via RETURN_FEE_CENTS environment variable (default 3000) |

### 7.8 Observability (P1)

| ID | Requirement | Acceptance Criteria |
|----|------------|-------------------|
| OBS-01 | Per-deposit decision trace | Structured JSON logs for every state transition, rule evaluation, and operator action |
| OBS-02 | Correlation via transfer_id | All log entries for a single deposit share the same transfer_id |
| OBS-03 | Redacted PII | Account numbers masked to last 4 digits in all logs |
| OBS-04 | Health check endpoint | GET /health returns Postgres and Redis connectivity status |

---

## 8. Data Model

### 8.1 Core Tables

**transfers** — central entity tracking each deposit through its lifecycle

| Column | Type | Notes |
|--------|------|-------|
| id | UUID (PK) | Generated on creation |
| account_id | VARCHAR(50) | Investor account identifier |
| amount_cents | BIGINT | Deposit amount; CHECK > 0 |
| declared_amount_cents | BIGINT | Amount entered by investor |
| status | VARCHAR(20) | Current state machine state |
| flagged | BOOLEAN | True if routed to operator review |
| flag_reason | VARCHAR(100) | Why flagged (micr_failure, amount_mismatch) |
| vendor_transaction_id | VARCHAR(100) | Vendor-side reference |
| micr_routing | VARCHAR(9) | Extracted routing number |
| micr_account | VARCHAR(20) | Extracted account number |
| micr_serial | VARCHAR(20) | Extracted check serial |
| micr_confidence | DECIMAL(3,2) | MICR read confidence score |
| ocr_amount_cents | BIGINT | OCR-recognized amount |
| front_image_ref | VARCHAR(255) | Path to front image |
| back_image_ref | VARCHAR(255) | Path to back image |
| settlement_batch_id | UUID (FK) | Links to settlement batch |
| return_reason | VARCHAR(100) | If returned, why |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |
| updated_at | TIMESTAMPTZ | DEFAULT NOW() |

**ledger_entries** — append-only financial records

| Column | Type | Notes |
|--------|------|-------|
| id | UUID (PK) | Generated on creation |
| transfer_id | UUID (FK) | References transfers.id |
| to_account_id | VARCHAR(50) | Destination account |
| from_account_id | VARCHAR(50) | Source account (omnibus) |
| type | VARCHAR(20) | MOVEMENT |
| sub_type | VARCHAR(20) | DEPOSIT |
| transfer_type | VARCHAR(20) | CHECK |
| currency | VARCHAR(3) | USD |
| amount_cents | BIGINT | Amount in cents |
| memo | VARCHAR(50) | FREE |
| source_application_id | UUID | TransferID |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |

**state_transitions** — audit trail for every state change

| Column | Type | Notes |
|--------|------|-------|
| id | UUID (PK) | Generated on creation |
| transfer_id | UUID (FK) | References transfers.id |
| from_state | VARCHAR(20) | Previous state |
| to_state | VARCHAR(20) | New state |
| triggered_by | VARCHAR(50) | system, operator:OP-001, etc. |
| metadata | JSONB | Additional context |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |

**audit_logs** — operator action history

| Column | Type | Notes |
|--------|------|-------|
| id | UUID (PK) | Generated on creation |
| operator_id | VARCHAR(50) | Who performed the action |
| action | VARCHAR(20) | approve, reject, override |
| transfer_id | UUID (FK) | Which deposit |
| notes | TEXT | Operator's rationale |
| metadata | JSONB | Before/after state, override details |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |

**settlement_batches** — EOD batch records

| Column | Type | Notes |
|--------|------|-------|
| id | UUID (PK) | Generated on creation |
| batch_date | DATE | Settlement date |
| file_path | VARCHAR(255) | Path to generated X9/JSON file |
| deposit_count | INTEGER | Number of deposits in batch |
| total_amount_cents | BIGINT | Sum of deposit amounts |
| status | VARCHAR(20) | pending, submitted, acknowledged |
| bank_reference | VARCHAR(100) | Bank ACK reference |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |

**accounts** — seed data for demo

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(50) (PK) | Account identifier |
| correspondent_id | VARCHAR(50) (FK) | Which broker-dealer |
| account_type | VARCHAR(20) | individual, retirement, joint |
| status | VARCHAR(20) | active, suspended, closed |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |

**correspondents** — broker-dealer configuration

| Column | Type | Notes |
|--------|------|-------|
| id | VARCHAR(50) (PK) | Correspondent identifier |
| name | VARCHAR(100) | Display name (e.g., "SoFi") |
| omnibus_account_id | VARCHAR(50) | Pooled funding account |
| created_at | TIMESTAMPTZ | DEFAULT NOW() |

### 8.2 Indexes

- `transfers`: status, account_id, created_at, settlement_batch_id
- `ledger_entries`: transfer_id, to_account_id
- `state_transitions`: transfer_id
- `audit_logs`: transfer_id, operator_id

### 8.3 Constraints

- All monetary amounts: BIGINT (cents), CHECK > 0
- Ledger entries: no UPDATE or DELETE operations exposed
- Foreign keys: ledger_entries.transfer_id → transfers.id, audit_logs.transfer_id → transfers.id
- Cascade deletes: forbidden (ledger entries are permanent)

---

## 9. API Endpoints

| Method | Path | Description | Auth |
|--------|------|-------------|------|
| POST | /api/v1/deposits | Submit new check deposit | Investor |
| GET | /api/v1/deposits/:id | Get transfer status + history | Investor |
| GET | /api/v1/deposits | List deposits (with filters) | Investor |
| GET | /api/v1/operator/queue | Get operator review queue | Operator |
| POST | /api/v1/operator/deposits/:id/approve | Approve flagged deposit | Operator |
| POST | /api/v1/operator/deposits/:id/reject | Reject flagged deposit | Operator |
| POST | /api/v1/settlement/trigger | Trigger EOD settlement batch | Operator |
| POST | /api/v1/deposits/:id/return | Simulate check return | Operator |
| GET | /api/v1/ledger/:account_id | View account ledger entries | Investor |
| GET | /api/v1/operator/audit | View operator audit log | Operator |
| GET | /health | Health check (Postgres + Redis) | None |

---

## 10. Vendor Stub Scenario Mapping

| Account Suffix | Scenario | Vendor Response | System Behavior |
|---------------|----------|-----------------|-----------------|
| `*1001` | IQA Fail (Blur) | status: fail, iqa: fail_blur | Rejected; investor prompted to retake |
| `*1002` | IQA Fail (Glare) | status: fail, iqa: fail_glare | Rejected; investor prompted to retake |
| `*1003` | MICR Read Failure | status: flagged, micr: null | Analyzing with flagged=true; enters operator queue |
| `*1004` | Duplicate Detected | status: fail, dupe: found | Rejected; reason: duplicate |
| `*1005` | Amount Mismatch | status: flagged, amounts differ | Analyzing with flagged=true; enters operator queue |
| `*1006` | Clean Pass | status: pass, all data populated | Analyzing; proceeds to business rules |
| `*0000` | IQA Pass (basic) | status: pass, basic data | Analyzing; proceeds to business rules |
| Any other | Default Clean Pass | status: pass, all data populated | Analyzing; proceeds to business rules |

---

## 11. Business Rules

| Rule | Condition | Action |
|------|-----------|--------|
| Deposit Limit | amount_cents > 500000 | Reject; transition to Rejected; reason: over_limit |
| Duplicate Check (Funding) | SHA256(routing + account + amount + serial) exists in Redis with TTL < 90 days | Reject; reason: duplicate_funding |
| Account Eligibility | Account status != active | Reject; reason: account_ineligible |
| Contribution Type Default | Account type = retirement | Set contribution type to INDIVIDUAL |
| EOD Cutoff | Deposit submitted after 6:30 PM CT | Rolls to next business day batch |

---

## 12. Performance Requirements

| Metric | Target | Measurement |
|--------|--------|-------------|
| Vendor stub round-trip | < 1 second | Time from vendor call to response |
| Ledger posting | < 2 seconds from approval | Time from Approved to FundsPosted |
| Settlement generation | < 5 seconds per batch | Time from trigger to file written |
| State transitions | < 1 second, queryable immediately | Time from transition to GET reflecting new state |
| Operator queue | Immediate | Flagged items visible in queue within 1 second |
| Setup time | < 10 minutes | Time from clone to running system |

---

## 13. Testing Requirements

### 13.1 Required Test Cases (Minimum 10)

| # | Test | Category | Points |
|---|------|----------|--------|
| 1 | Happy path end-to-end | Core correctness | 25 |
| 2 | IQA Fail — Blur (account `*1001`) | Vendor stub | 15 |
| 3 | IQA Fail — Glare (account `*1002`) | Vendor stub | 15 |
| 4 | MICR Read Failure → operator review | Vendor stub | 15 |
| 5 | Duplicate Detected | Vendor stub | 15 |
| 6 | Amount Mismatch → flagged | Vendor stub | 15 |
| 7 | Deposit over $5,000 limit | Business rules | 25 |
| 8 | Invalid state transitions rejected | State machine | 25 |
| 9 | Reversal with $30 fee calculation | Return handling | 10 |
| 10 | Settlement file contains only approved deposits | Settlement | 25 |

### 13.2 Testing Approach

- **Unit tests**: Table-driven tests with testify for state machine, business rules, fee calculation, stub routing
- **Integration tests**: End-to-end flows against test Postgres in Docker
- **Demo scripts**: Shell scripts exercising all paths via curl commands with formatted output
- **Test report**: Generated in /reports directory with scenario coverage matrix

---

## 14. Sprint Plan

| Day | Focus | Hours | Deliverable |
|-----|-------|-------|-------------|
| 1 (first 2h) | Pre-search | 2 | Pre-search document committed |
| 1 (remaining) | Scaffold, Docker Compose, DB schema, state machine | 6 | Skeleton compiles, migrations run |
| 2 | Vendor stub (7 scenarios) + Funding Service (rules) | 8 | Stub works, rules enforce limits |
| 3 | Ledger posting + reversal engine + state integration | 8 | Happy path e2e, reversal with fee |
| 4 | Settlement engine (moov-io X9) + operator API | 8 | X9 file generates, approve/reject works |
| 5 | React operator UI + deposit submission UI | 6 | Functional frontend |
| 6 | Tests (10+), demo scripts, integration testing | 8 | All tests pass |
| 7 | Documentation, decision log, README, final testing | 6 | Submission-ready |

---

## 15. Evaluation Alignment

| Rubric Category | Points | Key Deliverables |
|----------------|--------|-----------------|
| System design and architecture | 20 | Service boundary diagram, state machine, decision log, architecture.md |
| Core correctness | 25 | Happy path e2e, business rules enforced, ledger postings accurate |
| Vendor Service stub quality | 15 | 7 deterministic scenarios via account suffix, no code changes |
| Operator workflow and observability | 10 | Review queue, approve/reject, audit trail, decision traces |
| Return/reversal handling | 10 | Reversal entries correct, $30 fee included, state transitions valid |
| Tests and evaluation rigor | 10 | 10+ tests, all paths exercised, test report generated |
| Developer experience | 10 | `docker compose up`, README, demo scripts, decision log |

---

## 16. Risks and Mitigations

| Risk | Likelihood | Impact | Mitigation |
|------|-----------|--------|------------|
| moov-io/imagecashletter learning curve | Medium | Medium | Fallback to structured JSON settlement format (spec allows this) |
| React UI consumes too much time | Medium | Low | Cut to CLI demo scripts; backend API testable via curl |
| State machine race conditions | Low | High | Optimistic locking via WHERE status = expected; test with concurrent goroutines |
| Schema migrations break on rebuild | Low | Medium | Idempotent migrations with IF NOT EXISTS; docker compose down -v as reset |
| Reversal fee math incorrect | Low | High | Dedicated unit test; amounts in int64 cents eliminate float rounding |
| EOD cutoff timezone handling | Medium | Low | Use America/Chicago explicitly; skip weekends/holidays for MVP |
| Evaluator Docker environment differs | Low | Medium | Pin all image versions; test on clean machine before submission |

---

## 17. Out of Scope (Production Considerations)

These items are documented in the decision log as production enhancements but are explicitly excluded from the MVP:

- **Event-driven architecture**: Kafka/Pub/Sub for state transitions (Apex runs both)
- **gRPC internal communication**: Type-safe service contracts (Apex uses gRPC)
- **Real authentication**: OAuth/JWT with correspondent identity providers
- **Horizontal scaling**: Multiple backend instances with distributed locking
- **Check image encryption**: At-rest and in-transit encryption for PCI-DSS
- **Weekend/holiday calendar**: EOD cutoff adjustments for non-business days
- **Multi-currency support**: Only USD for MVP
- **Notification service**: Real push notifications or email for status updates
- **Monitoring/alerting**: Prometheus metrics, Grafana dashboards, PagerDuty

---

## 18. Success Criteria

The project is considered successful when:

1. `docker compose up` starts the full stack and is demo-ready within 10 minutes of clone
2. Happy path completes end-to-end: submit → validate → analyze → approve → post → settle → complete
3. All 7 vendor stub scenarios produce correct downstream behavior via different account suffixes
4. No deposit reaches FundsPosted without passing both vendor validation and business rules
5. Reversal of a completed deposit creates correct ledger entries (original debit + $30 fee debit) and transitions to Returned
6. Settlement file contains exactly the approved deposits, excludes rejected/returned, and respects EOD cutoff
7. Operator can review, approve, and reject flagged deposits with all actions logged
8. All 10+ tests pass and a test report is generated in /reports
9. README, decision log, and architecture doc are complete and accurate

---

## 19. Appendices

### A. Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| DATABASE_URL | Yes | — | Postgres connection string |
| REDIS_URL | Yes | — | Redis connection string |
| SERVER_PORT | No | 8080 | Backend server port |
| EOD_CUTOFF_HOUR | No | 18 | Settlement cutoff hour |
| EOD_CUTOFF_MINUTE | No | 30 | Settlement cutoff minute |
| SETTLEMENT_OUTPUT_DIR | No | ./output/settlement | X9 file output path |
| RETURN_FEE_CENTS | No | 3000 | Return fee ($30.00) |
| VENDOR_STUB_MODE | No | deterministic | Stub mode |

### B. Seed Data

The system ships with seed data for demo:

- 3 correspondents (SoFi, Webull, CashApp) with omnibus accounts
- 10 investor accounts across correspondents, including retirement-type accounts
- Account IDs with suffixes matching vendor stub scenarios (e.g., ACC-SOFI-1001 triggers blur)

### C. Related Documents

| Document | Location | Purpose |
|----------|----------|---------|
| Pre-Search | /docs/pre-search.md | Constraint analysis and architecture discovery |
| Architecture | /docs/architecture.md | System diagram, service boundaries, data flow |
| Decision Log | /docs/decision_log.md | Key decisions with alternatives and rationale |
| Risks | /docs/risks.md | Risks and limitations note |
| API Contracts | .claude/rules/prompts.md | Full request/response schemas for all endpoints |
| Test Report | /reports/scenario-coverage.md | Test results and scenario coverage matrix |
