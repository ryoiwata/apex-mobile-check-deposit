# Testing Rules

## Philosophy

This is a one-week take-home. Test what the rubric scores, skip what it doesn't.

**Test rigorously:** state machine transitions, business rules (funding service), ledger posting/reversal math, vendor stub response routing, settlement file contents.
**Test lightly:** API endpoint smoke tests, operator workflow handlers.
**Don't test:** React frontend (manual testing), Redis caching logic, Docker Compose orchestration.

The rubric allocates 10 points to "tests and evaluation rigor" — minimum 10 tests exercising all paths. Aim for 15+ to show thoroughness.

## Framework

- **Go:** `go test` with `testify/assert` and `testify/require`
- **Run:** `go test ./... -v`
- **No external test frameworks.** Just stdlib + testify.

## Directory Structure

Tests live alongside the code they test (Go convention):

```
backend/internal/
├── state/
│   ├── machine.go
│   └── machine_test.go          # State transition tests — CRITICAL
├── funding/
│   ├── rules.go
│   └── rules_test.go            # Business rule tests — CRITICAL
├── ledger/
│   ├── service.go
│   └── service_test.go          # Posting + reversal tests — CRITICAL
├── vendor/
│   ├── stub.go
│   └── stub_test.go             # Stub response routing tests
├── settlement/
│   ├── generator.go
│   └── generator_test.go        # X9 file content validation
└── operator/
    ├── service.go
    └── service_test.go           # Approve/reject workflow tests

tests/                            # Integration tests (cross-service)
├── integration_test.go          # Happy path end-to-end
└── scenario_test.go             # All vendor stub scenarios end-to-end
```

## Required Test Cases (Minimum 10)

These map directly to the rubric. Every one of these must pass:

### 1. Happy Path End-to-End
- Submit deposit → vendor validates (clean pass) → funding rules pass → ledger posts → settlement file includes deposit → mark completed
- Assert: transfer ends in `Completed` state, ledger has one DEPOSIT entry, settlement file contains the deposit

### 2. IQA Fail — Blur (account `*1001`)
- Submit deposit with blur-triggering account → vendor returns IQA_FAIL_BLUR
- Assert: transfer moves to `Rejected`, error message mentions "blurry", no ledger entry created

### 3. IQA Fail — Glare (account `*1002`)
- Submit deposit with glare-triggering account → vendor returns IQA_FAIL_GLARE
- Assert: transfer moves to `Rejected`, error message mentions "glare", no ledger entry created

### 4. MICR Read Failure (account `*1003`)
- Submit deposit → vendor returns MICR_FAILURE → deposit flagged for manual review
- Assert: transfer moves to `Analyzing` with `flagged=true`, appears in operator review queue

### 5. Duplicate Detected (account `*1004`)
- Submit deposit → vendor detects duplicate
- Assert: transfer moves to `Rejected`, reason is "duplicate"

### 6. Amount Mismatch (account `*1005`)
- Submit deposit → vendor OCR amount differs from entered amount → flagged for review
- Assert: transfer is flagged, review queue shows both amounts

### 7. Deposit Over Limit
- Submit deposit for $5,001 → funding service rejects
- Assert: transfer moves to `Rejected` from `Analyzing`, reason mentions limit, no ledger entry

### 8. State Machine — Invalid Transitions
- Attempt: `Requested → Approved` (skipping Validating and Analyzing)
- Attempt: `Completed → Approved` (backwards)
- Attempt: `Rejected → FundsPosted` (terminal state)
- Assert: all three return `ErrInvalidStateTransition`

### 9. Reversal with Fee Calculation
- Create a completed deposit for $1,000 → simulate return
- Assert: reversal debits $1,000 from investor, fee debits $30 from investor, transfer moves to `Returned`, two new ledger entries exist

### 10. Settlement File Validation
- Create 3 approved deposits, trigger settlement generation
- Assert: X9 file contains exactly 3 check records, amounts match, no rejected deposits included, batch total is correct

### Bonus Tests (aim for these)

### 11. Contribution Type Default
- Submit deposit to a retirement-type account → contribution type defaults to INDIVIDUAL
- Assert: ledger entry has correct contribution type

### 12. Funding Service Duplicate Detection
- Submit two deposits with identical check hash within 90-day window
- Assert: second deposit rejected by funding service (independent of vendor-level dupe check)

### 13. EOD Cutoff Rollover
- Submit deposit at 6:31 PM CT → should roll to next business day batch
- Assert: deposit not included in current day's settlement file, included in next day's

### 14. Operator Approve/Reject Audit
- Flag a deposit → operator approves → check audit log
- Assert: audit entry contains operator ID, action, timestamp, transfer ID

### 15. Concurrent State Transitions
- Two goroutines attempt to transition the same transfer simultaneously
- Assert: exactly one succeeds, the other gets a conflict error

## Test Fixture Strategy

Create helper functions, not fixture files:

```go
// test_helpers.go (in each package that needs them)
func newTestTransfer(t *testing.T, opts ...TransferOption) *Transfer {
    t.Helper()
    transfer := &Transfer{
        ID:        uuid.New(),
        AccountID: "ACC-TEST-1006",  // default: clean pass
        Amount:    100000,            // $1,000
        Status:    Requested,
        CreatedAt: time.Now(),
    }
    for _, opt := range opts {
        opt(transfer)
    }
    return transfer
}

func WithAmount(cents int64) TransferOption {
    return func(t *Transfer) { t.Amount = cents }
}

func WithAccount(acct string) TransferOption {
    return func(t *Transfer) { t.AccountID = acct }
}

func WithStatus(s TransferStatus) TransferOption {
    return func(t *Transfer) { t.Status = s }
}
```

## Table-Driven Tests

Use table-driven tests for business rules and state transitions:

```go
func TestDepositLimitRule(t *testing.T) {
    tests := []struct {
        name      string
        amount    int64
        wantErr   bool
        errType   error
    }{
        {"under limit", 100000, false, nil},
        {"at limit", 500000, false, nil},
        {"over limit", 500001, true, ErrDepositOverLimit},
        {"zero amount", 0, true, ErrInvalidAmount},
        {"negative amount", -100, true, ErrInvalidAmount},
    }
    for _, tt := range tests {
        t.Run(tt.name, func(t *testing.T) {
            err := applyDepositLimit(tt.amount)
            if tt.wantErr {
                require.Error(t, err)
                assert.ErrorIs(t, err, tt.errType)
            } else {
                require.NoError(t, err)
            }
        })
    }
}
```

## Mocking Strategy

Mock at interface boundaries. Define interfaces for external dependencies:

```go
// Interfaces to mock
type VendorService interface {
    Validate(ctx context.Context, req *VendorRequest) (*VendorResponse, error)
}

type TransferRepository interface {
    Create(ctx context.Context, t *Transfer) error
    GetByID(ctx context.Context, id uuid.UUID) (*Transfer, error)
    UpdateStatus(ctx context.Context, id uuid.UUID, status TransferStatus) error
}

type LedgerRepository interface {
    PostEntry(ctx context.Context, entry *LedgerEntry) error
    GetByTransferID(ctx context.Context, transferID uuid.UUID) ([]LedgerEntry, error)
}
```

Use testify mocks or simple struct implementations in tests. Don't over-abstract — if a mock has more lines than the real implementation, you've gone too far.

## Integration Tests

Integration tests use a real Postgres instance (from Docker Compose):

```go
//go:build integration

func TestHappyPathEndToEnd(t *testing.T) {
    // Use test database
    db := setupTestDB(t)
    defer cleanupTestDB(t, db)

    // Wire up real services with test DB
    // Submit deposit → validate → analyze → approve → post → settle
    // Assert final state and ledger entries
}
```

Run integration tests separately: `go test ./tests/ -v -tags=integration`

## Demo Scripts as Tests

The demo scripts (`scripts/demo-*.sh`) serve as additional validation. They should:
- Exit with non-zero status on any failure
- Print clear pass/fail for each scenario
- Be runnable against a fresh `docker compose up`
- Generate output suitable for the `/reports` directory

## What Not to Test

- Don't write tests for the React frontend. Manual testing during demo is sufficient.
- Don't test Gin framework internals (routing, middleware chaining).
- Don't test `moov-io/imagecashletter` library internals — test that YOUR generator produces valid output.
- Don't test Docker Compose orchestration.
- Don't test Redis connection pooling.
- Don't aim for coverage numbers. Aim for "does every rubric-scored path work."

## Test Report

Generate a test report for the `/reports` directory:

```bash
go test ./... -v -json > reports/test-results.json
# Or human-readable:
go test ./... -v 2>&1 | tee reports/test-results.txt
```

Include a summary table in `reports/scenario-coverage.md` mapping each vendor stub scenario to its test and result.
