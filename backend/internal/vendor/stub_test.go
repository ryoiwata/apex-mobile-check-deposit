package vendor

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestStub_CleanPass_1006(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-SOFI-1006",
		DeclaredAmountCents: 100000,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "pass", resp.Status)
	assert.Equal(t, "pass", resp.IQAResult)
	require.NotNil(t, resp.MICRData)
	assert.True(t, resp.AmountMatch)
	require.NotNil(t, resp.OCRAmountCents)
	assert.Equal(t, int64(100000), *resp.OCRAmountCents)
	assert.Equal(t, "clear", resp.DuplicateCheck)
	assert.NotEmpty(t, resp.TransactionID)
}

func TestStub_CleanPass_DefaultSuffix(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-TEST-9999",
		DeclaredAmountCents: 50000,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "pass", resp.Status)
	assert.Equal(t, "pass", resp.IQAResult)
	require.NotNil(t, resp.MICRData)
	assert.True(t, resp.AmountMatch)
	require.NotNil(t, resp.OCRAmountCents)
	assert.Equal(t, int64(50000), *resp.OCRAmountCents)
}

func TestStub_IQABlur_1001(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-SOFI-1001",
		DeclaredAmountCents: 100000,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Status)
	assert.Equal(t, "fail_blur", resp.IQAResult)
	require.NotNil(t, resp.ErrorCode)
	assert.Equal(t, "IQA_FAIL_BLUR", *resp.ErrorCode)
	require.NotNil(t, resp.ErrorMessage)
	assert.NotEmpty(t, *resp.ErrorMessage)
}

func TestStub_IQAGlare_1002(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-SOFI-1002",
		DeclaredAmountCents: 100000,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Status)
	assert.Equal(t, "fail_glare", resp.IQAResult)
	require.NotNil(t, resp.ErrorCode)
	assert.Equal(t, "IQA_FAIL_GLARE", *resp.ErrorCode)
}

func TestStub_MICRFailure_1003_Flagged(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-SOFI-1003",
		DeclaredAmountCents: 100000,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "flagged", resp.Status)
	assert.Equal(t, "pass", resp.IQAResult)
	assert.Nil(t, resp.MICRData)
	assert.False(t, resp.AmountMatch)
}

func TestStub_DuplicateDetected_1004(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-SOFI-1004",
		DeclaredAmountCents: 100000,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Status)
	assert.Equal(t, "duplicate_found", resp.DuplicateCheck)
	require.NotNil(t, resp.MICRData)
}

func TestStub_AmountMismatch_1005_Flagged(t *testing.T) {
	stub := NewStub()
	declared := int64(100000)
	req := &Request{
		AccountID:           "ACC-SOFI-1005",
		DeclaredAmountCents: declared,
	}
	resp, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)
	assert.Equal(t, "flagged", resp.Status)
	assert.False(t, resp.AmountMatch)
	require.NotNil(t, resp.OCRAmountCents)
	assert.Equal(t, declared+5000, *resp.OCRAmountCents)
}

func TestVendorFlow_IQABlur_RetakeGuidance(t *testing.T) {
	stub := NewStub()
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "IQA_FAIL_BLUR", DeclaredAmountCents: 100000})
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Status)
	assert.Equal(t, "fail_blur", resp.IQAResult)
	assert.NotEmpty(t, resp.RetakeGuidance, "blur failure must include retake guidance")
}

func TestVendorFlow_IQAGlare_RetakeGuidance(t *testing.T) {
	stub := NewStub()
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "IQA_FAIL_GLARE", DeclaredAmountCents: 100000})
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Status)
	assert.Equal(t, "fail_glare", resp.IQAResult)
	assert.NotEmpty(t, resp.RetakeGuidance, "glare failure must include retake guidance")
}

func TestVendorFlow_CleanPass_NoRetakeGuidance(t *testing.T) {
	stub := NewStub()
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "CLEAN_PASS", DeclaredAmountCents: 100000})
	require.NoError(t, err)
	assert.Equal(t, "pass", resp.Status)
	assert.Empty(t, resp.RetakeGuidance, "clean pass should have no retake guidance")
}

func TestVendorFlow_MICRFail_RoutedToOperator(t *testing.T) {
	stub := NewStub()
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "MICR_READ_FAILURE", DeclaredAmountCents: 100000})
	require.NoError(t, err)
	assert.Equal(t, "flagged", resp.Status, "MICR failure must produce flagged status for operator review")
	assert.Nil(t, resp.MICRData, "MICR failure must have nil MICRData (cannot read MICR line)")
}

func TestVendorFlow_AmountMismatch_RoutedToOperator(t *testing.T) {
	stub := NewStub()
	declared := int64(100000)
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "AMOUNT_MISMATCH", DeclaredAmountCents: declared})
	require.NoError(t, err)
	assert.Equal(t, "flagged", resp.Status, "amount mismatch must produce flagged status for operator review")
	require.NotNil(t, resp.OCRAmountCents, "amount mismatch must include OCR amount for operator comparison")
	assert.NotEqual(t, declared, *resp.OCRAmountCents, "OCR amount must differ from declared amount")
}

func TestVendorFlow_DuplicateDetected_Rejected(t *testing.T) {
	stub := NewStub()
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "DUPLICATE_DETECTED", DeclaredAmountCents: 100000})
	require.NoError(t, err)
	assert.Equal(t, "fail", resp.Status, "duplicate detected must produce fail status")
	assert.Equal(t, "duplicate_found", resp.DuplicateCheck)
}

func TestVendorFlow_CleanPass_StructuredResult(t *testing.T) {
	stub := NewStub()
	declared := int64(200000)
	resp, err := stub.Validate(context.Background(), &Request{Scenario: "CLEAN_PASS", DeclaredAmountCents: declared})
	require.NoError(t, err)
	assert.Equal(t, "pass", resp.Status)
	assert.Equal(t, "pass", resp.IQAResult)
	require.NotNil(t, resp.MICRData, "clean pass must include MICR data")
	assert.NotEmpty(t, resp.MICRData.RoutingNumber)
	assert.NotEmpty(t, resp.MICRData.AccountNumber)
	require.NotNil(t, resp.OCRAmountCents)
	assert.Equal(t, declared, *resp.OCRAmountCents, "OCR amount must match declared on clean pass")
	assert.True(t, resp.AmountMatch)
	assert.Equal(t, "clear", resp.DuplicateCheck)
}

func TestStub_Stateless_SameInputSameOutput(t *testing.T) {
	stub := NewStub()
	req := &Request{
		AccountID:           "ACC-SOFI-1006",
		DeclaredAmountCents: 100000,
	}
	resp1, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)

	resp2, err := stub.Validate(context.Background(), req)
	require.NoError(t, err)

	assert.Equal(t, resp1.Status, resp2.Status)
	assert.Equal(t, resp1.IQAResult, resp2.IQAResult)
	assert.Equal(t, resp1.AmountMatch, resp2.AmountMatch)
	// TransactionID is unique per call (uuid), so we don't assert equality on it
}
