package models

import "time"

// Account represents an investor account in the system.
type Account struct {
	ID              string    `json:"id" db:"id"`
	CorrespondentID string    `json:"correspondent_id" db:"correspondent_id"`
	AccountType     string    `json:"account_type" db:"account_type"`
	Status          string    `json:"status" db:"status"`
	CreatedAt       time.Time `json:"created_at" db:"created_at"`
}

// Correspondent represents a broker-dealer client (e.g., SoFi, Webull).
type Correspondent struct {
	ID               string    `json:"id" db:"id"`
	Name             string    `json:"name" db:"name"`
	OmnibusAccountID string    `json:"omnibus_account_id" db:"omnibus_account_id"`
	CreatedAt        time.Time `json:"created_at" db:"created_at"`
}

// AccountWithCorrespondent is used by funding service account resolution.
type AccountWithCorrespondent struct {
	Account
	OmnibusAccountID string `db:"omnibus_account_id"`
}
