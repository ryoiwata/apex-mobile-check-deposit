package vendor

import (
	"context"

	"github.com/google/uuid"
)

// Stub returns deterministic responses keyed by the last 4 chars of AccountID.
type Stub struct{}

// NewStub creates a new vendor stub.
func NewStub() *Stub { return &Stub{} }

// Validate returns a deterministic response based on req.Scenario when set, falling
// back to the last 4 characters of AccountID for backward compatibility with tests
// that construct Request without a Scenario field.
func (s *Stub) Validate(ctx context.Context, req *Request) (*Response, error) {
	txID := "VND-" + uuid.New().String()

	scenario := req.Scenario
	if scenario == "" {
		scenario = scenarioFromSuffix(extractSuffix(req.AccountID))
	}

	switch scenario {
	case "IQA_FAIL_BLUR":
		return iqaFailBlur(txID), nil
	case "IQA_FAIL_GLARE":
		return iqaFailGlare(txID), nil
	case "MICR_READ_FAILURE":
		return micrFailure(txID), nil
	case "DUPLICATE_DETECTED":
		return duplicateDetected(txID), nil
	case "AMOUNT_MISMATCH":
		return amountMismatch(txID, req.DeclaredAmountCents), nil
	default:
		return cleanPass(txID, req.DeclaredAmountCents), nil
	}
}

// scenarioFromSuffix maps the legacy account ID suffix to a scenario code.
// Preserves backward compatibility for existing tests that set AccountID but not Scenario.
func scenarioFromSuffix(suffix string) string {
	switch suffix {
	case "1001":
		return "IQA_FAIL_BLUR"
	case "1002":
		return "IQA_FAIL_GLARE"
	case "1003":
		return "MICR_READ_FAILURE"
	case "1004":
		return "DUPLICATE_DETECTED"
	case "1005":
		return "AMOUNT_MISMATCH"
	default:
		return "CLEAN_PASS"
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
