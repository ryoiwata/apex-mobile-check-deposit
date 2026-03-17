package operator

import (
	"context"
	"database/sql"
	"testing"

	"github.com/apex/mcd/internal/ledger"
	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/state"
	"github.com/apex/mcd/internal/vendor"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// insertMismatchTransferDB inserts a transfer in analyzing+flagged state with
// flag_reason=amount_mismatch, carrying the given declared and OCR amounts.
func insertMismatchTransferDB(t *testing.T, db *sql.DB, declaredCents, ocrCents int64) (uuid.UUID, error) {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO transfers
			(id, account_id, amount_cents, declared_amount_cents, ocr_amount_cents,
			 status, flagged, flag_reason)
		VALUES ($1, $2, $3, $4, $5, 'analyzing', true, 'amount_mismatch')`,
		id, "ACC-SOFI-1006", declaredCents, declaredCents, ocrCents)
	return id, err
}

// ─── Pure unit tests (no DB) ──────────────────────────────────────────────────

// TestAmountMismatch_StubReturnsProvidedOCRAmount verifies that the vendor stub uses
// SimulatedOCRAmountCents when provided instead of generating its own OCR value.
func TestAmountMismatch_StubReturnsProvidedOCRAmount(t *testing.T) {
	stub := vendor.NewStub()
	declaredCents := int64(100000) // $1,000
	simulatedOCR := int64(80000)   // $800

	resp, err := stub.Validate(context.Background(), &vendor.Request{
		TransferID:              "test-id",
		AccountID:               "ACC-TEST-1006",
		DeclaredAmountCents:     declaredCents,
		Scenario:                "AMOUNT_MISMATCH",
		SimulatedOCRAmountCents: simulatedOCR,
	})

	require.NoError(t, err)
	require.NotNil(t, resp.OCRAmountCents)
	assert.Equal(t, simulatedOCR, *resp.OCRAmountCents,
		"stub must return SimulatedOCRAmountCents as OCR reading")
	assert.Equal(t, "flagged", resp.Status, "amount mismatch must flag for operator review")
	assert.False(t, resp.AmountMatch, "amount_match must be false for mismatch scenario")
}

// TestAmountMismatch_StubFallbackWhenNoOCRProvided verifies that when SimulatedOCRAmountCents
// is zero, the stub falls back to its default (80% of declared).
func TestAmountMismatch_StubFallbackWhenNoOCRProvided(t *testing.T) {
	stub := vendor.NewStub()
	declaredCents := int64(100000) // $1,000

	resp, err := stub.Validate(context.Background(), &vendor.Request{
		TransferID:          "test-id",
		AccountID:           "ACC-TEST-1006",
		DeclaredAmountCents: declaredCents,
		Scenario:            "AMOUNT_MISMATCH",
		// SimulatedOCRAmountCents not set → uses fallback
	})

	require.NoError(t, err)
	require.NotNil(t, resp.OCRAmountCents)
	assert.NotEqual(t, declaredCents, *resp.OCRAmountCents,
		"fallback OCR must differ from declared amount")
	assert.Equal(t, int64(80000), *resp.OCRAmountCents,
		"fallback must be 80%% of declared (100000 * 80/100 = 80000)")
	assert.Equal(t, "flagged", resp.Status)
}

// TestAmountMismatch_FlaggedAndRoutedToOperator verifies stub returns flagged status
// (not fail), which routes to the operator review queue (analyzing state).
func TestAmountMismatch_FlaggedAndRoutedToOperator(t *testing.T) {
	stub := vendor.NewStub()

	resp, err := stub.Validate(context.Background(), &vendor.Request{
		TransferID:          "test-flag",
		AccountID:           "ACC-TEST-1005", // account suffix 1005 → AMOUNT_MISMATCH
		DeclaredAmountCents: 50000,
	})

	require.NoError(t, err)
	assert.Equal(t, "flagged", resp.Status,
		"amount mismatch must produce 'flagged' not 'fail' — routes to operator review, not rejection")
	assert.NotNil(t, resp.MICRData, "MICR data must be present for amount mismatch (MICR read succeeded)")
}

// ─── DB-backed service tests ──────────────────────────────────────────────────

// TestAmountMismatch_ApproveWithVerifiedAmount_PostsCorrectAmount verifies that approving
// an amount_mismatch deposit with a verified amount posts that amount to the ledger.
func TestAmountMismatch_ApproveWithVerifiedAmount_PostsCorrectAmount(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id, err := insertMismatchTransferDB(t, db, 100000, 80000)
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	verifiedCents := int64(90000) // $900 — operator chose a midpoint
	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	transfer, err := svc.Approve(context.Background(), id, "OP-MISMATCH", "verified amount test", nil, &verifiedCents)
	require.NoError(t, err)
	assert.Equal(t, models.StatusFundsPosted, transfer.Status)
	assert.Equal(t, verifiedCents, transfer.AmountCents,
		"transfer.AmountCents must equal verified amount after approval")

	// Verify the ledger entry uses the verified amount
	var ledgerAmount int64
	err = db.QueryRowContext(context.Background(),
		`SELECT amount_cents FROM ledger_entries WHERE transfer_id = $1 AND sub_type = 'DEPOSIT'`,
		id).Scan(&ledgerAmount)
	require.NoError(t, err)
	assert.Equal(t, verifiedCents, ledgerAmount,
		"ledger DEPOSIT entry must use verified_amount_cents, not declared amount")
}

// TestAmountMismatch_ApproveWithoutVerifiedAmount_Returns422 verifies that attempting to
// approve an amount_mismatch deposit without specifying verified_amount_cents is rejected.
func TestAmountMismatch_ApproveWithoutVerifiedAmount_Returns422(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id, err := insertMismatchTransferDB(t, db, 100000, 80000)
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	_, err = svc.Approve(context.Background(), id, "OP-MISMATCH", "missing verified amount", nil, nil)
	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrInvalidInput,
		"missing verified_amount_cents on mismatch deposit must return ErrInvalidInput")
}

// TestAmountMismatch_VerifiedAmountExceedsLimit_Returns422 verifies that verified_amount_cents
// > $5,000 is rejected.
func TestAmountMismatch_VerifiedAmountExceedsLimit_Returns422(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id, err := insertMismatchTransferDB(t, db, 100000, 80000)
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	overLimit := int64(500001) // $5,000.01
	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	_, err = svc.Approve(context.Background(), id, "OP-MISMATCH", "over limit test", nil, &overLimit)
	require.Error(t, err)
	assert.ErrorIs(t, err, models.ErrDepositOverLimit,
		"verified_amount_cents > 500000 must return ErrDepositOverLimit")
}

// TestAmountMismatch_AuditLogIncludesAmountResolution verifies that the audit log entry
// for an approved mismatch deposit contains amount_resolution metadata.
func TestAmountMismatch_AuditLogIncludesAmountResolution(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id, err := insertMismatchTransferDB(t, db, 100000, 80000)
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	verifiedCents := int64(90000)
	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	_, err = svc.Approve(context.Background(), id, "OP-AUDIT", "audit resolution test", nil, &verifiedCents)
	require.NoError(t, err)

	entries, err := GetAuditLog(context.Background(), db, &id)
	require.NoError(t, err)
	require.NotEmpty(t, entries)

	var approveEntry *AuditEntry
	for i := range entries {
		if entries[i].Action == "approve" {
			approveEntry = &entries[i]
			break
		}
	}
	require.NotNil(t, approveEntry, "approve audit entry must exist")
	require.NotNil(t, approveEntry.Metadata, "audit entry metadata must be non-nil for mismatch approval")
	_, hasResolution := approveEntry.Metadata["amount_resolution"]
	assert.True(t, hasResolution, "audit metadata must include amount_resolution key")
}

// TestAmountMismatch_LedgerEntryUsesVerifiedAmount verifies that when the verified amount
// differs from both declared and OCR, the ledger uses exactly the verified amount.
func TestAmountMismatch_LedgerEntryUsesVerifiedAmount(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id, err := insertMismatchTransferDB(t, db, 200000, 150000) // $2,000 declared, $1,500 OCR
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	verifiedCents := int64(175000) // $1,750 — midpoint chosen by operator
	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	_, err = svc.Approve(context.Background(), id, "OP-LEDGER", "ledger test", nil, &verifiedCents)
	require.NoError(t, err)

	var amt int64
	err = db.QueryRowContext(context.Background(),
		`SELECT amount_cents FROM ledger_entries WHERE transfer_id = $1 AND sub_type = 'DEPOSIT'`,
		id).Scan(&amt)
	require.NoError(t, err)
	assert.Equal(t, verifiedCents, amt,
		"ledger amount must be exactly the verified amount, not declared or OCR")
}

// TestAmountMismatch_RejectDoesNotRequireVerifiedAmount verifies that rejecting a mismatch
// deposit succeeds without providing verified_amount_cents.
func TestAmountMismatch_RejectDoesNotRequireVerifiedAmount(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id, err := insertMismatchTransferDB(t, db, 100000, 80000)
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	transfer, err := svc.Reject(context.Background(), id, "OP-REJECT", "amounts disputed by investor", "")
	require.NoError(t, err)
	assert.Equal(t, models.StatusRejected, transfer.Status,
		"reject must succeed without verified_amount_cents")

	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1`, id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no ledger entries for rejected transfer")
}

// TestAmountMismatch_TransferStoresAllThreeAmounts verifies that after approval,
// the transfer row stores declared_amount_cents, ocr_amount_cents, and verified_amount_cents.
func TestAmountMismatch_TransferStoresAllThreeAmounts(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	declaredCents := int64(100000)
	ocrCents := int64(80000)
	verifiedCents := int64(90000)

	id, err := insertMismatchTransferDB(t, db, declaredCents, ocrCents)
	require.NoError(t, err)
	defer cleanupTransfer(t, db, id)

	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	_, err = svc.Approve(context.Background(), id, "OP-THREE", "three amounts test", nil, &verifiedCents)
	require.NoError(t, err)

	var declared, ocr int64
	var verifiedNullable sql.NullInt64
	err = db.QueryRowContext(context.Background(),
		`SELECT declared_amount_cents, ocr_amount_cents, verified_amount_cents FROM transfers WHERE id = $1`,
		id).Scan(&declared, &ocr, &verifiedNullable)
	require.NoError(t, err)
	assert.Equal(t, declaredCents, declared, "declared_amount_cents must be preserved")
	assert.Equal(t, ocrCents, ocr, "ocr_amount_cents must be preserved")
	require.True(t, verifiedNullable.Valid, "verified_amount_cents must be set after approval")
	assert.Equal(t, verifiedCents, verifiedNullable.Int64, "verified_amount_cents must match operator input")
}
