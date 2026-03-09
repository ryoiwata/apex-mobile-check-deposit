package ledger

import (
	"time"

	"github.com/google/uuid"
)

// Entry represents one row in ledger_entries. Always append-only.
type Entry struct {
	ID                  uuid.UUID `json:"id"`
	TransferID          uuid.UUID `json:"transfer_id"`
	ToAccountID         string    `json:"to_account_id"`
	FromAccountID       string    `json:"from_account_id"`
	Type                string    `json:"type"`          // always "MOVEMENT"
	SubType             string    `json:"sub_type"`      // "DEPOSIT", "REVERSAL", "RETURN_FEE"
	TransferType        string    `json:"transfer_type"` // "CHECK", "RETURN_FEE"
	Currency            string    `json:"currency"`      // always "USD"
	AmountCents         int64     `json:"amount_cents"`
	Memo                string    `json:"memo"`                  // always "FREE"
	SourceApplicationID uuid.UUID `json:"source_application_id"` //nolint:tagliatelle
	CreatedAt           time.Time `json:"created_at"`
}
