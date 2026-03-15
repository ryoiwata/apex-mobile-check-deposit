# Mobile Check Deposit System

A minimal end-to-end mobile check deposit system for brokerage accounts. Investors submit check images through a React UI, a stubbed Vendor Service validates images (IQA, MICR/OCR, duplicate detection), a Funding Service enforces business rules using a **collect-all validation approach** and posts to a ledger, operators review flagged deposits, and approved deposits settle via X9 ICL files to a Settlement Bank.

The collect-all approach evaluates every business rule regardless of prior failures and returns the full list of violations at once — preventing the frustrating loop where an investor fixes one issue, resubmits, and immediately hits a different rejection. Every non-terminal error path loops back to allow correction and resubmission.

Built for the Apex Fintech Services Week 4 technical assessment.

**Documentation:**
- [Architecture & Data Flows](docs/architecture.md) — service boundaries, state machine, data flow diagrams
- [Decision Log](docs/decision_log.md) — 20 key decisions with alternatives and trade-offs
- [Risks & Limitations](docs/risks.md) — known gaps and production path
- [Scenario Coverage](reports/scenario-coverage.md) — test results and rubric alignment
- [Submission](SUBMISSION.md) — summary, how to run, evaluation criteria

## Architecture

```
┌─────────────┐     ┌──────────────┐     ┌─────────────────┐     ┌──────────────┐
│  React UI   │────▶│  Gin Router  │────▶│ Vendor Service  │     │  PostgreSQL  │
│  (Vite)     │◀─ ─ │  + Middleware │     │  (Stub)         │     │              │
│             │ IQA │              │     │  IQA / MICR /   │     │  Transfers   │
│ • Deposit   │ fail│ • Auth       │     │  OCR / Dupe     │     │  Ledger      │
│   Form      │ loop│ • Rate Limit │     └────────┬────────┘     │  Audit Log   │
│ • Operator  │     │ • Routing    │              │              │  Accounts    │
│   Dashboard │     └──────┬───────┘              ▼              └──────┬───────┘
│ • Ledger    │            │              ┌─────────────────┐          │
│   View      │            ├─────────────▶│ Funding Service │◀─────────┘
└──────┬──────┘            │              │ (COLLECT-ALL)   │
       │                   │              │                 │   ┌──────────────┐
       │ ALL violations    │              │ ├ Deposit Limits│──▶│    Redis      │
       │ returned at once  │              │ ├ Contrib Caps  │   │              │
       │◀ ─ ─ ─ ─ ─ ─ ─ ─ ┤              │ ├ Dupe Detection│   │ • Rate Limits│
       │                   │              │ └ Acct Eligib.  │   │ • Check Hash │
       │                   │              └────────┬────────┘   │   Cache      │
       │                   │                       │            └──────────────┘
       │                   │                       ▼
       │                   │              ┌─────────────────┐
       │                   ├─────────────▶│ Ledger Service  │
       │                   │              │                 │
       │                   │              │ • Post Funds    │
       │                   │              │ • Reversals     │
       │                   │              │ • Fee Deduction │
       │                   │              └────────┬────────┘
       │                   │                       │
       │                   │                       ▼
       │                   │              ┌─────────────────┐
       │                   └─────────────▶│  Settlement     │
       │                                  │  Engine         │
       │◀ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ ─ │                 │
       return + fee notification          │ • EOD Batching  │
       (loop-back: new deposit)           │ • X9 ICL Gen    │
                                          │ • Bank ACK/Retry│
                                          └─────────────────┘

─── = data flow    ─ ─ = loop-back / error path
```

### Transfer State Machine

```
Requested ──▶ Validating ──▶ Analyzing ──▶ Approved ──▶ FundsPosted ──▶ Completed
                  │              │                                           │
                  ▼              ▼                                           ▼
               Rejected      Rejected                                    Returned
              (IQA fail,    (collect-all:                             (bounced check,
              duplicate)    all violations                           reversal + $30 fee)
                  │         returned at once)                             │
                  │              │                                        │
                  └──────────────┴──── Loop-back: investor may ──────────┘
                                       resubmit → new Requested
```

Deposits flagged by the Vendor Service (MICR failure, amount mismatch) enter `Analyzing` with a `flagged=true` marker and appear in the operator review queue. Operators can approve (→ Approved) or reject (→ Rejected). The collect-all approach ensures that when business rules fail, ALL violations are returned in a single response so the investor can fix everything at once. Every error path loops back — IQA failures prompt retake, rule failures allow resubmission, operator rejections allow new deposits, and returned checks allow resubmission with a different check.

### Data Flow

1. **Investor** submits check images + amount + account ID via React UI
2. **API Gateway** (Gin) validates input, checks rate limits, authenticates session — session failure loops back for re-authentication
3. **Vendor Service Stub** runs image quality assessment, MICR extraction, OCR, and duplicate detection — IQA failures (blur/glare) loop back to investor for retake with specific guidance
4. **Funding Service** validates session, resolves account identifiers, then applies **all business rules in parallel using collect-all approach**: deposit limits ($5,000 max), contribution caps, duplicate detection (Redis). If any rules fail, ALL violations returned at once — investor fixes everything and resubmits (loop-back)
5. **Ledger Service** creates transfer record (Type: MOVEMENT, SubType: DEPOSIT, TransferType: CHECK) with correct omnibus account mapping
6. **Operator** reviews flagged items via dashboard — can override contribution type, then approves or rejects with mandatory audit logging. Rejection notifies investor who may resubmit (loop-back). Approval proceeds to next queue item (queue cycling loop).
7. **Settlement Engine** checks EOD cutoff (6:30 PM CT) — late deposits roll to next business day (loop-back). Batches approved deposits, generates X9 ICL file, submits to bank. If bank doesn't acknowledge, retries submission (loop-back).
8. **Return handling** — bounced checks trigger reversal postings (original amount + $30 return fee), investor notified, may submit new deposit (loop-back)

## Tech Stack

| Layer | Technology | Rationale |
|-------|-----------|-----------|
| Backend | Go 1.22+ | Apex's primary language; concurrency primitives for async state transitions |
| HTTP Framework | Gin | Largest Go web framework community; strong middleware support |
| Database | PostgreSQL | FK constraints across transfers/ledger/audit; transactional guarantees for reversals |
| Cache | Redis | Duplicate check hash storage with TTL; rate limiting; Apex already runs Redis |
| Frontend | React (Vite) | Apex uses React; reactive updates for operator review queue |
| Settlement | moov-io/imagecashletter | Purpose-built Go library for X9 ICL format |
| Testing | go test + testify | Standard Go testing with assertion library |
| Infrastructure | Docker Compose | One-command setup for all services |

## Quick Start

### Prerequisites

- Docker and Docker Compose
- Go 1.22+ (for local development without Docker)
- Node.js 18+ (for frontend development without Docker)

### One-Command Setup

```bash
git clone https://github.com/<your-username>/mobile-check-deposit.git
cd mobile-check-deposit
cp .env.example .env
docker compose up --build
```

This starts the Go backend (`:8080`), React frontend (`:5173`), PostgreSQL (`:5432`), and Redis (`:6379`).

### Local Development (Without Docker)

**Backend:**
```bash
cd backend
cp .env.example .env
# Edit .env with your local Postgres and Redis URLs
go mod download
go run ./cmd/server
```

**Frontend:**
```bash
cd web
npm install
npm run dev
```

### Run Tests

```bash
# Unit tests
go test ./... -v

# Integration tests (requires running Postgres)
go test ./tests/ -v -tags=integration

# All tests with report
go test ./... -v 2>&1 | tee reports/test-results.txt
```

## Demo Walkthrough

### Happy Path

```bash
./scripts/demo-happy-path.sh
```

This runs the full lifecycle: submit check → vendor validates → funding rules pass → ledger posts → operator reviews → settlement file generated → deposit marked completed.

### All Vendor Scenarios

```bash
./scripts/demo-all-scenarios.sh
```

Exercises every vendor stub response by using different test account suffixes:

| Account Suffix | Scenario | Expected Result |
|---------------|----------|-----------------|
| `*1001` | IQA Fail (Blur) | Rejected — prompt retake |
| `*1002` | IQA Fail (Glare) | Rejected — prompt retake |
| `*1003` | MICR Read Failure | Flagged — enters operator review |
| `*1004` | Duplicate Detected | Rejected |
| `*1005` | Amount Mismatch | Flagged — enters operator review |
| `*1006` | Clean Pass | Approved — proceeds to posting |
| `*0000` | IQA Pass (basic) | Approved — proceeds to posting |

### Return/Reversal

```bash
./scripts/demo-return.sh
```

Simulates a bounced check after settlement: creates reversal postings (original amount debit + $30 fee debit), transitions transfer to `Returned` state.

### Settlement Generation

```bash
./scripts/trigger-settlement.sh
```

Triggers EOD batch processing, generates X9 ICL file for all deposits in `FundsPosted` state before the 6:30 PM CT cutoff.

## Project Structure

```
mobile-check-deposit/
├── backend/
│   ├── cmd/server/              # Entrypoint and dependency wiring
│   ├── internal/
│   │   ├── vendor/              # Vendor Service stub (IQA, MICR, OCR, dupe)
│   │   ├── funding/             # Business rules, account resolution, limits
│   │   ├── ledger/              # Transfer posting, reversals, fee deduction
│   │   ├── state/               # Transfer state machine with transition validation
│   │   ├── settlement/          # X9 ICL generation, EOD batching
│   │   ├── operator/            # Review queue, approve/reject, audit trail
│   │   ├── models/              # Shared domain types
│   │   ├── middleware/          # Auth, rate limiting
│   │   └── db/                  # Postgres/Redis setup, migrations
│   ├── go.mod
│   └── .env.example
├── web/                         # React frontend (Vite)
│   ├── src/
│   │   ├── api.js               # Single API client module
│   │   └── components/          # DepositForm, ReviewQueue, TransferStatus, LedgerView
│   └── package.json
├── scripts/                     # Demo and utility scripts
├── docs/
│   ├── architecture.md          # System diagram and service boundaries
│   ├── decision_log.md          # Key decisions and alternatives considered
│   └── risks.md                 # Risks and limitations
├── tests/                       # Integration tests
├── reports/                     # Test results and scenario coverage
├── docker-compose.yml
├── Makefile
├── .env.example
└── README.md
```

## Environment Variables

| Variable | Required | Default | Description |
|----------|----------|---------|-------------|
| `DATABASE_URL` | Yes | — | Postgres connection string |
| `REDIS_URL` | Yes | — | Redis connection string |
| `SERVER_PORT` | No | `8080` | Backend server port |
| `EOD_CUTOFF_HOUR` | No | `18` | Settlement cutoff hour (UTC-adjusted) |
| `EOD_CUTOFF_MINUTE` | No | `30` | Settlement cutoff minute |
| `SETTLEMENT_OUTPUT_DIR` | No | `./output/settlement` | Where X9 ICL files are written |
| `RETURN_FEE_CENTS` | No | `3000` | Return fee in cents ($30.00) |
| `VENDOR_STUB_MODE` | No | `deterministic` | Stub mode: `deterministic` or `random` |

## API Endpoints

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/api/v1/deposits` | Submit a new check deposit |
| `GET` | `/api/v1/deposits/:id` | Get transfer status and history |
| `GET` | `/api/v1/deposits` | List deposits with filters |
| `GET` | `/api/v1/operator/queue` | Get operator review queue |
| `POST` | `/api/v1/operator/deposits/:id/approve` | Approve a flagged deposit |
| `POST` | `/api/v1/operator/deposits/:id/reject` | Reject a flagged deposit |
| `POST` | `/api/v1/settlement/trigger` | Trigger EOD settlement batch |
| `POST` | `/api/v1/deposits/:id/return` | Simulate a check return |
| `GET` | `/api/v1/ledger/:account_id` | View account ledger entries |
| `GET` | `/api/v1/operator/audit` | View operator audit log |
| `GET` | `/health` | Health check (Postgres + Redis) |

Full request/response schemas are documented in `.claude/rules/prompts.md`.

## Key Design Decisions

| Decision | Choice | Alternative Considered | Rationale |
|----------|--------|----------------------|-----------|
| Language | Go | Java + Spring Boot | Apex is "mostly Golang"; lighter footprint; better concurrency model for this workload |
| Framework | Gin | Chi, Echo | Largest community; evaluators recognize it; middleware ecosystem |
| Database | PostgreSQL | SQLite | FK constraints, transactional guarantees for reversals, matches Apex's stack |
| Rule evaluation | Collect-all (parallel) | Fail-fast (stop at first) | Returns ALL violations at once; prevents frustrating fix-one-resubmit-hit-another loop |
| Error handling | Loop-back (every path) | Terminal errors | No dead ends; IQA→retake, rules→fix all, reject→resubmit, return→new deposit |
| Settlement format | moov-io/imagecashletter | Custom X9 parser | Purpose-built Go library; avoid reinventing a niche financial standard |
| Stub design | Account suffix mapping | Config file, request headers | Deterministic, no code changes needed, self-documenting in tests |
| Money representation | int64 cents | float64 | Eliminates floating-point rounding; standard practice in financial systems |
| State management | Explicit state machine | Direct DB updates | Enforces valid transitions; prevents impossible states; audit-friendly |

Full decision log with trade-off analysis: [`docs/decision_log.md`](docs/decision_log.md)

## Tests

The test suite covers all paths required by the evaluation rubric:

| # | Test Case | Category |
|---|-----------|----------|
| 1 | Happy path end-to-end | Core correctness |
| 2 | IQA Fail — Blur with retake loop-back | Vendor stub |
| 3 | IQA Fail — Glare with retake loop-back | Vendor stub |
| 4 | MICR Read Failure → operator review → approve/reject | Vendor stub |
| 5 | Duplicate Detected | Vendor stub |
| 6 | Amount Mismatch → flagged → operator override | Vendor stub |
| 7 | Deposit over $5,000 limit (collect-all) | Business rules |
| 8 | Invalid state transitions rejected | State machine |
| 9 | Reversal with $30 fee calculation | Return handling |
| 10 | Settlement file contains only approved deposits | Settlement |
| 11 | Collect-all: multiple simultaneous rule failures | Business rules |
| 12 | Settlement bank ACK retry loop | Settlement |
| 13 | EOD cutoff roll-over to next business day | Settlement |

Test results and scenario coverage report: [`reports/`](reports/)

## Risks and Limitations

- **Stubbed vendor integration only.** No real check image processing, MICR reading, or OCR. The stub simulates all validation scenarios deterministically via account suffix mapping.
- **No real authentication.** Session validation is simplified for the demo. Production would require OAuth/JWT integration with the correspondent's identity provider.
- **Single-node deployment.** No horizontal scaling, leader election for settlement batching, or distributed locking for concurrent state transitions. Production would need these for high availability.
- **No encryption at rest.** Settlement files and check images are stored unencrypted. Production would require encryption for PCI-DSS and banking regulatory compliance.
- **Synthetic data only.** No real PII, account numbers, routing numbers, or check images are used anywhere in the system.
- **No compliance or regulatory claims.** This is a technical demonstration, not a production-ready system. Real deployment would require compliance review, security audit, and regulatory approval.
- **EOD cutoff is simplified.** Does not account for bank holidays, weekends, or timezone edge cases beyond basic CT conversion.

Full risk assessment: [`docs/risks.md`](docs/risks.md)

## With One More Week, We Would

- Add WebSocket/SSE for real-time transfer status updates in the UI
- Implement Kafka-based event sourcing for state transitions (aligns with Apex's Pub/Sub + Kafka stack)
- Add gRPC for internal service communication (aligns with Apex's use of gRPC)
- Build comprehensive load testing with concurrent deposit submissions
- Add Prometheus metrics and Grafana dashboards for observability
- Implement proper JWT authentication with role-based access control
- Add database connection pooling tuning and query optimization
- Build a CI/CD pipeline with automated test runs and Docker image publishing

## How Should Apex Evaluate Production Readiness?

1. **State machine correctness** — Verify no deposit can reach `FundsPosted` without passing both vendor validation and funding service business rules. Attempt every invalid state transition and confirm rejection.
2. **Collect-all validation** — Submit a deposit that violates multiple business rules simultaneously (e.g., over $5,000 AND duplicate). Confirm ALL violations are returned in a single response, not just the first one found.
3. **Loop-back completeness** — Verify every non-terminal error provides a clear re-entry path: IQA failure → retake, rule failure → fix all and resubmit, operator rejection → new deposit, returned check → new deposit, session expiration → re-auth, bank non-ACK → retry.
4. **Ledger integrity** — Confirm every posted deposit has a matching ledger entry, every reversal creates exactly two entries (debit + fee), and the sum of all ledger entries for an account matches the reported balance.
5. **Settlement reconciliation** — Verify the X9 ICL file contains exactly the deposits that should be settled (no rejected, no duplicates, respects EOD cutoff) and that batch totals are mathematically correct. Confirm bank ACK retry works on non-acknowledgment.
6. **Stub coverage** — Confirm all 7 vendor response scenarios produce the correct downstream behavior and that switching between scenarios requires zero code changes.
7. **Audit completeness** — Verify every operator action (approve, reject, contribution type override) is logged with operator ID, timestamp, and the transfer's before/after state.
