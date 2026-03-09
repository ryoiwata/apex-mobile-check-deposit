package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Repository handles all ledger_entries database operations.
type Repository struct {
	db *sql.DB
}

// NewRepository creates a new Repository.
func NewRepository(db *sql.DB) *Repository {
	return &Repository{db: db}
}

// PostEntryTx inserts a ledger entry within an existing transaction.
// This is the only write method — ledger entries are never updated or deleted.
func (r *Repository) PostEntryTx(ctx context.Context, tx *sql.Tx, entry *Entry) error {
	_, err := tx.ExecContext(ctx, `
		INSERT INTO ledger_entries
			(transfer_id, to_account_id, from_account_id, type, sub_type,
			 transfer_type, currency, amount_cents, memo, source_application_id)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)`,
		entry.TransferID, entry.ToAccountID, entry.FromAccountID,
		entry.Type, entry.SubType, entry.TransferType,
		entry.Currency, entry.AmountCents, entry.Memo, entry.SourceApplicationID,
	)
	if err != nil {
		return fmt.Errorf("ledger: posting entry for transfer %s: %w", entry.TransferID, err)
	}
	return nil
}

// GetByTransferID returns all ledger entries for a transfer, ordered by created_at ASC.
func (r *Repository) GetByTransferID(ctx context.Context, transferID uuid.UUID) ([]Entry, error) {
	rows, err := r.db.QueryContext(ctx, `
		SELECT id, transfer_id, to_account_id, from_account_id, type, sub_type,
		       transfer_type, currency, amount_cents, memo, source_application_id, created_at
		FROM ledger_entries
		WHERE transfer_id = $1
		ORDER BY created_at ASC`,
		transferID,
	)
	if err != nil {
		return nil, fmt.Errorf("ledger: querying entries for transfer %s: %w", transferID, err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(
			&e.ID, &e.TransferID, &e.ToAccountID, &e.FromAccountID,
			&e.Type, &e.SubType, &e.TransferType, &e.Currency,
			&e.AmountCents, &e.Memo, &e.SourceApplicationID, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("ledger: scanning entry for transfer %s: %w", transferID, err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: iterating entries for transfer %s: %w", transferID, err)
	}
	return entries, nil
}

// GetByAccountID returns entries where to_account_id or from_account_id matches,
// with optional date range filter. Returns all entries if from and to are nil.
// Ordered by created_at ASC.
func (r *Repository) GetByAccountID(ctx context.Context, accountID string, from, to *time.Time) ([]Entry, error) {
	query := `
		SELECT id, transfer_id, to_account_id, from_account_id, type, sub_type,
		       transfer_type, currency, amount_cents, memo, source_application_id, created_at
		FROM ledger_entries
		WHERE (to_account_id = $1 OR from_account_id = $1)`

	args := []any{accountID}
	argIdx := 2

	if from != nil {
		query += fmt.Sprintf(" AND created_at >= $%d", argIdx)
		args = append(args, *from)
		argIdx++
	}
	if to != nil {
		query += fmt.Sprintf(" AND created_at <= $%d", argIdx)
		args = append(args, *to)
	}
	query += " ORDER BY created_at ASC"

	rows, err := r.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("ledger: querying entries for account %s: %w", accountID, err)
	}
	defer rows.Close()

	var entries []Entry
	for rows.Next() {
		var e Entry
		if err := rows.Scan(
			&e.ID, &e.TransferID, &e.ToAccountID, &e.FromAccountID,
			&e.Type, &e.SubType, &e.TransferType, &e.Currency,
			&e.AmountCents, &e.Memo, &e.SourceApplicationID, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("ledger: scanning entry for account %s: %w", accountID, err)
		}
		entries = append(entries, e)
	}
	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("ledger: iterating entries for account %s: %w", accountID, err)
	}
	return entries, nil
}
