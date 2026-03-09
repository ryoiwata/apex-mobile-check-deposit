package state

import "github.com/apex/mcd/internal/models"

// Allowed defines every valid from→to pair.
// Any combination not listed here returns ErrInvalidStateTransition.
var Allowed = map[models.TransferStatus][]models.TransferStatus{
	models.StatusRequested:   {models.StatusValidating},
	models.StatusValidating:  {models.StatusAnalyzing, models.StatusRejected},
	models.StatusAnalyzing:   {models.StatusApproved, models.StatusRejected},
	models.StatusApproved:    {models.StatusFundsPosted, models.StatusRejected},
	models.StatusFundsPosted: {models.StatusCompleted},
	models.StatusCompleted:   {models.StatusReturned},
	// Terminal states: rejected, returned — no outgoing transitions
}

// IsTerminal returns true for states with no valid outgoing transitions.
func IsTerminal(s models.TransferStatus) bool {
	tos, ok := Allowed[s]
	return !ok || len(tos) == 0
}

// IsValid checks whether from→to is in the allowed table.
func IsValid(from, to models.TransferStatus) bool {
	tos, ok := Allowed[from]
	if !ok {
		return false
	}
	for _, t := range tos {
		if t == to {
			return true
		}
	}
	return false
}
