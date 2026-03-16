package notification

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"time"

	"github.com/google/uuid"
)

// Notification represents an investor notification for a transfer event.
type Notification struct {
	ID         string          `json:"id"`
	AccountID  string          `json:"account_id"`
	TransferID string          `json:"transfer_id"`
	Type       string          `json:"type"`
	Title      string          `json:"title"`
	Message    string          `json:"message"`
	Metadata   json.RawMessage `json:"metadata"`
	Read       bool            `json:"read"`
	CreatedAt  time.Time       `json:"created_at"`
}

// Repo handles notification persistence.
type Repo struct {
	db *sql.DB
}

// NewRepo creates a notification Repo.
func NewRepo(db *sql.DB) *Repo {
	return &Repo{db: db}
}

// Create inserts a new notification. Assigns a UUID if ID is empty.
func (r *Repo) Create(ctx context.Context, n *Notification) error {
	if n.ID == "" {
		n.ID = uuid.New().String()
	}
	meta := n.Metadata
	if meta == nil {
		meta = json.RawMessage("{}")
	}
	_, err := r.db.ExecContext(ctx,
		`INSERT INTO notifications (id, account_id, transfer_id, type, title, message, metadata)
		 VALUES ($1, $2, $3, $4, $5, $6, $7)`,
		n.ID, n.AccountID, n.TransferID, n.Type, n.Title, n.Message, []byte(meta),
	)
	if err != nil {
		return fmt.Errorf("notification: creating: %w", err)
	}
	return nil
}

// GetByAccount returns notifications for an account, unread first then newest.
func (r *Repo) GetByAccount(ctx context.Context, accountID string, unreadOnly bool) ([]Notification, error) {
	query := `SELECT id, account_id, transfer_id, type, title, message, metadata, read, created_at
	          FROM notifications WHERE account_id = $1`
	if unreadOnly {
		query += ` AND read = false`
	}
	query += ` ORDER BY read ASC, created_at DESC`

	rows, err := r.db.QueryContext(ctx, query, accountID)
	if err != nil {
		return nil, fmt.Errorf("notification: querying by account: %w", err)
	}
	defer rows.Close()

	var out []Notification
	for rows.Next() {
		var n Notification
		var meta []byte
		if err := rows.Scan(&n.ID, &n.AccountID, &n.TransferID, &n.Type,
			&n.Title, &n.Message, &meta, &n.Read, &n.CreatedAt); err != nil {
			return nil, fmt.Errorf("notification: scanning row: %w", err)
		}
		if meta != nil {
			n.Metadata = json.RawMessage(meta)
		} else {
			n.Metadata = json.RawMessage("{}")
		}
		out = append(out, n)
	}
	return out, rows.Err()
}

// GetUnreadCount returns the number of unread notifications for an account.
func (r *Repo) GetUnreadCount(ctx context.Context, accountID string) (int, error) {
	var count int
	err := r.db.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM notifications WHERE account_id = $1 AND read = false`,
		accountID,
	).Scan(&count)
	if err != nil {
		return 0, fmt.Errorf("notification: counting unread: %w", err)
	}
	return count, nil
}

// MarkRead marks a single notification as read.
func (r *Repo) MarkRead(ctx context.Context, notificationID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read = true WHERE id = $1`,
		notificationID,
	)
	if err != nil {
		return fmt.Errorf("notification: marking read: %w", err)
	}
	return nil
}

// MarkAllRead marks all notifications for an account as read.
func (r *Repo) MarkAllRead(ctx context.Context, accountID string) error {
	_, err := r.db.ExecContext(ctx,
		`UPDATE notifications SET read = true WHERE account_id = $1 AND read = false`,
		accountID,
	)
	if err != nil {
		return fmt.Errorf("notification: marking all read: %w", err)
	}
	return nil
}

// FormatCents formats an int64 cent amount as a dollar string (e.g. 150000 → "$1,500.00").
func FormatCents(cents int64) string {
	dollars := cents / 100
	remainder := cents % 100
	return fmt.Sprintf("$%d.%02d", dollars, remainder)
}
