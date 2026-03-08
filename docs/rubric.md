# Project Spec

## Project Title: Mobile Check Deposit System

## Project Overview

Design and build a minimal end-to-end mobile check deposit system that allows investors to deposit checks into brokerage accounts via a mobile application. The system integrates with an external Vendor Service for check image capture, validation, and verification, routes validated deposits through a Funding Service middleware for business rule enforcement and ledger posting, and settles approved deposits with a Settlement Bank. The solution must handle the full lifecycle â€” capture, validation, compliance gating, operational review, ledger posting, settlement, and return/reversal scenarios â€” with a clear operator workflow, audit trail, and stubbed vendor integration that supports differentiated responses for comprehensive scenario testing.

## Problem Statement

Build a system that:

- Enables investors to photograph checks via a mobile app and submit them for deposit into brokerage accounts.
- Integrates with a Vendor Service (via Vendor API) for image quality assessment (IQA), MICR extraction, OCR, and duplicate detection.
- Routes validated deposits through a Funding Service that enforces business rules (deposit limits, contribution caps, duplicate checks), resolves account identifiers, and posts transactions to the core ledger.
- Provides an operator workflow for manual review, approval/rejection, and audit of flagged deposits.
- Handles non-happy-path scenarios including IQA failures, flagged items, rejected deposits, and returned/bounced checks with appropriate reversal postings and fee handling.
- Settles approved deposits with a Settlement Bank via X9 ICL file submission.
- The Vendor Service integration may be stubbed out, but the stub must provide differentiated responses (pass, fail, flagged for review, various IQA error types, MICR read failures, duplicate detection hits, etc.) so that all validation scenarios can be exercised and tested end-to-end.

## Business Context & Impact

### Business Context

Mobile check deposit is a standard capability in consumer banking and an expected feature for modern brokerage platforms. Offering this functionality reduces reliance on manual or mail-based deposit processes, accelerates fund availability for investors, and enables correspondents to provide a seamless mobile experience. The system must balance convenience with compliance â€” enforcing contribution limits, deposit thresholds, and operational review gates â€” while maintaining full auditability for regulatory purposes.

### Key Impact Metrics

- **Validation Accuracy:** 100% of deposits passing through Vendor Service validation are correctly categorized (pass/fail/flagged) with appropriate routing
- **Gating Correctness:** 0 deposits posted without passing required business rules (contribution limits, deposit caps, account eligibility)
- **Settlement Integrity:** 100% of approved deposits reconcile correctly between ledger postings and Settlement Bank X9 file submissions
- **Return Handling:** Bounced/returned checks are reversed with correct fee deductions and investor notification within one processing cycle
- **Operator Efficiency:** Flagged deposits surface in a review queue with check images, risk scores, and MICR data; approve/reject actions are logged with full audit trail
- **Processing Compliance:** All deposits respect the EOD processing cutoff (6:30 PM CT); late submissions roll to next business day
- **Scenario Coverage:** Stubbed Vendor Service supports at minimum 5 differentiated response types to validate all paths through the system

## Technical Requirements

### Required Programming Languages

- Golang or Java; be prepared to justify choices
- Shell/Make for one-command setup

### AI/ML Frameworks

- Not required

### Development Tools

- Vendor API stub providing differentiated responses for check image validation scenarios (IQA pass, IQA fail â€” blur, IQA fail â€” glare, MICR read failure, duplicate detected, amount mismatch, clean pass)
- Funding Service middleware for business rule enforcement, account resolution, and ledger posting
- REST API endpoints for deposit submission, status tracking, operator review, and settlement file generation
- Local data store (SQLite, JSON, or equivalent) for transfers, ledger entries, operator actions, and audit logs
- Minimal UI or CLI for:
  - Simulating mobile deposit submission (image upload + amount + account info)
  - Operator review queue (view flagged deposits, approve/reject, view check images)
  - Transfer status tracking through all states
  - Cap-table / ledger view showing account balances and posted deposits

### Cloud Platforms

- Not required; local development acceptable
- If used, free-tier on AWS/GCP/Azure is acceptable

### Other Specific Requirements

- Synthetic data only; no real PII, account numbers, or check images
- Secrets via environment variables; provide `.env.example`
- Vendor Service stub must be configurable to return different response scenarios without code changes (e.g., via request parameters, test account numbers, or configuration file)
- Include a short risks/limitations note (no compliance or regulatory claims)
- One-command setup (e.g., `make dev` or `docker compose up`)

## Success Criteria

What does success look like for this project?

- **Happy path works end-to-end:** Investor submits check â†’ Vendor API captures image â†’ Vendor Service validates â†’ Funding Service applies business rules â†’ ledger posting created â†’ operator can review â†’ settlement file generated â†’ deposit marked completed
- **Validation gating works:** IQA failures prompt retake; MICR/OCR failures route to manual review; duplicate checks are rejected; over-limit deposits are blocked
- **Operator workflow functions:** Flagged deposits appear in review queue with images, risk data, and MICR details; approve/reject actions update transfer state and are logged
- **Return/reversal path works:** Bounced checks trigger reversal postings with fee deduction ($30 return fee for MVP); transfer moves to "returned" state; investor is notified
- **Transfer state machine is complete:** Deposits transition correctly through: Requested â†’ Validating â†’ Analyzing â†’ Approved â†’ FundsPosted â†’ Completed (happy path) and Requested â†’ Validating â†’ Rejected / Returned (non-happy paths)
- **Settlement file generation demonstrated:** X9 ICL file (or structured equivalent) produced with correct deposit data, respecting the EOD cutoff
- **Stub produces varied responses:** Vendor Service stub demonstrably returns different outcomes for different test inputs, enabling all paths to be tested
- **Tests validate key invariants:** No deposit posts to ledger without passing validation and business rules; no settlement file includes rejected deposits; reversal amounts include fee deduction

## Functional Requirements (Must-Haves)

### Deposit Submission & Capture

- Endpoint or UI to simulate mobile check deposit submission (front image, back image, deposit amount, account identifier)
- Vendor API stub accepts image payloads and returns structured validation results
- Support for re-submission on IQA failure with actionable error messages

### Vendor Service Integration (Stubbed)

- Stub must support the following differentiated response scenarios:
  - **IQA Pass** â€” image quality acceptable, proceed to MICR/OCR
  - **IQA Fail (Blur)** â€” image too blurry, prompt retake
  - **IQA Fail (Glare)** â€” glare detected, prompt retake
  - **MICR Read Failure** â€” cannot read magnetic ink line, flag for manual review
  - **Duplicate Detected** â€” check has been previously deposited, reject
  - **Amount Mismatch** â€” OCR amount differs from user-entered amount, flag for review
  - **Clean Pass** â€” all checks pass, return extracted MICR data, amounts, and transaction ID
- Stub responses should be deterministic and selectable (via test account number, request header, or configuration)

### Funding Service Middleware

- Validate investor session and account eligibility
- Resolve account identifiers to internal account/routing numbers
- Apply business rules:
  - Deposit amount limits (reject if > $5,000 for MVP)
  - Contribution type defaults (individual contribution for retirement-type accounts)
  - Duplicate deposit detection (beyond Vendor Service check)
- Create transfer records in the ledger with the following attributes:
  - **To AccountId:** Investor account (determined from transfer)
  - **From AccountId:** Omnibus account for the investor's correspondent (looked up via client config)
  - **Type:** MOVEMENT
  - **Memo:** FREE
  - **SubType:** DEPOSIT
  - **Transfer Type:** CHECK
  - **Currency:** USD
  - **Amount:** Validated deposit amount
  - **SourceApplicationId:** TransferID

### Transfer State Machine

Implement the following states and transitions:

| State | Description |
|---|---|
| Requested | Deposit submitted by investor |
| Validating | Sent to Vendor Service for IQA/MICR/OCR |
| Analyzing | Business rules being applied by Funding Service |
| Approved | Passed all checks; awaiting ledger posting |
| FundsPosted | Provisional credit posted to investor account |
| Completed | Settlement confirmed by Settlement Bank |
| Rejected | Failed validation, business rules, or operator review |
| Returned | Check bounced after settlement; reversal posted |

### Operator Review Workflow

- Review queue showing flagged deposits with:
  - Check images (front and back)
  - MICR data and confidence scores
  - Risk indicators and Vendor Service scores
  - Recognized vs. entered amount comparison
- Approve/reject controls with mandatory action logging
- Ability to override contribution type defaults if needed
- Search and filter by date, status, account, amount
- Audit log of all operator actions (who, what, when)

### Settlement & Posting

- Generate settlement file (X9 ICL format or structured JSON equivalent) containing:
  - MICR data populating check detail records
  - Binary image references (front and back)
  - Amount and sequence/batch metadata
- Batch approved deposits for EOD submission (6:30 PM CT cutoff)
  - Deposits after cutoff roll to next business day
- Settlement Bank acknowledgment tracking

### Return / Reversal Handling

- Accept return notifications (simulated for stub)
- Create reversal postings that:
  - Debit the investor account for the original deposit amount
  - Deduct return fee ($30 hard-coded for MVP)
  - Transition transfer to "Returned" state
- Notify investor of returned check and fee

### Observability & Monitoring

- Per-deposit decision trace: inputs â†’ Vendor Service response â†’ business rules applied â†’ operator actions â†’ settlement status
- Differentiate between deposit sources in logs for debugging
- Monitor for missing or delayed settlement files
- Redacted logs (no real PII in any scenario)

## Performance Benchmarks

- **Validation round-trip:** Vendor Service stub responds within testnet/local norms (< 1 second)
- **Ledger posting:** Transfer created and posted within seconds of approval
- **Settlement file generation:** Batch file produced within seconds of EOD trigger
- **Operator queue:** Flagged items surface in review queue immediately upon flagging
- **State transitions:** All state changes propagate and are queryable within 1 second

## Code Quality Expectations

- Clean, readable code with clear separation of concerns:
  - Deposit capture / Vendor API integration
  - Vendor Service stub (independently configurable)
  - Funding Service business rules
  - Ledger posting
  - Operator review workflow
  - Settlement file generation
  - Return/reversal handling
- One-command setup (e.g., `make dev` or `docker compose up`) and concise README
- Minimum 10 tests covering:
  - Happy path end-to-end
  - Each Vendor Service stub response scenario
  - Business rule enforcement (deposit limits, contribution defaults)
  - State machine transitions (valid and invalid)
  - Reversal posting with fee calculation
  - Settlement file contents validation
- Deterministic demo scripts that exercise all paths
- Decision log documenting key choices and alternatives

## Recommended Steps

1. **Design the system architecture**
   - Define service boundaries: mobile client simulation, Vendor API stub, Vendor Service stub, Funding Service, ledger/data store, operator UI, settlement engine
   - Choose data store and schema for transfers, ledger entries, operator actions, and audit logs
   - Design the transfer state machine with valid transitions

2. **Build the Vendor Service stub**
   - Implement a configurable stub that returns differentiated responses based on input parameters
   - Support at minimum: IQA pass, IQA fail (blur), IQA fail (glare), MICR failure, duplicate, amount mismatch, clean pass
   - Make response selection deterministic and documented (e.g., specific test account numbers trigger specific responses)

3. **Build the Funding Service middleware**
   - Session validation and account resolution
   - Business rule engine (deposit limits, contribution type, duplicate detection)
   - Ledger posting logic with correct account mapping (investor account, omnibus account lookup)
   - Transfer state management

4. **Implement the operator review workflow**
   - Review queue with deposit details, images, and risk data
   - Approve/reject with audit logging
   - Search, filter, and override capabilities

5. **Implement settlement file generation**
   - Batch approved deposits into X9-style structured output
   - EOD cutoff enforcement with rollover logic
   - Settlement Bank acknowledgment tracking

6. **Implement return/reversal handling**
   - Return notification processing
   - Reversal posting with fee deduction
   - State transition to "Returned" with investor notification

7. **Build the demo interface**
   - CLI or minimal UI to simulate: deposit submission â†’ validation â†’ approval â†’ posting â†’ settlement
   - Exercise all Vendor Service stub scenarios
   - Show operator review workflow
   - Demonstrate return/reversal path

8. **Write tests and evaluation**
   - Unit tests for business rules, state machine, fee calculation
   - Integration tests for end-to-end flows
   - Scenario tests exercising each stub response type
   - Generate test report artifact

9. **Documentation and packaging**
   - README with setup, architecture, flows, and demo instructions
   - Decision log with key trade-offs
   - Risks and limitations note
   - `.env.example` with required configuration

## Deliverables

- **GitHub repo** with:
  - `README.md` â€” setup, architecture, data flow, how to demo, disclaimers
  - `/docs/decision_log.md` â€” key decisions and alternatives considered (e.g., settlement file format, stub design, state machine choices)
  - `/docs/architecture.md` â€” system diagram, service boundaries, data flow
  - `/tests` â€” unit and integration tests with minimum 10 test cases
  - `/reports` â€” test results and scenario coverage report
  - `.env.example` â€” required environment variables
  - Vendor Service stub with documented response scenarios and configuration
  - Demo scripts exercising all paths (happy, rejection, manual review, return/reversal)
  - Short write-up (â‰¤ 1 page): architecture choices, Vendor Service stub design, state machine rationale, risks/limitations

## Evaluation Rubric (100 pts total)

| Category | Points |
|---|---|
| **System design and architecture** â€” clear service boundaries, data flow, state machine design, and trade-off rationale | 20 |
| **Core correctness** â€” happy path works end-to-end; business rules enforced; state transitions correct; ledger postings accurate | 25 |
| **Vendor Service stub quality** â€” differentiated responses are configurable, deterministic, and cover all required scenarios | 15 |
| **Operator workflow and observability** â€” review queue functions; audit trail complete; decision traces available | 10 |
| **Return/reversal handling** â€” bounced checks reversed correctly with fee; state transitions correct | 10 |
| **Tests and evaluation rigor** â€” minimum 10 tests; all paths exercised; test report generated | 10 |
| **Developer experience** â€” one-command setup; clear README; demo scripts; decision log | 10 |

## Common Submission Format

Include the following in your README or a separate `SUBMISSION.md`:

- **Project name:**
- **Summary (3â€“5 sentences):**
  - What did you build?
  - Why these design choices? Key trade-offs?
- **How to run (copy-paste commands):**
- **Test/eval results** (screenshot or brief log; link to report in `/reports`):
- **With one more week, we would:**
- **Risks and limitations:**
- **How should ACME evaluate production readiness?**