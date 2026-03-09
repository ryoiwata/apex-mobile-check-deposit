package vendor

import "context"

// Request contains the data sent to the vendor for validation.
type Request struct {
	TransferID          string `json:"transfer_id"`
	AccountID           string `json:"account_id"`
	FrontImageRef       string `json:"front_image_ref"`
	BackImageRef        string `json:"back_image_ref"`
	DeclaredAmountCents int64  `json:"declared_amount_cents"`
}

// MICRData represents the extracted MICR line data.
type MICRData struct {
	RoutingNumber string  `json:"routing_number"`
	AccountNumber string  `json:"account_number"`
	CheckSerial   string  `json:"check_serial"`
	Confidence    float64 `json:"confidence"`
}

// Response is what the vendor returns for every validation request.
type Response struct {
	Status         string    `json:"status"`          // "pass", "fail", "flagged"
	IQAResult      string    `json:"iqa_result"`       // "pass", "fail_blur", "fail_glare"
	MICRData       *MICRData `json:"micr_data"`        // nil on MICR failure
	OCRAmountCents *int64    `json:"ocr_amount_cents"` // nil on IQA fail
	DuplicateCheck string    `json:"duplicate_check"`  // "clear", "duplicate_found"
	AmountMatch    bool      `json:"amount_match"`
	TransactionID  string    `json:"transaction_id"`  // vendor-side reference
	ErrorCode      *string   `json:"error_code"`
	ErrorMessage   *string   `json:"error_message"`
}

// Service is the interface all vendor implementations satisfy.
// Production would swap Stub for an HTTP client.
type Service interface {
	Validate(ctx context.Context, req *Request) (*Response, error)
}
