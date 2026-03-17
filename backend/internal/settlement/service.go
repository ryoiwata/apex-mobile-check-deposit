package settlement

import (
	"context"
	"database/sql"
	"fmt"
	"time"
	_ "time/tzdata" // embed IANA timezone database for Alpine/distroless containers

	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/state"
	"github.com/google/uuid"
)

const (
	// DefaultMaxRetries is the default maximum bank ACK retries before escalation.
	DefaultMaxRetries = 3
)

// Batch represents a settlement batch record, mapping 1:1 to settlement_batches table.
type Batch struct {
	ID                      uuid.UUID  `json:"batch_id"`
	BatchDate               time.Time  `json:"batch_date"`
	FilePath                *string    `json:"file_path,omitempty"`
	DepositCount            int        `json:"deposit_count"`
	TotalAmountCents        int64      `json:"total_amount_cents"`
	Status                  string     `json:"status"`
	BankReference           *string    `json:"bank_reference,omitempty"`
	RetryCount              int        `json:"retry_count,omitempty"`
	LastRetryAt             *time.Time `json:"last_retry_at,omitempty"`
	DepositsRolledToNextDay int        `json:"deposits_rolled_to_next_day,omitempty"`
	NextSettlementDate      *string    `json:"next_settlement_date,omitempty"`
	CreatedAt               time.Time  `json:"created_at"`
}

// Service handles EOD batch settlement processing.
type Service struct {
	db          *sql.DB
	machine     *state.Machine
	outputDir   string
	bankAckMode string // "pass" (default) or "fail" (for testing)
	maxRetries  int
}

// NewService creates a settlement Service.
func NewService(db *sql.DB, machine *state.Machine, outputDir string) *Service {
	return &Service{
		db:          db,
		machine:     machine,
		outputDir:   outputDir,
		bankAckMode: "pass",
		maxRetries:  DefaultMaxRetries,
	}
}

// SetBankAckMode configures the bank ACK stub behavior.
// Use "pass" (default) for normal operation, "fail" to simulate ACK failures in tests.
func (s *Service) SetBankAckMode(mode string) {
	s.bankAckMode = mode
}

// SetMaxRetries configures how many ACK retries before escalating to operator.
func (s *Service) SetMaxRetries(n int) {
	s.maxRetries = n
}

// simulateBankAck returns true if the bank acknowledged the submission.
// In production this would call the bank's ACK endpoint.
func (s *Service) simulateBankAck() bool {
	return s.bankAckMode != "fail"
}

// CutoffTime returns the UTC time representing 6:30 PM CT for the given date.
func CutoffTime(date time.Time) time.Time {
	ct, _ := time.LoadLocation("America/Chicago")
	y, m, d := date.In(ct).Date()
	return time.Date(y, m, d, 18, 30, 0, 0, ct).UTC()
}

// getEligibleDeposits returns FundsPosted transfers created at or before the cutoff
// that have not yet been assigned to a settlement batch.
func (s *Service) getEligibleDeposits(ctx context.Context, cutoff time.Time) ([]*models.Transfer, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, amount_cents, declared_amount_cents, status, flagged,
		       flag_reason, contribution_type, vendor_transaction_id, micr_routing,
		       micr_account, micr_serial, micr_confidence, ocr_amount_cents,
		       front_image_ref, back_image_ref, settlement_batch_id, return_reason,
		       created_at, updated_at
		FROM transfers
		WHERE status = 'funds_posted'
		  AND created_at <= $1
		  AND settlement_batch_id IS NULL
		ORDER BY created_at ASC`,
		cutoff)
	if err != nil {
		return nil, fmt.Errorf("settlement: querying eligible deposits: %w", err)
	}
	defer rows.Close()

	var transfers []*models.Transfer
	for rows.Next() {
		var t models.Transfer
		var settlementBatchIDStr sql.NullString
		if err := rows.Scan(
			&t.ID, &t.AccountID, &t.AmountCents, &t.DeclaredAmountCents,
			&t.Status, &t.Flagged, &t.FlagReason, &t.ContributionType,
			&t.VendorTransactionID, &t.MICRRouting, &t.MICRAccount,
			&t.MICRSerial, &t.MICRConfidence, &t.OCRAmountCents,
			&t.FrontImageRef, &t.BackImageRef, &settlementBatchIDStr,
			&t.ReturnReason, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("settlement: scanning transfer row: %w", err)
		}
		if settlementBatchIDStr.Valid {
			id, _ := uuid.Parse(settlementBatchIDStr.String)
			t.SettlementBatchID = &id
		}
		transfers = append(transfers, &t)
	}
	return transfers, rows.Err()
}

// countDepositsAfterCutoff returns the number of FundsPosted deposits created after the cutoff
// that have no settlement batch assigned — these are queued for the next business day.
func (s *Service) countDepositsAfterCutoff(ctx context.Context, cutoff time.Time) (int, error) {
	var n int
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*) FROM transfers
		WHERE status = 'funds_posted'
		  AND created_at > $1
		  AND settlement_batch_id IS NULL`,
		cutoff).Scan(&n)
	if err != nil {
		return 0, fmt.Errorf("settlement: counting post-cutoff deposits: %w", err)
	}
	return n, nil
}

// nextBusinessDay returns the next business day (Mon-Fri) after date.
func nextBusinessDay(date time.Time) time.Time {
	next := date.AddDate(0, 0, 1)
	switch next.Weekday() {
	case time.Saturday:
		return next.AddDate(0, 0, 2)
	case time.Sunday:
		return next.AddDate(0, 0, 1)
	}
	return next
}

// getBatch retrieves a settlement batch by ID.
func (s *Service) getBatch(ctx context.Context, batchID uuid.UUID) (*Batch, error) {
	var b Batch
	var filePath, bankRef sql.NullString
	var lastRetryAt sql.NullTime
	err := s.db.QueryRowContext(ctx, `
		SELECT id, batch_date, file_path, deposit_count, total_amount_cents,
		       status, bank_reference, retry_count, last_retry_at, created_at
		FROM settlement_batches WHERE id = $1`, batchID).Scan(
		&b.ID, &b.BatchDate, &filePath, &b.DepositCount, &b.TotalAmountCents,
		&b.Status, &bankRef, &b.RetryCount, &lastRetryAt, &b.CreatedAt,
	)
	if err == sql.ErrNoRows {
		return nil, fmt.Errorf("settlement: batch %s not found", batchID)
	}
	if err != nil {
		return nil, fmt.Errorf("settlement: getting batch %s: %w", batchID, err)
	}
	if filePath.Valid {
		b.FilePath = &filePath.String
	}
	if bankRef.Valid {
		b.BankReference = &bankRef.String
	}
	if lastRetryAt.Valid {
		b.LastRetryAt = &lastRetryAt.Time
	}
	return &b, nil
}

// RunSettlement executes the EOD batch settlement for the given date.
//
// Processing order:
//  1. Calculate cutoff via CutoffTime(batchDate)
//  2. Query eligible FundsPosted deposits (created_at <= cutoff, not yet batched)
//  3. Return early with zero-deposit result if none eligible (no DB record created)
//  4. Create settlement_batches record in 'pending' status
//  5. Generate settlement file BEFORE any state transitions — safe to retry on failure
//  6. For each transfer: open tx, transition FundsPosted→Completed, set settlement_batch_id, commit
//  7. Update batch record with final counts and mark 'submitted'
//  8. Simulate bank ACK — if acknowledged, mark 'acknowledged'; if not, mark 'retry_pending'
func (s *Service) RunSettlement(ctx context.Context, batchDate time.Time) (*Batch, error) {
	cutoff := CutoffTime(batchDate)
	now := time.Now().UTC()

	// Count deposits queued for next business day (created after today's cutoff).
	// Always computed so the response is informative even when batching today's deposits.
	rolledCount := 0
	if now.After(cutoff) {
		var err error
		rolledCount, err = s.countDepositsAfterCutoff(ctx, cutoff)
		if err != nil {
			return nil, err
		}
	}

	transfers, err := s.getEligibleDeposits(ctx, cutoff)
	if err != nil {
		return nil, err
	}

	// No eligible deposits for today's cutoff window.
	if len(transfers) == 0 {
		result := &Batch{
			ID:               uuid.New(),
			BatchDate:        batchDate,
			DepositCount:     0,
			TotalAmountCents: 0,
			CreatedAt:        time.Now().UTC(),
		}
		if rolledCount > 0 {
			nextDay := nextBusinessDay(batchDate)
			nextDayStr := nextDay.Format("2006-01-02")
			result.Status = "rolled_to_next_day"
			result.NextSettlementDate = &nextDayStr
			result.DepositsRolledToNextDay = rolledCount
		} else {
			result.Status = "submitted"
		}
		return result, nil
	}

	// Create the batch record in pending status before generating the file.
	batch := &Batch{
		ID:        uuid.New(),
		BatchDate: batchDate,
		Status:    "pending",
		CreatedAt: time.Now().UTC(),
	}
	if _, err := s.db.ExecContext(ctx, `
		INSERT INTO settlement_batches (id, batch_date, status)
		VALUES ($1, $2, $3)`,
		batch.ID, batch.BatchDate.Format("2006-01-02"), batch.Status,
	); err != nil {
		return nil, fmt.Errorf("settlement: creating batch record: %w", err)
	}

	// Generate the settlement file FIRST — before any state changes.
	// If generation fails, the batch record stays pending but no transfers
	// have moved state, so the entire run is safe to retry.
	filePath, err := Generate(transfers, s.outputDir, batchDate)
	if err != nil {
		return nil, fmt.Errorf("settlement: generating settlement file: %w", err)
	}

	// Transition each eligible transfer individually.
	// Partial success is acceptable: the X9 file already exists, and
	// successfully committed transfers will not be re-processed (settlement_batch_id is set).
	var totalCents int64
	completed := 0
	for _, t := range transfers {
		txn, err := s.db.BeginTx(ctx, nil)
		if err != nil {
			continue
		}

		if err := s.machine.Transition(ctx, txn, t.ID,
			models.StatusFundsPosted, models.StatusCompleted,
			"system:settlement",
			map[string]any{"batch_id": batch.ID.String()},
		); err != nil {
			txn.Rollback()
			continue
		}

		if _, err := txn.ExecContext(ctx,
			`UPDATE transfers SET settlement_batch_id = $1, updated_at = NOW() WHERE id = $2`,
			batch.ID, t.ID,
		); err != nil {
			txn.Rollback()
			continue
		}

		if err := txn.Commit(); err != nil {
			txn.Rollback()
			continue
		}

		totalCents += t.AmountCents
		completed++
	}

	batch.DepositCount = completed
	batch.TotalAmountCents = totalCents
	batch.FilePath = &filePath
	batch.DepositsRolledToNextDay = rolledCount
	if rolledCount > 0 {
		nextDay := nextBusinessDay(batchDate)
		nextDayStr := nextDay.Format("2006-01-02")
		batch.NextSettlementDate = &nextDayStr
	}

	// Simulate bank ACK. If acknowledged, mark the batch submitted/acknowledged.
	// If not acknowledged, mark retry_pending with retry_count=1.
	if s.simulateBankAck() {
		batch.Status = "submitted"
		if _, err := s.db.ExecContext(ctx, `
			UPDATE settlement_batches
			SET file_path = $1, deposit_count = $2, total_amount_cents = $3, status = 'submitted'
			WHERE id = $4`,
			filePath, completed, totalCents, batch.ID,
		); err != nil {
			return nil, fmt.Errorf("settlement: updating batch record: %w", err)
		}
	} else {
		// Bank did not acknowledge — mark as retry_pending
		batch.Status = "retry_pending"
		batch.RetryCount = 1
		now := time.Now().UTC()
		batch.LastRetryAt = &now
		if _, err := s.db.ExecContext(ctx, `
			UPDATE settlement_batches
			SET file_path = $1, deposit_count = $2, total_amount_cents = $3,
			    status = 'retry_pending', retry_count = 1, last_retry_at = NOW()
			WHERE id = $4`,
			filePath, completed, totalCents, batch.ID,
		); err != nil {
			return nil, fmt.Errorf("settlement: updating batch record (retry_pending): %w", err)
		}
	}

	return batch, nil
}

// RetryBatch re-attempts bank submission for a batch in retry_pending state.
// Increments retry_count on each attempt. After maxRetries failures, escalates to operator.
func (s *Service) RetryBatch(ctx context.Context, batchID uuid.UUID) (*Batch, error) {
	batch, err := s.getBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}

	if batch.Status != "retry_pending" {
		return nil, fmt.Errorf("settlement: batch %s is not in retry_pending state (current: %s)", batchID, batch.Status)
	}

	newRetryCount := batch.RetryCount + 1
	now := time.Now().UTC()

	if s.simulateBankAck() {
		// Bank acknowledged — mark as submitted
		batch.Status = "submitted"
		batch.RetryCount = newRetryCount
		batch.LastRetryAt = &now
		if _, err := s.db.ExecContext(ctx, `
			UPDATE settlement_batches
			SET status = 'submitted', retry_count = $1, last_retry_at = NOW()
			WHERE id = $2`,
			newRetryCount, batchID,
		); err != nil {
			return nil, fmt.Errorf("settlement: updating batch after successful retry: %w", err)
		}
	} else if newRetryCount >= s.maxRetries {
		// Max retries exceeded — escalate to operator
		batch.Status = "escalated"
		batch.RetryCount = newRetryCount
		batch.LastRetryAt = &now
		if _, err := s.db.ExecContext(ctx, `
			UPDATE settlement_batches
			SET status = 'escalated', retry_count = $1, last_retry_at = NOW()
			WHERE id = $2`,
			newRetryCount, batchID,
		); err != nil {
			return nil, fmt.Errorf("settlement: escalating batch: %w", err)
		}
	} else {
		// Still failing — stay in retry_pending with incremented count
		batch.Status = "retry_pending"
		batch.RetryCount = newRetryCount
		batch.LastRetryAt = &now
		if _, err := s.db.ExecContext(ctx, `
			UPDATE settlement_batches
			SET retry_count = $1, last_retry_at = NOW()
			WHERE id = $2`,
			newRetryCount, batchID,
		); err != nil {
			return nil, fmt.Errorf("settlement: updating retry count: %w", err)
		}
	}

	return batch, nil
}

// BatchDetail extends Batch with the list of deposits included in the batch.
type BatchDetail struct {
	*Batch
	Deposits []*models.Transfer `json:"deposits"`
}

// EODStatus describes the current settlement window.
type EODStatus struct {
	CurrentTime         time.Time `json:"current_time"`
	CutoffTime          time.Time `json:"cutoff_time"`
	PastCutoff          bool      `json:"past_cutoff"`
	PendingDepositCount int       `json:"pending_deposit_count"`
	PendingAmountCents  int64     `json:"pending_amount_cents"`
}

// ListBatches returns all settlement batches ordered newest first.
func (s *Service) ListBatches(ctx context.Context) ([]Batch, error) {
	rows, err := s.db.QueryContext(ctx, `
		SELECT id, batch_date, file_path, deposit_count, total_amount_cents,
		       status, bank_reference, retry_count, last_retry_at, created_at
		FROM settlement_batches
		ORDER BY created_at DESC`)
	if err != nil {
		return nil, fmt.Errorf("settlement: listing batches: %w", err)
	}
	defer rows.Close()

	var batches []Batch
	for rows.Next() {
		var b Batch
		var filePath, bankRef sql.NullString
		var lastRetryAt sql.NullTime
		if err := rows.Scan(
			&b.ID, &b.BatchDate, &filePath, &b.DepositCount, &b.TotalAmountCents,
			&b.Status, &bankRef, &b.RetryCount, &lastRetryAt, &b.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("settlement: scanning batch row: %w", err)
		}
		if filePath.Valid {
			b.FilePath = &filePath.String
		}
		if bankRef.Valid {
			b.BankReference = &bankRef.String
		}
		if lastRetryAt.Valid {
			b.LastRetryAt = &lastRetryAt.Time
		}
		batches = append(batches, b)
	}
	return batches, rows.Err()
}

// GetBatchWithDeposits returns a batch and the transfers assigned to it.
func (s *Service) GetBatchWithDeposits(ctx context.Context, batchID uuid.UUID) (*BatchDetail, error) {
	batch, err := s.getBatch(ctx, batchID)
	if err != nil {
		return nil, err
	}

	rows, err := s.db.QueryContext(ctx, `
		SELECT id, account_id, amount_cents, declared_amount_cents, status, flagged,
		       flag_reason, contribution_type, vendor_transaction_id, micr_routing,
		       micr_account, micr_serial, micr_confidence, ocr_amount_cents,
		       front_image_ref, back_image_ref, settlement_batch_id, return_reason,
		       created_at, updated_at
		FROM transfers
		WHERE settlement_batch_id = $1
		ORDER BY created_at ASC`, batchID)
	if err != nil {
		return nil, fmt.Errorf("settlement: querying batch deposits: %w", err)
	}
	defer rows.Close()

	var deposits []*models.Transfer
	for rows.Next() {
		var t models.Transfer
		var settlementBatchIDStr sql.NullString
		if err := rows.Scan(
			&t.ID, &t.AccountID, &t.AmountCents, &t.DeclaredAmountCents,
			&t.Status, &t.Flagged, &t.FlagReason, &t.ContributionType,
			&t.VendorTransactionID, &t.MICRRouting, &t.MICRAccount,
			&t.MICRSerial, &t.MICRConfidence, &t.OCRAmountCents,
			&t.FrontImageRef, &t.BackImageRef, &settlementBatchIDStr,
			&t.ReturnReason, &t.CreatedAt, &t.UpdatedAt,
		); err != nil {
			return nil, fmt.Errorf("settlement: scanning deposit row: %w", err)
		}
		if settlementBatchIDStr.Valid {
			id, _ := uuid.Parse(settlementBatchIDStr.String)
			t.SettlementBatchID = &id
		}
		deposits = append(deposits, &t)
	}
	if err := rows.Err(); err != nil {
		return nil, err
	}
	if deposits == nil {
		deposits = []*models.Transfer{}
	}

	return &BatchDetail{Batch: batch, Deposits: deposits}, nil
}

// GetEODStatus returns the current cutoff state and count of deposits awaiting settlement.
func (s *Service) GetEODStatus(ctx context.Context) (*EODStatus, error) {
	now := time.Now().UTC()
	cutoff := CutoffTime(now)

	var count int
	var totalCents int64
	err := s.db.QueryRowContext(ctx, `
		SELECT COUNT(*), COALESCE(SUM(amount_cents),0)
		FROM transfers
		WHERE status = 'funds_posted' AND settlement_batch_id IS NULL`).
		Scan(&count, &totalCents)
	if err != nil {
		return nil, fmt.Errorf("settlement: querying pending deposits: %w", err)
	}

	return &EODStatus{
		CurrentTime:         now,
		CutoffTime:          cutoff,
		PastCutoff:          now.After(cutoff),
		PendingDepositCount: count,
		PendingAmountCents:  totalCents,
	}, nil
}
