package models

import (
	"time"

	"github.com/google/uuid"
)

// TransferStatus represents the state machine states.
// Use state package constants; this type lives in models to avoid import cycles.
type TransferStatus string

const (
	StatusRequested   TransferStatus = "requested"
	StatusValidating  TransferStatus = "validating"
	StatusAnalyzing   TransferStatus = "analyzing"
	StatusApproved    TransferStatus = "approved"
	StatusFundsPosted TransferStatus = "funds_posted"
	StatusCompleted   TransferStatus = "completed"
	StatusRejected    TransferStatus = "rejected"
	StatusReturned    TransferStatus = "returned"
)

// Transfer is the central domain entity. Maps 1:1 to the transfers table.
type Transfer struct {
	ID                  uuid.UUID      `json:"transfer_id" db:"id"`
	AccountID           string         `json:"account_id" db:"account_id"`
	AmountCents         int64          `json:"amount_cents" db:"amount_cents"`
	DeclaredAmountCents int64          `json:"declared_amount_cents" db:"declared_amount_cents"`
	Status              TransferStatus `json:"status" db:"status"`
	Flagged             bool           `json:"flagged" db:"flagged"`
	FlagReason          *string        `json:"flag_reason,omitempty" db:"flag_reason"`
	ContributionType    *string        `json:"contribution_type,omitempty" db:"contribution_type"`
	VendorTransactionID *string        `json:"vendor_transaction_id,omitempty" db:"vendor_transaction_id"`
	MICRRouting         *string        `json:"micr_routing,omitempty" db:"micr_routing"`
	MICRAccount         *string        `json:"micr_account,omitempty" db:"micr_account"`
	MICRSerial          *string        `json:"micr_serial,omitempty" db:"micr_serial"`
	MICRConfidence      *float64       `json:"micr_confidence,omitempty" db:"micr_confidence"`
	OCRAmountCents      *int64         `json:"ocr_amount_cents,omitempty" db:"ocr_amount_cents"`
	FrontImageRef       *string        `json:"front_image_ref,omitempty" db:"front_image_ref"`
	BackImageRef        *string        `json:"back_image_ref,omitempty" db:"back_image_ref"`
	SettlementBatchID   *uuid.UUID     `json:"settlement_batch_id,omitempty" db:"settlement_batch_id"`
	ReturnReason         *string        `json:"return_reason,omitempty" db:"return_reason"`
	RejectionReason      *string        `json:"rejection_reason,omitempty" db:"rejection_reason"`
	VerifiedAmountCents  *int64         `json:"verified_amount_cents,omitempty" db:"verified_amount_cents"`
	CreatedAt            time.Time      `json:"created_at" db:"created_at"`
	UpdatedAt            time.Time      `json:"updated_at" db:"updated_at"`
	// Transient fields — not stored in DB, populated in-memory from vendor response.
	RetakeGuidance  *string `json:"retake_guidance,omitempty" db:"-"`
	VendorErrorCode *string `json:"vendor_error_code,omitempty" db:"-"`
}

// StateTransition is an entry in the state_transitions audit table.
type StateTransition struct {
	ID          uuid.UUID      `json:"id"`
	TransferID  uuid.UUID      `json:"transfer_id"`
	FromState   TransferStatus `json:"from_state"`
	ToState     TransferStatus `json:"to_state"`
	TriggeredBy string         `json:"triggered_by"`
	Metadata    map[string]any `json:"metadata,omitempty"`
	CreatedAt   time.Time      `json:"created_at"`
}
