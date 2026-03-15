# System Architecture: Mobile Check Deposit

## System Overview

This system provides a minimal end-to-end mobile check deposit pipeline for brokerage accounts. Investors submit front and back check images through a React UI along with a deposit amount and account ID. The system validates the images via a deterministic Vendor Service stub (IQA, MICR extraction, OCR, duplicate detection), enforces business rules through a Funding Service using a **collect-all validation approach** (deposit limits, account eligibility, contribution caps, Redis-backed duplicate check), posts provisional credit to a ledger, routes flagged deposits to an operator review queue, and batches approved deposits into X9 ICL settlement files for bank submission.

The collect-all approach evaluates every business rule regardless of prior failures and returns the full list of violations. From a UX standpoint, this prevents the frustrating loop where an investor fixes one issue, resubmits, and immediately hits a different rejection they weren't told about.

The backend is a single Go binary with clearly separated internal packages. Every deposit follows a strict state machine (Requested → Validating → Analyzing → Approved → FundsPosted → Completed) with validated transitions, optimistic locking, and an append-only audit trail. No deposit can reach FundsPosted without passing both vendor validation and funding service business rules. Every non-terminal error path loops back to allow the investor to correct and resubmit — IQA failures prompt retake, business rule violations surface all issues at once for correction, operator rejections allow resubmission, and returned checks allow new deposits.

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
│  1. Validate session │                    │ • Reject()           │
│  2. Create transfer  │                    │ • GetAuditLog()      │
│  3. → Validating     │                    │ • OverrideContrib()  │
│  4. Call vendor      │                    └──────────┬───────────┘
│  5. → Analyzing      │                               │
│  6. Apply ALL rules  │◄──── Collect-All              │
│  7. → FundsPosted    │      (parallel eval)          │
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
│ (IQA,    │  │ COLLECT-ALL: │           │ • Check EOD cutoff   │
│  MICR,   │  │ ├ DepositLim │           │ • Generate X9/JSON   │
│  OCR,    │  │ ├ AccountEli │           │ • Submit to bank     │
│  Dupe)   │  │ ├ ContribCap │           │ • Retry on ACK fail  │
│          │  │ └ DupeCheck  │           │ • Transition →       │
│ Returns: │  │ (all at once)│           │   Completed          │
│ pass/    │  │              │           │ • Write batch record │
│ fail/    │  │ AccountRes.  │           └──────────────────────┘
│ flagged  │  │ (DB lookup)  │
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
3. **Session validation**: Funding Service validates the investor session and account eligibility. If the session is invalid, the investor is prompted to re-authenticate and loops back to retry.
4. **Deposit handler** saves images to `/data/images/{transfer_id}/front.png` and `back.png`. Creates a `Transfer` record in `Requested` state.
5. **State machine** transitions `Requested → Validating` in its own transaction.
6. **Vendor stub** validates the request. For a clean-pass account (suffix `*1006` or `*0000`), returns `status: "pass"` with populated MICR data and OCR amount.
7. **State machine** transitions `Validating → Analyzing`. Transfer's vendor fields (MICR routing/account/serial, OCR amount, vendor transaction ID) are updated in the same transaction.
8. **Funding service** applies **all rules in parallel using the collect-all approach**:
   - Deposit limit check: amount ≤ $5,000 (500,000 cents)
   - Account resolver: queries Postgres for account + correspondent, returns omnibus account ID
   - Contribution type: sets `INDIVIDUAL` for retirement-type accounts
   - Duplicate detection: stores `SHA256(routing+account+amount+serial)` in Redis with 90-day TTL
   - **All rules are evaluated regardless of individual failures. If any fail, the complete list of violations is returned to the investor in a single response.**
9. **Single atomic transaction**: state machine transitions `Analyzing → Approved`, ledger service inserts a DEPOSIT entry, state machine transitions `Approved → FundsPosted`. All three writes in one `BEGIN/COMMIT`.
10. **Response** returned to investor: `{status: "funds_posted", transfer_id: "..."}`.
11. **Operator triggers settlement**: `POST /api/v1/settlement/trigger` queries all `FundsPosted` transfers with `created_at ≤ EOD cutoff (6:30 PM CT)` and no existing `settlement_batch_id`.
12. **Settlement engine** creates a `settlement_batches` record, generates an X9 ICL file (or JSON equivalent), submits to Settlement Bank, and tracks acknowledgment.
13. **Bank acknowledgment**: If acknowledged, each eligible transfer transitions `FundsPosted → Completed` with `settlement_batch_id` set. If not acknowledged, the settlement engine retries submission or alerts operations. The retry loops back to the submission step.
14. **Transfer** is now in `Completed` state with ledger entry, settlement batch ID, and full state history.

---

## Data Flow — Loop-Back / Retry Paths

All non-terminal error states loop back to allow correction and resubmission. No path is a dead end unless the deposit is explicitly finalized (Completed) or the investor chooses not to retry.

### IQA Failure Loop (Investor → Vendor → Investor)
1. Investor submits check images.
2. Vendor stub returns IQA failure (blur, glare, etc.) with actionable error message.
3. Investor receives specific guidance (e.g., "Image too blurry — retake in better lighting").
4. **Loop back**: Investor retakes photo and resubmits → re-enters the flow at the capture step.

### Collect-All Business Rule Failure Loop (Investor → Funding → Investor)
1. Validated deposit reaches the Funding Service.
2. Funding Service evaluates ALL business rules in parallel (deposit limit, contribution cap, duplicate check, account eligibility).
3. **ALL violations returned at once** — e.g., "Over $5,000 limit AND duplicate deposit detected."
4. Investor fixes all reported issues (adjusts amount, selects different account, etc.).
5. **Loop back**: Investor resubmits corrected deposit → re-enters the flow at the submission step.

### Operator Rejection Loop (Operator → Investor)
1. Flagged deposit enters the operator review queue.
2. Operator reviews check images, MICR data, risk scores, and amount comparison.
3. Operator rejects the deposit with a logged reason.
4. Investor is notified of rejection with the reason.
5. **Loop back**: Investor may initiate a new deposit → re-enters the flow at the capture step.

### Settlement Retry Loop (Settlement → Bank → Settlement)
1. Settlement engine generates X9 ICL file and submits to Settlement Bank.
2. Bank does not acknowledge within the expected window.
3. Settlement engine monitors for the missing acknowledgment.
4. **Loop back**: Settlement engine retries submission or escalates to operations → re-enters the flow at the bank submission step.

### EOD Cutoff Roll Loop (Settlement → Next Business Day)
1. Approved deposits are queued for settlement.
2. System checks if the current time is before the 6:30 PM CT cutoff.
3. Deposits after cutoff are rolled to the next business day.
4. **Loop back**: Deposits wait until the next EOD cycle → re-enter the settlement flow at the cutoff check.

### Return / New Deposit Loop (Settlement → Investor)
1. Settled deposit is returned/bounced by the bank.
2. Reversal postings created (original amount debit + $30 fee debit).
3. Investor notified of the return and fee.
4. **Loop back**: Investor may initiate a new deposit with a different check → re-enters the flow at the capture step.

---

## Data Flow — Return/Reversal

1. **Operator receives return notification** from bank (simulated via `POST /api/v1/operator/deposits/:id/return`).
2. **Return handler** validates transfer is in `Completed` state. Returns 409 if not.
3. **Single atomic transaction**:
   - Ledger service inserts REVERSAL entry: `from=investor, to=omnibus, amount=original_amount, sub_type=REVERSAL`
   - Ledger service inserts RETURN_FEE entry: `from=investor, to=omnibus, amount=3000 ($30), sub_type=RETURN_FEE`
   - State machine transitions `Completed → Returned`
4. **Transfer** ends in `Returned` state with three ledger entries (DEPOSIT, REVERSAL, RETURN_FEE) and a `return_reason`.
5. **Investor notified** of the returned check and fee deduction.
6. **Loop back**: Investor may initiate a new deposit with a different check.

---

## Data Flow — Operator Review (Flagged Deposit)

1. **Deposit submitted** with MICR-failure account (suffix `*1003`) or amount-mismatch account (`*1005`).
2. **Vendor stub** returns `status: "flagged"`. Deposit transitions to `Analyzing` with `flagged=true` and a `flag_reason` (micr_failure or amount_mismatch).
3. **Deposit** appears in `GET /api/v1/operator/queue` — operator dashboard polls every 5 seconds.
4. **Operator reviews**: sees check images, MICR data (or null for MICR failure), OCR amount vs. declared amount, flag reason, risk indicators, and confidence scores.
5. **Contribution type override** (optional): Before approving, operator may override the default contribution type if the system-assigned type is incorrect (e.g., for retirement accounts). This is a separate decision step.
6. **Approve path**: `POST /api/v1/operator/deposits/:id/approve`. Single atomic transaction: funding rules run (deposit limit + account + dupe check), `Analyzing → Approved`, DEPOSIT ledger entry posted, `Approved → FundsPosted`, audit log written. Transfer reaches `FundsPosted`.
7. **Reject path**: `POST /api/v1/operator/deposits/:id/reject`. Transaction: `Analyzing → Rejected`, audit log written with operator ID, reason, and notes.
8. **After approval**: Operator proceeds to the next item in the queue. If the queue is empty, the session ends. **Loop back**: Queue cycling — operator returns to the queue view and selects the next flagged deposit.
9. **After rejection**: Investor is notified and may resubmit a new deposit. **Loop back**: Rejected investor re-enters the flow at the capture step.
10. **Audit log** persists operator_id, action, timestamp, notes, and before/after state. Queryable via `GET /api/v1/operator/audit`.

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
                                                     │                     │ [collect-all: rules fail]
                                                     │ [all rules pass]    │
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

Loop-back paths (new deposit, not state transition):
  Rejected ──(investor resubmits)──▶ new Requested
  Returned ──(investor resubmits)──▶ new Requested
  IQA fail ──(investor retakes)────▶ new Requested
  Rule fail ──(investor fixes all)──▶ new Requested
```

| Transition | Trigger | Atomic with |
|-----------|---------|------------|
| Requested → Validating | POST /deposits received | own tx |
| Validating → Analyzing | Vendor returns pass or flagged | vendor data UPDATE |
| Validating → Rejected | Vendor returns fail | — |
| Analyzing → Approved | Funding rules pass (collect-all: all rules OK) | Approved→FundsPosted + ledger POST |
| Analyzing → Rejected | Funding rules fail (collect-all: any rule fails) or operator reject | audit log |
| Approved → FundsPosted | Ledger entry posted | Analyzing→Approved + ledger POST |
| FundsPosted → Completed | Settlement batch processed, bank acknowledged | settlement_batch_id UPDATE |
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

**Session validation loop-back:** If the investor's session is invalid or expired, the system returns an authentication error. The investor is prompted to re-authenticate and can then retry the deposit submission. This loop-back prevents the deposit from being silently dropped due to an expired session.

**Production path:** Real system would use OAuth/JWT with tokens issued by the correspondent's identity provider (e.g., SoFi's auth service). The JWT payload would contain `account_id` and `correspondent_id`, which would be used to scope all database queries. Apex's existing auth infrastructure handles this via gRPC interceptors.
