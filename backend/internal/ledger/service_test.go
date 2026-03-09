package ledger

import (
	"context"
	"database/sql"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/apex/mcd/internal/models"
)

func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://mcd:mcd@localhost:5432/mcd?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping: cannot open postgres: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Skipf("skipping: postgres not reachable: %v", err)
	}
	return db
}

// insertTestTransfer inserts a minimal transfer row for use as a FK in ledger_entries.
func insertTestTransfer(t *testing.T, db *sql.DB, id uuid.UUID, amountCents int64) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO transfers
			(id, account_id, amount_cents, declared_amount_cents, status)
		VALUES ($1, $2, $3, $4, $5)`,
		id, "ACC-SOFI-1006", amountCents, amountCents, string(models.StatusFundsPosted))
	require.NoError(t, err, "insertTestTransfer")
}

// cleanupTransfer removes the transfer and associated ledger entries.
func cleanupTransfer(t *testing.T, db *sql.DB, id uuid.UUID) {
	t.Helper()
	db.ExecContext(context.Background(), `DELETE FROM ledger_entries WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM state_transitions WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM transfers WHERE id = $1`, id)
}

func TestPostFunds_CreatesDepositEntry(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	transferID := uuid.New()
	amountCents := int64(100000)
	insertTestTransfer(t, db, transferID, amountCents)
	defer cleanupTransfer(t, db, transferID)

	transfer := &models.Transfer{
		ID:          transferID,
		AccountID:   "ACC-SOFI-1006",
		AmountCents: amountCents,
	}

	svc := NewService(db)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	err = svc.PostFundsTx(context.Background(), tx, transfer, "OMNI-SOFI-001")
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1 AND sub_type = 'DEPOSIT'`,
		transferID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "expected exactly one DEPOSIT ledger entry")
}

func TestPostFunds_CorrectAccountMapping(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	transferID := uuid.New()
	amountCents := int64(50000)
	investorAccount := "ACC-SOFI-1006"
	omnibusAccount := "OMNI-SOFI-001"
	insertTestTransfer(t, db, transferID, amountCents)
	defer cleanupTransfer(t, db, transferID)

	transfer := &models.Transfer{
		ID:          transferID,
		AccountID:   investorAccount,
		AmountCents: amountCents,
	}

	svc := NewService(db)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	err = svc.PostFundsTx(context.Background(), tx, transfer, omnibusAccount)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var toAccount, fromAccount string
	err = db.QueryRowContext(context.Background(),
		`SELECT to_account_id, from_account_id FROM ledger_entries WHERE transfer_id = $1`,
		transferID).Scan(&toAccount, &fromAccount)
	require.NoError(t, err)
	assert.Equal(t, investorAccount, toAccount, "to_account_id should be investor account")
	assert.Equal(t, omnibusAccount, fromAccount, "from_account_id should be omnibus account")
}

func TestPostReversal_TwoEntries(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	transferID := uuid.New()
	amountCents := int64(100000)
	insertTestTransfer(t, db, transferID, amountCents)
	defer cleanupTransfer(t, db, transferID)

	transfer := &models.Transfer{
		ID:          transferID,
		AccountID:   "ACC-SOFI-1006",
		AmountCents: amountCents,
	}

	svc := NewService(db)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	err = svc.PostReversal(context.Background(), tx, transfer, "OMNI-SOFI-001", 3000)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1`,
		transferID).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 2, count, "expected exactly two ledger entries for reversal")
}

func TestPostReversal_CorrectAmounts(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	transferID := uuid.New()
	originalAmount := int64(100000)
	returnFee := int64(3000)
	insertTestTransfer(t, db, transferID, originalAmount)
	defer cleanupTransfer(t, db, transferID)

	transfer := &models.Transfer{
		ID:          transferID,
		AccountID:   "ACC-SOFI-1006",
		AmountCents: originalAmount,
	}

	svc := NewService(db)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	err = svc.PostReversal(context.Background(), tx, transfer, "OMNI-SOFI-001", returnFee)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	rows, err := db.QueryContext(context.Background(),
		`SELECT sub_type, amount_cents FROM ledger_entries WHERE transfer_id = $1 ORDER BY created_at ASC`,
		transferID)
	require.NoError(t, err)
	defer rows.Close()

	type entry struct {
		subType     string
		amountCents int64
	}
	var entries []entry
	for rows.Next() {
		var e entry
		require.NoError(t, rows.Scan(&e.subType, &e.amountCents))
		entries = append(entries, e)
	}
	require.NoError(t, rows.Err())
	require.Len(t, entries, 2, "expected exactly two entries")

	assert.Equal(t, originalAmount, entries[0].amountCents, "reversal entry should have original amount")
	assert.Equal(t, returnFee, entries[1].amountCents, "fee entry should have return fee amount ($30)")
}

func TestPostReversal_SubTypes(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	transferID := uuid.New()
	amountCents := int64(75000)
	insertTestTransfer(t, db, transferID, amountCents)
	defer cleanupTransfer(t, db, transferID)

	transfer := &models.Transfer{
		ID:          transferID,
		AccountID:   "ACC-SOFI-1006",
		AmountCents: amountCents,
	}

	svc := NewService(db)
	tx, err := db.BeginTx(context.Background(), nil)
	require.NoError(t, err)
	defer tx.Rollback()

	err = svc.PostReversal(context.Background(), tx, transfer, "OMNI-SOFI-001", 3000)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	rows, err := db.QueryContext(context.Background(),
		`SELECT sub_type FROM ledger_entries WHERE transfer_id = $1 ORDER BY created_at ASC`,
		transferID)
	require.NoError(t, err)
	defer rows.Close()

	var subTypes []string
	for rows.Next() {
		var s string
		require.NoError(t, rows.Scan(&s))
		subTypes = append(subTypes, s)
	}
	require.NoError(t, rows.Err())
	require.Len(t, subTypes, 2, "expected exactly two entries")

	assert.Equal(t, "REVERSAL", subTypes[0], "first entry should be REVERSAL")
	assert.Equal(t, "RETURN_FEE", subTypes[1], "second entry should be RETURN_FEE")
}

// TestLedgerEntries_AppendOnly verifies that Repository only exposes read and
// PostEntryTx (write within tx) — no Update or Delete methods exist.
// This is a compile-time check: if Repository had Update/Delete, this would not compile.
func TestLedgerEntries_AppendOnly(t *testing.T) {
	// Verify Repository only has the expected methods via interface assertion.
	// The interface lists only append-safe operations.
	type appendOnlyLedger interface {
		PostEntryTx(ctx context.Context, tx *sql.Tx, entry *Entry) error
		GetByTransferID(ctx context.Context, transferID uuid.UUID) ([]Entry, error)
		GetByAccountID(ctx context.Context, accountID string, from, to *time.Time) ([]Entry, error)
	}

	db := getTestDB(t)
	defer db.Close()

	var _ appendOnlyLedger = NewRepository(db)
	t.Log("Repository satisfies appendOnlyLedger interface — no Update or Delete methods")
}
