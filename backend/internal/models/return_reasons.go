package models

// ReturnReason represents a standard bank check return reason code.
type ReturnReason struct {
	Code        string `json:"code"`
	Label       string `json:"label"`
	Description string `json:"description"`
}

// ReturnReasons is the canonical list of supported check return reason codes.
// These mirror real-world bank return reasons per NACHA/X9 standards.
var ReturnReasons = []ReturnReason{
	{
		Code:        "insufficient_funds",
		Label:       "Insufficient Funds",
		Description: "The check writer's account does not have enough funds to cover the check.",
	},
	{
		Code:        "account_closed",
		Label:       "Account Closed",
		Description: "The check writer's bank account has been closed.",
	},
	{
		Code:        "stop_payment",
		Label:       "Stop Payment",
		Description: "The check writer placed a stop payment order on this check.",
	},
	{
		Code:        "signature_mismatch",
		Label:       "Signature Mismatch",
		Description: "The signature on the check does not match the bank's records.",
	},
	{
		Code:        "stale_dated",
		Label:       "Stale Dated",
		Description: "The check is too old to be honored (typically over 180 days).",
	},
	{
		Code:        "unable_to_locate",
		Label:       "Unable to Locate Account",
		Description: "The originating bank cannot locate the account referenced on the check.",
	},
	{
		Code:        "frozen_account",
		Label:       "Frozen Account",
		Description: "The check writer's account is frozen due to legal or compliance action.",
	},
}

// ValidReturnReasonCode returns the ReturnReason for the given code, or nil if not found.
func ValidReturnReasonCode(code string) *ReturnReason {
	for i := range ReturnReasons {
		if ReturnReasons[i].Code == code {
			return &ReturnReasons[i]
		}
	}
	return nil
}
