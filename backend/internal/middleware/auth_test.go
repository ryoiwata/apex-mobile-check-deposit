package middleware

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/gin-gonic/gin"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func init() {
	gin.SetMode(gin.TestMode)
}

// newTestRouter builds a minimal Gin router protected by InvestorAuth.
func newTestRouter(token string) *gin.Engine {
	r := gin.New()
	r.Use(InvestorAuth(token))
	r.GET("/test", func(c *gin.Context) {
		c.JSON(http.StatusOK, gin.H{"ok": true})
	})
	return r
}

// TestFundingFlow_SessionInvalid_ReauthLoop verifies that a mismatched Bearer token
// returns 401 with error="session_expired" and action="re_authenticate" so the React
// client can show a re-auth prompt and retry the deposit without losing form state.
func TestFundingFlow_SessionInvalid_ReauthLoop(t *testing.T) {
	r := newTestRouter("tok_valid")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer tok_invalid")
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	assert.Equal(t, "session_expired", body["error"],
		"invalid token must return error=session_expired so client can trigger re-auth")
	assert.Equal(t, "re_authenticate", body["action"],
		"response must include action=re_authenticate for client-side flow routing")
}

// TestInvestorAuth_MissingHeader returns 401 without session_expired
// (missing header = never authenticated, not an expired session).
func TestInvestorAuth_MissingHeader(t *testing.T) {
	r := newTestRouter("tok_valid")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	r.ServeHTTP(w, req)

	require.Equal(t, http.StatusUnauthorized, w.Code)

	var body map[string]string
	err := json.Unmarshal(w.Body.Bytes(), &body)
	require.NoError(t, err)
	// Missing header should NOT claim session_expired — it was never authenticated
	assert.NotEqual(t, "session_expired", body["error"])
}

// TestInvestorAuth_ValidToken passes through successfully.
func TestInvestorAuth_ValidToken(t *testing.T) {
	r := newTestRouter("tok_valid")

	w := httptest.NewRecorder()
	req, _ := http.NewRequest("GET", "/test", nil)
	req.Header.Set("Authorization", "Bearer tok_valid")
	r.ServeHTTP(w, req)

	assert.Equal(t, http.StatusOK, w.Code)
}
