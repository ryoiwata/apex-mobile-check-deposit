package operator

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// AuditEntry represents one row in the audit_logs table.
type AuditEntry struct {
	ID         uuid.UUID      `json:"id"`
	OperatorID string         `json:"operator_id"`
	Action     string         `json:"action"`
	TransferID uuid.UUID      `json:"transfer_id"`
	Notes      string         `json:"notes"`
	Metadata   map[string]any `json:"metadata,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
}

// LogActionTx inserts an audit_log entry within an existing transaction.
// metadata is JSON-marshaled before insertion.
func LogActionTx(
	ctx context.Context,
	tx *sql.Tx,
	operatorID, action string,
	transferID uuid.UUID,
	notes string,
	metadata map[string]any,
) error {
	metaJSON, _ := json.Marshal(metadata)
	_, err := tx.ExecContext(ctx, `
		INSERT INTO audit_logs (operator_id, action, transfer_id, notes, metadata)
		VALUES ($1, $2, $3, $4, $5)`,
		operatorID, action, transferID, notes, metaJSON,
	)
	if err != nil {
		return fmt.Errorf("operator: logging audit action %s for transfer %s: %w",
			action, transferID, err)
	}
	return nil
}

// GetAuditLog retrieves audit entries ordered by created_at DESC.
// If transferID is non-nil, filters to entries for that transfer only.
func GetAuditLog(ctx context.Context, db *sql.DB, transferID *uuid.UUID) ([]AuditEntry, error) {
	query := `
		SELECT id, operator_id, action, transfer_id, notes, metadata, created_at
		FROM audit_logs`
	args := []any{}

	if transferID != nil {
		query += " WHERE transfer_id = $1"
		args = append(args, *transferID)
	}
	query += " ORDER BY created_at DESC"

	rows, err := db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, fmt.Errorf("operator: querying audit log: %w", err)
	}
	defer rows.Close()

	var entries []AuditEntry
	for rows.Next() {
		var e AuditEntry
		var metaRaw []byte
		if err := rows.Scan(
			&e.ID, &e.OperatorID, &e.Action, &e.TransferID,
			&e.Notes, &metaRaw, &e.CreatedAt,
		); err != nil {
			return nil, fmt.Errorf("operator: scanning audit entry: %w", err)
		}
		if metaRaw != nil {
			_ = json.Unmarshal(metaRaw, &e.Metadata)
		}
		entries = append(entries, e)
	}
	return entries, rows.Err()
}
