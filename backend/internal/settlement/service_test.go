package settlement

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"testing"
	"time"

	"github.com/apex/mcd/internal/state"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertSettlementTestTransferAt inserts a transfer with a specific created_at timestamp.
// Use this when EOD cutoff eligibility depends on precise creation time.
func insertSettlementTestTransferAt(t *testing.T, db *sql.DB, id uuid.UUID, amountCents int64, status string, createdAt time.Time) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO transfers
			(id, account_id, amount_cents, declared_amount_cents, status, created_at, updated_at)
		VALUES ($1, $2, $3, $4, $5, $6, $6)`,
		id, "ACC-SOFI-1006", amountCents, amountCents, status, createdAt)
	require.NoError(t, err, "insertSettlementTestTransferAt")
}

// cleanupSettlementBatch removes a settlement batch record and unlinks it from any transfers.
func cleanupSettlementBatch(t *testing.T, db *sql.DB, batchID uuid.UUID) {
	t.Helper()
	db.ExecContext(context.Background(),
		`UPDATE transfers SET settlement_batch_id = NULL WHERE settlement_batch_id = $1`, batchID)
	db.ExecContext(context.Background(),
		`DELETE FROM settlement_batches WHERE id = $1`, batchID)
}

// TestSettlement_HappyPath: create 3 FundsPosted deposits, run settlement, assert all 3 reach
// Completed, a settlement file is written to disk, and the batch totals are correct.
func TestSettlement_HappyPath(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id1, id2, id3 := uuid.New(), uuid.New(), uuid.New()
	insertSettlementTestTransfer(t, db, id1, 100000, "funds_posted") // $1,000
	insertSettlementTestTransfer(t, db, id2, 200000, "funds_posted") // $2,000
	insertSettlementTestTransfer(t, db, id3, 300000, "funds_posted") // $3,000
	defer cleanupSettlementTransfer(t, db, id1)
	defer cleanupSettlementTransfer(t, db, id2)
	defer cleanupSettlementTransfer(t, db, id3)

	dir := t.TempDir()
	svc := NewService(db, state.New(db), dir)

	// Far-future batch date ensures all deposits fall before the cutoff.
	batchDate := time.Now().AddDate(1, 0, 0)
	batch, err := svc.RunSettlement(context.Background(), batchDate)
	require.NoError(t, err)
	require.NotNil(t, batch)
	defer cleanupSettlementBatch(t, db, batch.ID)

	assert.Equal(t, 3, batch.DepositCount, "all 3 FundsPosted deposits should be in the batch")
	assert.Equal(t, int64(600000), batch.TotalAmountCents, "total should be $6,000")
	assert.Equal(t, "submitted", batch.Status)
	require.NotNil(t, batch.FilePath, "settlement file path should be set")

	// Settlement file must exist on disk.
	_, statErr := os.Stat(*batch.FilePath)
	assert.NoError(t, statErr, "settlement file must be written to disk")

	// All 3 deposits must have transitioned to Completed.
	for _, id := range []uuid.UUID{id1, id2, id3} {
		var status string
		err := db.QueryRowContext(context.Background(),
			`SELECT status FROM transfers WHERE id = $1`, id).Scan(&status)
		require.NoError(t, err)
		assert.Equal(t, "completed", status, "transfer %s must be Completed after settlement", id)
	}
}

// TestSettlement_EODCutoff: one deposit created before the cutoff and one after.
// Only the pre-cutoff deposit should be included in the batch; the post-cutoff
// deposit must remain FundsPosted and be counted as rolled to the next business day.
func TestSettlement_EODCutoff(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	// Use a past batch date so its 6:30 PM CT cutoff is clearly in the past.
	batchDate := time.Now().AddDate(-1, 0, 0)
	cutoff := CutoffTime(batchDate)

	beforeID := uuid.New()
	afterID := uuid.New()

	// Deposit created 1 hour before the cutoff — eligible for today's batch.
	insertSettlementTestTransferAt(t, db, beforeID, 100000, "funds_posted", cutoff.Add(-1*time.Hour))
	// Deposit created now — after the past cutoff, rolls to next business day.
	insertSettlementTestTransferAt(t, db, afterID, 200000, "funds_posted", time.Now())
	defer cleanupSettlementTransfer(t, db, beforeID)
	defer cleanupSettlementTransfer(t, db, afterID)

	svc := NewService(db, state.New(db), t.TempDir())
	batch, err := svc.RunSettlement(context.Background(), batchDate)
	require.NoError(t, err)
	require.NotNil(t, batch)
	defer cleanupSettlementBatch(t, db, batch.ID)

	// Only the pre-cutoff deposit settles.
	assert.Equal(t, 1, batch.DepositCount, "only the pre-cutoff deposit should be settled")
	assert.Equal(t, int64(100000), batch.TotalAmountCents)

	// Pre-cutoff deposit: must be Completed.
	var beforeStatus string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT status FROM transfers WHERE id = $1`, beforeID).Scan(&beforeStatus))
	assert.Equal(t, "completed", beforeStatus, "pre-cutoff deposit must be Completed")

	// Post-cutoff deposit: must still be FundsPosted.
	var afterStatus string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT status FROM transfers WHERE id = $1`, afterID).Scan(&afterStatus))
	assert.Equal(t, "funds_posted", afterStatus, "post-cutoff deposit must remain FundsPosted")

	// Rolled count must reflect the post-cutoff deposit.
	assert.Greater(t, batch.DepositsRolledToNextDay, 0, "rolled count must be at least 1")
	assert.NotNil(t, batch.NextSettlementDate)
}

// TestSettlement_NoDuplicates: run settlement twice against the same deposit.
// The second run must produce an empty batch — the already-Completed, already-batched
// deposit must not be re-included.
func TestSettlement_NoDuplicates(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertSettlementTestTransfer(t, db, id, 125000, "funds_posted")
	defer cleanupSettlementTransfer(t, db, id)

	svc := NewService(db, state.New(db), t.TempDir())
	batchDate := time.Now().AddDate(1, 0, 0)

	// First settlement — deposit is eligible and should be included.
	batch1, err := svc.RunSettlement(context.Background(), batchDate)
	require.NoError(t, err)
	assert.Equal(t, 1, batch1.DepositCount, "first settlement should include the deposit")
	defer cleanupSettlementBatch(t, db, batch1.ID)

	// Second settlement — deposit is now Completed with a settlement_batch_id set.
	// Neither condition in the WHERE clause (status='funds_posted', batch_id IS NULL) is satisfied.
	batch2, err := svc.RunSettlement(context.Background(), batchDate)
	require.NoError(t, err)
	assert.Equal(t, 0, batch2.DepositCount, "second settlement must not re-include the already-settled deposit")
	assert.Equal(t, int64(0), batch2.TotalAmountCents)
}

// TestSettlement_Reconciliation: the settlement file's TotalAmountCents must equal the
// arithmetic sum of every CheckDetail's AmountCents — no rounding, no omissions.
func TestSettlement_Reconciliation(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	// Use amounts with irregular cents to catch any rounding mistakes.
	type deposit struct {
		id          uuid.UUID
		amountCents int64
	}
	deposits := []deposit{
		{uuid.New(), 111111}, // $1,111.11
		{uuid.New(), 222222}, // $2,222.22
		{uuid.New(), 333333}, // $3,333.33
	}
	var wantTotal int64
	for _, d := range deposits {
		insertSettlementTestTransfer(t, db, d.id, d.amountCents, "funds_posted")
		defer cleanupSettlementTransfer(t, db, d.id)
		wantTotal += d.amountCents
	}

	dir := t.TempDir()
	svc := NewService(db, state.New(db), dir)

	batchDate := time.Now().AddDate(1, 0, 0)
	batch, err := svc.RunSettlement(context.Background(), batchDate)
	require.NoError(t, err)
	require.NotNil(t, batch.FilePath, "settlement file path must be set")
	defer cleanupSettlementBatch(t, db, batch.ID)

	// Parse the settlement file.
	raw, err := os.ReadFile(*batch.FilePath)
	require.NoError(t, err, "settlement file must be readable")

	var file settlementFile
	require.NoError(t, json.Unmarshal(raw, &file), "settlement file must be valid JSON")

	// Recompute the sum from the check records.
	var checksTotal int64
	for _, check := range file.Checks {
		checksTotal += check.AmountCents
	}

	assert.Equal(t, int64(3), int64(len(file.Checks)), "file must contain exactly 3 check records")
	assert.Equal(t, wantTotal, file.TotalAmountCents,
		"file TotalAmountCents must match the known sum of all deposit amounts")
	assert.Equal(t, checksTotal, file.TotalAmountCents,
		"file TotalAmountCents must equal the arithmetic sum of all CheckDetail amounts (reconciliation)")
}

// TestSettlement_BankAcknowledgment: after a successful settlement run with ACK mode "pass",
// the batch status must be "submitted" and every included deposit must be in Completed state.
func TestSettlement_BankAcknowledgment(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertSettlementTestTransfer(t, db, id, 150000, "funds_posted") // $1,500
	defer cleanupSettlementTransfer(t, db, id)

	svc := NewService(db, state.New(db), t.TempDir())
	svc.SetBankAckMode("pass") // bank acknowledges on first attempt

	batchDate := time.Now().AddDate(1, 0, 0)
	batch, err := svc.RunSettlement(context.Background(), batchDate)
	require.NoError(t, err)
	require.NotNil(t, batch)
	defer cleanupSettlementBatch(t, db, batch.ID)

	assert.Equal(t, "submitted", batch.Status,
		"bank-acknowledged batch must have 'submitted' status")
	assert.Equal(t, 1, batch.DepositCount)
	assert.Equal(t, int64(150000), batch.TotalAmountCents)
	assert.Equal(t, 0, batch.RetryCount, "no retries should occur when bank acknowledges immediately")

	// The deposit must be in Completed state — bank ACK confirms the funds transfer.
	var status string
	require.NoError(t, db.QueryRowContext(context.Background(),
		`SELECT status FROM transfers WHERE id = $1`, id).Scan(&status))
	assert.Equal(t, "completed", status,
		"deposit must be Completed after bank acknowledges the settlement batch")
}
