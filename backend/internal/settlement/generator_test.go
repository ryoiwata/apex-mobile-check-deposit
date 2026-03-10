package settlement

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

func insertSettlementTestTransfer(t *testing.T, db *sql.DB, id uuid.UUID, amountCents int64, status string) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO transfers
			(id, account_id, amount_cents, declared_amount_cents, status)
		VALUES ($1, $2, $3, $4, $5)`,
		id, "ACC-SOFI-1006", amountCents, amountCents, status)
	require.NoError(t, err, "insertSettlementTestTransfer")
}

func cleanupSettlementTransfer(t *testing.T, db *sql.DB, id uuid.UUID) {
	t.Helper()
	db.ExecContext(context.Background(), `DELETE FROM ledger_entries WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM state_transitions WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM transfers WHERE id = $1`, id)
}

// TestCutoffTime_CorrectUTCConversion verifies that CutoffTime returns 6:30 PM CT in UTC.
// In winter (CST = UTC-6), 6:30 PM CST = 00:30 UTC the following day.
func TestCutoffTime_CorrectUTCConversion(t *testing.T) {
	// January 15, 2026 — standard time (CST = UTC-6)
	winterDate := time.Date(2026, 1, 15, 12, 0, 0, 0, time.UTC)
	cutoff := CutoffTime(winterDate)

	// 6:30 PM CST (UTC-6) = 00:30 UTC next day
	assert.Equal(t, 0, cutoff.Hour(), "cutoff hour in UTC should be 00 (winter: CST=UTC-6)")
	assert.Equal(t, 30, cutoff.Minute(), "cutoff minute should be 30")
	assert.Equal(t, 16, cutoff.Day(), "cutoff should be Jan 16 in UTC (next day from CST offset)")
}

// TestCutoffTime_DST_Summer verifies CutoffTime correctly shifts for CDT (UTC-5).
// In summer (CDT = UTC-5), 6:30 PM CDT = 23:30 UTC the same day.
func TestCutoffTime_DST_Summer(t *testing.T) {
	// July 15, 2026 — daylight saving time (CDT = UTC-5)
	summerDate := time.Date(2026, 7, 15, 12, 0, 0, 0, time.UTC)
	cutoff := CutoffTime(summerDate)

	// 6:30 PM CDT (UTC-5) = 23:30 UTC same day
	assert.Equal(t, 23, cutoff.Hour(), "cutoff hour in UTC should be 23 (summer: CDT=UTC-5)")
	assert.Equal(t, 30, cutoff.Minute(), "cutoff minute should be 30")
	assert.Equal(t, 15, cutoff.Day(), "cutoff should be same day (Jul 15) in UTC during summer")
}

// TestSettlement_ExcludesRejected verifies that rejected transfers do not appear
// in getEligibleDeposits even when their created_at is before the cutoff.
func TestSettlement_ExcludesRejected(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	rejectedID := uuid.New()
	fundsPostedID := uuid.New()

	insertSettlementTestTransfer(t, db, rejectedID, 50000, "rejected")
	insertSettlementTestTransfer(t, db, fundsPostedID, 75000, "funds_posted")
	defer cleanupSettlementTransfer(t, db, rejectedID)
	defer cleanupSettlementTransfer(t, db, fundsPostedID)

	svc := NewService(db, state.New(db), t.TempDir())
	cutoff := time.Now().Add(1 * time.Hour) // includes both inserts

	transfers, err := svc.getEligibleDeposits(context.Background(), cutoff)
	require.NoError(t, err)

	for _, tr := range transfers {
		assert.NotEqual(t, rejectedID, tr.ID, "rejected transfer should not appear in eligible deposits")
	}

	found := false
	for _, tr := range transfers {
		if tr.ID == fundsPostedID {
			found = true
			break
		}
	}
	assert.True(t, found, "funds_posted transfer should appear in eligible deposits")
}

// TestSettlement_ExcludesAlreadyBatched verifies that transfers already assigned
// to a settlement batch are excluded from getEligibleDeposits.
func TestSettlement_ExcludesAlreadyBatched(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	batchedID := uuid.New()
	insertSettlementTestTransfer(t, db, batchedID, 60000, "funds_posted")
	defer cleanupSettlementTransfer(t, db, batchedID)

	// Assign a batch ID directly, simulating an already-processed transfer.
	existingBatchID := uuid.New()
	_, err := db.ExecContext(context.Background(),
		`UPDATE transfers SET settlement_batch_id = $1 WHERE id = $2`,
		existingBatchID, batchedID)
	require.NoError(t, err)

	svc := NewService(db, state.New(db), t.TempDir())
	cutoff := time.Now().Add(1 * time.Hour)

	transfers, err := svc.getEligibleDeposits(context.Background(), cutoff)
	require.NoError(t, err)

	for _, tr := range transfers {
		assert.NotEqual(t, batchedID, tr.ID, "already-batched transfer should not appear in eligible deposits")
	}
}
