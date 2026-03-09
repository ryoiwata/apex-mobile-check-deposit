package funding

import (
	"context"
	"database/sql"

	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/vendor"
	"github.com/redis/go-redis/v9"
)

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

// ApplyRules runs all funding rules for a deposit submission.
// vendorResp provides MICR data for duplicate check.
// Returns RuleResult on pass, or error (wrapping domain errors) on failure.
func (s *Service) ApplyRules(
	ctx context.Context,
	transfer *models.Transfer,
	vendorResp *vendor.Response,
) (*RuleResult, error) {
	result := &RuleResult{RulesApplied: []string{}}

	// Rule 1: Deposit limit
	result.RulesApplied = append(result.RulesApplied, "deposit_limit")
	if err := applyDepositLimit(transfer.AmountCents); err != nil {
		return nil, err
	}

	// Rule 2: Account eligibility + omnibus lookup
	result.RulesApplied = append(result.RulesApplied, "account_eligibility")
	acct, err := s.resolver.Resolve(ctx, transfer.AccountID)
	if err != nil {
		return nil, err
	}
	result.OmnibusAccountID = acct.OmnibusAccountID

	// Rule 3: Contribution type default
	result.RulesApplied = append(result.RulesApplied, "contribution_type")
	result.ContributionType = applyContributionType(acct.AccountType)

	// Rule 4: Duplicate check (only if MICR data is available)
	result.RulesApplied = append(result.RulesApplied, "duplicate_check")
	if vendorResp.MICRData != nil {
		if err := applyDuplicateCheck(ctx, s.rdb,
			vendorResp.MICRData.RoutingNumber,
			vendorResp.MICRData.AccountNumber,
			vendorResp.MICRData.CheckSerial,
			transfer.AmountCents,
		); err != nil {
			return nil, err
		}
	}

	result.RulesPassed = true
	return result, nil
}
