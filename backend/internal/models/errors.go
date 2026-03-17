package models

import "errors"

// Domain sentinel errors used across services.
var (
	ErrInvalidStateTransition = errors.New("invalid state transition")
	ErrTransferNotFound       = errors.New("transfer not found")
	ErrAccountNotFound        = errors.New("account not found")
	ErrAccountIneligible      = errors.New("account not eligible for deposits")
	ErrDepositOverLimit       = errors.New("deposit amount exceeds maximum limit")
	ErrDuplicateDeposit       = errors.New("duplicate deposit detected")
	ErrTransferNotReturnable  = errors.New("transfer must be in completed state to be returned")
	ErrTransferNotReviewable  = errors.New("transfer must be flagged and in analyzing state")
	ErrInvalidInput           = errors.New("invalid input")
)
