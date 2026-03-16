package funding

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/vendor"
	"github.com/redis/go-redis/v9"
)

// RuleViolation describes a single business rule failure.
type RuleViolation struct {
	Code    string `json:"code"`
	Rule    string `json:"rule"`
	Message string `json:"message"`
}

// CollectAllError wraps multiple rule violations so the handler can surface all
// failures at once rather than stopping at the first one.
type CollectAllError struct {
	Violations []RuleViolation
}

func (e *CollectAllError) Error() string {
	return fmt.Sprintf("funding: %d rule violation(s)", len(e.Violations))
}

// RuleResult contains everything the funding service determined about a deposit.
type RuleResult struct {
	OmnibusAccountID string
	ContributionType string
	RulesApplied     []string
	RulesPassed      bool
	FailReason       string
}

// Service applies all business rules to a deposit.
type Service struct {
	db       *sql.DB
	rdb      *redis.Client
	resolver *AccountResolver
}

// NewService creates a funding Service with the given Postgres and Redis clients.
func NewService(db *sql.DB, rdb *redis.Client) *Service {
	return &Service{
		db:       db,
		rdb:      rdb,
		resolver: NewAccountResolver(db),
	}
}

// ApplyRules runs all funding rules for a deposit submission using a collect-all
// strategy: all three rules are evaluated unconditionally so every violation is
// returned at once rather than stopping on the first failure.
//
// Processing order:
//  1. Prerequisite: resolve account + omnibus ID (hard fail — data needed downstream)
//  2. Rule 1: deposit limit (amount ≤ $5,000)
//  3. Rule 2: contribution cap (retirement accounts only, $6,000 per-transaction)
//  4. Rule 3: duplicate detection (Redis SHA-256 hash, non-destructive check)
//  5. If any violations: return *CollectAllError
//  6. If all pass: mark check as deposited in Redis, return RuleResult
func (s *Service) ApplyRules(
	ctx context.Context,
	transfer *models.Transfer,
	vendorResp *vendor.Response,
) (*RuleResult, error) {
	result := &RuleResult{RulesApplied: []string{}}

	// Prerequisite: account eligibility + omnibus lookup.
	// Hard fail — without a valid account we cannot evaluate contribution cap
	// or determine where to post funds.
	result.RulesApplied = append(result.RulesApplied, "account_eligibility")
	acct, err := s.resolver.Resolve(ctx, transfer.AccountID)
	if err != nil {
		return nil, err
	}
	result.OmnibusAccountID = acct.OmnibusAccountID
	result.ContributionType = applyContributionType(acct.AccountType)

	// Collect-all: evaluate all three rules unconditionally.
	var violations []RuleViolation

	// Rule 1: Deposit limit
	result.RulesApplied = append(result.RulesApplied, "deposit_limit")
	if transfer.AmountCents > MaxDepositAmountCents {
		violations = append(violations, RuleViolation{
			Code:    "over_limit",
			Rule:    "deposit_limit",
			Message: fmt.Sprintf("Deposit amount $%.2f exceeds maximum limit of $%.2f", float64(transfer.AmountCents)/100, float64(MaxDepositAmountCents)/100),
		})
	}

	// Rule 2: Contribution cap (retirement accounts only)
	result.RulesApplied = append(result.RulesApplied, "contribution_cap")
	if err := applyContributionCap(acct.AccountType, transfer.AmountCents); err != nil {
		violations = append(violations, RuleViolation{
			Code:    "contribution_cap_exceeded",
			Rule:    "contribution_cap",
			Message: err.Error(),
		})
	}

	// Rule 3: Duplicate check (non-destructive EXISTS — only marks after all pass)
	result.RulesApplied = append(result.RulesApplied, "duplicate_check")
	if vendorResp.MICRData != nil {
		if checkDuplicateExists(ctx, s.rdb,
			vendorResp.MICRData.RoutingNumber,
			vendorResp.MICRData.AccountNumber,
			vendorResp.MICRData.CheckSerial,
			transfer.AmountCents,
		) {
			violations = append(violations, RuleViolation{
				Code:    "duplicate_funding",
				Rule:    "duplicate_check",
				Message: "This check has already been deposited",
			})
		}
	}

	if len(violations) > 0 {
		return nil, &CollectAllError{Violations: violations}
	}

	// All rules passed — now mark the check as deposited so future attempts are rejected.
	if vendorResp.MICRData != nil {
		markCheckDeposited(ctx, s.rdb,
			vendorResp.MICRData.RoutingNumber,
			vendorResp.MICRData.AccountNumber,
			vendorResp.MICRData.CheckSerial,
			transfer.AmountCents,
		)
	}

	result.RulesPassed = true
	return result, nil
}
