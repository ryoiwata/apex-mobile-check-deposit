# API Contracts & Request/Response Schemas

## General Principles

- All endpoints return JSON with consistent envelope: `{"data": ...}` for success, `{"error": "message", "code": "CODE"}` for failure.
- All monetary amounts are integers in cents. `$1,000.00` = `100000`.
- All timestamps are ISO 8601 in UTC: `2026-03-08T14:30:00Z`.
- All IDs are UUIDs v4.
- Authentication is via `Authorization: Bearer <session_token>` header (simplified for MVP).
- Operator endpoints require `X-Operator-ID` header for audit trail.

## Endpoints

### Deposit Submission

```
POST /api/v1/deposits
Content-Type: multipart/form-data

Fields:
  front_image:    file (required) — front of check, JPEG/PNG, max 10MB
  back_image:     file (required) — back of check, JPEG/PNG, max 10MB
  amount_cents:   integer (required) — deposit amount in cents, > 0, <= 500000
  account_id:     string (required) — investor account identifier

Response 201:
{
  "data": {
    "transfer_id": "uuid",
    "status": "requested",
    "amount_cents": 100000,
    "account_id": "ACC-12345-1006",
    "created_at": "2026-03-08T14:30:00Z"
  }
}

Response 400: missing/invalid fields
Response 422: business rule violation (over limit, ineligible account)
Response 429: rate limited
```

### Transfer Status

```
GET /api/v1/deposits/:transfer_id

Response 200:
{
  "data": {
    "transfer_id": "uuid",
    "status": "validating",
    "amount_cents": 100000,
    "account_id": "ACC-12345-1006",
    "vendor_result": {
      "iqa_status": "pass",
      "micr_data": { ... },
      "confidence_score": 0.95
    },
    "funding_result": {
      "rules_applied": ["deposit_limit", "duplicate_check", "account_eligibility"],
      "rules_passed": true
    },
    "flagged": false,
    "created_at": "2026-03-08T14:30:00Z",
    "updated_at": "2026-03-08T14:30:05Z",
    "state_history": [
      {"from": "requested", "to": "validating", "at": "2026-03-08T14:30:01Z"},
      {"from": "validating", "to": "analyzing", "at": "2026-03-08T14:30:03Z"}
    ]
  }
}

Response 404: transfer not found
```

### List Deposits

```
GET /api/v1/deposits?status=flagged&account_id=ACC-12345&page=1&per_page=20

Response 200:
{
  "data": [ ... ],
  "pagination": {
    "page": 1,
    "per_page": 20,
    "total": 42
  }
}
```

### Operator Review Queue

```
GET /api/v1/operator/queue?status=flagged&sort=created_at&order=asc
Headers: X-Operator-ID: OP-001

Response 200:
{
  "data": [
    {
      "transfer_id": "uuid",
      "status": "analyzing",
      "flagged": true,
      "flag_reason": "micr_failure",
      "amount_cents": 100000,
      "account_id": "ACC-12345-1003",
      "vendor_result": { ... },
      "front_image_url": "/api/v1/deposits/uuid/images/front",
      "back_image_url": "/api/v1/deposits/uuid/images/back",
      "created_at": "2026-03-08T14:30:00Z"
    }
  ]
}
```

### Operator Approve

```
POST /api/v1/operator/deposits/:transfer_id/approve
Headers: X-Operator-ID: OP-001
Content-Type: application/json

Body:
{
  "notes": "MICR verified manually, data matches",
  "contribution_type_override": null
}

Response 200:
{
  "data": {
    "transfer_id": "uuid",
    "status": "approved",
    "approved_by": "OP-001",
    "approved_at": "2026-03-08T15:00:00Z"
  }
}

Response 409: transfer not in reviewable state
```

### Operator Reject

```
POST /api/v1/operator/deposits/:transfer_id/reject
Headers: X-Operator-ID: OP-001
Content-Type: application/json

Body:
{
  "reason": "Check image appears altered",
  "notes": "Irregular ink pattern on MICR line"
}

Response 200:
{
  "data": {
    "transfer_id": "uuid",
    "status": "rejected",
    "rejected_by": "OP-001",
    "rejected_at": "2026-03-08T15:00:00Z",
    "reason": "Check image appears altered"
  }
}

Response 409: transfer not in reviewable state
```

### Settlement Trigger

```
POST /api/v1/settlement/trigger
Headers: X-Operator-ID: OP-001

Body:
{
  "batch_date": "2026-03-08"
}

Response 200:
{
  "data": {
    "batch_id": "uuid",
    "file_path": "output/settlement/20260308_batch_001.x9",
    "deposit_count": 15,
    "total_amount_cents": 1250000,
    "cutoff_time": "2026-03-08T23:30:00Z",
    "deposits_rolled_to_next_day": 3
  }
}
```

### Simulate Return

```
POST /api/v1/deposits/:transfer_id/return
Headers: X-Operator-ID: OP-001

Body:
{
  "return_reason": "insufficient_funds",
  "bank_reference": "RET-2026-03-08-001"
}

Response 200:
{
  "data": {
    "transfer_id": "uuid",
    "status": "returned",
    "original_amount_cents": 100000,
    "return_fee_cents": 3000,
    "total_debit_cents": 103000,
    "reversal_entries": [
      {"type": "REVERSAL", "amount_cents": 100000},
      {"type": "FEE", "amount_cents": 3000}
    ]
  }
}

Response 409: transfer not in returnable state (must be Completed)
```

### Ledger View

```
GET /api/v1/ledger/:account_id?from=2026-03-01&to=2026-03-08

Response 200:
{
  "data": {
    "account_id": "ACC-12345-1006",
    "balance_cents": 350000,
    "entries": [
      {
        "id": "uuid",
        "transfer_id": "uuid",
        "to_account_id": "ACC-12345-1006",
        "from_account_id": "OMNI-BROKER-001",
        "type": "MOVEMENT",
        "sub_type": "DEPOSIT",
        "transfer_type": "CHECK",
        "amount_cents": 100000,
        "currency": "USD",
        "memo": "FREE",
        "created_at": "2026-03-08T14:30:10Z"
      }
    ]
  }
}
```

### Audit Log

```
GET /api/v1/operator/audit?transfer_id=uuid&from=2026-03-01
Headers: X-Operator-ID: OP-001

Response 200:
{
  "data": [
    {
      "id": "uuid",
      "operator_id": "OP-001",
      "action": "approve",
      "transfer_id": "uuid",
      "notes": "MICR verified manually",
      "timestamp": "2026-03-08T15:00:00Z",
      "metadata": {
        "previous_status": "analyzing",
        "new_status": "approved"
      }
    }
  ]
}
```

### Health Check

```
GET /health

Response 200:
{
  "status": "ok",
  "postgres": "connected",
  "redis": "connected",
  "timestamp": "2026-03-08T14:30:00Z"
}

Response 503: one or more dependencies down
```

## Vendor Stub Internal API

This is internal (called by the funding service, not exposed to clients directly):

```
POST /internal/vendor/validate
Content-Type: application/json

Body:
{
  "transfer_id": "uuid",
  "account_id": "ACC-12345-1006",
  "front_image_ref": "path/to/front.jpg",
  "back_image_ref": "path/to/back.jpg",
  "declared_amount_cents": 100000
}

Response 200:
{
  "status": "pass",          // pass | fail | flagged
  "iqa_result": "pass",      // pass | fail_blur | fail_glare
  "micr_data": {
    "routing_number": "021000021",
    "account_number": "123456789",
    "check_serial": "1001",
    "confidence_score": 0.97
  },
  "ocr_amount_cents": 100000,
  "duplicate_check": "clear",  // clear | duplicate_found
  "amount_match": true,
  "transaction_id": "VND-uuid",
  "error_code": null,
  "error_message": null
}
```

## Error Codes

| Code | HTTP Status | Description |
|------|------------|-------------|
| `INVALID_INPUT` | 400 | Missing or malformed request fields |
| `DEPOSIT_OVER_LIMIT` | 422 | Amount exceeds $5,000 max |
| `ACCOUNT_INELIGIBLE` | 422 | Account not found or not in good standing |
| `DUPLICATE_CHECK` | 422 | Check already deposited (vendor or funding detected) |
| `INVALID_STATE_TRANSITION` | 409 | Transfer cannot move to requested state |
| `TRANSFER_NOT_FOUND` | 404 | Transfer ID does not exist |
| `RATE_LIMITED` | 429 | Too many requests |
| `VENDOR_ERROR` | 502 | Vendor stub returned unexpected error |
| `INTERNAL_ERROR` | 500 | Unexpected server error |
