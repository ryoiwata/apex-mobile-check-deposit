package state

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"sync"
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

// insertTestTransfer creates a minimal transfer row with the given status.
// It inserts a row directly to bypass the state machine.
func insertTestTransfer(t *testing.T, db *sql.DB, id uuid.UUID, status models.TransferStatus) {
	t.Helper()
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO transfers
			(id, account_id, amount_cents, declared_amount_cents, status)
		VALUES ($1, $2, $3, $4, $5)`,
		id, "ACC-SOFI-1006", 100000, 100000, string(status))
	require.NoError(t, err, "insertTestTransfer")
}

// cleanupTransfer removes the transfer and associated state_transitions rows.
func cleanupTransfer(t *testing.T, db *sql.DB, id uuid.UUID) {
	t.Helper()
	db.ExecContext(context.Background(), `DELETE FROM state_transitions WHERE transfer_id = $1`, id)
	db.ExecContext(context.Background(), `DELETE FROM transfers WHERE id = $1`, id)
}

func TestValidTransition_RequestedToValidating(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertTestTransfer(t, db, id, models.StatusRequested)
	defer cleanupTransfer(t, db, id)

	machine := New(db)
	tx, err := machine.BeginAndTransition(context.Background(), id,
		models.StatusRequested, models.StatusValidating, "test", nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	// Verify state_transitions row was created
	var count int
	err = db.QueryRowContext(context.Background(),
		`SELECT COUNT(*) FROM state_transitions WHERE transfer_id = $1 AND from_state = $2 AND to_state = $3`,
		id, "requested", "validating").Scan(&count)
	require.NoError(t, err)
	assert.Equal(t, 1, count, "expected one state_transitions row")

	// Verify transfer status updated
	var status string
	err = db.QueryRowContext(context.Background(),
		`SELECT status FROM transfers WHERE id = $1`, id).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "validating", status)
}

func TestValidTransition_CompletedToReturned(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertTestTransfer(t, db, id, models.StatusCompleted)
	defer cleanupTransfer(t, db, id)

	machine := New(db)
	tx, err := machine.BeginAndTransition(context.Background(), id,
		models.StatusCompleted, models.StatusReturned, "test", nil)
	require.NoError(t, err)
	require.NoError(t, tx.Commit())

	var status string
	err = db.QueryRowContext(context.Background(),
		`SELECT status FROM transfers WHERE id = $1`, id).Scan(&status)
	require.NoError(t, err)
	assert.Equal(t, "returned", status)
}

func TestInvalidTransitions(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	tests := []struct {
		name   string
		from   models.TransferStatus
		to     models.TransferStatus
		status models.TransferStatus // initial status to insert
	}{
		{
			name:   "requested to approved skips states",
			from:   models.StatusRequested,
			to:     models.StatusApproved,
			status: models.StatusRequested,
		},
		{
			name:   "completed to approved is backwards",
			from:   models.StatusCompleted,
			to:     models.StatusApproved,
			status: models.StatusCompleted,
		},
		{
			name:   "rejected to funds_posted is terminal",
			from:   models.StatusRejected,
			to:     models.StatusFundsPosted,
			status: models.StatusRejected,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			id := uuid.New()
			insertTestTransfer(t, db, id, tt.status)
			defer cleanupTransfer(t, db, id)

			machine := New(db)
			_, err := machine.BeginAndTransition(context.Background(), id,
				tt.from, tt.to, "test", nil)
			require.Error(t, err)
			assert.True(t, errors.Is(err, models.ErrInvalidStateTransition),
				"expected ErrInvalidStateTransition, got: %v", err)
		})
	}
}

func TestInvalidTransition_RequestedToApproved(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertTestTransfer(t, db, id, models.StatusRequested)
	defer cleanupTransfer(t, db, id)

	machine := New(db)
	_, err := machine.BeginAndTransition(context.Background(), id,
		models.StatusRequested, models.StatusApproved, "test", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrInvalidStateTransition))
}

func TestInvalidTransition_CompletedToApproved(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertTestTransfer(t, db, id, models.StatusCompleted)
	defer cleanupTransfer(t, db, id)

	machine := New(db)
	_, err := machine.BeginAndTransition(context.Background(), id,
		models.StatusCompleted, models.StatusApproved, "test", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrInvalidStateTransition))
}

func TestInvalidTransition_RejectedToFundsPosted(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertTestTransfer(t, db, id, models.StatusRejected)
	defer cleanupTransfer(t, db, id)

	machine := New(db)
	_, err := machine.BeginAndTransition(context.Background(), id,
		models.StatusRejected, models.StatusFundsPosted, "test", nil)
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrInvalidStateTransition))
}

func TestOptimisticLock_ConcurrentTransition(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	id := uuid.New()
	insertTestTransfer(t, db, id, models.StatusRequested)
	defer cleanupTransfer(t, db, id)

	machine := New(db)

	var wg sync.WaitGroup
	errs := make([]error, 2)
	txs := make([]*sql.Tx, 2)

	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func(idx int) {
			defer wg.Done()
			tx, err := machine.BeginAndTransition(context.Background(), id,
				models.StatusRequested, models.StatusValidating,
				fmt.Sprintf("goroutine-%d", idx), nil)
			txs[idx] = tx
			errs[idx] = err
			if tx != nil {
				tx.Commit()
			}
		}(i)
	}
	wg.Wait()

	successCount := 0
	failCount := 0
	for _, err := range errs {
		if err == nil {
			successCount++
		} else if errors.Is(err, models.ErrInvalidStateTransition) {
			failCount++
		} else {
			t.Errorf("unexpected error type: %v", err)
		}
	}

	assert.Equal(t, 1, successCount, "exactly one goroutine should succeed")
	assert.Equal(t, 1, failCount, "exactly one goroutine should fail with conflict")
}
