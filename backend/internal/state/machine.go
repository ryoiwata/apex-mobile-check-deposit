package state

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"

	"github.com/apex/mcd/internal/models"
	"github.com/google/uuid"
)

// Machine manages transfer state transitions with optimistic locking.
type Machine struct {
	db *sql.DB
}

// New creates a new Machine backed by the given database connection.
func New(db *sql.DB) *Machine {
	return &Machine{db: db}
}

// Transition validates and applies a state change within the provided transaction.
// Writes to both transfers (status) and state_transitions (audit) atomically.
// Returns ErrInvalidStateTransition if the from→to pair is not allowed,
// or if another goroutine already changed the status (0 rows affected).
func (m *Machine) Transition(
	ctx context.Context,
	tx *sql.Tx,
	id uuid.UUID,
	from, to models.TransferStatus,
	triggeredBy string,
	metadata map[string]any,
) error {
	if !IsValid(from, to) {
		return fmt.Errorf("%w: %s → %s", models.ErrInvalidStateTransition, from, to)
	}

	result, err := tx.ExecContext(ctx, `
		UPDATE transfers
		SET status = $1, updated_at = NOW()
		WHERE id = $2 AND status = $3`,
		string(to), id, string(from))
	if err != nil {
		return fmt.Errorf("state: updating transfer %s: %w", id, err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		return fmt.Errorf("state: checking rows affected: %w", err)
	}
	if rows == 0 {
		return fmt.Errorf("%w: %s → %s (conflict or transfer not found)",
			models.ErrInvalidStateTransition, from, to)
	}

	metaJSON, _ := json.Marshal(metadata)
	_, err = tx.ExecContext(ctx, `
		INSERT INTO state_transitions (transfer_id, from_state, to_state, triggered_by, metadata)
		VALUES ($1, $2, $3, $4, $5)`,
		id, string(from), string(to), triggeredBy, metaJSON)
	if err != nil {
		return fmt.Errorf("state: logging transition for %s: %w", id, err)
	}

	return nil
}

// BeginAndTransition opens a new transaction, transitions state, and returns the tx.
// Caller is responsible for Commit or Rollback.
func (m *Machine) BeginAndTransition(
	ctx context.Context,
	id uuid.UUID,
	from, to models.TransferStatus,
	triggeredBy string,
	metadata map[string]any,
) (*sql.Tx, error) {
	tx, err := m.db.BeginTx(ctx, nil)
	if err != nil {
		return nil, fmt.Errorf("state: beginning transaction: %w", err)
	}
	if err := m.Transition(ctx, tx, id, from, to, triggeredBy, metadata); err != nil {
		tx.Rollback()
		return nil, err
	}
	return tx, nil
}
