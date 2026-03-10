package operator

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

	"github.com/apex/mcd/internal/ledger"
	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/state"
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

// insertFlaggedTransfer inserts a transfer in analyzing+flagged state for test use.
func insertFlaggedTransfer(t *testing.T, db *sql.DB) uuid.UUID {
	t.Helper()
	id := uuid.New()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO transfers
			(id, account_id, amount_cents, declared_amount_cents, status, flagged, flag_reason)
		VALUES ($1, $2, $3, $4, 'analyzing', true, 'micr_failure')`,
		id, "ACC-SOFI-1006", 100000, 100000)
	require.NoError(t, err, "insertFlaggedTransfer")
	return id
}

// cleanupTransfer removes a test transfer and all related rows.
func cleanupTransfer(t *testing.T, db *sql.DB, id uuid.UUID) {
	t.Helper()
	db.ExecContext(context.Background(), `DELETE FROM audit_logs WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM ledger_entries WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM state_transitions WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM transfers WHERE id = $1`, id)
}

// TestApprove_MovesToFundsPosted verifies that approving a flagged+analyzing deposit
// transitions it to funds_posted and posts a DEPOSIT ledger entry.
func TestApprove_MovesToFundsPosted(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := insertFlaggedTransfer(t, db)
	defer cleanupTransfer(t, db, id)

	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	transfer, err := svc.Approve(context.Background(), id, "OP-TEST", "unit test approve", nil)
	require.NoError(t, err)
	assert.Equal(t, models.StatusFundsPosted, transfer.Status,
		"approved transfer must reach funds_posted")

	// Verify a DEPOSIT ledger entry was created.
	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1 AND sub_type = 'DEPOSIT'`,
		id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "exactly one DEPOSIT ledger entry should exist after approval")
}

// TestApprove_WritesAuditLog verifies that approving a deposit writes an audit_logs row
// with action="approve" and the correct operator_id.
func TestApprove_WritesAuditLog(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := insertFlaggedTransfer(t, db)
	defer cleanupTransfer(t, db, id)

	const operatorID = "OP-AUDIT-TEST"
	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	_, err := svc.Approve(context.Background(), id, operatorID, "audit log test", nil)
	require.NoError(t, err)

	entries, err := GetAuditLog(context.Background(), db, &id)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "audit_logs must have at least one entry after approve")

	found := false
	for _, e := range entries {
		if e.Action == "approve" && e.OperatorID == operatorID {
			found = true
			break
		}
	}
	assert.True(t, found,
		"audit_logs must contain action=approve with operator_id=%s", operatorID)
}

// TestReject_MovesToRejected verifies that rejecting a flagged+analyzing deposit
// transitions it to rejected and writes an audit log entry.
func TestReject_MovesToRejected(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := insertFlaggedTransfer(t, db)
	defer cleanupTransfer(t, db, id)

	const operatorID = "OP-REJECT-TEST"
	svc := NewService(db, state.New(db), ledger.NewService(db), nil)
	transfer, err := svc.Reject(context.Background(), id, operatorID,
		"check appears altered", "irregular ink on MICR line")
	require.NoError(t, err)
	assert.Equal(t, models.StatusRejected, transfer.Status,
		"rejected transfer must reach rejected state")

	// Verify no DEPOSIT ledger entry exists.
	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM ledger_entries WHERE transfer_id = $1`, id).Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 0, count, "no ledger entries should exist for a rejected transfer")

	// Verify audit log entry.
	entries, err := GetAuditLog(context.Background(), db, &id)
	require.NoError(t, err)
	require.NotEmpty(t, entries, "audit_logs must have at least one entry after reject")

	found := false
	for _, e := range entries {
		if e.Action == "reject" && e.OperatorID == operatorID {
			found = true
			break
		}
	}
	assert.True(t, found,
		"audit_logs must contain action=reject with operator_id=%s", operatorID)
}
