package operator

import (
	"context"
	"database/sql"
	"fmt"

	"github.com/apex/mcd/internal/funding"
	"github.com/apex/mcd/internal/ledger"
	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/state"
	"github.com/google/uuid"
)

// transferSelectCols is the ordered column list for SELECT queries on the transfers table.
const transferSelectCols = `
	id, account_id, amount_cents, declared_amount_cents, status, flagged,
	flag_reason, contribution_type, vendor_transaction_id, micr_routing,
	micr_account, micr_serial, micr_confidence, ocr_amount_cents,
	front_image_ref, back_image_ref, settlement_batch_id, return_reason,
	verified_amount_cents, created_at, updated_at`

// Service handles operator review queue and approve/reject workflow.
type Service struct {
	db       *sql.DB
	machine  *state.Machine
	ledger   *ledger.Service
	resolver *funding.AccountResolver
}

// NewService creates an operator Service with all dependencies wired in.
func NewService(db *sql.DB, machine *state.Machine, led *ledger.Service, fund *funding.Service) *Service {
	return &Service{
		db:       db,
		machine:  machine,
		ledger:   led,
		resolver: funding.NewAccountResolver(db),
	}
}

// GetQueue returns all transfers in Analyzing state with flagged=true,
// ordered oldest-first for fair review ordering.
func (s *Service) GetQueue(ctx context.Context) ([]*models.Transfer, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT`+transferSelectCols+`
		FROM transfers
		WHERE status = 'analyzing' AND flagged = true
		ORDER BY created_at ASC`)
	if err != nil {
		return nil, fmt.Errorf("operator: querying review queue: %w", err)
	}
	defer rows.Close()

	var transfers []*models.Transfer
	for rows.Next() {
		t, err := scanTransfer(rows.Scan)
		if err != nil {
			return nil, fmt.Errorf("operator: scanning transfer row: %w", err)
		}
		transfers = append(transfers, t)
	}
	return transfers, rows.Err()
}

// Approve moves a flagged deposit from Analyzing to FundsPosted in a single transaction:
// Analyzing→Approved (state machine) + ledger.PostFundsTx + Approved→FundsPosted + audit log.
// Returns ErrTransferNotReviewable if the transfer is not in the expected state.
// verifiedAmountCents is required (and validated) when the transfer flag_reason is "amount_mismatch".
func (s *Service) Approve(
	ctx context.Context,
	transferID uuid.UUID,
	operatorID, notes string,
	contributionTypeOverride *string,
	verifiedAmountCents *int64,
) (*models.Transfer, error) {
	transfer, err := s.getTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	if transfer.Status != models.StatusAnalyzing || !transfer.Flagged {
		return nil, fmt.Errorf("operator: %w: transfer %s (status=%s, flagged=%v)",
			models.ErrTransferNotReviewable, transferID, transfer.Status, transfer.Flagged)
	}

	// Amount mismatch deposits require the operator to specify the correct amount.
	isMismatch := transfer.FlagReason != nil && *transfer.FlagReason == "amount_mismatch"
	if isMismatch {
		if verifiedAmountCents == nil || *verifiedAmountCents <= 0 {
			return nil, fmt.Errorf("operator: verified_amount_cents is required for amount_mismatch deposits: %w",
				models.ErrInvalidInput)
		}
		if *verifiedAmountCents > 500000 {
			return nil, fmt.Errorf("operator: verified_amount_cents %d exceeds $5,000 limit: %w",
				*verifiedAmountCents, models.ErrDepositOverLimit)
		}
	}

	// Resolve omnibus account for ledger posting.
	acct, err := s.resolver.Resolve(ctx, transfer.AccountID)
	if err != nil {
		return nil, fmt.Errorf("operator: resolving account for approve: %w", err)
	}
	omnibusAccountID := acct.OmnibusAccountID

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("operator: beginning approve transaction: %w", err)
	}
	defer tx.Rollback()

	// Apply contribution type override if provided.
	if contributionTypeOverride != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE transfers SET contribution_type=$1, updated_at=NOW() WHERE id=$2`,
			*contributionTypeOverride, transferID); err != nil {
			return nil, fmt.Errorf("operator: updating contribution type: %w", err)
		}
		transfer.ContributionType = contributionTypeOverride
	}

	// For amount_mismatch: override posting amount with operator-verified value and persist it.
	if isMismatch && verifiedAmountCents != nil {
		if _, err := tx.ExecContext(ctx,
			`UPDATE transfers SET verified_amount_cents=$1, amount_cents=$1, updated_at=NOW() WHERE id=$2`,
			*verifiedAmountCents, transferID); err != nil {
			return nil, fmt.Errorf("operator: storing verified amount: %w", err)
		}
		transfer.VerifiedAmountCents = verifiedAmountCents
		transfer.AmountCents = *verifiedAmountCents
	}

	triggeredBy := "operator:" + operatorID

	// Analyzing → Approved
	if err := s.machine.Transition(ctx, tx, transferID,
		models.StatusAnalyzing, models.StatusApproved, triggeredBy, nil); err != nil {
		return nil, fmt.Errorf("operator: transitioning to approved: %w", err)
	}

	// Post deposit ledger entry (omnibus → investor).
	if err := s.ledger.PostFundsTx(ctx, tx, transfer, omnibusAccountID); err != nil {
		return nil, fmt.Errorf("operator: posting funds: %w", err)
	}

	// Approved → FundsPosted
	if err := s.machine.Transition(ctx, tx, transferID,
		models.StatusApproved, models.StatusFundsPosted, triggeredBy, nil); err != nil {
		return nil, fmt.Errorf("operator: transitioning to funds_posted: %w", err)
	}

	// Build audit metadata — include amount resolution details for mismatch deposits.
	auditMeta := map[string]any{
		"previous_status": string(models.StatusAnalyzing),
		"new_status":      string(models.StatusFundsPosted),
	}
	if isMismatch && verifiedAmountCents != nil {
		auditMeta["amount_resolution"] = map[string]any{
			"declared_amount_cents": transfer.DeclaredAmountCents,
			"ocr_amount_cents":      transfer.OCRAmountCents,
			"verified_amount_cents": *verifiedAmountCents,
		}
	}

	// Write audit log entry in the same transaction.
	if err := LogActionTx(ctx, tx, operatorID, "approve", transferID, notes, auditMeta); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("operator: committing approve: %w", err)
	}

	transfer.Status = models.StatusFundsPosted
	return transfer, nil
}

// Reject moves a deposit from Analyzing to Rejected in a single transaction:
// Analyzing→Rejected (state machine) + audit log.
// Returns ErrTransferNotReviewable if the transfer is not in Analyzing state.
func (s *Service) Reject(
	ctx context.Context,
	transferID uuid.UUID,
	operatorID, reason, notes string,
) (*models.Transfer, error) {
	transfer, err := s.getTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	if transfer.Status != models.StatusAnalyzing {
		return nil, fmt.Errorf("operator: %w: transfer %s (status=%s)",
			models.ErrTransferNotReviewable, transferID, transfer.Status)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("operator: beginning reject transaction: %w", err)
	}
	defer tx.Rollback()

	triggeredBy := "operator:" + operatorID

	// Analyzing → Rejected
	if err := s.machine.Transition(ctx, tx, transferID,
		models.StatusAnalyzing, models.StatusRejected, triggeredBy,
		map[string]any{"reason": reason}); err != nil {
		return nil, fmt.Errorf("operator: transitioning to rejected: %w", err)
	}

	// Write audit log entry in the same transaction.
	if err := LogActionTx(ctx, tx, operatorID, "reject", transferID, notes,
		map[string]any{
			"reason":          reason,
			"previous_status": string(models.StatusAnalyzing),
			"new_status":      string(models.StatusRejected),
		}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("operator: committing reject: %w", err)
	}

	transfer.Status = models.StatusRejected
	return transfer, nil
}

// OverrideContributionType updates the contribution type on a flagged deposit before approval
// and writes an "override" audit log entry. Returns ErrTransferNotReviewable if the transfer
// is not in Analyzing state with flagged=true.
func (s *Service) OverrideContributionType(
	ctx context.Context,
	transferID uuid.UUID,
	operatorID, contributionType string,
) (*models.Transfer, error) {
	transfer, err := s.getTransferByID(ctx, transferID)
	if err != nil {
		return nil, err
	}

	if transfer.Status != models.StatusAnalyzing || !transfer.Flagged {
		return nil, fmt.Errorf("operator: %w: transfer %s (status=%s, flagged=%v)",
			models.ErrTransferNotReviewable, transferID, transfer.Status, transfer.Flagged)
	}

	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("operator: beginning override transaction: %w", err)
	}
	defer tx.Rollback()

	prev := ""
	if transfer.ContributionType != nil {
		prev = *transfer.ContributionType
	}

	if _, err := tx.ExecContext(ctx,
		`UPDATE transfers SET contribution_type=$1, updated_at=NOW() WHERE id=$2`,
		contributionType, transferID); err != nil {
		return nil, fmt.Errorf("operator: updating contribution type: %w", err)
	}

	if err := LogActionTx(ctx, tx, operatorID, "override", transferID, "",
		map[string]any{
			"field":      "contribution_type",
			"prev_value": prev,
			"new_value":  contributionType,
		}); err != nil {
		return nil, err
	}

	if err := tx.Commit(); err != nil {
		return nil, fmt.Errorf("operator: committing override: %w", err)
	}

	transfer.ContributionType = &contributionType
	return transfer, nil
}

// GetAuditLog retrieves audit entries, optionally filtered by transfer ID.
func (s *Service) GetAuditLog(ctx context.Context, transferID *uuid.UUID) ([]AuditEntry, error) {
	return GetAuditLog(ctx, s.db, transferID)
}

// getTransferByID fetches a single transfer by ID.
func (s *Service) getTransferByID(ctx context.Context, id uuid.UUID) (*models.Transfer, error) {
	row := s.db.QueryRowContext(ctx,
		`SELECT`+transferSelectCols+` FROM transfers WHERE id = $1`, id)
	t, err := scanTransfer(row.Scan)
	if err == sql.ErrNoRows {
		return nil, models.ErrTransferNotFound
	}
	if err != nil {
		return nil, fmt.Errorf("operator: getting transfer %s: %w", id, err)
	}
	return t, nil
}

// scanTransfer scans a transfer row using the provided scan function.
func scanTransfer(scanFn func(dest ...any) error) (*models.Transfer, error) {
	var t models.Transfer
	var settlementBatchIDStr sql.NullString
	err := scanFn(
		&t.ID, &t.AccountID, &t.AmountCents, &t.DeclaredAmountCents,
		&t.Status, &t.Flagged, &t.FlagReason, &t.ContributionType,
		&t.VendorTransactionID, &t.MICRRouting, &t.MICRAccount,
		&t.MICRSerial, &t.MICRConfidence, &t.OCRAmountCents,
		&t.FrontImageRef, &t.BackImageRef, &settlementBatchIDStr,
		&t.ReturnReason, &t.VerifiedAmountCents, &t.CreatedAt, &t.UpdatedAt,
	)
	if err != nil {
		return nil, err
	}
	if settlementBatchIDStr.Valid {
		id, _ := uuid.Parse(settlementBatchIDStr.String)
		t.SettlementBatchID = &id
	}
	return &t, nil
}
