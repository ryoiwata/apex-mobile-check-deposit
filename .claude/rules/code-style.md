# Code Style Rules

## Go (Backend)

### General
- Go 1.22+ features are fine (range over int, enhanced routing patterns, etc.)
- All exported functions and types get doc comments. Skip for obvious unexported helpers.
- Use `context.Context` as the first parameter on all service methods and DB calls.
- Prefer returning `error` over panic. Reserve panic for truly unrecoverable states (missing DB at startup).

### Formatting
- `gofmt` / `goimports` is the standard. No debate.
- Imports: stdlib → third-party → local, separated by blank lines.
- No dot imports. No underscore imports except for side-effect drivers (`_ "github.com/lib/pq"`).

### Gin Conventions
- Route handlers are thin — extract request, call service, return response. No business logic in handlers.
- Use Gin's `c.ShouldBindJSON()` for request parsing. Return 400 with specific error on bind failure.
- Use middleware for cross-cutting concerns: auth, rate limiting, request ID, logging.
- Return proper HTTP status codes: 400 for bad input, 404 for not found, 409 for state conflicts, 422 for business rule violations, 500 for internal errors, 503 if Postgres/Redis is down.
- Always return JSON responses with consistent envelope: `{"data": ...}` for success, `{"error": "message", "code": "ERROR_CODE"}` for failures.

### Naming
- `camelCase` for unexported functions, variables.
- `PascalCase` for exported functions, types, interfaces.
- `UPPER_SNAKE_CASE` is not idiomatic Go — use `PascalCase` constants (e.g., `MaxDepositAmountCents`).
- Prefix interfaces with the behavior they describe, not `I` (e.g., `TransferRepository`, not `ITransferRepository`).
- Receiver names: short, consistent, 1-2 chars (e.g., `s` for service, `r` for repository, `h` for handler).

### Error Handling
- Always wrap errors with context: `fmt.Errorf("funding: applying deposit limit rule: %w", err)`.
- Use custom error types for domain errors that handlers need to distinguish (e.g., `ErrDepositOverLimit`, `ErrInvalidStateTransition`, `ErrDuplicateCheck`).
- Never swallow errors silently. If you don't return it, log it with context.
- Use `errors.Is()` and `errors.As()` for error checking, not string matching.

### Project-Specific
- **All monetary amounts are `int64` cents.** Never use `float64` for money. $5,000 = `500000`. $30 fee = `3000`.
- **Transfer state changes go through the state machine only.** Never `UPDATE transfers SET status = ...` directly. Call `stateMachine.Transition(ctx, transferID, newState)`.
- **Vendor stub responses are keyed by account suffix.** The stub must be stateless — given the same input, always return the same output.
- **Ledger entries are append-only.** Never update or delete a ledger entry. Reversals create new entries.
- **Use database transactions** for any operation that touches multiple tables (e.g., posting funds = insert ledger entry + update transfer state).
- **Log with structured fields**, not string interpolation: `log.With("transfer_id", id, "state", state).Info("transition")`.
- **Redact PII in logs.** Even though data is synthetic, build the habit. Never log full account numbers — mask to last 4 digits.

### Testing (Go)
- Use `testify/assert` and `testify/require`. `require` for preconditions (fails fast), `assert` for checks.
- Table-driven tests for business rules and state transitions.
- Use test helpers to create fixtures (e.g., `newTestTransfer(t, WithStatus(Requested), WithAmount(100000))`).
- Mock interfaces at service boundaries. Don't mock the database — use a test Postgres instance in Docker.
- Test file naming: `foo_test.go` in the same package.

## React (Frontend)

### General
- Functional components only. No class components.
- Use hooks (`useState`, `useEffect`, `useCallback`). Keep state minimal.
- Props get JSDoc comments for non-obvious types.

### Styling
- Tailwind utility classes. No separate CSS files.
- Don't over-design the frontend. The rubric allocates 10 points to operator workflow — focus on functionality over polish.
- Mobile-responsive is nice-to-have, not required.

### Structure
- One component per file for anything non-trivial.
- All API calls go through a single `api.js` module — not scattered in components.
- Handle loading and error states. Show a spinner during API calls. Show error messages from the backend.
- Use consistent patterns: `const [data, setData] = useState(null)` + `const [loading, setLoading] = useState(false)` + `const [error, setError] = useState(null)`.

### Operator UI Specifics
- Review queue must show: check images (front/back), MICR data, confidence scores, recognized vs. entered amount, risk indicators.
- Approve/reject buttons must confirm the action before submitting.
- Audit log must be visible — show who did what and when.
- Filter/search by: date range, status, account number, amount range.

## SQL (Migrations)

- Use numbered migration files: `001_create_transfers.sql`, `002_create_ledger_entries.sql`, etc.
- Every `CREATE TABLE` includes `created_at TIMESTAMPTZ DEFAULT NOW()` and `updated_at TIMESTAMPTZ DEFAULT NOW()`.
- Use `BIGINT` for monetary amounts (cents). Never `DECIMAL` or `NUMERIC` for amounts that flow through Go code.
- Add indexes on columns used in WHERE clauses: `status`, `account_id`, `created_at`, `check_hash`.
- Use `CHECK` constraints where applicable (e.g., `CHECK (amount > 0)`, `CHECK (status IN ('requested', 'validating', ...))`).
- Foreign keys between transfers and ledger entries. Cascade deletes are forbidden — ledger entries are permanent.
