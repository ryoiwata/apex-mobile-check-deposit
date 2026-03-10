# Decision Log: Mobile Check Deposit System

Key architectural and implementation decisions made during development. Each entry records what was chosen, what was considered, and why — plus what would change in a production system.

---

## Language

**Choice:** Go 1.22+

**Alternatives considered:** Java + Spring Boot

**Rationale:** Apex's platform is described as "mostly Golang." Go's concurrency model (goroutines, channels) is well-suited for the async state transitions and batch settlement processing this system needs. Smaller Docker image footprint (multi-stage build produces a ~10MB binary vs. a JVM container). Faster compile/feedback loop for a one-week project. The `go test` standard library is sufficient without a testing framework.

**Production note:** No change. Apex's production services are Go. This decision is already aligned.

---

## HTTP Framework

**Choice:** Gin

**Alternatives considered:** Chi, Echo, stdlib `net/http`

**Rationale:** Gin has the largest Go web framework community. Strong middleware ecosystem. Evaluators and reviewers will recognize it immediately. Chi and Echo are both excellent, but Gin's `c.ShouldBindJSON()`, `c.SaveUploadedFile()`, and group-based routing were directly useful for this project. Echo's slightly cleaner API was considered but Gin's wider familiarity won.

**Production note:** No change. Gin is appropriate for REST APIs at this scale. For internal gRPC services, Apex's existing gRPC infrastructure would be used instead.

---

## Database

**Choice:** PostgreSQL 16

**Alternatives considered:** SQLite

**Rationale:** Foreign key constraints across `transfers → ledger_entries → audit_logs` enforce referential integrity without application-layer checks. ACID transactions are required for the reversal posting (original debit + fee debit + state update must be atomic). JSONB columns on `state_transitions.metadata` allow flexible context without schema changes. PostgreSQL signals production thinking to evaluators — SQLite would signal a toy project. Apex runs Postgres in production.

**Production note:** No change. Would add connection pooling (PgBouncer), read replicas for reporting queries, and partitioning on `transfers` by `created_at` at high volume.

---

## Cache / Duplicate Detection

**Choice:** Redis with SHA256 hash + 90-day TTL

**Alternatives considered:** Postgres-only duplicate check (query `transfers` table), in-memory map

**Rationale:** Redis `SETNX` provides atomic check-and-set in O(1). The 90-day TTL is a business requirement. Postgres would require a separate index on check hash and wouldn't give us TTL semantics without a cron job. In-memory would not survive restarts. Redis is also used for rate limiting (sliding window counter), so it's already in the stack.

**Graceful degradation:** If Redis is unavailable, the duplicate check logs a warning and continues processing. This is a deliberate trade-off for demo availability — production would fail-closed or use a Postgres fallback.

**Production note:** Redis Cluster for HA. Consider adding the duplicate hash to the `transfers` table as well for a permanent audit record (Redis TTL means the hash disappears after 90 days, which may be too short for compliance requirements).

---

## Synchronous Pipeline vs. Async/Event-Driven

**Choice:** Synchronous. `POST /deposits` runs `Requested → FundsPosted` in a single HTTP request.

**Alternatives considered:** Kafka/Pub-Sub event sourcing. Each state transition publishes an event; downstream services consume asynchronously.

**Rationale:** Synchronous is simpler to implement, debug, and demo. The evaluator can observe the full lifecycle in a single API call. Async would require Kafka in Docker Compose, consumer group management, and offset tracking — substantial infrastructure overhead with no rubric benefit.

**Production note:** Apex runs Kafka and Pub/Sub. A production system would use event-driven state transitions. `DepositSubmitted → VendorValidationRequested → FundingAnalysisRequested → LedgerPostingRequested` — each published to a topic, each consumed by the appropriate service. This provides resilience (each step retries independently), observability (events are the audit trail), and horizontal scaling (multiple consumer instances). The state machine's transition validation logic would move to each consumer.

---

## In-Process Vendor Stub vs. Separate HTTP Service

**Choice:** In-process function call. `vendor.Service` interface with `Stub` implementation in the same binary.

**Alternatives considered:** Separate HTTP service on a different port, mock server via Docker Compose

**Rationale:** An in-process stub is deterministic and zero-latency. The `vendor.Service` interface means swapping the stub for a real HTTP client requires changing one line in `main.go` (the DI wiring). A separate HTTP service would add Docker Compose complexity, network error handling, and latency — none of which serve the demo goal.

**Production note:** Replace `vendor.NewStub()` in `main.go` with `vendor.NewHTTPClient(cfg.VendorBaseURL)`. The `Validate(ctx, req)` interface contract is identical.

---

## State Machine Locking Strategy

**Choice:** Optimistic locking via `UPDATE transfers SET status=$1 WHERE id=$2 AND status=$3`. If 0 rows affected → conflict.

**Alternatives considered:** `SELECT FOR UPDATE` (pessimistic locking), application-level mutex (Redis SETNX on transfer ID)

**Rationale:** The `WHERE status = expected` pattern is lockless until contention occurs. For a single-node demo, there is rarely contention. When contention does occur (tested in `TestOptimisticLock_ConcurrentTransition`), exactly one goroutine wins and the other gets `ErrInvalidStateTransition`. Pessimistic locking (`SELECT FOR UPDATE`) holds a row lock for the duration of the transaction, which would cause more contention under load. Redis SETNX adds a dependency and a failure mode.

**Production note:** At high concurrency, optimistic locking leads to more retries. For settlement batching specifically (multiple transfers being transitioned simultaneously), consider a queue-based approach where the settlement service claims transfers in batches with a `FOR UPDATE SKIP LOCKED` query.

---

## Operator Approve: Auto-Post in Same Transaction

**Choice:** When operator approves a flagged deposit, the system runs `Analyzing → Approved → FundsPosted` + ledger POST + audit log all in one Postgres transaction.

**Alternatives considered:** Approve as a separate step (→ Approved only), then a background job transitions to FundsPosted.

**Rationale:** Consistent with the synchronous pipeline design. The operator's approval intent is to get the deposit posted — making this a two-step async process adds complexity with no benefit for the demo. The single transaction means the operator either sees `funds_posted` immediately or an error (nothing stuck in a half-approved state).

**Production note:** Same decision, but would add a compensating saga for the case where the ledger service is temporarily unavailable. The approval would be recorded and the posting retried via a background reconciliation job.

---

## Contribution Type: In `transfers` Table

**Choice:** `contribution_type` is a column on the `transfers` table, set during `Analyzing` by the funding rules engine.

**Alternatives considered:** On `ledger_entries`, as metadata in `state_transitions.metadata`

**Rationale:** The operator needs to be able to override the contribution type during approval (for retirement accounts where the investor chose the wrong type). An override must update the transfer before the ledger entry is created. Since ledger entries are append-only, the contribution type must live on the transfer. Storing it on `state_transitions.metadata` would work for audit but makes it hard to query.

**Production note:** No change. This is the correct place for a field that can be overridden before posting.

---

## Reversal Entries: Both from Investor → Omnibus

**Choice:** Both the REVERSAL entry and the RETURN_FEE entry debit the investor (`from_account_id = investor`) and credit the omnibus account (`to_account_id = omnibus`).

**Alternatives considered:** RETURN_FEE going to a dedicated fee revenue account, not the omnibus

**Rationale:** For a demo, both entries following the same direction (investor → omnibus) is simpler and consistent. The original DEPOSIT was omnibus → investor; the reversal mirrors it. The fee going to omnibus is a simplification — in a real system, fees would go to a revenue account.

**Production note:** The `to_account_id` on the RETURN_FEE entry would be a correspondent-specific fee account, not the omnibus. This requires adding a `fee_account_id` to the correspondent configuration.

---

## EOD Cutoff Based on `created_at`, Not `updated_at`

**Choice:** Settlement queries use `created_at <= cutoff AND settlement_batch_id IS NULL`.

**Alternatives considered:** `updated_at <= cutoff` (last state change time)

**Rationale:** `created_at` represents when the investor submitted the deposit — the time the investor's banking day ended. `updated_at` changes every time any status update happens, which could mean a deposit submitted at 5 PM gets excluded from the day's batch because the operator approved it at 7 PM. Using `created_at` correctly captures the investor's submission time.

**Production note:** No change. This is the correct semantics. Would add explicit handling for deposits submitted on weekends/holidays (roll to next business day).

---

## X9 Images: Read Real Uploaded Bytes

**Choice:** Settlement generator reads actual image bytes from `/data/images/{transfer_id}/front.png` and `/data/images/{transfer_id}/back.png` and embeds them in the X9 ICL record.

**Alternatives considered:** Placeholder bytes, URL references only

**Rationale:** The X9 ICL format requires actual image data in the `ImageViewData` records. Sending a settlement file with empty image data would be meaningless. The uploaded images (even dummy PNGs from the demo scripts) are real bytes that produce a valid X9 structure.

**Production note:** Images would be stored in S3 (or equivalent object store) with signed URLs. The settlement generator would fetch images from S3 using the `front_image_ref` and `back_image_ref` paths stored on the transfer. Encryption at rest and in transit required for PCI-DSS compliance.

---

## Settlement File Format: JSON Fallback Used

**Choice:** JSON settlement file (structured JSON matching the X9 logical structure).

**Alternatives considered:** X9 ICL binary format via `moov-io/imagecashletter`

**Rationale:** `moov-io/imagecashletter` was investigated. The library's `ImagCashLetter` writer API works, but the X9 format has strict field-width constraints (e.g., MICR routing must be exactly 9 digits, amount fields are fixed-width) that require careful zero-padding of the test data. Given the time budget, the JSON fallback produces equivalent information for evaluation purposes. The spec explicitly allows "X9 ICL file or structured JSON equivalent." The `generator.go` file documents exactly what the X9 structure would contain and why JSON was used.

**Production note:** Replace `generator.go` with a proper moov-io integration. All the field mapping logic is already written — it would move from JSON serialization to `imagecashletter.CheckDetail` struct population. The `Generate()` function signature is identical; only the implementation changes.

---

## Token Auth from `.env` over Login Endpoint

**Choice:** `INVESTOR_TOKEN` and `OPERATOR_TOKEN` environment variables. Auth middleware validates the header matches the configured value.

**Alternatives considered:** Login endpoint returning JWT, hardcoded tokens in source code

**Rationale:** The rubric allocates 0 points to authentication correctness. A login endpoint would require user management, session storage, and logout logic — none of which serves any rubric goal. Environment-variable tokens are simple, secure (not in source code), and sufficient for demo.

**Production note:** OAuth/JWT via the correspondent's identity provider. Apex's existing auth service handles token issuance and validation. The `InvestorAuth` middleware would call the auth service (gRPC) to validate the JWT and extract the `account_id` claim.

---

## Auto-Run Migrations at Startup

**Choice:** `db.RunMigrations(db)` called in `main.go` before starting the HTTP server.

**Alternatives considered:** Separate `make migrate` command, manual Flyway/Liquibase setup

**Rationale:** One-command setup is a rubric goal. Requiring a separate migration step before `docker compose up` would break the `docker compose up --build` → ready in 30 seconds experience. Migrations are idempotent (`CREATE TABLE IF NOT EXISTS`, `ON CONFLICT DO NOTHING` for seed data), so running them on every startup is safe.

**Production note:** Use a dedicated migration tool (Flyway, golang-migrate) with a separate deployment step. Auto-running migrations on startup creates a race condition in multi-instance deployments (two instances both trying to migrate simultaneously). Use a distributed lock or a separate migration job in CI/CD.

---

## Polling vs. WebSockets for Operator Queue

**Choice:** `setInterval` polling in React. Operator queue polls every 5 seconds. Transfer status polls every 2 seconds while non-terminal.

**Alternatives considered:** WebSocket connection, Server-Sent Events (SSE)

**Rationale:** Polling is simpler to implement and sufficient for a single-operator demo. WebSockets require a persistent connection handler on the Go side, proper close handling, and reconnect logic on the React side. For one evaluator running the demo, the 5-second poll latency is acceptable.

**Production note:** SSE or WebSockets for the operator queue. A deposit flagged at 10:00:00 AM should appear in the operator's queue at 10:00:01 AM, not 10:00:05 AM. Gin supports SSE via streaming responses. The state machine's transition logging could publish to a Redis Pub/Sub channel that the SSE handler subscribes to.

---

## Manual Settlement Trigger vs. Cron

**Choice:** `POST /api/v1/settlement/trigger` API endpoint. No background scheduler.

**Alternatives considered:** Cron job at 6:30 PM CT, Go ticker in `main.go`

**Rationale:** Manual trigger is deterministic for demo purposes. The evaluator can run settlement at any time, on any batch date, and observe the exact result. A cron job would mean settlement only runs at 6:30 PM CT — inconvenient for evaluation. A Go ticker in `main.go` adds background goroutine lifecycle management.

**Production note:** Both. Add a cron-style scheduler (Kubernetes CronJob or equivalent) that calls the settlement endpoint at 6:30 PM CT daily. Keep the manual trigger for operator override and emergency processing. Idempotency is already enforced (`settlement_batch_id IS NULL` filter).

---

## Named Docker Volumes vs. Bind Mounts

**Choice:** Named volumes (`postgres-data`, `redis-data`, `image-data`, `settlement-data`) in `docker-compose.yml`.

**Alternatives considered:** Bind mounts (`./data/images:/data/images`)

**Rationale:** Named volumes are cross-platform (no Windows path separator issues), owned by Docker (correct file permissions), and easier to clean up (`docker compose down -v`). Bind mounts expose host filesystem paths which vary by OS and can cause permission issues in the Docker container when running as a non-root user.

**Production note:** Object storage (S3/GCS) for images and settlement files. Named volumes are not appropriate for production — they don't survive host failures and can't be shared across instances.

---

## Vendor Stub: Account Suffix Mapping

**Choice:** Last 4 characters of `account_id` determine the stub response. `ACC-SOFI-1003` → MICR failure.

**Alternatives considered:** Request header (`X-Vendor-Scenario`), environment variable, separate config file, random mode

**Rationale:** Account suffix is deterministic (same account always gets same result), self-documenting (the test shows which account → which scenario), requires zero code changes or environment tweaks to switch scenarios, and is composable (tests can run in any order with no shared state).

**Production note:** The `vendor.Service` interface is replaced with an HTTP client pointing at the real vendor API. The stub is only used in testing.

---

## Monolith with Packages vs. Microservices

**Choice:** Single Go binary with `internal/vendor`, `internal/funding`, `internal/ledger`, etc. packages.

**Alternatives considered:** Separate microservices per domain (vendor-service, funding-service, ledger-service), each in its own Docker container

**Rationale:** Microservices would require service discovery (Consul/k8s DNS), network serialization (protobuf/JSON), distributed tracing, and 4+ Docker containers in Compose. The operational overhead is significant and the rubric rewards correct behavior, not deployment topology. The internal package structure provides identical separation of concerns.

**Production note:** Promote each package to a microservice when team size or release velocity requires it. The interface boundaries are already in place. The synchronous call `vendorSvc.Validate(ctx, req)` would become a gRPC call to the vendor service; `fundingSvc.ApplyRules(ctx, transfer, resp)` would become a gRPC call to the funding service.

---

## Money Representation: int64 Cents

**Choice:** All monetary amounts stored and computed as `int64` cents. $5,000 = 500,000. $30 fee = 3,000.

**Alternatives considered:** `float64`, `decimal.Decimal` (shopspring/decimal library)

**Rationale:** `float64` has well-documented rounding issues in financial systems ($0.10 + $0.20 ≠ $0.30 in IEEE 754). `decimal.Decimal` is correct but adds a dependency and requires discipline to use consistently. `int64` cents is simple, efficient, and the standard practice in financial systems (ISO 4217 minor units). The $5,000 limit check is `> 500000` — no rounding involved.

**Production note:** No change. int64 cents is correct for USD. For multi-currency support, add a `currency` field and handle minor unit differences (JPY has no cents, BHD has 3 decimal places).
