# Implementation Plan: Mobile Check Deposit System

**Generated:** 2026-03-08
**Status:** Ready to build (v2 — reviewed)
**Build order:** Phases 1–18, each has file paths, signatures, data structures, and acceptance criteria

---

## Architectural Decisions (Locked)

| Decision | Choice | Notes |
|---|---|---|
| Pipeline | Fully synchronous | POST /deposits runs Requested→FundsPosted in one request. Settlement to Completed is a separate EOD batch. |
| Vendor stub | In-process function | `VendorService` interface with `Stub` implementation. Swap for HTTP client in production. |
| State machine locking | `WHERE status = expected`, check rows affected | 0 rows = conflict → return ErrInvalidStateTransition |
| Transition logging | `machine.Transition()` logs atomically | Takes `*sql.Tx`, writes UPDATE + INSERT to state_transitions in same tx |
| Operator approve | Auto-posts in same tx | Analyzing→Approved→FundsPosted + ledger POST + audit log in one Postgres transaction |
| Contribution type | In `transfers` table | Set during Analyzing by funding rules (retirement→INDIVIDUAL). Operator can override on approve. |
| Reversal entries | investor→omnibus for both | Entry 1: SubType=REVERSAL, amount=original. Entry 2: SubType=RETURN_FEE, amount=3000 |
| EOD cutoff basis | `created_at <= cutoff` | Submission time, not processing time. Avoids race condition. |
| X9 images | Read real uploaded bytes | settlement/generator reads from /data/images/{transfer_id}/ |
| Auth | Token validates role, account_id from body | One investor token for all 7 scenarios. Operator token for OP-001. |
| Migrations | Auto-run at startup | `RunMigrations()` in main.go. Idempotent `CREATE TABLE IF NOT EXISTS`. |
| UI polling | setInterval | Queue: 5s. Transfer status: 2s while non-terminal. |
| Settlement trigger | Manual API only | POST /api/v1/settlement/trigger. No background cron. |
| Image storage | Named volumes, /data/images | Docker volume: mcd-images. Created at startup. |
| Module name | `github.com/apex/mcd` | Short for clean imports. |
| Seed data | 3 correspondents, 8 accounts | See Phase 2 for exact IDs. |

---

## Phase 1: Project Scaffold

**Day 1 — 1–2 hours**
**Goal:** Repo compiles, `docker compose up` starts all containers, health endpoint returns 200.

### 1.1 Directory Structure

```
apex-mobile-check-deposit/
├── backend/
│   ├── cmd/server/main.go
│   ├── internal/
│   │   ├── db/
│   │   │   ├── postgres.go
│   │   │   ├── redis.go
│   │   │   ├── migrations.go
│   │   │   └── migrations/
│   │   │       ├── 001_create_tables.sql
│   │   │       └── 002_seed_data.sql
│   │   ├── models/
│   │   │   ├── transfer.go
│   │   │   ├── account.go
│   │   │   └── errors.go
│   │   ├── state/
│   │   │   ├── states.go
│   │   │   └── machine.go
│   │   ├── vendor/
│   │   │   ├── models.go
│   │   │   └── stub.go
│   │   ├── funding/
│   │   │   ├── service.go
│   │   │   ├── rules.go
│   │   │   └── accounts.go
│   │   ├── ledger/
│   │   │   ├── models.go
│   │   │   ├── repository.go
│   │   │   └── service.go
│   │   ├── deposit/
│   │   │   ├── handler.go
│   │   │   └── service.go
│   │   ├── operator/
│   │   │   ├── service.go
│   │   │   ├── handler.go
│   │   │   └── audit.go
│   │   ├── settlement/
│   │   │   ├── service.go
│   │   │   ├── generator.go
│   │   │   └── handler.go
│   │   └── middleware/
│   │       ├── auth.go
│   │       └── ratelimit.go
│   ├── go.mod
│   ├── go.sum
│   └── .env.example
├── web/
│   ├── src/
│   │   ├── App.jsx
│   │   ├── api.js
│   │   └── components/
│   │       ├── DepositForm.jsx
│   │       ├── TransferStatus.jsx
│   │       ├── ReviewQueue.jsx
│   │       └── LedgerView.jsx
│   ├── package.json
│   └── vite.config.js
├── scripts/
│   ├── fixtures/
│   │   ├── check-front.png      # placeholder check image
│   │   └── check-back.png       # placeholder check image
│   ├── demo-happy-path.sh
│   ├── demo-all-scenarios.sh
│   ├── demo-return.sh
│   └── trigger-settlement.sh
├── docs/
├── tests/
│   └── integration_test.go
├── reports/
├── docker-compose.yml
├── Makefile
└── .env.example
```

### 1.2 `backend/go.mod`

```go
module github.com/apex/mcd

go 1.22

require (
    github.com/gin-gonic/gin v1.9.1
    github.com/lib/pq v1.10.9
    github.com/redis/go-redis/v9 v9.5.1
    github.com/moov-io/imagecashletter v0.10.0
    github.com/google/uuid v1.6.0
    github.com/stretchr/testify v1.9.0
)
```

### 1.3 `docker-compose.yml`

```yaml
version: "3.9"

services:
  postgres:
    image: postgres:16-alpine
    environment:
      POSTGRES_USER: mcd
      POSTGRES_PASSWORD: mcd
      POSTGRES_DB: mcd
    volumes:
      - postgres-data:/var/lib/postgresql/data
    healthcheck:
      test: ["CMD-SHELL", "pg_isready -U mcd"]
      interval: 5s
      timeout: 5s
      retries: 5

  redis:
    image: redis:7-alpine
    volumes:
      - redis-data:/data
    healthcheck:
      test: ["CMD", "redis-cli", "ping"]
      interval: 5s
      timeout: 5s
      retries: 5

  backend:
    build:
      context: ./backend
      dockerfile: Dockerfile
    ports:
      - "8080:8080"
    env_file:
      - ./backend/.env
    volumes:
      - image-data:/data/images
      - settlement-data:/output/settlement
    depends_on:
      postgres:
        condition: service_healthy
      redis:
        condition: service_healthy

  frontend:
    build:
      context: ./web
      dockerfile: Dockerfile
    ports:
      - "5173:5173"
    environment:
      - VITE_API_URL=http://localhost:8080

volumes:
  postgres-data:
  redis-data:
  image-data:
  settlement-data:
```

### 1.4 `backend/.env.example`

```
DATABASE_URL=postgres://mcd:mcd@postgres:5432/mcd?sslmode=disable
REDIS_URL=redis://redis:6379

SERVER_PORT=8080
EOD_CUTOFF_HOUR=18
EOD_CUTOFF_MINUTE=30

IMAGE_STORAGE_DIR=/data/images
SETTLEMENT_OUTPUT_DIR=/output/settlement
RETURN_FEE_CENTS=3000

VENDOR_STUB_MODE=deterministic

INVESTOR_TOKEN=tok_investor_test
OPERATOR_TOKEN=tok_operator_test
```

### 1.5 `Makefile`

```makefile
.PHONY: up down test lint

up:
	docker compose up --build

down:
	docker compose down -v

test:
	cd backend && go test ./... -v

test-integration:
	cd backend && go test ./tests/ -v -tags=integration

lint:
	cd backend && go vet ./...

demo-happy-path:
	./scripts/demo-happy-path.sh

demo-all:
	./scripts/demo-all-scenarios.sh
```

**Acceptance criteria:**
- `go mod download` succeeds without errors
- `docker compose up --build` starts all 4 containers
- `curl http://localhost:8080/health` returns `{"status":"ok"}` (stub at this stage)

---

## Phase 2: Database Schema & Migrations

**Day 1 — 2–3 hours**
**Goal:** All tables created, seed data inserted, `RunMigrations()` is idempotent.

### 2.1 `internal/db/migrations/001_create_tables.sql`

```sql
-- Track executed migrations
CREATE TABLE IF NOT EXISTS schema_migrations (
    version VARCHAR(50) PRIMARY KEY,
    applied_at TIMESTAMPTZ DEFAULT NOW()
);

-- Correspondents (broker-dealers)
CREATE TABLE IF NOT EXISTS correspondents (
    id           VARCHAR(50) PRIMARY KEY,
    name         VARCHAR(100) NOT NULL,
    omnibus_account_id VARCHAR(50) NOT NULL,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

-- Investor accounts
CREATE TABLE IF NOT EXISTS accounts (
    id               VARCHAR(50) PRIMARY KEY,
    correspondent_id VARCHAR(50) NOT NULL REFERENCES correspondents(id),
    account_type     VARCHAR(20) NOT NULL CHECK (account_type IN ('individual','retirement','joint')),
    status           VARCHAR(20) NOT NULL DEFAULT 'active' CHECK (status IN ('active','suspended','closed')),
    created_at       TIMESTAMPTZ DEFAULT NOW()
);

-- Transfers: central entity
CREATE TABLE IF NOT EXISTS transfers (
    id                     UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    account_id             VARCHAR(50) NOT NULL,
    amount_cents           BIGINT NOT NULL CHECK (amount_cents > 0),
    declared_amount_cents  BIGINT NOT NULL CHECK (declared_amount_cents > 0),
    status                 VARCHAR(20) NOT NULL DEFAULT 'requested'
                               CHECK (status IN (
                                   'requested','validating','analyzing','approved',
                                   'funds_posted','completed','rejected','returned'
                               )),
    flagged                BOOLEAN NOT NULL DEFAULT FALSE,
    flag_reason            VARCHAR(100),
    contribution_type      VARCHAR(20),
    vendor_transaction_id  VARCHAR(100),
    micr_routing           VARCHAR(9),
    micr_account           VARCHAR(20),
    micr_serial            VARCHAR(20),
    micr_confidence        DECIMAL(3,2),
    ocr_amount_cents       BIGINT,
    front_image_ref        VARCHAR(500),
    back_image_ref         VARCHAR(500),
    settlement_batch_id    UUID,
    return_reason          VARCHAR(100),
    created_at             TIMESTAMPTZ DEFAULT NOW(),
    updated_at             TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_transfers_status     ON transfers(status);
CREATE INDEX IF NOT EXISTS idx_transfers_account    ON transfers(account_id);
CREATE INDEX IF NOT EXISTS idx_transfers_created    ON transfers(created_at);
CREATE INDEX IF NOT EXISTS idx_transfers_batch      ON transfers(settlement_batch_id);

-- Ledger entries: append-only
CREATE TABLE IF NOT EXISTS ledger_entries (
    id                    UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id           UUID NOT NULL REFERENCES transfers(id),
    to_account_id         VARCHAR(50) NOT NULL,
    from_account_id       VARCHAR(50) NOT NULL,
    type                  VARCHAR(20) NOT NULL DEFAULT 'MOVEMENT',
    sub_type              VARCHAR(20) NOT NULL,
    transfer_type         VARCHAR(20) NOT NULL DEFAULT 'CHECK',
    currency              VARCHAR(3) NOT NULL DEFAULT 'USD',
    amount_cents          BIGINT NOT NULL CHECK (amount_cents > 0),
    memo                  VARCHAR(50) DEFAULT 'FREE',
    source_application_id UUID,
    created_at            TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_ledger_transfer   ON ledger_entries(transfer_id);
CREATE INDEX IF NOT EXISTS idx_ledger_to_account ON ledger_entries(to_account_id);

-- State transition audit trail
CREATE TABLE IF NOT EXISTS state_transitions (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    transfer_id  UUID NOT NULL REFERENCES transfers(id),
    from_state   VARCHAR(20) NOT NULL,
    to_state     VARCHAR(20) NOT NULL,
    triggered_by VARCHAR(50),
    metadata     JSONB,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_state_transfer ON state_transitions(transfer_id);

-- Operator audit log
CREATE TABLE IF NOT EXISTS audit_logs (
    id           UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    operator_id  VARCHAR(50) NOT NULL,
    action       VARCHAR(20) NOT NULL CHECK (action IN ('approve','reject','override')),
    transfer_id  UUID NOT NULL REFERENCES transfers(id),
    notes        TEXT,
    metadata     JSONB,
    created_at   TIMESTAMPTZ DEFAULT NOW()
);

CREATE INDEX IF NOT EXISTS idx_audit_transfer ON audit_logs(transfer_id);
CREATE INDEX IF NOT EXISTS idx_audit_operator ON audit_logs(operator_id);

-- Settlement batches
CREATE TABLE IF NOT EXISTS settlement_batches (
    id                  UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    batch_date          DATE NOT NULL,
    file_path           VARCHAR(500),
    deposit_count       INTEGER NOT NULL DEFAULT 0,
    total_amount_cents  BIGINT NOT NULL DEFAULT 0,
    status              VARCHAR(20) NOT NULL DEFAULT 'pending'
                            CHECK (status IN ('pending','submitted','acknowledged')),
    bank_reference      VARCHAR(100),
    created_at          TIMESTAMPTZ DEFAULT NOW()
);
```

### 2.2 `internal/db/migrations/002_seed_data.sql`

```sql
-- Correspondents
INSERT INTO correspondents (id, name, omnibus_account_id) VALUES
    ('CORR-SOFI',  'SoFi',    'OMNI-SOFI-001'),
    ('CORR-WBL',   'Webull',  'OMNI-WBL-001'),
    ('CORR-CASH',  'CashApp', 'OMNI-CASH-001')
ON CONFLICT (id) DO NOTHING;

-- Investor accounts — suffixes map to vendor stub scenarios
INSERT INTO accounts (id, correspondent_id, account_type, status) VALUES
    ('ACC-SOFI-1001', 'CORR-SOFI', 'individual', 'active'),  -- IQA blur
    ('ACC-SOFI-1002', 'CORR-SOFI', 'individual', 'active'),  -- IQA glare
    ('ACC-SOFI-1003', 'CORR-SOFI', 'individual', 'active'),  -- MICR failure
    ('ACC-SOFI-1004', 'CORR-SOFI', 'individual', 'active'),  -- duplicate
    ('ACC-SOFI-1005', 'CORR-SOFI', 'individual', 'active'),  -- amount mismatch
    ('ACC-SOFI-1006', 'CORR-SOFI', 'individual', 'active'),  -- clean pass
    ('ACC-SOFI-0000', 'CORR-SOFI', 'individual', 'active'),  -- basic pass
    ('ACC-RETIRE-001','CORR-WBL',  'retirement', 'active')   -- contribution type test
ON CONFLICT (id) DO NOTHING;
```

### 2.3 `internal/db/migrations.go`

```go
package db

import (
    "database/sql"
    "embed"
    "fmt"
    "sort"
    "strings"
)

//go:embed migrations/*.sql
var migrationFiles embed.FS

// RunMigrations executes all unapplied migration files in order.
// Creates schema_migrations table if needed. Idempotent.
func RunMigrations(db *sql.DB) error {
    // Ensure migrations tracking table exists
    _, err := db.Exec(`CREATE TABLE IF NOT EXISTS schema_migrations (
        version VARCHAR(50) PRIMARY KEY,
        applied_at TIMESTAMPTZ DEFAULT NOW()
    )`)
    if err != nil {
        return fmt.Errorf("migrations: creating tracking table: %w", err)
    }

    // Read all .sql files from embedded FS
    entries, err := migrationFiles.ReadDir("migrations")
    // ... sort by name, execute each if not already applied
    // INSERT INTO schema_migrations (version) VALUES ($1) ON CONFLICT DO NOTHING
}
```

**Key function signature:**
```go
func RunMigrations(db *sql.DB) error
func Connect(databaseURL string) (*sql.DB, error)  // internal/db/postgres.go
func NewRedisClient(redisURL string) (*redis.Client, error)  // internal/db/redis.go
```

**Acceptance criteria:**
- `docker compose up` runs migrations automatically
- `docker compose down -v && docker compose up` re-runs migrations cleanly
- All 8 accounts and 3 correspondents present in DB after startup

---

## Phase 3: Models

**Day 1 — 1 hour**
**Goal:** All shared domain types defined. Zero business logic here.

### 3.1 `internal/models/transfer.go`

```go
package models

import (
    "time"
    "github.com/google/uuid"
)

// TransferStatus represents the state machine states.
// Use state package constants; this type lives in models to avoid import cycles.
type TransferStatus string

const (
    StatusRequested   TransferStatus = "requested"
    StatusValidating  TransferStatus = "validating"
    StatusAnalyzing   TransferStatus = "analyzing"
    StatusApproved    TransferStatus = "approved"
    StatusFundsPosted TransferStatus = "funds_posted"
    StatusCompleted   TransferStatus = "completed"
    StatusRejected    TransferStatus = "rejected"
    StatusReturned    TransferStatus = "returned"
)

// Transfer is the central domain entity. Maps 1:1 to the transfers table.
type Transfer struct {
    ID                   uuid.UUID      `json:"transfer_id" db:"id"`
    AccountID            string         `json:"account_id" db:"account_id"`
    AmountCents          int64          `json:"amount_cents" db:"amount_cents"`
    DeclaredAmountCents  int64          `json:"declared_amount_cents" db:"declared_amount_cents"`
    Status               TransferStatus `json:"status" db:"status"`
    Flagged              bool           `json:"flagged" db:"flagged"`
    FlagReason           *string        `json:"flag_reason,omitempty" db:"flag_reason"`
    ContributionType     *string        `json:"contribution_type,omitempty" db:"contribution_type"`
    VendorTransactionID  *string        `json:"vendor_transaction_id,omitempty" db:"vendor_transaction_id"`
    MICRRouting          *string        `json:"micr_routing,omitempty" db:"micr_routing"`
    MICRAccount          *string        `json:"micr_account,omitempty" db:"micr_account"`
    MICRSerial           *string        `json:"micr_serial,omitempty" db:"micr_serial"`
    MICRConfidence       *float64       `json:"micr_confidence,omitempty" db:"micr_confidence"`
    OCRAmountCents       *int64         `json:"ocr_amount_cents,omitempty" db:"ocr_amount_cents"`
    FrontImageRef        *string        `json:"front_image_ref,omitempty" db:"front_image_ref"`
    BackImageRef         *string        `json:"back_image_ref,omitempty" db:"back_image_ref"`
    SettlementBatchID    *uuid.UUID     `json:"settlement_batch_id,omitempty" db:"settlement_batch_id"`
    ReturnReason         *string        `json:"return_reason,omitempty" db:"return_reason"`
    CreatedAt            time.Time      `json:"created_at" db:"created_at"`
    UpdatedAt            time.Time      `json:"updated_at" db:"updated_at"`
}

// StateTransition is an entry in the state_transitions audit table.
type StateTransition struct {
    ID          uuid.UUID      `json:"id"`
    TransferID  uuid.UUID      `json:"transfer_id"`
    FromState   TransferStatus `json:"from_state"`
    ToState     TransferStatus `json:"to_state"`
    TriggeredBy string         `json:"triggered_by"`
    Metadata    map[string]any `json:"metadata,omitempty"`
    CreatedAt   time.Time      `json:"created_at"`
}
```

### 3.2 `internal/models/account.go`

```go
package models

import "time"

type Account struct {
    ID               string    `json:"id" db:"id"`
    CorrespondentID  string    `json:"correspondent_id" db:"correspondent_id"`
    AccountType      string    `json:"account_type" db:"account_type"`
    Status           string    `json:"status" db:"status"`
    CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

type Correspondent struct {
    ID               string    `json:"id" db:"id"`
    Name             string    `json:"name" db:"name"`
    OmnibusAccountID string    `json:"omnibus_account_id" db:"omnibus_account_id"`
    CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// AccountWithCorrespondent is used by funding service account resolution.
type AccountWithCorrespondent struct {
    Account
    OmnibusAccountID string `db:"omnibus_account_id"`
}
```

### 3.3 `internal/models/errors.go`

```go
package models

import "errors"

var (
    ErrInvalidStateTransition = errors.New("invalid state transition")
    ErrTransferNotFound       = errors.New("transfer not found")
    ErrAccountNotFound        = errors.New("account not found")
    ErrAccountIneligible      = errors.New("account not eligible for deposits")
    ErrDepositOverLimit       = errors.New("deposit amount exceeds maximum limit")
    ErrDuplicateDeposit       = errors.New("duplicate deposit detected")
    ErrTransferNotReturnable  = errors.New("transfer must be in completed state to be returned")
    ErrTransferNotReviewable  = errors.New("transfer must be flagged and in analyzing state")
)
```

**Acceptance criteria:**
- `go build ./...` succeeds with all model types defined
- No circular imports between packages

---

## Phase 4: State Machine

**Day 1–2 — 2 hours**
**Goal:** All valid transitions enforced. Optimistic locking. Every transition auto-logs.

### 4.1 `internal/state/states.go`

```go
package state

import "github.com/apex/mcd/internal/models"

// Allowed defines every valid from→to pair.
// Any combination not listed here returns ErrInvalidStateTransition.
var Allowed = map[models.TransferStatus][]models.TransferStatus{
    models.StatusRequested:   {models.StatusValidating},
    models.StatusValidating:  {models.StatusAnalyzing, models.StatusRejected},
    models.StatusAnalyzing:   {models.StatusApproved, models.StatusRejected},
    models.StatusApproved:    {models.StatusFundsPosted, models.StatusRejected},
    models.StatusFundsPosted: {models.StatusCompleted},
    models.StatusCompleted:   {models.StatusReturned},
    // Terminal states: rejected, returned — no outgoing transitions
}

// IsTerminal returns true for states with no valid outgoing transitions.
func IsTerminal(s models.TransferStatus) bool {
    tos, ok := Allowed[s]
    return !ok || len(tos) == 0
}

// IsValid checks whether from→to is in the allowed table.
func IsValid(from, to models.TransferStatus) bool {
    tos, ok := Allowed[from]
    if !ok {
        return false
    }
    for _, t := range tos {
        if t == to {
            return true
        }
    }
    return false
}
```

### 4.2 `internal/state/machine.go`

```go
package state

import (
    "context"
    "database/sql"
    "encoding/json"
    "fmt"

    "github.com/apex/mcd/internal/models"
    "github.com/google/uuid"
)

// Machine manages transfer state transitions with optimistic locking.
type Machine struct {
    db *sql.DB
}

func New(db *sql.DB) *Machine {
    return &Machine{db: db}
}

// Transition validates and applies a state change within the provided transaction.
// Writes to both transfers (status) and state_transitions (audit) atomically.
// Returns ErrInvalidStateTransition if the from→to pair is not allowed,
// or if another goroutine already changed the status (0 rows affected).
func (m *Machine) Transition(
    ctx context.Context,
    tx *sql.Tx,
    id uuid.UUID,
    from, to models.TransferStatus,
    triggeredBy string,
    metadata map[string]any,
) error {
    if !IsValid(from, to) {
        return fmt.Errorf("%w: %s → %s", models.ErrInvalidStateTransition, from, to)
    }

    result, err := tx.ExecContext(ctx, `
        UPDATE transfers
        SET status = $1, updated_at = NOW()
        WHERE id = $2 AND status = $3`,
        string(to), id, string(from))
    if err != nil {
        return fmt.Errorf("state: updating transfer %s: %w", id, err)
    }

    rows, err := result.RowsAffected()
    if err != nil {
        return fmt.Errorf("state: checking rows affected: %w", err)
    }
    if rows == 0 {
        return fmt.Errorf("%w: %s → %s (conflict or transfer not found)",
            models.ErrInvalidStateTransition, from, to)
    }

    metaJSON, _ := json.Marshal(metadata)
    _, err = tx.ExecContext(ctx, `
        INSERT INTO state_transitions (transfer_id, from_state, to_state, triggered_by, metadata)
        VALUES ($1, $2, $3, $4, $5)`,
        id, string(from), string(to), triggeredBy, metaJSON)
    if err != nil {
        return fmt.Errorf("state: logging transition for %s: %w", id, err)
    }

    return nil
}

// BeginAndTransition opens a new transaction, transitions state, and returns the tx.
// Caller is responsible for Commit or Rollback.
func (m *Machine) BeginAndTransition(
    ctx context.Context,
    id uuid.UUID,
    from, to models.TransferStatus,
    triggeredBy string,
    metadata map[string]any,
) (*sql.Tx, error) {
    tx, err := m.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, fmt.Errorf("state: beginning transaction: %w", err)
    }
    if err := m.Transition(ctx, tx, id, from, to, triggeredBy, metadata); err != nil {
        tx.Rollback()
        return nil, err
    }
    return tx, nil
}
```

### 4.3 `internal/state/machine_test.go`

Required tests:
```
TestValidTransition_RequestedToValidating
TestValidTransition_CompletedToReturned
TestInvalidTransition_RequestedToApproved       // skips states
TestInvalidTransition_CompletedToApproved        // backwards
TestInvalidTransition_RejectedToFundsPosted      // terminal state
TestOptimisticLock_ConcurrentTransition          // two goroutines, one wins
```

**Acceptance criteria:**
- All 6 tests pass
- Invalid transitions return `ErrInvalidStateTransition`
- Concurrent test: exactly one goroutine succeeds, other gets conflict error
- Every successful transition has a matching `state_transitions` row

---

## Phase 5: Vendor Stub

**Day 2 — 2 hours**
**Goal:** 7 deterministic scenarios keyed by account suffix. Stateless. Interface-based.

### 5.1 `internal/vendor/models.go`

```go
package vendor

// Request contains the data sent to the vendor for validation.
type Request struct {
    TransferID          string `json:"transfer_id"`
    AccountID           string `json:"account_id"`
    FrontImageRef       string `json:"front_image_ref"`
    BackImageRef        string `json:"back_image_ref"`
    DeclaredAmountCents int64  `json:"declared_amount_cents"`
}

// MICRData represents the extracted MICR line data.
type MICRData struct {
    RoutingNumber string  `json:"routing_number"`
    AccountNumber string  `json:"account_number"`
    CheckSerial   string  `json:"check_serial"`
    Confidence    float64 `json:"confidence"`
}

// Response is what the vendor returns for every validation request.
type Response struct {
    Status         string    `json:"status"`          // "pass", "fail", "flagged"
    IQAResult      string    `json:"iqa_result"`       // "pass", "fail_blur", "fail_glare"
    MICRData       *MICRData `json:"micr_data"`        // nil on MICR failure
    OCRAmountCents *int64    `json:"ocr_amount_cents"` // nil on IQA fail
    DuplicateCheck string    `json:"duplicate_check"`  // "clear", "duplicate_found"
    AmountMatch    bool      `json:"amount_match"`
    TransactionID  string    `json:"transaction_id"`   // vendor-side reference
    ErrorCode      *string   `json:"error_code"`
    ErrorMessage   *string   `json:"error_message"`
}

// Service is the interface all vendor implementations satisfy.
// Production would swap Stub for an HTTP client.
type Service interface {
    Validate(ctx context.Context, req *Request) (*Response, error)
}
```

### 5.2 `internal/vendor/stub.go`

```go
package vendor

import (
    "context"
    "fmt"
    "github.com/google/uuid"
)

// Stub returns deterministic responses keyed by the last 4 chars of AccountID.
type Stub struct{}

func NewStub() *Stub { return &Stub{} }

func (s *Stub) Validate(ctx context.Context, req *Request) (*Response, error) {
    suffix := extractSuffix(req.AccountID)
    txID := "VND-" + uuid.New().String()

    switch suffix {
    case "1001":
        return iqaFailBlur(txID), nil
    case "1002":
        return iqaFailGlare(txID), nil
    case "1003":
        return micrFailure(txID), nil
    case "1004":
        return duplicateDetected(txID), nil
    case "1005":
        return amountMismatch(txID, req.DeclaredAmountCents), nil
    case "1006", "0000":
        return cleanPass(txID, req.DeclaredAmountCents), nil
    default:
        return cleanPass(txID, req.DeclaredAmountCents), nil
    }
}

// extractSuffix returns the last 4 chars of accountID.
// "ACC-SOFI-1003" → "1003"
func extractSuffix(accountID string) string {
    if len(accountID) < 4 {
        return accountID
    }
    return accountID[len(accountID)-4:]
}

func iqaFailBlur(txID string) *Response {
    code, msg := "IQA_FAIL_BLUR", "Image is too blurry. Please retake the photo."
    return &Response{
        Status:        "fail",
        IQAResult:     "fail_blur",
        DuplicateCheck: "clear",
        TransactionID: txID,
        ErrorCode:     &code,
        ErrorMessage:  &msg,
    }
}

func iqaFailGlare(txID string) *Response {
    code, msg := "IQA_FAIL_GLARE", "Image has too much glare. Please retake in better lighting."
    return &Response{
        Status:        "fail",
        IQAResult:     "fail_glare",
        DuplicateCheck: "clear",
        TransactionID: txID,
        ErrorCode:     &code,
        ErrorMessage:  &msg,
    }
}

func micrFailure(txID string) *Response {
    // MICR failure → flagged for operator review, MICRData is nil
    return &Response{
        Status:        "flagged",
        IQAResult:     "pass",
        MICRData:      nil,
        DuplicateCheck: "clear",
        AmountMatch:   false,
        TransactionID: txID,
    }
}

func duplicateDetected(txID string) *Response {
    code, msg := "DUPLICATE_CHECK", "This check has already been deposited."
    return &Response{
        Status:        "fail",
        IQAResult:     "pass",
        MICRData:      standardMICR(),
        DuplicateCheck: "duplicate_found",
        TransactionID: txID,
        ErrorCode:     &code,
        ErrorMessage:  &msg,
    }
}

func amountMismatch(txID string, declared int64) *Response {
    // OCR reads a different amount than declared; flagged for operator review
    ocr := declared + 5000 // OCR reads $50 more than declared
    return &Response{
        Status:         "flagged",
        IQAResult:      "pass",
        MICRData:       standardMICR(),
        OCRAmountCents: &ocr,
        DuplicateCheck: "clear",
        AmountMatch:    false,
        TransactionID:  txID,
    }
}

func cleanPass(txID string, declared int64) *Response {
    return &Response{
        Status:         "pass",
        IQAResult:      "pass",
        MICRData:       standardMICR(),
        OCRAmountCents: &declared,
        DuplicateCheck: "clear",
        AmountMatch:    true,
        TransactionID:  txID,
    }
}

func standardMICR() *MICRData {
    return &MICRData{
        RoutingNumber: "021000021",
        AccountNumber: "123456789",
        CheckSerial:   "0001",
        Confidence:    0.97,
    }
}
```

### 5.3 `internal/vendor/stub_test.go`

```
TestStub_CleanPass_1006
TestStub_CleanPass_DefaultSuffix
TestStub_IQABlur_1001
TestStub_IQAGlare_1002
TestStub_MICRFailure_1003_Flagged
TestStub_DuplicateDetected_1004
TestStub_AmountMismatch_1005_Flagged
TestStub_Stateless_SameInputSameOutput
```

**Acceptance criteria:**
- Each account suffix returns correct status ("pass"/"fail"/"flagged")
- `*1003` and `*1005` return status="flagged", MICRData nil for 1003
- `*1001` and `*1002` return status="fail" with correct IQAResult
- Same account suffix always returns the same response structure

---

## Phase 6: Funding Service

**Day 2 — 3 hours**
**Goal:** Business rules engine. Account resolver. Redis dupe check. Contribution type default.

### 6.1 `internal/funding/accounts.go`

```go
package funding

import (
    "context"
    "database/sql"
    "fmt"
    "github.com/apex/mcd/internal/models"
)

// AccountResolver looks up account + correspondent data from Postgres.
type AccountResolver struct {
    db *sql.DB
}

func NewAccountResolver(db *sql.DB) *AccountResolver {
    return &AccountResolver{db: db}
}

// Resolve returns the account and its correspondent's omnibus account ID.
// Returns ErrAccountNotFound if the account doesn't exist.
// Returns ErrAccountIneligible if the account is not active.
func (r *AccountResolver) Resolve(ctx context.Context, accountID string) (*models.AccountWithCorrespondent, error) {
    var acct models.AccountWithCorrespondent
    err := r.db.QueryRowContext(ctx, `
        SELECT a.id, a.correspondent_id, a.account_type, a.status, a.created_at,
               c.omnibus_account_id
        FROM accounts a
        JOIN correspondents c ON c.id = a.correspondent_id
        WHERE a.id = $1`, accountID).Scan(
        &acct.ID, &acct.CorrespondentID, &acct.AccountType,
        &acct.Status, &acct.CreatedAt, &acct.OmnibusAccountID,
    )
    if err == sql.ErrNoRows {
        return nil, fmt.Errorf("funding: %w: %s", models.ErrAccountNotFound, accountID)
    }
    if err != nil {
        return nil, fmt.Errorf("funding: resolving account %s: %w", accountID, err)
    }
    if acct.Status != "active" {
        return nil, fmt.Errorf("funding: %w: account %s status is %s",
            models.ErrAccountIneligible, accountID, acct.Status)
    }
    return &acct, nil
}
```

### 6.2 `internal/funding/rules.go`

```go
package funding

import (
    "context"
    "crypto/sha256"
    "fmt"
    "time"

    "github.com/apex/mcd/internal/models"
    "github.com/redis/go-redis/v9"
)

const (
    MaxDepositAmountCents = int64(500000) // $5,000
    DupeTTL               = 90 * 24 * time.Hour
)

// applyDepositLimit rejects amounts over $5,000.
func applyDepositLimit(amountCents int64) error {
    if amountCents > MaxDepositAmountCents {
        return fmt.Errorf("%w: %d cents exceeds limit of %d",
            models.ErrDepositOverLimit, amountCents, MaxDepositAmountCents)
    }
    return nil
}

// applyDuplicateCheck stores a hash of (routing+account+amount+serial) in Redis
// with 90-day TTL. Returns ErrDuplicateDeposit if hash already exists.
// Uses SETNX semantics — only sets if not exists.
func applyDuplicateCheck(ctx context.Context, rdb *redis.Client,
    routing, account, serial string, amountCents int64) error {

    raw := fmt.Sprintf("%s:%s:%d:%s", routing, account, amountCents, serial)
    hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
    key := "dupe:check:" + hash

    set, err := rdb.SetNX(ctx, key, "1", DupeTTL).Result()
    if err != nil {
        // Redis down: log warning but continue (graceful degradation)
        return nil
    }
    if !set {
        return fmt.Errorf("%w: check hash %s already exists", models.ErrDuplicateDeposit, hash[:8])
    }
    return nil
}

// applyContributionType sets contribution type based on account type.
// Returns the contribution type to store on the transfer ("INDIVIDUAL", etc.), or "" if not applicable.
func applyContributionType(accountType string) string {
    if accountType == "retirement" {
        return "INDIVIDUAL"
    }
    return ""
}
```

### 6.3 `internal/funding/service.go`

```go
package funding

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/apex/mcd/internal/models"
    "github.com/apex/mcd/internal/vendor"
    "github.com/redis/go-redis/v9"
)

// RuleResult contains everything the funding service determined about a deposit.
type RuleResult struct {
    OmnibusAccountID string
    ContributionType string
    RulesApplied     []string
    RulesPassed      bool
    FailReason       string
}

// Service applies all business rules to a deposit.
type Service struct {
    db       *sql.DB
    rdb      *redis.Client
    resolver *AccountResolver
}

func NewService(db *sql.DB, rdb *redis.Client) *Service {
    return &Service{
        db:       db,
        rdb:      rdb,
        resolver: NewAccountResolver(db),
    }
}

// ApplyRules runs all funding rules for a deposit submission.
// vendorResp provides MICR data for duplicate check.
// Returns RuleResult on pass, or error (wrapping domain errors) on failure.
func (s *Service) ApplyRules(
    ctx context.Context,
    transfer *models.Transfer,
    vendorResp *vendor.Response,
) (*RuleResult, error) {
    result := &RuleResult{RulesApplied: []string{}}

    // Rule 1: Deposit limit
    result.RulesApplied = append(result.RulesApplied, "deposit_limit")
    if err := applyDepositLimit(transfer.AmountCents); err != nil {
        return nil, err
    }

    // Rule 2: Account eligibility + omnibus lookup
    result.RulesApplied = append(result.RulesApplied, "account_eligibility")
    acct, err := s.resolver.Resolve(ctx, transfer.AccountID)
    if err != nil {
        return nil, err
    }
    result.OmnibusAccountID = acct.OmnibusAccountID

    // Rule 3: Contribution type default
    result.RulesApplied = append(result.RulesApplied, "contribution_type")
    result.ContributionType = applyContributionType(acct.AccountType)

    // Rule 4: Duplicate check (only if MICR data is available)
    result.RulesApplied = append(result.RulesApplied, "duplicate_check")
    if vendorResp.MICRData != nil {
        if err := applyDuplicateCheck(ctx, s.rdb,
            vendorResp.MICRData.RoutingNumber,
            vendorResp.MICRData.AccountNumber,
            vendorResp.MICRData.CheckSerial,
            transfer.AmountCents,
        ); err != nil {
            return nil, err
        }
    }

    result.RulesPassed = true
    return result, nil
}
```

### 6.4 `internal/funding/rules_test.go`

```
TestDepositLimit_UnderLimit
TestDepositLimit_AtLimit_500000
TestDepositLimit_OverLimit_500001
TestDuplicateCheck_FirstDeposit_Allowed
TestDuplicateCheck_SecondDeposit_Rejected
TestContributionType_Retirement_Individual
TestContributionType_Individual_Empty
TestAccountResolver_Active_ReturnsOmnibus
TestAccountResolver_NotFound
TestAccountResolver_Suspended_Ineligible
```

**Acceptance criteria:**
- `ErrDepositOverLimit` for amounts > 500000
- `ErrDuplicateDeposit` for same check hash within TTL
- Retirement accounts get ContributionType="INDIVIDUAL"
- Account resolver returns OmnibusAccountID from correspondent
- Suspended accounts return `ErrAccountIneligible`

---

## Phase 7: Ledger Service

**Day 3 — 2 hours**
**Goal:** Append-only ledger posting. Reversal with fee. Transactional.

### 7.1 `internal/ledger/models.go`

```go
package ledger

import (
    "time"
    "github.com/google/uuid"
)

// Entry represents one row in ledger_entries. Always append-only.
type Entry struct {
    ID                  uuid.UUID  `json:"id"`
    TransferID          uuid.UUID  `json:"transfer_id"`
    ToAccountID         string     `json:"to_account_id"`
    FromAccountID       string     `json:"from_account_id"`
    Type                string     `json:"type"`         // always "MOVEMENT"
    SubType             string     `json:"sub_type"`     // "DEPOSIT", "REVERSAL", "RETURN_FEE"
    TransferType        string     `json:"transfer_type"` // "CHECK", "RETURN_FEE"
    Currency            string     `json:"currency"`     // always "USD"
    AmountCents         int64      `json:"amount_cents"`
    Memo                string     `json:"memo"`         // always "FREE"
    SourceApplicationID uuid.UUID  `json:"source_application_id"`
    CreatedAt           time.Time  `json:"created_at"`
}
```

### 7.2 `internal/ledger/repository.go`

```go
package ledger

import (
    "context"
    "database/sql"
    "fmt"
    "github.com/google/uuid"
)

// Repository handles all ledger_entries database operations.
type Repository struct {
    db *sql.DB
}

func NewRepository(db *sql.DB) *Repository {
    return &Repository{db: db}
}

// PostEntryTx inserts a ledger entry within an existing transaction.
// This is the only write method — ledger entries are never updated or deleted.
func (r *Repository) PostEntryTx(ctx context.Context, tx *sql.Tx, entry *Entry) error {
    _, err := tx.ExecContext(ctx, `
        INSERT INTO ledger_entries
            (transfer_id, to_account_id, from_account_id, type, sub_type,
             transfer_type, currency, amount_cents, memo, source_application_id)
        VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
        entry.TransferID, entry.ToAccountID, entry.FromAccountID,
        entry.Type, entry.SubType, entry.TransferType,
        entry.Currency, entry.AmountCents, entry.Memo, entry.SourceApplicationID,
    )
    if err != nil {
        return fmt.Errorf("ledger: posting entry for transfer %s: %w", entry.TransferID, err)
    }
    return nil
}

// GetByTransferID returns all ledger entries for a transfer.
func (r *Repository) GetByTransferID(ctx context.Context, transferID uuid.UUID) ([]Entry, error) {
    // SELECT ... WHERE transfer_id = $1 ORDER BY created_at
}

// GetByAccountID returns entries where to_account_id matches, with optional date filter.
func (r *Repository) GetByAccountID(ctx context.Context, accountID string, from, to *time.Time) ([]Entry, error) {
    // SELECT ... WHERE to_account_id = $1 AND created_at BETWEEN...
}
```

### 7.3 `internal/ledger/service.go`

```go
package ledger

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/apex/mcd/internal/models"
    "github.com/google/uuid"
)

// Service handles ledger posting and reversals.
type Service struct {
    db   *sql.DB
    repo *Repository
}

func NewService(db *sql.DB) *Service {
    return &Service{db: db, repo: NewRepository(db)}
}

// PostFundsTx creates a DEPOSIT ledger entry within the provided transaction.
// Called after transition Approved→FundsPosted.
// toAccountID = investor, fromAccountID = omnibus account of correspondent.
func (s *Service) PostFundsTx(ctx context.Context, tx *sql.Tx, transfer *models.Transfer, omnibusAccountID string) error {
    entry := &Entry{
        TransferID:          transfer.ID,
        ToAccountID:         transfer.AccountID,
        FromAccountID:       omnibusAccountID,
        Type:                "MOVEMENT",
        SubType:             "DEPOSIT",
        TransferType:        "CHECK",
        Currency:            "USD",
        AmountCents:         transfer.AmountCents,
        Memo:                "FREE",
        SourceApplicationID: transfer.ID,
    }
    return s.repo.PostEntryTx(ctx, tx, entry)
}

// PostReversal creates two ledger entries for a bounced check:
// 1. REVERSAL: investor→omnibus for the original deposit amount
// 2. RETURN_FEE: investor→omnibus for the $30 return fee
// Both entries are in a single transaction with the Completed→Returned state transition.
func (s *Service) PostReversal(ctx context.Context, tx *sql.Tx,
    transfer *models.Transfer, omnibusAccountID string, returnFeeCents int64) error {

    reversalEntry := &Entry{
        TransferID:          transfer.ID,
        FromAccountID:       transfer.AccountID, // debit investor
        ToAccountID:         omnibusAccountID,   // credit omnibus
        Type:                "MOVEMENT",
        SubType:             "REVERSAL",
        TransferType:        "CHECK",
        Currency:            "USD",
        AmountCents:         transfer.AmountCents,
        Memo:                "FREE",
        SourceApplicationID: transfer.ID,
    }
    if err := s.repo.PostEntryTx(ctx, tx, reversalEntry); err != nil {
        return err
    }

    feeEntry := &Entry{
        TransferID:          transfer.ID,
        FromAccountID:       transfer.AccountID, // debit investor for fee
        ToAccountID:         omnibusAccountID,
        Type:                "MOVEMENT",
        SubType:             "RETURN_FEE",
        TransferType:        "RETURN_FEE",
        Currency:            "USD",
        AmountCents:         returnFeeCents,
        Memo:                "FREE",
        SourceApplicationID: transfer.ID,
    }
    return s.repo.PostEntryTx(ctx, tx, feeEntry)
}

// GetByTransferID delegates to repository.
func (s *Service) GetByTransferID(ctx context.Context, id uuid.UUID) ([]Entry, error) {
    return s.repo.GetByTransferID(ctx, id)
}
```

### 7.4 `internal/ledger/service_test.go`

```
TestPostFunds_CreatesDepositEntry
TestPostFunds_CorrectAccountMapping
TestPostReversal_TwoEntries
TestPostReversal_CorrectAmounts        // original + $30 fee
TestPostReversal_SubTypes              // REVERSAL + RETURN_FEE
TestLedgerEntries_AppendOnly           // no UPDATE/DELETE exposed
```

**Acceptance criteria:**
- `PostFundsTx` creates exactly one entry with SubType="DEPOSIT"
- `PostReversal` creates exactly two entries, amounts sum correctly
- All amounts in int64 cents, never float
- Both reversal entries have FromAccountID = investor (debit direction)

---

## Phase 8: Deposit Handler & Service (Pipeline Orchestrator)

**Day 3 — 3 hours**
**Goal:** POST /deposits runs the full pipeline synchronously. GET endpoints return current state.

### 8.1 `internal/deposit/service.go`

```go
package deposit

import (
    "context"
    "database/sql"
    "fmt"

    "github.com/apex/mcd/internal/funding"
    "github.com/apex/mcd/internal/ledger"
    "github.com/apex/mcd/internal/models"
    "github.com/apex/mcd/internal/state"
    "github.com/apex/mcd/internal/vendor"
    "github.com/google/uuid"
)

// SubmitRequest contains all data from the multipart form.
type SubmitRequest struct {
    AccountID           string
    AmountCents         int64
    DeclaredAmountCents int64
    FrontImageRef       string // path saved by handler before calling service
    BackImageRef        string
}

// Service orchestrates the full deposit pipeline.
type Service struct {
    db      *sql.DB
    machine *state.Machine
    vendor  vendor.Service
    funding *funding.Service
    ledger  *ledger.Service
}

func NewService(db *sql.DB, machine *state.Machine, vnd vendor.Service,
    fund *funding.Service, led *ledger.Service) *Service {
    return &Service{db: db, machine: machine, vendor: vnd, funding: fund, ledger: led}
}

// Submit runs the full deposit pipeline synchronously:
// Create(Requested) → Validating → Analyzing → Approved → FundsPosted
// OR → Rejected at any failure point
// OR → Analyzing (flagged=true) when vendor flags for review
func (s *Service) Submit(ctx context.Context, req *SubmitRequest) (*models.Transfer, error) {
    // 1. Create transfer in Requested state
    transfer := &models.Transfer{
        ID:                  uuid.New(),
        AccountID:           req.AccountID,
        AmountCents:         req.AmountCents,
        DeclaredAmountCents: req.DeclaredAmountCents,
        Status:              models.StatusRequested,
        FrontImageRef:       &req.FrontImageRef,
        BackImageRef:        &req.BackImageRef,
    }
    if err := s.createTransfer(ctx, transfer); err != nil {
        return nil, err
    }

    // NOTE: Steps 2-6 use separate transactions for each state transition before the
    // critical section. If the server crashes between commits, a transfer may be stuck
    // in an intermediate state (e.g., Validating with no vendor result). This is acceptable
    // for the demo — the critical section (Analyzing→Approved→FundsPosted + ledger posting)
    // is correctly atomic. Production would use a saga pattern or outbox table.

    // 2. Transition Requested → Validating
    tx1, err := s.machine.BeginAndTransition(ctx, transfer.ID,
        models.StatusRequested, models.StatusValidating, "system", nil)
    if err != nil {
        return nil, err
    }
    tx1.Commit()
    transfer.Status = models.StatusValidating

    // 3. Call vendor stub
    vendorReq := &vendor.Request{
        TransferID:          transfer.ID.String(),
        AccountID:           transfer.AccountID,
        FrontImageRef:       req.FrontImageRef,
        BackImageRef:        req.BackImageRef,
        DeclaredAmountCents: transfer.DeclaredAmountCents,
    }
    vendorResp, err := s.vendor.Validate(ctx, vendorReq)
    if err != nil {
        return nil, fmt.Errorf("deposit: vendor validation: %w", err)
    }

    // 4. Store vendor result on transfer
    s.updateTransferVendorData(ctx, transfer, vendorResp)

    // 5. Handle vendor response
    if vendorResp.Status == "fail" {
        // Validating → Rejected
        tx, _ := s.machine.BeginAndTransition(ctx, transfer.ID,
            models.StatusValidating, models.StatusRejected, "system",
            map[string]any{"vendor_error": vendorResp.ErrorCode})
        tx.Commit()
        transfer.Status = models.StatusRejected
        return transfer, nil
    }

    // 6. Validating → Analyzing (pass or flagged)
    flagged := vendorResp.Status == "flagged"
    flagReason := ""
    if flagged {
        if vendorResp.MICRData == nil {
            flagReason = "micr_failure"
        } else {
            flagReason = "amount_mismatch"
        }
    }
    tx2, err := s.machine.BeginAndTransition(ctx, transfer.ID,
        models.StatusValidating, models.StatusAnalyzing, "system",
        map[string]any{"flagged": flagged, "flag_reason": flagReason})
    if err != nil {
        return nil, err
    }
    // Update flagged status in same tx
    tx2.ExecContext(ctx, `UPDATE transfers SET flagged=$1, flag_reason=$2 WHERE id=$3`,
        flagged, flagReason, transfer.ID)
    tx2.Commit()
    transfer.Status = models.StatusAnalyzing
    transfer.Flagged = flagged

    // 7. If flagged, stop here — goes to operator queue
    if flagged {
        return transfer, nil
    }

    // 8. Apply funding rules (Analyzing state)
    ruleResult, err := s.funding.ApplyRules(ctx, transfer, vendorResp)
    if err != nil {
        // Analyzing → Rejected
        tx, _ := s.machine.BeginAndTransition(ctx, transfer.ID,
            models.StatusAnalyzing, models.StatusRejected, "system",
            map[string]any{"rule_failure": err.Error()})
        tx.Commit()
        transfer.Status = models.StatusRejected
        return transfer, nil
    }

    // 9. Set contribution type on transfer if determined
    if ruleResult.ContributionType != "" {
        s.db.ExecContext(ctx, `UPDATE transfers SET contribution_type=$1 WHERE id=$2`,
            ruleResult.ContributionType, transfer.ID)
        ct := ruleResult.ContributionType
        transfer.ContributionType = &ct
    }

    // 10. Analyzing → Approved → FundsPosted in single transaction
    return s.approveAndPost(ctx, transfer, ruleResult.OmnibusAccountID, "system")
}

// approveAndPost transitions Analyzing→Approved→FundsPosted and posts ledger,
// all in a single Postgres transaction.
// Also used by operator service when approving a flagged deposit.
func (s *Service) approveAndPost(ctx context.Context, transfer *models.Transfer,
    omnibusAccountID, triggeredBy string) (*models.Transfer, error) {

    tx, err := s.db.BeginTx(ctx, nil)
    if err != nil {
        return nil, fmt.Errorf("deposit: beginning approval transaction: %w", err)
    }
    defer tx.Rollback()

    // Analyzing → Approved
    if err := s.machine.Transition(ctx, tx, transfer.ID,
        models.StatusAnalyzing, models.StatusApproved, triggeredBy, nil); err != nil {
        return nil, fmt.Errorf("deposit: transitioning to approved: %w", err)
    }

    // Post ledger entry
    if err := s.ledger.PostFundsTx(ctx, tx, transfer, omnibusAccountID); err != nil {
        return nil, fmt.Errorf("deposit: posting funds: %w", err)
    }

    // Approved → FundsPosted
    if err := s.machine.Transition(ctx, tx, transfer.ID,
        models.StatusApproved, models.StatusFundsPosted, triggeredBy, nil); err != nil {
        return nil, fmt.Errorf("deposit: transitioning to funds_posted: %w", err)
    }

    if err := tx.Commit(); err != nil {
        return nil, fmt.Errorf("deposit: committing approval: %w", err)
    }

    transfer.Status = models.StatusFundsPosted
    return transfer, nil
}
```

### 8.2 `internal/deposit/handler.go`

Key handlers:
```go
// POST /api/v1/deposits
func (h *Handler) Submit(c *gin.Context)

// GET /api/v1/deposits/:id
func (h *Handler) GetByID(c *gin.Context)

// GetByID returns the transfer object plus state_history from state_transitions table.
// Response shape:
// {
//   "data": {
//     "transfer_id": "...",
//     "status": "funds_posted",
//     ...all transfer fields...
//     "state_history": [
//       {"from": "requested", "to": "validating", "triggered_by": "system", "at": "..."},
//       {"from": "validating", "to": "analyzing", "triggered_by": "system", "at": "..."},
//       ...
//     ]
//   }
// }
// Query: SELECT * FROM state_transitions WHERE transfer_id = $1 ORDER BY created_at ASC

// GET /api/v1/deposits
func (h *Handler) List(c *gin.Context)

// GET /api/v1/deposits/:id/images/front
// GET /api/v1/deposits/:id/images/back
func (h *Handler) ServeImage(c *gin.Context)

// POST /api/v1/deposits/:id/return
func (h *Handler) Return(c *gin.Context)
```

**Submit handler flow:**
1. Parse multipart form (front_image, back_image, amount_cents, account_id)
2. Validate: amount_cents > 0, <= 500000, account_id non-empty, both images present
3. Save images to `{IMAGE_STORAGE_DIR}/{uuid}/front.png` and `back.png`
4. Call `depositService.Submit(ctx, req)`
5. Return 201 with transfer JSON regardless of final state (requested, funds_posted, rejected, analyzing)

**Image saving:**
```go
transferID := uuid.New()
dir := filepath.Join(cfg.ImageStorageDir, transferID.String())
os.MkdirAll(dir, 0755)
c.SaveUploadedFile(front, filepath.Join(dir, "front.png"))
c.SaveUploadedFile(back, filepath.Join(dir, "back.png"))
frontRef := filepath.Join(dir, "front.png")
backRef := filepath.Join(dir, "back.png")
```

**Return handler:**
```go
// POST /api/v1/deposits/:id/return
// Body: { "return_reason": "insufficient_funds", "bank_reference": "RET-001" }
// Validates transfer is in Completed state
// Calls ledger.PostReversal() + machine.Transition(Completed→Returned) in one tx
```

**Acceptance criteria:**
- `POST /deposits` with clean-pass account returns `{status: "funds_posted"}` in response
- `POST /deposits` with blur account returns `{status: "rejected"}`
- `POST /deposits` with MICR-failure account returns `{status: "analyzing", flagged: true}`
- `GET /deposits/:id` returns full transfer with state_history
- `GET /deposits/:id/images/front` serves the uploaded image
- `POST /deposits/:id/return` on non-Completed returns 409

---

## Phase 9: Middleware

**Day 2 (alongside vendor stub) — 1 hour**

### 9.1 `internal/middleware/auth.go`

```go
package middleware

// InvestorAuth validates Authorization: Bearer <token> header.
// Token must be a known investor token (from cfg.InvestorToken env var).
// Adds "investor_token" to gin context. Does NOT enforce which account_id is used.
func InvestorAuth(investorToken string) gin.HandlerFunc

// OperatorAuth validates X-Operator-ID header.
// Any non-empty value is accepted (simplified for demo).
// Adds "operator_id" to gin context for audit logging.
func OperatorAuth() gin.HandlerFunc
```

### 9.2 `internal/middleware/ratelimit.go`

```go
package middleware

// RateLimit enforces max N requests per account_id per minute using Redis INCR.
// Key: "ratelimit:{account_id}:{minute_unix}". TTL: 90 seconds.
// Returns 429 if limit exceeded. Gracefully skips if Redis is unavailable.
func RateLimit(rdb *redis.Client, maxPerMinute int) gin.HandlerFunc
// Note: account_id must be in gin context before this middleware (set by handler or param)
```

Rate limit key strategy:
```
key = fmt.Sprintf("ratelimit:%s:%d", accountID, time.Now().Unix()/60)
count = INCR key
EXPIRE key 90  // 90s ensures the key expires even if INCR fails on second call
if count > maxPerMinute: return 429
```

---

## Phase 10: Operator Service

**Day 4 — 3 hours**
**Goal:** Review queue, approve/reject workflow, audit logging.

### 10.1 `internal/operator/audit.go`

```go
package operator

// LogActionTx inserts an audit_log entry within an existing transaction.
func LogActionTx(ctx context.Context, tx *sql.Tx,
    operatorID, action string,
    transferID uuid.UUID,
    notes string,
    metadata map[string]any) error

// GetAuditLog retrieves audit entries with optional transfer_id filter.
func (s *Service) GetAuditLog(ctx context.Context, transferID *uuid.UUID) ([]AuditEntry, error)
```

### 10.2 `internal/operator/service.go`

```go
package operator

// GetQueue returns all transfers in Analyzing state with flagged=true.
// Used by operator review dashboard.
func (s *Service) GetQueue(ctx context.Context) ([]*models.Transfer, error)

// Approve moves a flagged deposit from Analyzing to FundsPosted in one transaction:
// 1. Validate transfer is in Analyzing state with flagged=true
// 2. Apply contribution type override if provided
// 3. Transition Analyzing → Approved (machine.Transition with triggeredBy="operator:{id}")
// 4. Post ledger entry (ledger.PostFundsTx)
// 5. Transition Approved → FundsPosted
// 6. Write audit log (LogActionTx)
// All in one Postgres transaction.
func (s *Service) Approve(ctx context.Context, transferID uuid.UUID,
    operatorID, notes string, contributionTypeOverride *string) (*models.Transfer, error)

// Reject moves a flagged deposit from Analyzing to Rejected.
// 1. Validate transfer is in Analyzing state
// 2. Transition Analyzing → Rejected (machine.Transition with triggeredBy="operator:{id}")
// 3. Write audit log
// All in one Postgres transaction.
func (s *Service) Reject(ctx context.Context, transferID uuid.UUID,
    operatorID, reason, notes string) (*models.Transfer, error)
```

### 10.3 `internal/operator/handler.go`

```go
// GET /api/v1/operator/queue
// Requires X-Operator-ID header (OperatorAuth middleware)
func (h *Handler) GetQueue(c *gin.Context)

// POST /api/v1/operator/deposits/:id/approve
// Body: { "notes": "...", "contribution_type_override": null | "EMPLOYER" }
func (h *Handler) Approve(c *gin.Context)

// POST /api/v1/operator/deposits/:id/reject
// Body: { "reason": "...", "notes": "..." }
func (h *Handler) Reject(c *gin.Context)

// GET /api/v1/operator/audit
// Query params: transfer_id (optional), from, to (dates)
func (h *Handler) GetAuditLog(c *gin.Context)
```

**Acceptance criteria:**
- `GET /operator/queue` returns only flagged+analyzing deposits
- `POST /operator/deposits/:id/approve` moves deposit to `funds_posted`
- `POST /operator/deposits/:id/reject` moves deposit to `rejected`
- Both actions write to `audit_logs` with correct operator_id and action
- Both actions write two `state_transitions` rows (Analyzing→Approved, Approved→FundsPosted for approve; Analyzing→Rejected for reject)
- `GET /operator/audit` returns all entries, filterable by transfer_id

---

## Phase 11: Settlement Engine

**Day 4 — 3 hours**
**Goal:** X9 ICL file generation, EOD cutoff enforcement, batch tracking.

### 11.1 `internal/settlement/service.go`

```go
package settlement

// RunSettlement is the main entry point for EOD batch processing.
// 1. Calculate cutoff time (or use provided batch_date)
// 2. Query eligible transfers: status=funds_posted AND created_at <= cutoff AND settlement_batch_id IS NULL
// 3. Create settlement_batches record
// 4. Generate X9 ICL file to temp path via generator.Generate() — before any state changes
// 5. For each transfer: open tx, transition FundsPosted→Completed, set settlement_batch_id, commit
// 6. Move X9 file from temp to final path: {SETTLEMENT_OUTPUT_DIR}/{date}_batch_{uuid}.x9
// 7. UPDATE settlement_batches SET file_path, deposit_count, total_amount, status='submitted'
// If step 4 fails, no state changes have occurred — safe to retry.
// If step 5 partially fails, the X9 file exists and successful transitions are committed.
func (s *Service) RunSettlement(ctx context.Context, batchDate time.Time) (*Batch, error)

// getEligibleDeposits returns FundsPosted transfers before the cutoff.
func (s *Service) getEligibleDeposits(ctx context.Context, cutoff time.Time) ([]*models.Transfer, error)

// CutoffTime returns the UTC time representing 6:30 PM CT for the given date.
func CutoffTime(date time.Time) time.Time {
    ct, _ := time.LoadLocation("America/Chicago")
    y, m, d := date.In(ct).Date()
    return time.Date(y, m, d, 18, 30, 0, 0, ct).UTC()
}
```

### 11.2 `internal/settlement/generator.go`

```go
package settlement

import (
    "github.com/moov-io/imagecashletter"
)

// Generate creates an X9 ICL file for the batch and writes it to the output directory.
// Reads image bytes from each transfer's FrontImageRef and BackImageRef.
// Returns the file path on success.
//
// X9 ICL structure:
//   FileHeader
//   CashLetterHeader
//     BundleHeader
//       CheckDetail (one per transfer)
//         ImageViewDetail (front)
//         ImageViewData (front image bytes)
//         ImageViewDetail (back)
//         ImageViewData (back image bytes)
//     BundleControl
//   CashLetterControl
//   FileControl
func Generate(transfers []*models.Transfer, outputDir string, batchDate time.Time) (string, error)

// buildCheckDetail creates a CheckDetail record from a Transfer.
// Requires MICRRouting and MICRSerial (may be zero-filled for flagged deposits).
func buildCheckDetail(t *models.Transfer, sequenceNum int) imagecashletter.CheckDetail
```

**Fallback note:** If moov-io proves blocking at implementation time, fall back to a JSON settlement file with identical fields. Document in decision_log.md. The `Generate` function signature stays identical — only the implementation changes.

### 11.3 `internal/settlement/generator_test.go`

```
TestSettlement_CorrectDepositCount
TestSettlement_ExcludesRejectedDeposits
TestSettlement_ExcludesAlreadyBatchedDeposits
TestSettlement_TotalAmountCorrect
TestCutoffTime_BeforeCutoff_Included
TestCutoffTime_AfterCutoff_Excluded
```

**Acceptance criteria:**
- Settlement file contains exactly N check records for N eligible FundsPosted deposits
- Rejected/Returned deposits never appear in settlement file
- `settlement_batch_id` is set on all processed transfers
- All processed transfers move to Completed state
- X9 file written to `{SETTLEMENT_OUTPUT_DIR}/{date}_batch_{uuid}.x9`

---

## Phase 12: Main Server (DI Wiring)

**Day 1 skeleton, completed throughout**

### 12.1 `cmd/server/main.go`

```go
package main

func main() {
    cfg := loadConfig()   // read all env vars, fail fast if DATABASE_URL or REDIS_URL missing

    // 1. Connect to Postgres
    db, err := db.Connect(cfg.DatabaseURL)
    // fail fast on error

    // 2. Connect to Redis
    rdb, err := db.NewRedisClient(cfg.RedisURL)
    // fail fast on error

    // 3. Run migrations
    if err := db.RunMigrations(db); err != nil {
        log.Fatal("migrations failed", err)
    }

    // 4. Create data directories
    os.MkdirAll(cfg.ImageStorageDir, 0755)
    os.MkdirAll(cfg.SettlementOutputDir, 0755)

    // 5. Wire up services
    machine  := state.New(db)
    vendorSvc := vendor.NewStub()
    fundingSvc := funding.NewService(db, rdb)
    ledgerSvc := ledger.NewService(db)
    depositSvc := deposit.NewService(db, machine, vendorSvc, fundingSvc, ledgerSvc)
    operatorSvc := operator.NewService(db, machine, ledgerSvc, fundingSvc)
    settlementSvc := settlement.NewService(db, machine, cfg.SettlementOutputDir)

    // 6. Create handlers
    depositHandler    := deposit.NewHandler(depositSvc, cfg)
    operatorHandler   := operator.NewHandler(operatorSvc)
    settlementHandler := settlement.NewHandler(settlementSvc)
    ledgerHandler     := ledger.NewHandler(ledgerSvc)

    // 7. Configure Gin router
    r := gin.Default()
    r.Use(gin.Recovery())

    // Health check
    r.GET("/health", healthHandler(db, rdb))

    // Investor routes (auth required)
    inv := r.Group("/api/v1")
    inv.Use(middleware.InvestorAuth(cfg.InvestorToken))
    {
        inv.POST("/deposits", middleware.RateLimit(rdb, 10), depositHandler.Submit)
        inv.GET("/deposits", depositHandler.List)
        inv.GET("/deposits/:id", depositHandler.GetByID)
        inv.GET("/deposits/:id/images/:side", depositHandler.ServeImage)
        inv.GET("/ledger/:account_id", ledgerHandler.GetByAccount)
    }

    // Operator routes (operator auth)
    ops := r.Group("/api/v1/operator")
    ops.Use(middleware.OperatorAuth())
    {
        ops.GET("/queue", operatorHandler.GetQueue)
        ops.POST("/deposits/:id/approve", operatorHandler.Approve)
        ops.POST("/deposits/:id/reject", operatorHandler.Reject)
        ops.GET("/audit", operatorHandler.GetAuditLog)
        // Return lives in operator group — only operators can trigger returns
        ops.POST("/deposits/:id/return", depositHandler.Return)
    }

    // Settlement endpoint (operator)
    r.POST("/api/v1/settlement/trigger", middleware.OperatorAuth(), settlementHandler.Trigger)

    r.Run(":" + cfg.ServerPort)
}
```

### Config struct:
```go
type Config struct {
    DatabaseURL         string
    RedisURL            string
    ServerPort          string
    EODCutoffHour       int
    EODCutoffMinute     int
    ImageStorageDir     string
    SettlementOutputDir string
    ReturnFeeCents      int64
    VendorStubMode      string
    InvestorToken       string
    OperatorToken       string
}
```

---

## Phase 13: React Frontend

**Day 5 — 6 hours**

**Fallback decision point:** If React components are not functional by end of day 5, cut the frontend entirely. Redirect day 6 time to test coverage and demo scripts. The backend API is fully testable via curl — demo scripts in Phase 15 cover all scenarios. This trades 10 rubric points (operator UI) for stronger scores in core correctness (25 pts) and tests (10 pts).

### 13.1 `web/src/api.js`

```javascript
const BASE_URL = import.meta.env.VITE_API_URL || 'http://localhost:8080'
const INVESTOR_TOKEN = 'tok_investor_test'
const OPERATOR_TOKEN = 'tok_operator_test'

export const api = {
    // Investor endpoints
    submitDeposit: (formData) =>
        fetch(`${BASE_URL}/api/v1/deposits`, {
            method: 'POST',
            headers: { 'Authorization': `Bearer ${INVESTOR_TOKEN}` },
            body: formData,  // FormData for multipart
        }).then(handleResponse),

    getDeposit: (id) =>
        fetch(`${BASE_URL}/api/v1/deposits/${id}`, {
            headers: { 'Authorization': `Bearer ${INVESTOR_TOKEN}` },
        }).then(handleResponse),

    listDeposits: (params) => ...,

    getLedger: (accountId) => ...,

    // Operator endpoints
    getQueue: () =>
        fetch(`${BASE_URL}/api/v1/operator/queue`, {
            headers: { 'X-Operator-ID': 'OP-001' },
        }).then(handleResponse),

    approveDeposit: (id, body) =>
        fetch(`${BASE_URL}/api/v1/operator/deposits/${id}/approve`, {
            method: 'POST',
            headers: { 'X-Operator-ID': 'OP-001', 'Content-Type': 'application/json' },
            body: JSON.stringify(body),
        }).then(handleResponse),

    rejectDeposit: (id, body) => ...,

    triggerSettlement: (batchDate) =>
        fetch(`${BASE_URL}/api/v1/settlement/trigger`, {
            method: 'POST',
            headers: { 'X-Operator-ID': 'OP-001', 'Content-Type': 'application/json' },
            body: JSON.stringify({ batch_date: batchDate }),
        }).then(handleResponse),

    returnDeposit: (id, body) => ...,

    getAuditLog: (transferId) => ...,
}

function handleResponse(resp) {
    if (!resp.ok) return resp.json().then(err => Promise.reject(err))
    return resp.json()
}
```

### 13.2 `web/src/App.jsx`

Tab-based navigation with 4 views:
```jsx
// Tabs: Deposit | My Deposits | Operator Queue | Ledger
// Active tab shown in state
// No routing library — simple tab state
```

### 13.3 `web/src/components/DepositForm.jsx`

```jsx
// State: accountId, amountCents, frontFile, backFile, loading, result, error

// Account dropdown with labeled options:
const ACCOUNTS = [
  { id: 'ACC-SOFI-1006', label: 'ACC-SOFI-1006 — Clean Pass (Happy Path)' },
  { id: 'ACC-SOFI-1001', label: 'ACC-SOFI-1001 — IQA Blur (Rejected)' },
  { id: 'ACC-SOFI-1002', label: 'ACC-SOFI-1002 — IQA Glare (Rejected)' },
  { id: 'ACC-SOFI-1003', label: 'ACC-SOFI-1003 — MICR Failure (Operator Review)' },
  { id: 'ACC-SOFI-1004', label: 'ACC-SOFI-1004 — Duplicate (Rejected)' },
  { id: 'ACC-SOFI-1005', label: 'ACC-SOFI-1005 — Amount Mismatch (Operator Review)' },
  { id: 'ACC-SOFI-0000', label: 'ACC-SOFI-0000 — Basic Pass' },
  { id: 'ACC-RETIRE-001', label: 'ACC-RETIRE-001 — Retirement (Contribution Type)' },
]

// Amount input (defaults to $100.00 = 10000 cents)
// File inputs for front and back images
// Submit → calls api.submitDeposit()
// Shows result: status badge + transfer_id + actionable message
// Status color coding: funds_posted=green, rejected=red, analyzing=yellow
```

### 13.4 `web/src/components/ReviewQueue.jsx`

```jsx
// Polls GET /operator/queue every 5 seconds via setInterval
// Cleanup on unmount
// Shows table: transfer_id | account | amount | flag_reason | action buttons
// Image display: <img src={`/api/v1/deposits/${id}/images/front`} />
// Approve button → confirm dialog → api.approveDeposit()
// Reject button → confirm + reason input → api.rejectDeposit()
// Settlement trigger button → api.triggerSettlement(today)
// Refresh on approve/reject
```

### 13.5 `web/src/components/TransferStatus.jsx`

```jsx
// Input: transfer_id to track
// Polls GET /deposits/:id every 2 seconds while status is non-terminal
// Terminal states: rejected, returned, completed, funds_posted
// Shows: status badge, state history timeline, vendor result, ledger entries
// Calls api.getDeposit(id) + api.getLedger(accountId) for ledger section
```

### 13.6 `web/src/components/LedgerView.jsx`

```jsx
// Account selector dropdown (same as deposit form)
// Calls api.getLedger(accountId)
// Shows table: date | type | sub_type | amount | from | to
// Color coding: DEPOSIT=green, REVERSAL=red, RETURN_FEE=red
```

---

## Phase 14: Tests

**Day 6 — 6 hours**
**Minimum 15 tests. See `.claude/rules/testing.md` for full strategy.**

### Required test files and test functions:

**`internal/state/machine_test.go`** (6 tests)
```
TestTransition_ValidRequestedToValidating
TestTransition_ValidCompletedToReturned
TestTransition_InvalidRequestedToApproved         // ErrInvalidStateTransition
TestTransition_InvalidCompletedToApproved          // backwards
TestTransition_InvalidRejectedToFundsPosted        // terminal state
TestTransition_OptimisticLock_Concurrent           // two goroutines, one wins
```

**`internal/vendor/stub_test.go`** (7 tests)
```
TestStub_1001_IQABlur
TestStub_1002_IQAGlare
TestStub_1003_MICRFailure_Flagged
TestStub_1004_DuplicateDetected
TestStub_1005_AmountMismatch_Flagged
TestStub_1006_CleanPass
TestStub_DefaultSuffix_CleanPass
```

**`internal/funding/rules_test.go`** (5 tests)
```
TestDepositLimit_Table                             // table-driven: under/at/over limit
TestDuplicateCheck_FirstAllowed_SecondRejected
TestContributionType_Retirement_Individual
TestContributionType_Individual_Empty
TestAccountResolver_ReturnsOmnibusAccountID
```

**`internal/ledger/service_test.go`** (4 tests)
```
TestPostFunds_CreatesDepositEntry_CorrectFields
TestPostReversal_TwoEntries_CorrectSubTypes
TestPostReversal_FeeAmount_3000Cents
TestPostReversal_DirectionCorrect_InvestorToOmnibus
```

**`internal/settlement/generator_test.go`** (4 tests)
```
TestSettlement_DepositCountCorrect
TestSettlement_ExcludesRejected
TestSettlement_ExcludesAlreadyBatched
TestSettlement_TotalAmountCorrect
```

**`internal/operator/service_test.go`** (3 tests)
```
TestApprove_MovesToFundsPosted
TestApprove_WritesAuditLog
TestReject_MovesToRejected
```

**Total: 29 tests** — well above the 15 minimum.

### Test helpers (`internal/state/testhelpers_test.go`, etc.)

```go
// In each package that needs test fixtures:
func newTestTransfer(t *testing.T, opts ...TransferOption) *models.Transfer
func WithStatus(s models.TransferStatus) TransferOption
func WithAmount(cents int64) TransferOption
func WithAccount(acct string) TransferOption
func WithFlagged(reason string) TransferOption
```

### Running tests:
```bash
# Unit tests (no DB required)
cd backend && go test ./internal/... -v

# Integration tests (requires running Docker Compose)
cd backend && go test ./tests/ -v -tags=integration

# Generate test report
cd backend && go test ./... -v 2>&1 | tee ../reports/test-results.txt
```

---

## Phase 15: Demo Scripts

**Day 6 — 2 hours**

All scripts in `scripts/`. Include placeholder images in `scripts/fixtures/`:
- `check-front.png` — a 1KB valid PNG (created with ImageMagick: `convert -size 400x200 xc:white -font Helvetica -pointsize 20 -draw "text 10,100 'SYNTHETIC CHECK - NOT REAL'" front.png`)
- `check-back.png` — similar

### `scripts/demo-happy-path.sh`

```bash
#!/bin/bash
set -e
BASE="http://localhost:8080"
TOKEN="tok_investor_test"
OP="OP-001"
FRONT="scripts/fixtures/check-front.png"
BACK="scripts/fixtures/check-back.png"

echo "=== HAPPY PATH DEMO ==="
echo ""
echo "1. Submitting clean-pass deposit (ACC-SOFI-1006)..."
RESP=$(curl -s -X POST "$BASE/api/v1/deposits" \
  -H "Authorization: Bearer $TOKEN" \
  -F "account_id=ACC-SOFI-1006" \
  -F "amount_cents=100000" \
  -F "front_image=@$FRONT" \
  -F "back_image=@$BACK")
echo "$RESP" | jq .
ID=$(echo "$RESP" | jq -r '.data.transfer_id')
STATUS=$(echo "$RESP" | jq -r '.data.status')
echo ""
echo "→ Transfer ID: $ID"
echo "→ Status: $STATUS"
[ "$STATUS" = "funds_posted" ] && echo "✓ PASS: deposit reached funds_posted" || { echo "✗ FAIL: expected funds_posted, got $STATUS"; exit 1; }

echo ""
echo "2. Triggering EOD settlement..."
SETTLE=$(curl -s -X POST "$BASE/api/v1/settlement/trigger" \
  -H "X-Operator-ID: $OP" \
  -H "Content-Type: application/json" \
  -d "{\"batch_date\": \"$(date +%Y-%m-%d)\"}")
echo "$SETTLE" | jq .
COUNT=$(echo "$SETTLE" | jq -r '.data.deposit_count')
echo "→ Batch settled $COUNT deposit(s)"
[ "$COUNT" -ge "1" ] && echo "✓ PASS: settlement batch created" || echo "✗ FAIL: no deposits in batch"

echo ""
echo "3. Verifying transfer is now Completed..."
FINAL=$(curl -s "$BASE/api/v1/deposits/$ID" -H "Authorization: Bearer $TOKEN")
FINAL_STATUS=$(echo "$FINAL" | jq -r '.data.status')
[ "$FINAL_STATUS" = "completed" ] && echo "✓ PASS: transfer is completed" || echo "✗ FAIL: expected completed, got $FINAL_STATUS"

echo ""
echo "=== HAPPY PATH COMPLETE ==="
```

### `scripts/demo-all-scenarios.sh`

Exercises all 7 vendor stub scenarios in sequence:
```bash
# Tests each account suffix, validates expected status and flag_reason
# ACC-SOFI-1001 → expect rejected
# ACC-SOFI-1002 → expect rejected
# ACC-SOFI-1003 → expect analyzing + flagged=true + flag_reason=micr_failure
# ACC-SOFI-1004 → expect rejected
# ACC-SOFI-1005 → expect analyzing + flagged=true + flag_reason=amount_mismatch
# ACC-SOFI-1006 → expect funds_posted
# ACC-SOFI-0000 → expect funds_posted
# Prints PASS/FAIL for each
```

### `scripts/demo-return.sh`

```bash
# 1. Submit clean deposit → funds_posted
# 2. Trigger settlement → completed
# 3. POST /deposits/:id/return
# 4. Assert status=returned
# 5. GET /ledger/:account_id → assert 3 entries (DEPOSIT + REVERSAL + RETURN_FEE)
# 6. Assert REVERSAL amount = original, RETURN_FEE amount = 3000
```

### `scripts/trigger-settlement.sh`

```bash
# Simple settlement trigger script
# Outputs: batch_id, deposit_count, total_amount, file_path
```

---

## Phase 16: Docker Backend Dockerfile

**`backend/Dockerfile`** (multi-stage):
```dockerfile
FROM golang:1.22-alpine AS builder
WORKDIR /app
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 GOOS=linux go build -o server ./cmd/server

FROM alpine:3.19
RUN addgroup -S mcd && adduser -S mcd -G mcd
WORKDIR /app
COPY --from=builder /app/server .
USER mcd
EXPOSE 8080
CMD ["./server"]
```

**`web/Dockerfile`** (for Vite dev server in Docker):
```dockerfile
FROM node:20-alpine
WORKDIR /app
COPY package*.json ./
RUN npm install
COPY . .
EXPOSE 5173
CMD ["npm", "run", "dev", "--", "--host", "0.0.0.0"]
```

---

## Phase 17: Health Check

**`GET /health`** — implemented in `cmd/server/main.go`:

```go
func healthHandler(db *sql.DB, rdb *redis.Client) gin.HandlerFunc {
    return func(c *gin.Context) {
        pgStatus := "connected"
        if err := db.PingContext(c.Request.Context()); err != nil {
            pgStatus = "disconnected"
        }

        redisStatus := "connected"
        if err := rdb.Ping(c.Request.Context()).Err(); err != nil {
            redisStatus = "disconnected"
        }

        status := "ok"
        httpCode := http.StatusOK
        if pgStatus != "connected" || redisStatus != "connected" {
            status = "degraded"
            httpCode = http.StatusServiceUnavailable
        }

        c.JSON(httpCode, gin.H{
            "status":    status,
            "postgres":  pgStatus,
            "redis":     redisStatus,
            "timestamp": time.Now().UTC(),
        })
    }
}
```

---

## Phase 18: Documentation

**Day 7 — 4 hours**

### Files to create/complete:

**`docs/architecture.md`** — System diagram (text-based), service boundaries, data flow for happy path and return/reversal

**`docs/decision_log.md`** — All decisions from presearch.md + new decisions made during implementation:
- Synchronous pipeline vs async (noted Kafka for production)
- In-process vendor stub (noted HTTP client for production)
- WHERE-status optimistic locking vs SELECT FOR UPDATE
- Auto-posting on operator approve (vs two-step)
- Contribution type in transfers table (vs ledger_entries)
- JSON settlement fallback if moov-io blocked

**`docs/risks.md`** — From presearch + realized risks during implementation

**`reports/scenario-coverage.md`** — Test results table:

| Scenario | Account | Test | Script | Status |
|---|---|---|---|---|
| Happy path | ACC-SOFI-1006 | TestStub_1006 + integration | demo-happy-path.sh | ✓ |
| IQA Blur | ACC-SOFI-1001 | TestStub_1001 | demo-all-scenarios.sh | ✓ |
| IQA Glare | ACC-SOFI-1002 | TestStub_1002 | demo-all-scenarios.sh | ✓ |
| MICR Failure | ACC-SOFI-1003 | TestStub_1003 | demo-all-scenarios.sh | ✓ |
| Duplicate | ACC-SOFI-1004 | TestStub_1004 | demo-all-scenarios.sh | ✓ |
| Amount Mismatch | ACC-SOFI-1005 | TestStub_1005 | demo-all-scenarios.sh | ✓ |
| Over Limit | any | TestDepositLimit_Table | demo-all-scenarios.sh | ✓ |
| Invalid State | n/a | TestTransition_Invalid* | n/a | ✓ |
| Reversal + Fee | any | TestPostReversal* | demo-return.sh | ✓ |
| Settlement | any | TestSettlement* | trigger-settlement.sh | ✓ |

---

## Build Order Summary

| Phase | Content | Day | Hours | Depends On |
|---|---|---|---|---|
| 1 | Scaffold, go.mod, Docker Compose, Makefile | 1 | 2h | — |
| 2 | DB migrations, seed data | 1 | 2h | 1 |
| 3 | Models (Transfer, Account, errors) | 1 | 1h | — |
| 4 | State machine (states + machine + tests) | 1–2 | 2h | 2, 3 |
| 5 | Vendor stub + tests | 2 | 2h | 3 |
| 9 | Middleware (auth + rate limit) | 2 | 1h | 2 |
| 6 | Funding service + tests | 2 | 3h | 3, 4, 5 |
| 7 | Ledger service + tests | 3 | 2h | 3, 4 |
| 8 | Deposit handler + service (pipeline) | 3 | 3h | 4, 5, 6, 7 |
| 12 | Main server / DI wiring | 3 | 1h | all above |
| 17 | Health check endpoint | 3 | 0.5h | 12 |
| 10 | Operator service + handler + tests | 4 | 3h | 4, 7, 8 |
| 11 | Settlement engine + generator + tests | 4 | 3h | 4, 8 |
| 13 | React frontend (all components) | 5 | 6h | 8, 10, 11 |
| 14 | Tests (all remaining unit + integration) | 6 | 6h | all |
| 15 | Demo scripts + fixtures | 6 | 2h | all |
| 16 | Dockerfiles finalized | 6 | 1h | all |
| 18 | Documentation + reports | 7 | 4h | all |

---

## Key Implementation Invariants

These must be enforced throughout:

1. **All monetary amounts are `int64` cents.** Never `float64`. Never divide without thinking about truncation.

2. **Every state transition goes through `machine.Transition()`** — never `UPDATE transfers SET status = ...` directly.

3. **Ledger entries are append-only.** The repository exposes no UPDATE or DELETE for `ledger_entries`.

4. **Funding rules + ledger posting are gated by state.** No deposit can have a ledger entry without having passed through `Analyzing → Approved`.

5. **`machine.Transition()` always takes a `*sql.Tx`** to ensure atomicity with ledger posting and audit logging.

6. **No deposit appears in a settlement file without being in `funds_posted` state** and having `settlement_batch_id IS NULL`.

7. **Reversal requires `Completed` state** — checked by the return handler before opening any transaction.

8. **All DB calls and service methods accept `context.Context` as first parameter.**

9. **Account numbers masked to last 4 digits in all log output.**

10. **`DATABASE_URL` and `REDIS_URL` missing at startup = `log.Fatal()`**, never silent fallback.
