# SUBMISSION.md — Mobile Check Deposit System

**Project:** Mobile Check Deposit System
**Assessment:** Apex Fintech Services — Week 4 Technical Assessment
**Date:** March 2026

---

## Summary

This project implements a minimal but complete end-to-end mobile check deposit pipeline for brokerage accounts. Investors submit check images via a React UI; a deterministic Vendor Service stub validates images across 7 scenarios (IQA blur/glare, MICR failure, duplicate detection, amount mismatch, clean pass); a Funding Service enforces business rules (deposit limits, account eligibility, Redis-backed duplicate detection); a strict state machine ensures no deposit can reach `FundsPosted` without passing all validation; and a Settlement Engine batches approved deposits into X9-structured files at EOD cutoff.

The key design choices were: Go monolith with clean internal package boundaries (each package has an interface for production swap-out); synchronous pipeline for demo determinism with a documented Kafka/event-driven path for production; `int64` cents throughout for financial correctness; optimistic locking on state transitions (tested concurrently); and account-suffix-keyed vendor responses that require zero code changes to exercise any of the 7 test scenarios.

The main trade-offs: the vendor integration is a stub (no real image processing), auth is simplified (static tokens, no account scoping), settlement files are structured JSON rather than binary X9 ICL format (documented in decision log with the moov-io path for production), and the system is single-node only (no distributed locking for multi-instance settlement).

---

## How to Run

**Requirements:** Docker and Docker Compose (Docker Desktop or `docker compose` CLI)

```bash
# Clone and start
git clone <repo-url>
cd apex-mobile-check-deposit
cp .env.example backend/.env
docker compose up --build

# Wait ~12 seconds for services to be ready, then:
curl -s http://localhost:8080/health | jq .
# Expected: {"status":"ok","postgres":"connected","redis":"connected",...}
```

**React UI:** http://localhost:5173
**API:** http://localhost:8080

**Demo Scripts (in a separate terminal):**
```bash
./scripts/demo-happy-path.sh       # Full lifecycle with settlement
./scripts/demo-all-scenarios.sh    # All 7 vendor stub scenarios + operator review
./scripts/demo-return.sh           # Bounced check reversal with $30 fee
./scripts/trigger-settlement.sh    # EOD settlement generation
```

**Go Tests:**
```bash
cd backend
go test ./... -v
```

**Reset everything:**
```bash
docker compose down -v
docker compose up --build
```

---

## Test and Evaluation Results

| Test Suite | Count | Status |
|-----------|-------|--------|
| Go unit tests | 38 | ✅ All pass |
| demo-happy-path.sh | 9 assertions | ✅ All pass |
| demo-all-scenarios.sh | 13 assertions | ✅ All pass |
| demo-return.sh | 9 assertions | ✅ All pass |
| Phase 8 acceptance (deposit handler) | 16 assertions | ✅ All pass |
| Phase 10 acceptance (operator service) | 21 assertions | ✅ All pass |
| Phase 11 acceptance (settlement engine) | 22 assertions | ✅ All pass |
| Phase 13 acceptance (React frontend) | 25 assertions | ✅ All pass |

Detailed scenario coverage: [`reports/scenario-coverage.md`](reports/scenario-coverage.md)

---

## With One More Week, We Would

1. **Replace JSON settlement with binary X9 ICL** via `moov-io/imagecashletter`. The field mapping and file structure are already written in `settlement/generator.go` — it would be a 2–3 hour integration task with the moov-io writer API.

2. **Add event-driven state transitions via Kafka** to align with Apex's production architecture. Each state change would publish a domain event (`DepositSubmitted`, `VendorValidationCompleted`, etc.). Downstream services (funding, ledger, settlement) would consume and react. This decouples pipeline stages and makes each step independently retryable.

3. **Implement proper JWT authentication** with account-scoped queries. The `InvestorAuth` middleware would validate a JWT from the correspondent's identity provider and inject `account_id` into the Gin context. All deposit and ledger queries would filter by that account.

4. **Add gRPC internal service communication** as a step toward microservices decomposition. The `vendor.Service` and `funding.Service` interfaces are already defined — promoting them to gRPC services would be the correct production path.

5. **Implement comprehensive load testing** with `k6` or `vegeta`. Concurrent deposit submissions would surface any remaining race conditions in the state machine. Settlement batch processing under concurrent load would validate the `SELECT FOR UPDATE SKIP LOCKED` strategy.

6. **Add Prometheus metrics and structured logging** with a correlation `request_id` on every log line. Currently logs use `log.Printf` — production would use structured JSON (zerolog or zap) with transfer_id, state, operation, duration_ms, and outcome fields on every meaningful event.

---

## Risks and Limitations

See [`docs/risks.md`](docs/risks.md) for the full risk register. Key items:

- Stubbed vendor integration only — no real image processing
- Simplified authentication — no OAuth/JWT, no account-level access control
- Single-node deployment — settlement races possible with multiple instances
- No encryption at rest — settlement files and images are unencrypted
- JSON settlement file, not binary X9 ICL format
- EOD cutoff ignores weekends and bank holidays

---

## How Should Apex Evaluate Production Readiness?

**1. State machine correctness under concurrent load**
Submit 50 concurrent deposits to the same account and verify: (a) all reach `funds_posted` exactly once, (b) no duplicate ledger entries exist, (c) `TestOptimisticLock_ConcurrentTransition` passes consistently under load.

**2. Ledger integrity invariant**
For any account, verify `SUM(ledger_entries.amount_cents WHERE sub_type='DEPOSIT') - SUM(ledger_entries.amount_cents WHERE sub_type='REVERSAL') - SUM(ledger_entries.amount_cents WHERE sub_type='RETURN_FEE')` equals the expected account balance. There should never be a DEPOSIT entry for a transfer that is in `Rejected` or `Validating` state.

**3. Settlement exclusion invariant**
Trigger settlement and query: `SELECT COUNT(*) FROM transfers WHERE settlement_batch_id IS NOT NULL AND status NOT IN ('completed', 'returned')`. This must always be zero — no non-completed transfer should have a batch ID.

**4. Vendor stub scenario coverage without code changes**
Submit deposits using each of the 8 test account IDs (`ACC-SOFI-1001` through `ACC-SOFI-0000`, `ACC-RETIRE-001`) and verify the correct downstream behavior for each. Zero configuration or code changes required.

**5. Audit log completeness**
After running `demo-all-scenarios.sh`, query `GET /api/v1/operator/audit` and verify every operator action (approve and reject) has a corresponding audit log entry with the correct operator_id, action type, transfer_id, and timestamp. No operator action should be possible without an audit entry.
