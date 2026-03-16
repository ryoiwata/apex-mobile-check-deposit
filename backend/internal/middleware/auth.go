package middleware

import (
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
)

// InvestorAuth validates Authorization: Bearer <token> header.
// Token must match the configured investor token.
// Sets "investor_authenticated" and "investor_token" in gin context on success.
// Does NOT enforce which account_id is used — account_id comes from request body.
func InvestorAuth(investorToken string) gin.HandlerFunc {
	return func(c *gin.Context) {
		header := c.GetHeader("Authorization")
		if !strings.HasPrefix(header, "Bearer ") {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":      "missing_token",
				"error_type": "authentication",
				"message":    "Authorization header is required.",
				"action":     "authenticate",
			})
			return
		}
		token := strings.TrimPrefix(header, "Bearer ")
		if token != investorToken {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error":      "session_expired",
				"error_type": "authentication",
				"message":    "Your session has expired. Please sign in again.",
				"action":     "re_authenticate",
			})
			return
		}
		c.Set("investor_authenticated", true)
		c.Set("investor_token", token)
		c.Next()
	}
}

// OperatorAuth validates X-Operator-ID header.
// Any non-empty value is accepted (simplified for demo).
// Sets "operator_id" in gin context for audit logging.
func OperatorAuth() gin.HandlerFunc {
	return func(c *gin.Context) {
		operatorID := c.GetHeader("X-Operator-ID")
		if operatorID == "" {
			c.AbortWithStatusJSON(http.StatusUnauthorized, gin.H{
				"error": "operator ID required",
				"code":  "UNAUTHORIZED",
			})
			return
		}
		c.Set("operator_id", operatorID)
		c.Next()
	}
}
