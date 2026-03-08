# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project Overview

mobile-check-deposit is a minimal end-to-end mobile check deposit system for brokerage accounts. Investors submit check images via a React UI, a stubbed Vendor Service validates images (IQA, MICR, OCR, duplicate detection), a Funding Service enforces business rules and posts to a ledger, operators review flagged deposits, and approved deposits settle via X9 ICL files. Gauntlet AI Week 4 project for Apex Fintech Services.

**Stack:** Go 1.22+ · Gin · PostgreSQL · Redis · React (Vite) · Docker Compose · moov-io/imagecashletter

**Do not suggest switching frameworks or languages.** The stack is finalized and aligned with Apex's production environment (Go, Postgres, Redis, React, AWS/GCP).

## Commands

### Full Stack (one command)
```bash
docker compose up --build    # Starts Go backend, Postgres, Redis, React frontend
```

### Backend Only
```bash
cd backend
cp .env.example .env         # Configure environment
go mod download
go run ./cmd/server           # Run dev server on :8080
go test ./... -v              # Run all tests
go test ./... -v -count=1     # Run tests without cache
```

### Frontend Only
```bash
cd web
npm install
npm run dev                   # http://localhost:5173
```

### Demo Scripts
```bash
./scripts/demo-happy-path.sh       # Full happy path end-to-end
./scripts/demo-all-scenarios.sh    # Exercise all vendor stub responses
./scripts/demo-return.sh           # Return/reversal flow
./scripts/trigger-settlement.sh    # Generate EOD settlement file
```

## Environment Variables

`backend/.env` (gitignored):
```
# Database
DATABASE_URL=postgres://mcd:mcd@localhost:5432/mcd?sslmode=disable

# Redis
REDIS_URL=redis://localhost:6379

# Server
SERVER_PORT=8080
EOD_CUTOFF_HOUR=18        # 6 PM (represents 6:30 PM CT in UTC-adjusted form)
EOD_CUTOFF_MINUTE=30

# Settlement
SETTLEMENT_OUTPUT_DIR=./output/settlement
RETURN_FEE_CENTS=3000     # $30.00 hardcoded for MVP

# Vendor Stub
VENDOR_STUB_MODE=deterministic   # "deterministic" or "random"
```

Fail fast at startup if `DATABASE_URL` or `REDIS_URL` is missing. Never log secret values.

## Project Structure

```
mobile-check-deposit/
├── backend/
│   ├── cmd/
│   │   └── server/
│   │       └── main.go              # Entrypoint, DI wiring, server startup
│   ├── internal/
│   │   ├── vendor/                  # Vendor Service stub (IQA, MICR, OCR, dupe)
│   │   │   ├── stub.go             # Configurable stub with deterministic responses
│   │   │   ├── models.go           # Vendor request/response types
│   │   │   └── handler.go          # HTTP handlers for vendor endpoints
│   │   ├── funding/                 # Funding Service middleware
│   │   │   ├── service.go          # Business rules engine
│   │   │   ├── rules.go            # Deposit limits, contribution types, dupe detection
│   │   │   ├── accounts.go         # Account resolver (investor → internal IDs)
│   │   │   └── handler.go          # HTTP handlers
│   │   ├── ledger/                  # Ledger posting and reversals
│   │   │   ├── service.go          # Create transfers, post funds, reverse
│   │   │   ├── models.go           # Transfer, LedgerEntry types
│   │   │   └── repository.go       # Postgres queries
│   │   ├── state/                   # Transfer state machine
│   │   │   ├── machine.go          # State transitions with validation
│   │   │   └── states.go           # State enum and transition table
│   │   ├── settlement/             # X9 ICL settlement file generation
│   │   │   ├── service.go          # Batch processing, EOD cutoff logic
│   │   │   ├── generator.go        # moov-io/imagecashletter integration
│   │   │   └── handler.go          # Settlement trigger endpoint
│   │   ├── operator/               # Operator review workflow
│   │   │   ├── service.go          # Review queue, approve/reject logic
│   │   │   ├── handler.go          # HTTP handlers for operator UI
│   │   │   └── audit.go            # Audit logging
│   │   ├── models/                  # Shared domain types
│   │   │   ├── transfer.go         # Transfer struct with all fields
│   │   │   ├── account.go          # Account, Correspondent types
│   │   │   └── errors.go           # Domain error types
│   │   ├── middleware/              # Gin middleware
│   │   │   ├── auth.go             # Session validation
│   │   │   └── ratelimit.go        # Redis-backed rate limiting
│   │   └── db/                      # Database setup and migrations
│   │       ├── postgres.go         # Connection pool setup
│   │       ├── redis.go            # Redis client setup
│   │       └── migrations/         # SQL migration files
│   ├── go.mod
│   ├── go.sum
│   └── .env.example
├── web/                             # React frontend (Vite)
│   ├── src/
│   │   ├── App.jsx
│   │   ├── api.js                  # Single API module for all backend calls
│   │   ├── components/
│   │   │   ├── DepositForm.jsx     # Check submission simulator
│   │   │   ├── ReviewQueue.jsx     # Operator review dashboard
│   │   │   ├── TransferStatus.jsx  # Transfer state tracker
│   │   │   └── LedgerView.jsx      # Account balances and postings
│   │   └── hooks/
│   └── package.json
├── scripts/                         # Demo and utility scripts
├── docs/
│   ├── architecture.md             # System diagram, service boundaries, data flow
│   ├── decision_log.md             # Key decisions and alternatives
│   └── risks.md                    # Risks and limitations
├── tests/                           # Integration tests
├── reports/                         # Test results and scenario coverage
├── docker-compose.yml
├── Makefile
├── .env.example
└── README.md
```

## Architecture

### Service Boundaries

1. **API Layer (Gin)** — REST endpoints, auth middleware, rate limiting, request validation
2. **Vendor Service Stub** — simulates external check validation (IQA, MICR, OCR, duplicate detection)
3. **Funding Service** — business rules engine, account resolution, deposit limit enforcement
4. **Ledger Service** — transfer record creation, fund posting, reversal with fee deduction
5. **State Machine** — manages transfer lifecycle with validated transitions
6. **Settlement Engine** — batches approved deposits, generates X9 ICL files via moov-io
7. **Operator Service** — review queue, approve/reject workflow, audit trail

### Transfer State Machine

Valid transitions (enforce these strictly — reject any invalid transition):

```
Requested    → Validating                    (deposit submitted, sent to vendor)
Validating   → Analyzing                     (vendor passed, send to funding)
Validating   → Rejected                      (vendor failed: IQA, duplicate)
Validating   → Analyzing [flagged=true]      (vendor flagged: MICR failure, amount mismatch)
Analyzing    → Approved                      (business rules passed)
Analyzing    → Rejected                      (business rules failed: over limit, ineligible)
Approved     → FundsPosted                   (provisional credit posted to ledger)
FundsPosted  → Completed                     (settlement confirmed by bank)
Completed    → Returned                      (check bounced post-settlement)
Approved     → Rejected                      (operator rejected during review)
```

**No other transitions are valid.** The state machine must reject attempts to skip states or move backwards (except Completed → Returned for bounced checks).

### Vendor Stub Response Mapping

Deterministic responses keyed by **test account number suffix**:

| Account Suffix | Scenario          | Result   | Next State |
|---------------|-------------------|----------|------------|
| `*1001`       | IQA Fail (Blur)   | FAIL     | Rejected (prompt retake) |
| `*1002`       | IQA Fail (Glare)  | FAIL     | Rejected (prompt retake) |
| `*1003`       | MICR Read Failure | FLAGGED  | Analyzing (manual review) |
| `*1004`       | Duplicate Detected| FAIL     | Rejected |
| `*1005`       | Amount Mismatch   | FLAGGED  | Analyzing (manual review) |
| `*1006`       | Clean Pass        | PASS     | Analyzing |
| `*0000`       | IQA Pass (basic)  | PASS     | Analyzing |
| Any other     | Clean Pass        | PASS     | Analyzing |

### Ledger Record Schema

Every posted deposit creates a transfer with these fields:
```
ToAccountId:         <investor account ID>
FromAccountId:       <omnibus account for correspondent>  (looked up from client config)
Type:                MOVEMENT
Memo:                FREE
SubType:             DEPOSIT
TransferType:        CHECK
Currency:            USD
Amount:              <validated deposit amount in cents>
SourceApplicationId: <TransferID>
```

### Business Rules (Funding Service)

- Deposit amount limit: reject if > $5,000 (500000 cents)
- Contribution type: default to INDIVIDUAL for retirement-type accounts
- Duplicate detection: check Redis for matching check hash (routing + account + amount + serial) within 90-day TTL
- Account eligibility: verify account exists and is in good standing

### Settlement

- Use `moov-io/imagecashletter` for X9 ICL file generation
- EOD cutoff: 6:30 PM CT — deposits submitted after cutoff roll to next business day
- Batch all deposits in `FundsPosted` state into a single settlement file
- Track bank acknowledgment per batch

### Return/Reversal

- Return fee: $30.00 (3000 cents) hardcoded for MVP
- Reversal creates two ledger entries: debit investor for original amount, debit investor for return fee
- Transfer transitions: Completed → Returned

## Key Domain Knowledge

- **Correspondent** = broker-dealer client (SoFi, Webull, etc.) that uses the platform
- **Omnibus account** = correspondent's pooled account that holds funds for all their investors
- **MICR** = Magnetic Ink Character Recognition — the machine-readable line at the bottom of checks containing routing number, account number, check serial
- **IQA** = Image Quality Assessment — checks for blur, glare, truncation in captured images
- **X9 ICL** = industry standard file format for check image exchange between banks (ANSI X9.100-187)
- **EOD** = End of Day processing cutoff

## Key Decisions

- **Go over Java** — Apex's platform is "mostly Golang"; Go's concurrency model suits async state transitions and batch settlement; smaller deployment footprint for Docker
- **Gin over Chi/Echo** — largest community, well-documented middleware support, evaluators will recognize it
- **PostgreSQL over SQLite** — Apex runs Postgres; need FK constraints across transfers/ledger/audit; transactional guarantees for reversal + fee deduction
- **Redis for dupe detection** — store check hashes with 90-day TTL; also used for rate limiting; Apex already runs Redis
- **moov-io/imagecashletter** — purpose-built Go library for X9 ICL; writing a custom parser is wasted effort
- **Deterministic stub via account suffix** — no code changes needed to test different scenarios; documented and reproducible
- **Amounts in cents (int64)** — avoid floating point rounding in financial calculations; $5,000 limit = 500000 cents

## Testing

- Framework: `go test` with `testify` for assertions
- Minimum 10 tests required by rubric (aim for 15+)
- See `.claude/rules/testing.md` for test strategy and required cases
- Run: `go test ./... -v`

## Git Workflow

### Conventional Commits

```
<type>(scope): <description>
```

**Types:** feat, fix, test, docs, refactor, chore, style, perf

**Scopes:** vendor, funding, ledger, state, settlement, operator, api, middleware, models, db, web, scripts, docs

**Rules:**
- Lowercase type and description. No period at end.
- Imperative mood: "add", "fix", "update" — not "added", "fixes", "updated".
- Keep the first line under 72 characters.

**Examples:**
```
feat(vendor): add configurable stub with deterministic responses
feat(state): implement transfer state machine with transition validation
feat(funding): add deposit limit and duplicate detection rules
feat(ledger): add reversal posting with return fee deduction
feat(settlement): add X9 ICL generation via moov-io/imagecashletter
feat(operator): add review queue with approve/reject and audit logging
feat(api): add deposit submission and status tracking endpoints
feat(web): add operator review dashboard with image display
test(state): add valid and invalid state transition tests
test(funding): add deposit limit and contribution type tests
chore: add docker-compose with postgres, redis, go backend
docs: add architecture diagram and decision log
```

### Commit Cadence

One logical unit of work = one commit. Don't batch unrelated changes. Don't commit half-finished features.

## Rules

- Read `.claude/rules/` for code style, security, testing, and domain knowledge
- Never commit `.env` files or secrets
- All amounts are in cents (int64), never float
- All times are UTC internally; convert to CT only for EOD cutoff display
- Use `context.Context` on all database calls and service methods
- Wrap external calls in error handling with meaningful messages including the operation that failed
- Synthetic data only — no real PII, account numbers, or check images
- Every state transition must go through the state machine — never update transfer status directly in the DB
