# Risks and Limitations

This document describes known limitations of the current implementation. These are deliberate trade-offs made within the scope of a one-week technical assessment, not oversights.

---

## Stubbed Vendor Integration Only

**Risk:** No real check image processing, MICR reading, OCR, or duplicate detection at the image layer.

**Impact:** The vendor stub returns predetermined responses keyed by account ID suffix. The images uploaded by the investor are saved to disk and embedded in the settlement file, but their contents are never analyzed. A real deployment requires integration with a vendor such as Mitek, FIS, or NCR for real-time image quality assessment and MICR extraction.

**Mitigation in place:** The `vendor.Service` interface allows swapping the stub for a real HTTP client with zero changes to the pipeline logic.

---

## Simplified Authentication

**Risk:** Authentication is based on a static bearer token from an environment variable. Any request with the correct token can access any account's data.

**Impact:** No account-level access control. One investor token grants access to all deposits across all accounts. The operator token validates only that the header is present — it does not verify identity.

**Mitigation in place:** Auth middleware is wired and required. Anonymous requests are rejected (401). Operator actions are logged with the `X-Operator-ID` header value.

**Production path:** OAuth/JWT with tokens issued by the correspondent's identity provider. Token claims would contain `account_id` and `correspondent_id` for query scoping.

---

## Single-Node Deployment

**Risk:** No horizontal scaling, no distributed locking for settlement, no leader election for batch processing.

**Impact:** Running two backend instances simultaneously would cause settlement races (both instances could try to batch the same transfers). State machine optimistic locking handles concurrent transitions correctly, but the settlement batch selection query (`WHERE settlement_batch_id IS NULL`) is not atomic across instances.

**Mitigation in place:** Optimistic locking on state transitions (tested in `TestOptimisticLock_ConcurrentTransition`). Settlement uses a batch record creation before processing transfers.

**Production path:** Kubernetes Deployment with a single settlement worker pod, or `SELECT FOR UPDATE SKIP LOCKED` to claim transfers for a specific batch job.

---

## No Encryption at Rest

**Risk:** Check images stored on a Docker named volume (`mcd-images`) and settlement files stored in another volume (`mcd-settlement`) are unencrypted.

**Impact:** A host-level compromise exposes check image files and financial settlement data. This would be a PCI-DSS violation in production.

**Mitigation in place:** Non-root container user (`mcd`) with restricted filesystem permissions. Settlement output directory is configurable via environment variable.

**Production path:** Images stored in S3/GCS with server-side encryption. Settlement files encrypted at rest and in transit. Key management via AWS KMS or equivalent.

---

## Synthetic Data Only

**Risk:** The system is built and tested with synthetic account IDs, routing numbers, and check images. No real PII, bank account numbers, or live check images are used.

**Impact:** Not a risk for this assessment — this is a deliberate design requirement. The synthetic data prevents any accidental handling of real financial data during development and evaluation.

**Production path:** Real data would flow through the vendor service. Check images would be encrypted. MICR data would be masked in all logs.

---

## No Compliance or Regulatory Claims

**Risk:** This system has not been reviewed for compliance with Regulation CC (check holds), Regulation E (electronic transfers), NACHA rules, PCI-DSS, or FFIEC guidelines.

**Impact:** Not applicable for a technical assessment. Do not deploy to production without a compliance review.

---

## EOD Cutoff Simplified

**Risk:** The 6:30 PM CT EOD cutoff does not account for weekends, federal holidays, or bank processing calendars.

**Impact:** A deposit submitted at 5 PM on a Friday would be included in that day's batch, even though the bank won't process it until Monday. A deposit submitted on Christmas would behave identically to any other day.

**Mitigation in place:** The spec explicitly excludes weekend/holiday handling from MVP scope. The cutoff logic uses `America/Chicago` timezone correctly (including DST transitions — tested in `TestCutoffTime_DST_Summer`).

**Production path:** Use a bank processing calendar library or API to determine the next valid settlement date. NACHA provides ACH processing calendars.

---

## Redis Duplicate Check Gap

**Risk:** If Redis is unavailable during a deposit submission, the duplicate check is skipped (graceful degradation). If Redis recovers later and another deposit arrives for the same check, the second deposit will be treated as the first (hash not found) and allowed through.

**Impact:** A window exists where the same check could be deposited twice if Redis was down for the first deposit. The vendor stub's `*1004` account suffix provides vendor-level duplicate detection, but funding-level detection has this gap.

**Mitigation in place:** Graceful degradation is logged as a warning. The vendor stub marks check `*1004` as a duplicate regardless of Redis state.

**Production path:** Use a Redis Sentinel or Cluster setup for HA. Add a Postgres fallback duplicate check (slower but reliable) when Redis is unavailable. Consider storing check hash in the `transfers` table as a unique index for durable deduplication.

---

## Settlement File Partially Applied on Failure

**Risk:** The settlement engine generates the X9/JSON file first, then transitions each transfer from `FundsPosted → Completed` individually. If the process crashes after generating the file but before completing all transitions, the settlement file exists but not all transfers are marked `Completed`.

**Impact:** Manual reconciliation required. The settlement file accurately reflects the intended batch (it was generated from the correct set of transfers), but the database state may not match.

**Mitigation in place:** `settlement_batch_id` is set on each transfer atomically with its `FundsPosted → Completed` transition. Transfers that were successfully transitioned are correctly marked. Untransitioned transfers will be picked up by the next settlement run (they still have `settlement_batch_id IS NULL`).

**Production path:** Two-phase commit: first claim all transfers into a batch (set `settlement_batch_id`), then generate the file, then mark `Completed`. Use a saga with compensation: if file generation fails, release the batch claim. If transitions partially fail, a reconciliation job retries.

---

## Multi-Transaction Gap in Deposit Pipeline

**Risk:** The deposit pipeline uses separate transactions for each state transition before the critical section. Specifically: `Requested → Validating` commits before the vendor call. If the server crashes after that commit but before the vendor response, the transfer is stuck in `Validating`.

**Impact:** Orphaned transfers in intermediate states. A resubmit attempt by the investor would create a new transfer (no idempotency key), while the original sits in `Validating` indefinitely.

**Mitigation in place:** The critical section (`Analyzing → Approved → FundsPosted` + ledger posting) is correctly atomic. The intermediate steps are less critical — a stuck `Validating` transfer doesn't represent posted funds.

**Production note:** This is documented in the deposit service code. Production would use a saga pattern or outbox table to make each pipeline step idempotent and recoverable.

---

## No Account-Scoped Queries

**Risk:** The investor token does not scope queries to a specific account. `GET /api/v1/deposits` returns all deposits in the system, not just those for the authenticated account. `GET /api/v1/ledger/:account_id` accepts any account ID.

**Impact:** An investor could view another investor's deposits and ledger by guessing or enumerating transfer IDs and account IDs.

**Mitigation in place:** Not mitigated in this implementation. This is a known simplification documented in the pre-search document.

**Production path:** Bind `account_id` to the auth context from the JWT claims. Filter all deposit and ledger queries with `WHERE account_id = $jwt.account_id`.
