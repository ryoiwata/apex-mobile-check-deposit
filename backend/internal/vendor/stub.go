package vendor

import (
	"context"

	"github.com/google/uuid"
)

// Stub returns deterministic responses keyed by req.Scenario.
type Stub struct{}

// NewStub creates a new vendor stub.
func NewStub() *Stub { return &Stub{} }

// Validate returns a deterministic response based on req.Scenario.
// If Scenario is empty the stub defaults to clean pass.
func (s *Stub) Validate(ctx context.Context, req *Request) (*Response, error) {
	txID := "VND-" + uuid.New().String()

	switch req.Scenario {
	case "IQA_FAIL_BLUR":
		return iqaFailBlur(txID), nil
	case "IQA_FAIL_GLARE":
		return iqaFailGlare(txID), nil
	case "MICR_READ_FAILURE":
		return micrFailure(txID), nil
	case "DUPLICATE_DETECTED":
		return duplicateDetected(txID), nil
	case "AMOUNT_MISMATCH":
		return amountMismatch(txID, req.DeclaredAmountCents, req.SimulatedOCRAmountCents), nil
	default:
		return cleanPass(txID, req.DeclaredAmountCents), nil
	}
}

func iqaFailBlur(txID string) *Response {
	code, msg := "IQA_FAIL_BLUR", "Image is too blurry. Please retake the photo."
	return &Response{
		Status:         "fail",
		IQAResult:      "fail_blur",
		RetakeGuidance: "Image is too blurry. Hold the phone steady and ensure the check is in focus before capturing.",
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
		RetakeGuidance: "Glare detected on check image. Move to a location with even lighting and avoid direct light sources.",
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

func amountMismatch(txID string, declared, simulatedOCR int64) *Response {
	// OCR reads a different amount than declared; flagged for operator review.
	// Use the caller-provided OCR amount if given, otherwise default to 80% of declared
	// (simulates a check where OCR reads lower than written amount).
	ocr := simulatedOCR
	if ocr == 0 || ocr == declared {
		ocr = declared * 80 / 100 // ~20% lower than declared
		if ocr == 0 {
			ocr = declared - 100 // edge-case: very small declared amount
		}
	}
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
