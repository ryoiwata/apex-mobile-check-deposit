package ledger

import (
	"context"
	"database/sql"
	"fmt"
	"time"

	"github.com/apex/mcd/internal/models"
	"github.com/google/uuid"
)

// Service handles ledger posting and reversals.
type Service struct {
	db   *sql.DB
	repo *Repository
}

// NewService creates a new ledger Service.
func NewService(db *sql.DB) *Service {
	return &Service{db: db, repo: NewRepository(db)}
}

// PostFundsTx creates a DEPOSIT ledger entry within the provided transaction.
// Called after transition Approved→FundsPosted.
// toAccountID = investor, fromAccountID = omnibus account of correspondent.
func (s *Service) PostFundsTx(ctx context.Context, tx *sql.Tx, transfer *models.Transfer, omnibusAccountID string) error {
	entry := &Entry{
		TransferID:          transfer.ID,
		ToAccountID:         transfer.AccountID,
		FromAccountID:       omnibusAccountID,
		Type:                "MOVEMENT",
		SubType:             "DEPOSIT",
		TransferType:        "CHECK",
		Currency:            "USD",
		AmountCents:         transfer.AmountCents,
		Memo:                "FREE",
		SourceApplicationID: transfer.ID,
	}
	return s.repo.PostEntryTx(ctx, tx, entry)
}

// PostReversal creates two ledger entries for a bounced check within the provided transaction:
// 1. REVERSAL: investor→omnibus for the original deposit amount
// 2. RETURN_FEE: investor→omnibus for the $30 return fee
// Both entries are in a single transaction with the Completed→Returned state transition.
func (s *Service) PostReversal(ctx context.Context, tx *sql.Tx,
	transfer *models.Transfer, omnibusAccountID string, returnFeeCents int64) error {

	reversalEntry := &Entry{
		TransferID:          transfer.ID,
		FromAccountID:       transfer.AccountID, // debit investor
		ToAccountID:         omnibusAccountID,   // credit omnibus
		Type:                "MOVEMENT",
		SubType:             "REVERSAL",
		TransferType:        "CHECK",
		Currency:            "USD",
		AmountCents:         transfer.AmountCents,
		Memo:                "FREE",
		SourceApplicationID: transfer.ID,
	}
	if err := s.repo.PostEntryTx(ctx, tx, reversalEntry); err != nil {
		return fmt.Errorf("ledger: posting reversal entry: %w", err)
	}

	feeEntry := &Entry{
		TransferID:          transfer.ID,
		FromAccountID:       transfer.AccountID, // debit investor for fee
		ToAccountID:         omnibusAccountID,
		Type:                "MOVEMENT",
		SubType:             "RETURN_FEE",
		TransferType:        "RETURN_FEE",
		Currency:            "USD",
		AmountCents:         returnFeeCents,
		Memo:                "FREE",
		SourceApplicationID: transfer.ID,
	}
	return s.repo.PostEntryTx(ctx, tx, feeEntry)
}

// GetByTransferID delegates to repository.
func (s *Service) GetByTransferID(ctx context.Context, id uuid.UUID) ([]Entry, error) {
	return s.repo.GetByTransferID(ctx, id)
}

// GetByAccountID delegates to repository.
func (s *Service) GetByAccountID(ctx context.Context, accountID string, from, to *time.Time) ([]Entry, error) {
	return s.repo.GetByAccountID(ctx, accountID, from, to)
}
