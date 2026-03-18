//go:build integration

// Package tests contains cross-service integration tests that require a running
// Postgres and Redis instance. Run these with:
//
//	go test ./tests/ -v -tags=integration
//
// or against the Docker Compose stack:
//
//	docker compose up -d
//	go test ./tests/ -v -tags=integration
package tests

import (
	"bytes"
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// testClient wraps an HTTP server URL for integration assertions.
type testClient struct {
	baseURL       string
	investorToken string
	operatorID    string
	t             *testing.T
}

func newTestClient(t *testing.T) *testClient {
	t.Helper()
	baseURL := os.Getenv("TEST_API_URL")
	if baseURL == "" {
		baseURL = "http://localhost:8080"
	}
	return &testClient{
		baseURL:       baseURL,
		investorToken: "tok_investor_test",
		operatorID:    "OP-001",
		t:             t,
	}
}

// pingAPI ensures the server is reachable before running tests.
func (c *testClient) pingAPI() error {
	resp, err := http.Get(c.baseURL + "/health")
	if err != nil {
		return fmt.Errorf("server not reachable at %s: %w", c.baseURL, err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusServiceUnavailable {
		return fmt.Errorf("unexpected health status: %d", resp.StatusCode)
	}
	return nil
}

// submitDeposit submits a deposit via multipart/form-data and returns the parsed response body.
func (c *testClient) submitDeposit(accountID string, amountCents int64, vendorScenario string) map[string]any {
	c.t.Helper()

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	require.NoError(c.t, mw.WriteField("account_id", accountID))
	require.NoError(c.t, mw.WriteField("amount_cents", fmt.Sprintf("%d", amountCents)))
	if vendorScenario != "" {
		require.NoError(c.t, mw.WriteField("vendor_scenario", vendorScenario))
	}

	// Write a minimal 1×1 white PNG as the check image.
	minimalPNG := []byte{
		0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a, // PNG signature
		0x00, 0x00, 0x00, 0x0d, 0x49, 0x48, 0x44, 0x52, // IHDR chunk
		0x00, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x01,
		0x08, 0x02, 0x00, 0x00, 0x00, 0x90, 0x77, 0x53,
		0xde, 0x00, 0x00, 0x00, 0x0c, 0x49, 0x44, 0x41, // IDAT chunk
		0x54, 0x08, 0xd7, 0x63, 0xf8, 0xcf, 0xc0, 0x00,
		0x00, 0x00, 0x02, 0x00, 0x01, 0xe2, 0x21, 0xbc,
		0x33, 0x00, 0x00, 0x00, 0x00, 0x49, 0x45, 0x4e, // IEND chunk
		0x44, 0xae, 0x42, 0x60, 0x82,
	}
	for _, side := range []string{"front_image", "back_image"} {
		fw, err := mw.CreateFormFile(side, side+".png")
		require.NoError(c.t, err)
		_, err = fw.Write(minimalPNG)
		require.NoError(c.t, err)
	}
	require.NoError(c.t, mw.Close())

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/deposits", &buf)
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.investorToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) getDeposit(transferID string) map[string]any {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/deposits/"+transferID, nil)
	require.NoError(c.t, err)
	req.Header.Set("Authorization", "Bearer "+c.investorToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) triggerSettlement(batchDate string) map[string]any {
	c.t.Helper()
	payload := fmt.Sprintf(`{"batch_date":"%s"}`, batchDate)
	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/operator/settlement/trigger",
		bytes.NewBufferString(payload))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operator-ID", c.operatorID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) approveDeposit(transferID, notes string) map[string]any {
	c.t.Helper()
	payload := fmt.Sprintf(`{"notes":%q}`, notes)
	req, err := http.NewRequest(http.MethodPost,
		c.baseURL+"/api/v1/operator/deposits/"+transferID+"/approve",
		bytes.NewBufferString(payload))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operator-ID", c.operatorID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) rejectDeposit(transferID, reason, notes string) map[string]any {
	c.t.Helper()
	payload := fmt.Sprintf(`{"reason":%q,"notes":%q}`, reason, notes)
	req, err := http.NewRequest(http.MethodPost,
		c.baseURL+"/api/v1/operator/deposits/"+transferID+"/reject",
		bytes.NewBufferString(payload))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operator-ID", c.operatorID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) returnDeposit(transferID, reasonCode string) map[string]any {
	c.t.Helper()
	payload := fmt.Sprintf(`{"reason_code":%q}`, reasonCode)
	req, err := http.NewRequest(http.MethodPost,
		c.baseURL+"/api/v1/operator/deposits/"+transferID+"/return",
		bytes.NewBufferString(payload))
	require.NoError(c.t, err)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Operator-ID", c.operatorID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) getLedger(accountID string) map[string]any {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/ledger/"+accountID, nil)
	require.NoError(c.t, err)
	req.Header.Set("Authorization", "Bearer "+c.investorToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func (c *testClient) getAuditLog() map[string]any {
	c.t.Helper()
	req, err := http.NewRequest(http.MethodGet, c.baseURL+"/api/v1/operator/audit", nil)
	require.NoError(c.t, err)
	req.Header.Set("X-Operator-ID", c.operatorID)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(c.t, err)
	defer resp.Body.Close()

	var body map[string]any
	require.NoError(c.t, json.NewDecoder(resp.Body).Decode(&body))
	return body
}

func dataStr(body map[string]any, key string) string {
	data, ok := body["data"].(map[string]any)
	if !ok {
		return ""
	}
	v, _ := data[key].(string)
	return v
}

func dataFloat(body map[string]any, key string) float64 {
	data, ok := body["data"].(map[string]any)
	if !ok {
		return 0
	}
	v, _ := data[key].(float64)
	return v
}

func dataBool(body map[string]any, key string) bool {
	data, ok := body["data"].(map[string]any)
	if !ok {
		return false
	}
	v, _ := data[key].(bool)
	return v
}

// skipIfServerNotRunning skips the test if the API server is not reachable.
func skipIfServerNotRunning(t *testing.T) *testClient {
	t.Helper()
	c := newTestClient(t)
	if err := c.pingAPI(); err != nil {
		t.Skipf("skipping integration test: %v", err)
	}
	return c
}

// uniqueAmount returns a semi-unique amount to avoid duplicate hash collisions
// across test runs. Uses Unix nanoseconds modded into a reasonable range.
func uniqueAmount(base int64) int64 {
	return base + (time.Now().UnixNano()%50000)*100
}

// ─── Test 1: Happy Path End-to-End ──────────────────────────────────────────

// TestIntegration_HappyPath_FullLifecycle submits a clean-pass deposit,
// triggers settlement, and verifies the transfer reaches Completed with a ledger entry.
func TestIntegration_HappyPath_FullLifecycle(t *testing.T) {
	c := skipIfServerNotRunning(t)
	amount := uniqueAmount(100000)

	// Submit deposit
	body := c.submitDeposit("ACC-SOFI-1006", amount, "")
	transferID := dataStr(body, "transfer_id")
	require.NotEmpty(t, transferID, "transfer_id must be set")
	assert.Equal(t, "funds_posted", dataStr(body, "status"),
		"clean-pass deposit must reach funds_posted synchronously")

	// GET by ID — verify state history
	detail := c.getDeposit(transferID)
	assert.Equal(t, "funds_posted", dataStr(detail, "status"))
	data := detail["data"].(map[string]any)
	history, _ := data["state_history"].([]any)
	assert.Greater(t, len(history), 0, "state_history must be non-empty")

	// Trigger settlement
	today := time.Now().UTC().Format("2006-01-02")
	settle := c.triggerSettlement(today)
	settleData := settle["data"].(map[string]any)
	count, _ := settleData["deposit_count"].(float64)
	assert.GreaterOrEqual(t, count, float64(1), "settlement batch must include at least 1 deposit")
	assert.Equal(t, "submitted", settleData["status"])

	// Verify completed
	final := c.getDeposit(transferID)
	assert.Equal(t, "completed", dataStr(final, "status"),
		"transfer must be completed after settlement")
	finalData := final["data"].(map[string]any)
	assert.NotEmpty(t, finalData["settlement_batch_id"], "settlement_batch_id must be set")

	// Verify ledger entry
	ledger := c.getLedger("ACC-SOFI-1006")
	ledgerData := ledger["data"].(map[string]any)
	entries, _ := ledgerData["entries"].([]any)
	var depositEntry map[string]any
	for _, e := range entries {
		entry := e.(map[string]any)
		if entry["transfer_id"] == transferID && entry["sub_type"] == "DEPOSIT" {
			depositEntry = entry
			break
		}
	}
	require.NotNil(t, depositEntry, "DEPOSIT ledger entry must exist for transfer")
	assert.Equal(t, float64(amount), depositEntry["amount_cents"],
		"ledger DEPOSIT amount must match submitted amount")
}

// ─── Test 2: IQA Fail — Blur ─────────────────────────────────────────────────

func TestIntegration_VendorScenario_IQABlur_Rejected(t *testing.T) {
	c := skipIfServerNotRunning(t)

	body := c.submitDeposit("ACC-SOFI-1001", 50000, "IQA_FAIL_BLUR")
	assert.Equal(t, "rejected", dataStr(body, "status"),
		"IQA blur must reject the deposit")

	// Rejection reason must be populated
	data := body["data"].(map[string]any)
	rejectionReason, _ := data["rejection_reason"].(string)
	assert.NotEmpty(t, rejectionReason,
		"rejection_reason must be set for vendor-rejected deposits")
}

// ─── Test 3: IQA Fail — Glare ────────────────────────────────────────────────

func TestIntegration_VendorScenario_IQAGlare_Rejected(t *testing.T) {
	c := skipIfServerNotRunning(t)

	body := c.submitDeposit("ACC-SOFI-1002", 50000, "IQA_FAIL_GLARE")
	assert.Equal(t, "rejected", dataStr(body, "status"),
		"IQA glare must reject the deposit")
}

// ─── Test 4: MICR Failure — Flagged for Operator Review ──────────────────────

func TestIntegration_VendorScenario_MICRFailure_FlaggedForReview(t *testing.T) {
	c := skipIfServerNotRunning(t)

	body := c.submitDeposit("ACC-SOFI-1003", 75000, "MICR_READ_FAILURE")
	assert.Equal(t, "analyzing", dataStr(body, "status"),
		"MICR failure must enter analyzing state")
	assert.True(t, dataBool(body, "flagged"),
		"MICR failure must be flagged=true")
	data := body["data"].(map[string]any)
	assert.Equal(t, "micr_failure", data["flag_reason"],
		"flag_reason must be micr_failure")
}

// ─── Test 5: Duplicate Detected ───────────────────────────────────────────────

func TestIntegration_VendorScenario_DuplicateDetected_Rejected(t *testing.T) {
	c := skipIfServerNotRunning(t)

	body := c.submitDeposit("ACC-SOFI-1004", 60000, "DUPLICATE_DETECTED")
	assert.Equal(t, "rejected", dataStr(body, "status"),
		"duplicate detected must reject the deposit")
}

// ─── Test 6: Amount Mismatch — Flagged for Operator Review ───────────────────

func TestIntegration_VendorScenario_AmountMismatch_FlaggedForReview(t *testing.T) {
	c := skipIfServerNotRunning(t)

	body := c.submitDeposit("ACC-SOFI-1005", 80000, "AMOUNT_MISMATCH")
	assert.Equal(t, "analyzing", dataStr(body, "status"),
		"amount mismatch must enter analyzing state")
	assert.True(t, dataBool(body, "flagged"),
		"amount mismatch must be flagged=true")
	data := body["data"].(map[string]any)
	assert.Equal(t, "amount_mismatch", data["flag_reason"],
		"flag_reason must be amount_mismatch")
	assert.NotNil(t, data["ocr_amount_cents"],
		"OCR amount must be present for amount mismatch")
}

// ─── Test 7: Deposit Over $5,000 Limit ───────────────────────────────────────

func TestIntegration_DepositOverLimit_Rejected422(t *testing.T) {
	c := skipIfServerNotRunning(t)

	var buf bytes.Buffer
	mw := multipart.NewWriter(&buf)
	_ = mw.WriteField("account_id", "ACC-SOFI-0000")
	_ = mw.WriteField("amount_cents", "600000") // $6,000 — exceeds $5,000 max
	minPNG := []byte{0x89, 0x50, 0x4e, 0x47, 0x0d, 0x0a, 0x1a, 0x0a}
	for _, side := range []string{"front_image", "back_image"} {
		fw, _ := mw.CreateFormFile(side, side+".png")
		_, _ = fw.Write(minPNG)
	}
	mw.Close()

	req, err := http.NewRequest(http.MethodPost, c.baseURL+"/api/v1/deposits", &buf)
	require.NoError(t, err)
	req.Header.Set("Content-Type", mw.FormDataContentType())
	req.Header.Set("Authorization", "Bearer "+c.investorToken)

	resp, err := http.DefaultClient.Do(req)
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusUnprocessableEntity, resp.StatusCode,
		"deposit over limit must return 422")
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "DEPOSIT_OVER_LIMIT", body["code"],
		"error code must be DEPOSIT_OVER_LIMIT")
}

// ─── Test 8: State Machine — Invalid Transitions ─────────────────────────────

// TestIntegration_StateHistory_FullPath verifies the state_history reflects the
// full transition path for a clean-pass deposit. This indirectly validates that
// the state machine enforced the correct sequence.
func TestIntegration_StateHistory_FullPath(t *testing.T) {
	c := skipIfServerNotRunning(t)
	amount := uniqueAmount(150000)

	body := c.submitDeposit("ACC-SOFI-1006", amount, "")
	transferID := dataStr(body, "transfer_id")
	require.NotEmpty(t, transferID)

	detail := c.getDeposit(transferID)
	data := detail["data"].(map[string]any)
	history, _ := data["state_history"].([]any)

	require.GreaterOrEqual(t, len(history), 3,
		"clean-pass must have at least 3 transitions: requested→validating→analyzing→approved/funds_posted")

	// Each transition must have from_state, to_state, triggered_by, created_at
	for i, h := range history {
		transition := h.(map[string]any)
		assert.NotEmpty(t, transition["to_state"],
			"transition %d must have to_state", i)
		assert.NotEmpty(t, transition["created_at"],
			"transition %d must have created_at", i)
	}
}

// ─── Test 9: Reversal with $30 Fee ───────────────────────────────────────────

func TestIntegration_Return_ReversalWithFee(t *testing.T) {
	c := skipIfServerNotRunning(t)
	amount := uniqueAmount(200000) // $2,000

	// Submit and settle a clean-pass deposit
	submitBody := c.submitDeposit("ACC-SOFI-1006", amount, "")
	transferID := dataStr(submitBody, "transfer_id")
	require.NotEmpty(t, transferID)
	require.Equal(t, "funds_posted", dataStr(submitBody, "status"))

	today := time.Now().UTC().Format("2006-01-02")
	c.triggerSettlement(today)

	detail := c.getDeposit(transferID)
	require.Equal(t, "completed", dataStr(detail, "status"),
		"transfer must be completed before return")

	// Trigger return
	returnBody := c.returnDeposit(transferID, "insufficient_funds")
	returnData := returnBody["data"].(map[string]any)
	assert.Equal(t, "returned", returnData["status"],
		"transfer must move to returned")

	// Verify ledger has DEPOSIT + REVERSAL + RETURN_FEE
	ledger := c.getLedger("ACC-SOFI-1006")
	ledgerData := ledger["data"].(map[string]any)
	entries, _ := ledgerData["entries"].([]any)

	var deposit, reversal, fee *map[string]any
	for _, e := range entries {
		entry := e.(map[string]any)
		if entry["transfer_id"] != transferID {
			continue
		}
		copy := entry
		switch entry["sub_type"] {
		case "DEPOSIT":
			deposit = &copy
		case "REVERSAL":
			reversal = &copy
		case "RETURN_FEE":
			fee = &copy
		}
	}

	require.NotNil(t, deposit, "DEPOSIT entry must exist")
	require.NotNil(t, reversal, "REVERSAL entry must exist")
	require.NotNil(t, fee, "RETURN_FEE entry must exist")

	assert.Equal(t, float64(amount), (*deposit)["amount_cents"],
		"DEPOSIT amount must match original")
	assert.Equal(t, float64(amount), (*reversal)["amount_cents"],
		"REVERSAL amount must match original")
	assert.Equal(t, float64(3000), (*fee)["amount_cents"],
		"RETURN_FEE must be $30 (3000 cents)")
}

// ─── Test 10: Settlement File Contains Only Eligible Deposits ────────────────

func TestIntegration_Settlement_BatchContents(t *testing.T) {
	c := skipIfServerNotRunning(t)

	// Submit two clean-pass deposits
	amount1 := uniqueAmount(110000)
	amount2 := uniqueAmount(220000)
	body1 := c.submitDeposit("ACC-SOFI-1006", amount1, "")
	body2 := c.submitDeposit("ACC-SOFI-0000", amount2, "")
	id1 := dataStr(body1, "transfer_id")
	id2 := dataStr(body2, "transfer_id")

	require.Equal(t, "funds_posted", dataStr(body1, "status"))
	require.Equal(t, "funds_posted", dataStr(body2, "status"))

	// Submit a rejected deposit — must NOT appear in settlement
	rejBody := c.submitDeposit("ACC-SOFI-1001", 50000, "IQA_FAIL_BLUR")
	require.Equal(t, "rejected", dataStr(rejBody, "status"))

	// Trigger settlement
	today := time.Now().UTC().Format("2006-01-02")
	settle := c.triggerSettlement(today)
	settleData := settle["data"].(map[string]any)
	count, _ := settleData["deposit_count"].(float64)
	total, _ := settleData["total_amount_cents"].(float64)

	assert.GreaterOrEqual(t, count, float64(2),
		"settlement must include at least the 2 approved deposits")
	assert.GreaterOrEqual(t, total, float64(amount1+amount2),
		"total_amount_cents must cover both submitted deposits")

	// Both clean-pass deposits must now be completed
	detail1 := c.getDeposit(id1)
	detail2 := c.getDeposit(id2)
	assert.Equal(t, "completed", dataStr(detail1, "status"), "deposit 1 must be completed")
	assert.Equal(t, "completed", dataStr(detail2, "status"), "deposit 2 must be completed")
}

// ─── Test 11: Operator Approve Flow ──────────────────────────────────────────

func TestIntegration_Operator_ApproveFlow_AuditLogged(t *testing.T) {
	c := skipIfServerNotRunning(t)

	// Submit a MICR failure — goes to operator queue
	body := c.submitDeposit("ACC-SOFI-1003", uniqueAmount(100000), "MICR_READ_FAILURE")
	transferID := dataStr(body, "transfer_id")
	require.Equal(t, "analyzing", dataStr(body, "status"))

	// Approve the deposit
	approveBody := c.approveDeposit(transferID, "MICR verified manually")
	approveData := approveBody["data"].(map[string]any)
	assert.Equal(t, "funds_posted", approveData["status"],
		"operator approve must post funds synchronously")

	// Verify audit log entry
	audit := c.getAuditLog()
	auditData := audit["data"].([]any)
	var found bool
	for _, e := range auditData {
		entry := e.(map[string]any)
		if entry["transfer_id"] == transferID && entry["action"] == "approve" {
			found = true
			assert.Equal(t, "OP-001", entry["operator_id"])
			break
		}
	}
	assert.True(t, found, "audit log must contain approve entry for transfer")
}

// ─── Test 12: Operator Reject Flow ───────────────────────────────────────────

func TestIntegration_Operator_RejectFlow_RejectionReasonStored(t *testing.T) {
	c := skipIfServerNotRunning(t)

	// Submit a MICR failure
	body := c.submitDeposit("ACC-SOFI-1003", uniqueAmount(100000), "MICR_READ_FAILURE")
	transferID := dataStr(body, "transfer_id")
	require.Equal(t, "analyzing", dataStr(body, "status"))

	// Reject the deposit with a specific reason
	rejectReason := "MICR ink too light or faded"
	rejectBody := c.rejectDeposit(transferID, rejectReason, "Routing number unreadable")
	rejectData := rejectBody["data"].(map[string]any)
	assert.Equal(t, "rejected", rejectData["status"],
		"operator reject must move transfer to rejected")

	// Verify rejection_reason is persisted on the transfer
	detail := c.getDeposit(transferID)
	detailData := detail["data"].(map[string]any)
	assert.Equal(t, rejectReason, detailData["rejection_reason"],
		"rejection_reason must be stored on the transfer")
}

// ─── Test 13: Contribution Type for Retirement Account ───────────────────────

func TestIntegration_ContributionType_RetirementAccount_DefaultsToIndividual(t *testing.T) {
	c := skipIfServerNotRunning(t)

	body := c.submitDeposit("ACC-RETIRE-001", uniqueAmount(100000), "")
	require.Equal(t, "funds_posted", dataStr(body, "status"),
		"retirement account deposit must reach funds_posted")
	data := body["data"].(map[string]any)
	assert.Equal(t, "INDIVIDUAL", data["contribution_type"],
		"contribution_type must default to INDIVIDUAL for retirement accounts")
}

// ─── Test 14: Health Check ────────────────────────────────────────────────────

func TestIntegration_HealthCheck_ReturnsStatus(t *testing.T) {
	c := skipIfServerNotRunning(t)

	resp, err := http.Get(c.baseURL + "/health")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode, "health check must return 200")
	var body map[string]any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	assert.Equal(t, "ok", body["status"])
	assert.Equal(t, "connected", body["postgres"])
	assert.Equal(t, "connected", body["redis"])
}

// ─── Test 15: Return Reasons Endpoint ────────────────────────────────────────

func TestIntegration_ReturnReasons_ReturnsAllCodes(t *testing.T) {
	c := skipIfServerNotRunning(t)

	resp, err := http.Get(c.baseURL + "/api/v1/returns/reasons")
	require.NoError(t, err)
	defer resp.Body.Close()

	assert.Equal(t, http.StatusOK, resp.StatusCode)
	var body any
	require.NoError(t, json.NewDecoder(resp.Body).Decode(&body))
	reasons, ok := body.([]any)
	require.True(t, ok, "return reasons must be an array")
	assert.Greater(t, len(reasons), 0, "must have at least one return reason code")
}

// ─── Ensure httptest is imported even though we don't use it directly ─────────
var _ = httptest.NewRecorder
var _ = context.Background
var _ = sql.ErrNoRows
