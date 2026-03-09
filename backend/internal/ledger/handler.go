package ledger

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler handles HTTP requests for ledger operations.
type Handler struct {
	svc *Service
}

// NewHandler creates a ledger Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// GetByAccount handles GET /api/v1/ledger/:account_id.
// Returns all ledger entries for an account with computed balance.
func (h *Handler) GetByAccount(c *gin.Context) {
	accountID := c.Param("account_id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "account_id is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	entries, err := h.svc.GetByAccountID(c.Request.Context(), accountID, nil, nil)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve ledger entries",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	if entries == nil {
		entries = []Entry{}
	}

	// Compute balance: credit entries where investor is the recipient (DEPOSIT),
	// debit entries where investor is the sender (REVERSAL, RETURN_FEE).
	balance := int64(0)
	for _, e := range entries {
		if e.ToAccountID == accountID {
			balance += e.AmountCents
		} else if e.FromAccountID == accountID {
			balance -= e.AmountCents
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"account_id":    accountID,
			"balance_cents": balance,
			"entries":       entries,
		},
	})
}
