package funding

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/apex/mcd/internal/models"
)

// AccountResolver looks up account + correspondent data from Postgres.
type AccountResolver struct {
	db *sql.DB
}

// NewAccountResolver creates an AccountResolver backed by the given database.
func NewAccountResolver(db *sql.DB) *AccountResolver {
	return &AccountResolver{db: db}
}

// Resolve returns the account and its correspondent's omnibus account ID.
// Returns ErrAccountNotFound if the account doesn't exist — this is a hard gate.
// Does NOT check account status; ineligible accounts are reported as a collect-all
// violation in the funding service so all issues are surfaced at once.
func (r *AccountResolver) Resolve(ctx context.Context, accountID string) (*models.AccountWithCorrespondent, error) {
	var acct models.AccountWithCorrespondent
	err := r.db.QueryRowContext(ctx, `
		SELECT a.id, a.correspondent_id, a.account_type, a.status, a.created_at,
		       c.omnibus_account_id
		FROM accounts a
		JOIN correspondents c ON c.id = a.correspondent_id
		WHERE a.id = $1`, accountID).Scan(
		&acct.ID, &acct.CorrespondentID, &acct.AccountType,
		&acct.Status, &acct.CreatedAt, &acct.OmnibusAccountID,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("funding: %w: %s", models.ErrAccountNotFound, accountID)
	}
	if err != nil {
		return nil, fmt.Errorf("funding: resolving account %s: %w", accountID, err)
	}
	return &acct, nil
}
