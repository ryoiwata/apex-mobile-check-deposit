package vendor

import (
	"context"

	"github.com/google/uuid"
)

// Stub returns deterministic responses keyed by the last 4 chars of AccountID.
type Stub struct{}

// NewStub creates a new vendor stub.
func NewStub() *Stub { return &Stub{} }

// Validate returns a deterministic response based on the last 4 characters of the AccountID.
func (s *Stub) Validate(ctx context.Context, req *Request) (*Response, error) {
	suffix := extractSuffix(req.AccountID)
	txID := "VND-" + uuid.New().String()

	switch suffix {
	case "1001":
		return iqaFailBlur(txID), nil
	case "1002":
		return iqaFailGlare(txID), nil
	case "1003":
		return micrFailure(txID), nil
	case "1004":
		return duplicateDetected(txID), nil
	case "1005":
		return amountMismatch(txID, req.DeclaredAmountCents), nil
	case "1006", "0000":
		return cleanPass(txID, req.DeclaredAmountCents), nil
	default:
		return cleanPass(txID, req.DeclaredAmountCents), nil
	}
}

// extractSuffix returns the last 4 chars of accountID.
// "ACC-SOFI-1003" → "1003"
func extractSuffix(accountID string) string {
	if len(accountID) < 4 {
		return accountID
	}
	return accountID[len(accountID)-4:]
}

func iqaFailBlur(txID string) *Response {
	code, msg := "IQA_FAIL_BLUR", "Image is too blurry. Please retake the photo."
	return &Response{
		Status:         "fail",
		IQAResult:      "fail_blur",
		DuplicateCheck: "clear",
		TransactionID:  txID,
		ErrorCode:      &code,
		ErrorMessage:   &msg,
	}
}

func iqaFailGlare(txID string) *Response {
	code, msg := "IQA_FAIL_GLARE", "Image has too much glare. Please retake in better lighting."
	return &Response{
		Status:         "fail",
		IQAResult:      "fail_glare",
		DuplicateCheck: "clear",
		TransactionID:  txID,
		ErrorCode:      &code,
		ErrorMessage:   &msg,
	}
}

func micrFailure(txID string) *Response {
	// MICR failure → flagged for operator review, MICRData is nil
	return &Response{
		Status:         "flagged",
		IQAResult:      "pass",
		MICRData:       nil,
		DuplicateCheck: "clear",
		AmountMatch:    false,
		TransactionID:  txID,
	}
}

func duplicateDetected(txID string) *Response {
	code, msg := "DUPLICATE_CHECK", "This check has already been deposited."
	return &Response{
		Status:         "fail",
		IQAResult:      "pass",
		MICRData:       standardMICR(),
		DuplicateCheck: "duplicate_found",
		TransactionID:  txID,
		ErrorCode:      &code,
		ErrorMessage:   &msg,
	}
}

func amountMismatch(txID string, declared int64) *Response {
	// OCR reads a different amount than declared; flagged for operator review
	ocr := declared + 5000 // OCR reads $50 more than declared
	return &Response{
		Status:         "flagged",
		IQAResult:      "pass",
		MICRData:       standardMICR(),
		OCRAmountCents: &ocr,
		DuplicateCheck: "clear",
		AmountMatch:    false,
		TransactionID:  txID,
	}
}

func cleanPass(txID string, declared int64) *Response {
	return &Response{
		Status:         "pass",
		IQAResult:      "pass",
		MICRData:       standardMICR(),
		OCRAmountCents: &declared,
		DuplicateCheck: "clear",
		AmountMatch:    true,
		TransactionID:  txID,
	}
}

func standardMICR() *MICRData {
	return &MICRData{
		RoutingNumber: "021000021",
		AccountNumber: "123456789",
		CheckSerial:   "0001",
		Confidence:    0.97,
	}
}
