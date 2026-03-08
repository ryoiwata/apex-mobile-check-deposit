# Security Rules

## Secrets Management

- **Never hardcode API keys, tokens, credentials, or database URLs.** All secrets come from environment variables.
- Load secrets via `.env` file locally (gitignored). In Docker, use `env_file` in docker-compose.yml.
- Required env vars: `DATABASE_URL`, `REDIS_URL`.
- Optional env vars: `SERVER_PORT`, `EOD_CUTOFF_HOUR`, `EOD_CUTOFF_MINUTE`, `RETURN_FEE_CENTS`, `VENDOR_STUB_MODE`.
- If `DATABASE_URL` or `REDIS_URL` is missing at startup, fail fast with a clear error naming the missing variable. Don't silently fall back to defaults.
- Never log secret values. Log only that a variable "is set" or "is missing".
- For AWS deployment, secrets are set via environment variables in the container runtime, not in files.

## .gitignore

The following must always be gitignored:
```
.env
.env.*
*.pem
*.key
output/
tmp/
vendor/          # if using vendored deps
node_modules/
dist/
build/
.claude/
```

## API Input Validation

- All user input must be validated before processing. Specifically:
  - **Deposit amount:** must be positive integer, max 500000 cents ($5,000). Reject <= 0 or > limit with 422.
  - **Account identifier:** must match expected format (alphanumeric, length bounds). Strip/reject control characters.
  - **Image payloads:** validate content-type header, enforce max file size (10MB per image for MVP), reject empty payloads.
- Sanitize all string inputs — strip null bytes, control characters, excessive whitespace.
- Rate limit the deposit submission endpoint. For MVP, use Redis-backed counter: 10 deposits/minute per account. Use Gin middleware.
- Never pass raw user input directly into SQL queries. Use parameterized queries via `database/sql` or your query builder (`$1`, `$2` placeholders).
- Validate state transition requests: the state machine must reject invalid transitions with a 409 Conflict, not silently succeed.

## Authentication & Sessions

- For the MVP/demo, session validation can be simplified (header-based token or hardcoded test sessions).
- Even in demo mode, the auth middleware must be present and wired — don't skip it.
- Operator actions (approve/reject) must include an operator identifier in the request. Anonymous reviews are not acceptable.
- Log the operator ID with every review action for the audit trail.

## Financial Data Safety

- **All data is synthetic.** No real PII, bank account numbers, routing numbers, or check images.
- Even with synthetic data, practice production habits:
  - Mask account numbers in logs (show only last 4 digits).
  - Never return full account details in error messages.
  - Redact MICR data in user-facing error responses (show "check validation failed", not the raw MICR string).
- **Amounts are integers (cents).** This is both a correctness and security concern — floating point rounding in financial systems creates real vulnerabilities.
- **Ledger entries are append-only.** Never provide an API to delete or modify ledger entries. Corrections are new reversal entries.

## CORS

- In development (Docker Compose): allow `http://localhost:5173` (Vite dev server) and `http://localhost:3000`.
- In production: allow only the deployed frontend URL.
- Never use `AllowAllOrigins: true` in production Gin CORS config.

## Settlement File Security

- Settlement files (X9 ICL) contain financial data. In a production system these would be encrypted in transit and at rest.
- For the MVP, write settlement files to a configurable output directory (`SETTLEMENT_OUTPUT_DIR`), not a world-readable temp dir.
- Log settlement file generation (filename, batch size, total amount) but never log the file contents.
- Include the settlement file path in the audit trail.

## Dependencies

- Pin Go dependencies via `go.sum` (committed to git).
- Pin npm dependencies via `package-lock.json` (committed to git).
- Key Go dependencies and their purposes:
  - `github.com/gin-gonic/gin` — HTTP framework
  - `github.com/lib/pq` — Postgres driver
  - `github.com/redis/go-redis/v9` — Redis client
  - `github.com/moov-io/imagecashletter` — X9 ICL generation
  - `github.com/stretchr/testify` — test assertions
  - `github.com/google/uuid` — UUID generation
- Before adding a new dependency, check if the stdlib or an existing dep covers the need. This is a take-home project — minimize the dependency tree.

## Docker Security

- Don't run the Go backend as root in the container. Use a non-root user in the Dockerfile.
- Don't expose Postgres or Redis ports to the host unless needed for local development.
- Use specific image tags in Dockerfiles (e.g., `golang:1.22-alpine`), not `latest`.
- Don't copy `.env` files into the Docker image. Use `env_file` or runtime environment variables.

## Error Responses

- Never expose stack traces, internal error details, or SQL errors to API consumers.
- Use Gin's recovery middleware to catch panics and return clean 500 responses.
- Error responses should be structured and actionable:
  ```json
  {
    "error": "Deposit amount exceeds maximum limit of $5,000",
    "code": "DEPOSIT_OVER_LIMIT",
    "details": {"max_amount_cents": 500000, "submitted_amount_cents": 750000}
  }
  ```
- Log the full error internally (with request ID for correlation). Return only the safe message to the client.
