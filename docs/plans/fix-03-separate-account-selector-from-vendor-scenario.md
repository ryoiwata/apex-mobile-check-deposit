# Plan: Separate Account Selector from Vendor Stub Scenario Selector

**Branch:** `fix-01/separate-account-selector-from-vendor-scenario`
**Category:** architecture / UX correctness

---

## Problem

The current `DepositForm.jsx` uses a single `ACCOUNTS` array that conflates two distinct concepts:

```js
{ id: 'ACC-SOFI-1001', label: 'ACC-SOFI-1001 — IQA Blur (Rejected)' }
```

The account ID (`ACC-SOFI-1001`) is a **Funding Service concept** — used for eligibility checks, ledger posting, and contribution type logic. The scenario label (`IQA Blur`) is a **Vendor Service stub concept** — it controls how the stub responds. The current implementation derives the scenario from the account suffix in `stub.go:extractSuffix()`, which ties an internal testing mechanism to investor-facing data. This violates service boundary separation and makes the UI confusing (it implies scenario selection is part of account selection).

---

## Current Code Locations

| What | File | Key Detail |
|------|------|------------|
| Account dropdown with scenario labels | `web/src/components/DepositForm.jsx:4–13` | `ACCOUNTS` array, `account_id` sent in FormData |
| Scenario routing by account suffix | `backend/internal/vendor/stub.go:16–35` | `extractSuffix(req.AccountID)` drives switch |
| Vendor `Request` struct | `backend/internal/vendor/models.go` | Has `AccountID`, no `Scenario` field |
| Form field parsing | `backend/internal/deposit/handler.go:49–56` | Reads `account_id`, ignores scenario |
| Vendor call in pipeline | `backend/internal/deposit/service.go` | Passes `AccountID` to vendor `Request` |

---

## Changes Required

### 1. Frontend — `web/src/components/DepositForm.jsx`

**Split `ACCOUNTS` into two separate data structures:**

```js
// Clean account identifiers — Funding Service domain
const ACCOUNTS = [
  { id: 'ACC-SOFI-1006', label: 'ACC-SOFI-1006 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1001', label: 'ACC-SOFI-1001 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1002', label: 'ACC-SOFI-1002 — SoFi Joint Brokerage' },
  { id: 'ACC-SOFI-1003', label: 'ACC-SOFI-1003 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1004', label: 'ACC-SOFI-1004 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-1005', label: 'ACC-SOFI-1005 — SoFi Individual Brokerage' },
  { id: 'ACC-SOFI-0000', label: 'ACC-SOFI-0000 — SoFi Demo Account' },
  { id: 'ACC-RETIRE-001', label: 'ACC-RETIRE-001 — SoFi Traditional IRA' },
]

// Vendor Service stub scenarios — testing/configuration domain
const SCENARIOS = [
  { code: 'CLEAN_PASS',         label: 'Clean Pass',          description: 'All checks pass — extracted MICR data returned (Happy Path)' },
  { code: 'IQA_FAIL_BLUR',      label: 'IQA Fail — Blur',     description: 'Image too blurry, prompt retake' },
  { code: 'IQA_FAIL_GLARE',     label: 'IQA Fail — Glare',    description: 'Glare detected, prompt retake' },
  { code: 'MICR_READ_FAILURE',  label: 'MICR Read Failure',   description: 'Cannot read MICR line, flags for operator review' },
  { code: 'DUPLICATE_DETECTED', label: 'Duplicate Detected',  description: 'Check previously deposited, reject' },
  { code: 'AMOUNT_MISMATCH',    label: 'Amount Mismatch',     description: 'OCR amount differs from entered amount, flags for review' },
  { code: 'IQA_PASS',           label: 'IQA Pass (basic)',    description: 'Image quality acceptable, clean result' },
]
```

**Add `scenario` state and include it in FormData:**

```js
const [scenario, setScenario] = useState('CLEAN_PASS')

// in handleSubmit:
formData.append('vendor_scenario', scenario)
```

**Add a visually distinct "Test Configuration" panel** above the form proper, with a different background (e.g., `bg-amber-50 border border-amber-200`) to make clear it is not an investor-facing field:

```jsx
{/* Test Configuration Panel */}
<div className="mb-6 p-4 bg-amber-50 border border-amber-200 rounded-lg">
  <div className="flex items-center gap-2 mb-2">
    <span className="text-xs font-semibold text-amber-800 uppercase tracking-wide">
      Vendor Service Stub — Test Scenario
    </span>
  </div>
  <p className="text-xs text-amber-700 mb-3">
    Controls how the vendor stub responds. Not an investor-facing field.
  </p>
  <select
    value={scenario}
    onChange={e => setScenario(e.target.value)}
    className="w-full border border-amber-300 rounded px-3 py-2 text-sm bg-white focus:outline-none focus:ring-1 focus:ring-amber-500"
  >
    {SCENARIOS.map(s => (
      <option key={s.code} value={s.code}>{s.label} — {s.description}</option>
    ))}
  </select>
</div>
```

The deposit form fields (Account, Amount, Images) follow in a standard white section, unchanged except for the cleaned-up account labels.

---

### 2. Backend — `backend/internal/vendor/models.go`

Add `Scenario` field to the `Request` struct:

```go
type Request struct {
    TransferID          uuid.UUID
    AccountID           string
    FrontImageRef       string
    BackImageRef        string
    DeclaredAmountCents int64
    Scenario            string // optional; if empty, defaults to CLEAN_PASS
}
```

---

### 3. Backend — `backend/internal/vendor/stub.go`

Change `Validate()` to check `req.Scenario` first. Fall back to the suffix-based lookup only when `Scenario` is empty (backward compatibility for tests that construct `Request` without a scenario).

```go
func (s *Stub) Validate(ctx context.Context, req *Request) (*Response, error) {
    txID := "VND-" + uuid.New().String()

    // Prefer explicit scenario over account suffix derivation
    scenario := req.Scenario
    if scenario == "" {
        scenario = scenarioFromSuffix(extractSuffix(req.AccountID))
    }

    switch scenario {
    case "IQA_FAIL_BLUR":
        return iqaFailBlur(txID), nil
    case "IQA_FAIL_GLARE":
        return iqaFailGlare(txID), nil
    case "MICR_READ_FAILURE":
        return micrFailure(txID), nil
    case "DUPLICATE_DETECTED":
        return duplicateDetected(txID), nil
    case "AMOUNT_MISMATCH":
        return amountMismatch(txID, req.DeclaredAmountCents), nil
    case "IQA_PASS", "CLEAN_PASS":
        return cleanPass(txID, req.DeclaredAmountCents), nil
    default:
        return cleanPass(txID, req.DeclaredAmountCents), nil
    }
}

// scenarioFromSuffix maps legacy account suffix to a scenario code.
// Preserves backward compatibility for tests that don't set Scenario.
func scenarioFromSuffix(suffix string) string {
    switch suffix {
    case "1001": return "IQA_FAIL_BLUR"
    case "1002": return "IQA_FAIL_GLARE"
    case "1003": return "MICR_READ_FAILURE"
    case "1004": return "DUPLICATE_DETECTED"
    case "1005": return "AMOUNT_MISMATCH"
    default:     return "CLEAN_PASS"
    }
}
```

`extractSuffix()` is kept as-is.

---

### 4. Backend — `backend/internal/deposit/handler.go`

Read `vendor_scenario` from the multipart form and pass it into `SubmitRequest`:

```go
scenario := c.PostForm("vendor_scenario") // optional; empty string is valid
```

Pass it to the service call. The service already wraps this into the `vendor.Request`.

---

### 5. Backend — `backend/internal/deposit/service.go`

Locate where `vendor.Request` is constructed (inside `Submit()`) and add the `Scenario` field:

```go
vendorReq := &vendor.Request{
    TransferID:          transfer.ID,
    AccountID:           req.AccountID,
    FrontImageRef:       frontRef,
    BackImageRef:        backRef,
    DeclaredAmountCents: req.AmountCents,
    Scenario:            req.Scenario, // new field
}
```

`SubmitRequest` (the service-layer input struct) gains a `Scenario string` field.

---

### 6. Seed Data — No Changes Required

The seed data in `002_seed_data.sql` defines accounts with `account_type` (individual/retirement/joint). Account names don't encode scenario labels — that's already clean. No migration needed.

Account `ACC-RETIRE-001` continues to trigger contribution type logic in the Funding Service based on `account_type = 'retirement'`, independent of the vendor scenario.

---

## Acceptance Criteria Mapping

| Criterion | How Satisfied |
|-----------|---------------|
| Account dropdown has no scenario labels | `ACCOUNTS` array uses clean display names |
| Separate Test Scenario selector exists | New `SCENARIOS` array + separate `<select>` element |
| Scenario selector is visually distinct | Amber background panel, labeled "Vendor Service Stub — Test Scenario" |
| Backend accepts scenario as separate parameter | `vendor_scenario` form field, parsed in handler |
| Vendor stub reads scenario parameter | `req.Scenario` checked first in `Validate()` |
| All 7 rubric scenarios available | `SCENARIOS` array covers all 7 |
| Retirement account still triggers contribution type | Unchanged — Funding Service reads `account_type` from DB, not from vendor scenario |
| Existing tests still pass | Suffix fallback preserved in `scenarioFromSuffix()` |
| Default (no scenario) produces CLEAN_PASS | `default` case in both switch statements returns `cleanPass()` |

---

## File Change Summary

| File | Change Type | Description |
|------|------------|-------------|
| `web/src/components/DepositForm.jsx` | Modify | Split ACCOUNTS, add SCENARIOS, add Test Configuration panel, send `vendor_scenario` in FormData |
| `backend/internal/vendor/models.go` | Modify | Add `Scenario string` to `Request` struct |
| `backend/internal/vendor/stub.go` | Modify | Prefer `req.Scenario` over suffix; add `scenarioFromSuffix()` helper |
| `backend/internal/deposit/handler.go` | Modify | Parse `vendor_scenario` form field |
| `backend/internal/deposit/service.go` | Modify | Add `Scenario` to `SubmitRequest` and `vendor.Request` construction |

No database migrations. No new dependencies. No test changes needed (existing tests set `AccountID` without `Scenario`, which triggers the suffix fallback path).

---

## Implementation Order

1. `vendor/models.go` — add field (unblocks everything else)
2. `vendor/stub.go` — add scenario-first routing + suffix fallback
3. `deposit/service.go` — thread `Scenario` through `SubmitRequest` → `vendor.Request`
4. `deposit/handler.go` — parse `vendor_scenario` form field
5. `web/src/components/DepositForm.jsx` — split selectors, add panel
6. Run `go test ./...` to confirm no regressions
7. Manual smoke test: submit with each scenario, verify expected stub behavior
