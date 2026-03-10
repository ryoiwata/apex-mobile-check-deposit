# System Architecture: Mobile Check Deposit

## System Overview

This system provides a minimal end-to-end mobile check deposit pipeline for brokerage accounts. Investors submit front and back check images through a React UI along with a deposit amount and account ID. The system validates the images via a deterministic Vendor Service stub (IQA, MICR extraction, OCR, duplicate detection), enforces business rules through a Funding Service (deposit limits, account eligibility, Redis-backed duplicate check), posts provisional credit to a ledger, routes flagged deposits to an operator review queue, and batches approved deposits into X9 ICL settlement files for bank submission.

The backend is a single Go binary with clearly separated internal packages. Every deposit follows a strict state machine (Requested → Validating → Analyzing → Approved → FundsPosted → Completed) with validated transitions, optimistic locking, and an append-only audit trail. No deposit can reach FundsPosted without passing both vendor validation and funding service business rules.

---

## Architecture Diagram

```
┌─────────────────────────────────────────────────────────────────────┐
│                         Client Layer                                 │
│                                                                      │
│   ┌──────────────┐  ┌──────────────────┐  ┌──────────────────────┐  │
│   │ Deposit Form │  │ Operator Dashboard│  │ Ledger / Status View │  │
│   │ (file upload)│  │ (review queue)    │  │ (polling, 2s/5s)     │  │
│   └──────┬───────┘  └────────┬─────────┘  └──────────┬───────────┘  │
│          │                   │                        │              │
└──────────┼───────────────────┼────────────────────────┼─────────────┘
           │ POST /deposits    │ GET /operator/queue    │ GET /ledger
           │ multipart/form    │ POST .../approve       │ GET /deposits
           ▼                   ▼                        ▼
┌─────────────────────────────────────────────────────────────────────┐
│                     API Gateway (Gin Router)                         │
│                                                                      │
│   InvestorAuth middleware  ·  OperatorAuth middleware                │
│   RateLimit middleware (Redis, 10/min per account)                   │
│   gin.Recovery() · JSON envelope responses                          │
└─────────┬──────────────────────────────────────────────┬────────────┘
          │                                              │
          ▼                                              ▼
┌──────────────────────┐                    ┌──────────────────────┐
│   Deposit Service    │                    │   Operator Service   │
│   (pipeline orch.)   │                    │                      │
│                      │                    │ • GetQueue()         │
│ Submit() runs:       │                    │ • Approve()          │
│  1. Create transfer  │                    │ • Reject()           │
│  2. → Validating     │                    │ • GetAuditLog()      │
│  3. Call vendor      │                    └──────────┬───────────┘
│  4. → Analyzing      │                               │
│  5. Apply rules      │                               │
│  6. → FundsPosted    │                               │
└──────────┬───────────┘                               │
           │                                            │
     ┌─────┴──────────┐                                │
     │                │                                │
     ▼                ▼                                ▼
┌──────────┐  ┌──────────────┐           ┌──────────────────────┐
│  Vendor  │  │  Funding     │           │   Settlement Engine  │
│  Stub    │  │  Service     │           │                      │
│          │  │              │           │ RunSettlement()       │
│ Validate │  │ ApplyRules() │           │ • Query FundsPosted  │
│ (IQA,    │  │ • DepositLim │           │ • Generate X9/JSON   │
│  MICR,   │  │ • AccountEli │           │ • Transition →       │
│  OCR,    │  │ • DupeCheck  │           │   Completed          │
│  Dupe)   │  │ • ContribType│           │ • Write batch record │
│          │  │              │           └──────────────────────┘
│ Determin │  │ AccountRes.  │
│ istic by │  │ (DB lookup)  │
│ acct sfx │  │              │
└──────────┘  └──────┬───────┘
                     │
          ┌──────────┴──────────┐
          │                     │
          ▼                     ▼
┌──────────────────┐  ┌──────────────────────┐
│  State Machine   │  │   Ledger Service      │
│                  │  │                       │
│ Transition()     │  │ PostFundsTx()         │
│ • Validates from │  │ • DEPOSIT entry       │
│   → to pair      │  │                       │
│ • UPDATE WHERE   │  │ PostReversal()        │
│   status=from    │  │ • REVERSAL entry      │
│ • INSERT audit   │  │ • RETURN_FEE entry    │
│   row in same tx │  │                       │
└────────┬─────────┘  └──────────┬────────────┘
         │                        │
         └──────────┬─────────────┘
                    │
         ┌──────────┴──────────────────────┐
         │                                  │
         ▼                                  ▼
┌──────────────────────────┐   ┌──────────────────────┐
│       PostgreSQL          │   │        Redis          │
│                          │   │                       │
│ transfers                │   │ dupe:check:{hash}     │
│ ledger_entries (r/o)     │   │   → 90-day TTL        │
│ state_transitions        │   │                       │
│ audit_logs               │   │ ratelimit:{acct}:{min}│
│ accounts + correspondents│   │   → 90s TTL           │
│ settlement_batches       │   │                       │
└──────────────────────────┘   └──────────────────────┘
```

---

## Data Flow — Happy Path

1. **Investor** submits a multipart form (front image, back image, amount_cents, account_id) via React UI or curl.
2. **InvestorAuth middleware** validates the `Authorization: Bearer <token>` header. RateLimit middleware checks Redis counter (max 10/min per account).
3. **Deposit handler** saves images to `/data/images/{transfer_id}/front.png` and `back.png`. Creates a `Transfer` record in `Requested` state.
4. **State machine** transitions `Requested → Validating` in its own transaction.
5. **Vendor stub** validates the request. For a clean-pass account (suffix `*1006` or `*0000`), returns `status: "pass"` with populated MICR data and OCR amount.
6. **State machine** transitions `Validating → Analyzing`. Transfer's vendor fields (MICR routing/account/serial, OCR amount, vendor transaction ID) are updated in the same transaction.
7. **Funding service** applies all rules:
   - Deposit limit check: amount ≤ $5,000 (500,000 cents)
   - Account resolver: queries Postgres for account + correspondent, returns omnibus account ID
   - Contribution type: sets `INDIVIDUAL` for retirement-type accounts
   - Duplicate detection: stores `SHA256(routing+account+amount+serial)` in Redis with 90-day TTL
8. **Single atomic transaction**: state machine transitions `Analyzing → Approved`, ledger service inserts a DEPOSIT entry, state machine transitions `Approved → FundsPosted`. All three writes in one `BEGIN/COMMIT`.
9. **Response** returned to investor: `{status: "funds_posted", transfer_id: "..."}`.
10. **Operator triggers settlement**: `POST /api/v1/settlement/trigger` queries all `FundsPosted` transfers with `created_at ≤ EOD cutoff (6:30 PM CT)` and no existing `settlement_batch_id`.
11. **Settlement engine** creates a `settlement_batches` record, generates an X9 ICL file (or JSON equivalent), then for each eligible transfer transitions `FundsPosted → Completed` and sets `settlement_batch_id`.
12. **Transfer** is now in `Completed` state with ledger entry, settlement batch ID, and full state history.

---

## Data Flow — Return/Reversal

1. **Operator receives return notification** from bank (simulated via `POST /api/v1/operator/deposits/:id/return`).
2. **Return handler** validates transfer is in `Completed` state. Returns 409 if not.
3. **Single atomic transaction**:
   - Ledger service inserts REVERSAL entry: `from=investor, to=omnibus, amount=original_amount, sub_type=REVERSAL`
   - Ledger service inserts RETURN_FEE entry: `from=investor, to=omnibus, amount=3000 ($30), sub_type=RETURN_FEE`
   - State machine transitions `Completed → Returned`
4. **Transfer** ends in `Returned` state with three ledger entries (DEPOSIT, REVERSAL, RETURN_FEE) and a `return_reason`.

---

## Data Flow — Operator Review (Flagged Deposit)

1. **Deposit submitted** with MICR-failure account (suffix `*1003`) or amount-mismatch account (`*1005`).
2. **Vendor stub** returns `status: "flagged"`. Deposit transitions to `Analyzing` with `flagged=true` and a `flag_reason` (micr_failure or amount_mismatch).
3. **Deposit** appears in `GET /api/v1/operator/queue` — operator dashboard polls every 5 seconds.
4. **Operator reviews**: sees check images, MICR data (or null for MICR failure), OCR amount vs. declared amount, flag reason.
5. **Approve path**: `POST /api/v1/operator/deposits/:id/approve`. Single atomic transaction: funding rules run (deposit limit + account + dupe check), `Analyzing → Approved`, DEPOSIT ledger entry posted, `Approved → FundsPosted`, audit log written. Transfer reaches `FundsPosted`.
6. **Reject path**: `POST /api/v1/operator/deposits/:id/reject`. Transaction: `Analyzing → Rejected`, audit log written with operator ID, reason, and notes.
7. **Audit log** persists operator_id, action, timestamp, notes, and before/after state. Queryable via `GET /api/v1/operator/audit`.

---

## Transfer State Machine

```
                                    [vendor pass or flagged]
                                           │
Requested ──────────────────── Validating ─┤
                                           │ [vendor fail: IQA, dupe]
                                           │
                                           ├──────────────── Rejected ◀── Analyzing
                                           │                               │
                                           └──── Analyzing ────────────────┤
                                                     │                     │ [rules fail]
                                                     │ [rules pass]        │
                                                     │ [operator approve]  │ [operator reject]
                                                     ▼
                                                  Approved
                                                     │
                                                     ▼
                                              FundsPosted
                                                     │ [EOD settlement]
                                                     ▼
                                                 Completed
                                                     │ [check bounced]
                                                     ▼
                                                  Returned
```

| Transition | Trigger | Atomic with |
|-----------|---------|------------|
| Requested → Validating | POST /deposits received | own tx |
| Validating → Analyzing | Vendor returns pass or flagged | vendor data UPDATE |
| Validating → Rejected | Vendor returns fail | — |
| Analyzing → Approved | Funding rules pass | Approved→FundsPosted + ledger POST |
| Analyzing → Rejected | Funding rules fail or operator reject | audit log |
| Approved → FundsPosted | Ledger entry posted | Analyzing→Approved + ledger POST |
| FundsPosted → Completed | Settlement batch processed | settlement_batch_id UPDATE |
| Completed → Returned | Return notification received | REVERSAL + RETURN_FEE ledger entries |

All transitions use optimistic locking: `UPDATE transfers SET status=$1 WHERE id=$2 AND status=$3` — if 0 rows affected, another process beat us to it, and `ErrInvalidStateTransition` is returned.

---

## Database Schema

| Table | Purpose |
|-------|---------|
| `transfers` | Central entity. One row per deposit. Tracks status, MICR data, image refs, vendor result, settlement batch. |
| `ledger_entries` | **Append-only.** One DEPOSIT entry per approved transfer. Two entries (REVERSAL + RETURN_FEE) per returned transfer. Never updated or deleted. |
| `state_transitions` | Full audit trail of every state change. One row per transition. Contains from/to state, triggeredBy, metadata (JSONB). |
| `audit_logs` | Operator action log. One row per approve/reject/override. Contains operator_id, notes, transfer_id. |
| `settlement_batches` | One record per EOD settlement run. Tracks file path, deposit count, total amount, bank acknowledgment status. |
| `accounts` | Seed data for 8 investor accounts across 3 correspondents. Determines vendor stub scenario via ID suffix. |
| `correspondents` | 3 broker-dealers (SoFi, Webull, CashApp) with omnibus account IDs. Used by funding service account resolver. |

The ledger is strictly append-only. Reversals create new entries; they never modify the original DEPOSIT entry. This is enforced at the application layer (no UPDATE/DELETE in `ledger.Repository`) and noted in the schema (no DELETE cascade on ledger_entries).

---

## Service Boundaries

The system is a **monolith with clear internal package boundaries**, not microservices. All seven services run in a single Go binary under `internal/`.

**Why monolith for this project:** Microservices would require service discovery, network serialization, distributed tracing, and multiple Docker containers — operational overhead that does not serve the evaluation goal. The internal package structure (`vendor/`, `funding/`, `ledger/`, `state/`, `settlement/`, `operator/`, `deposit/`) provides the same separation of concerns without the complexity.

**Production path:** Each internal package is designed with an interface boundary (e.g., `vendor.Service`, `ledger.Repository`) that could be promoted to a separate service. The synchronous pipeline would become Kafka/Pub-Sub event sourcing. State transitions would publish events; downstream services would consume and act. This aligns with Apex's existing Kafka and gRPC infrastructure.

---

## Authentication Model

**Current (demo):** Two static tokens read from environment variables:
- `INVESTOR_TOKEN` — used in `Authorization: Bearer <token>` header for all deposit and ledger endpoints
- `OPERATOR_TOKEN` — `X-Operator-ID` header value accepted by operator endpoints; also used for audit logging

The `InvestorAuth` middleware validates the token against the configured value. The `OperatorAuth` middleware extracts `X-Operator-ID` into the Gin context for audit logging. Account-level access control is not enforced (any token can query any account — noted as a known limitation).

**Production path:** Real system would use OAuth/JWT with tokens issued by the correspondent's identity provider (e.g., SoFi's auth service). The JWT payload would contain `account_id` and `correspondent_id`, which would be used to scope all database queries. Apex's existing auth infrastructure handles this via gRPC interceptors.
