package operator

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"

	"github.com/apex/mcd/internal/models"
)

// Handler exposes operator service methods as HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a new operator Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// GetQueue returns all flagged deposits awaiting operator review.
// GET /api/v1/operator/queue
func (h *Handler) GetQueue(c *gin.Context) {
	transfers, err := h.svc.GetQueue(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve review queue",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	// Return empty array rather than null when queue is empty.
	if transfers == nil {
		transfers = []*models.Transfer{}
	}
	c.JSON(http.StatusOK, gin.H{"data": transfers})
}

// Approve moves a flagged deposit to FundsPosted.
// POST /api/v1/operator/deposits/:id/approve
func (h *Handler) Approve(c *gin.Context) {
	transferID, ok := parseTransferID(c)
	if !ok {
		return
	}

	var body struct {
		Notes                    string  `json:"notes"`
		ContributionTypeOverride *string `json:"contribution_type_override"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
			"code":  "INVALID_INPUT",
		})
		return
	}

	operatorID, _ := c.Get("operator_id")
	opID, _ := operatorID.(string)

	transfer, err := h.svc.Approve(c.Request.Context(), transferID, opID,
		body.Notes, body.ContributionTypeOverride)
	if err != nil {
		if errors.Is(err, models.ErrTransferNotReviewable) || errors.Is(err, models.ErrTransferNotFound) {
			c.JSON(http.StatusConflict, gin.H{
				"error": err.Error(),
				"code":  "INVALID_STATE_TRANSITION",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to approve deposit",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": transfer})
}

// Reject moves a deposit from Analyzing to Rejected.
// POST /api/v1/operator/deposits/:id/reject
func (h *Handler) Reject(c *gin.Context) {
	transferID, ok := parseTransferID(c)
	if !ok {
		return
	}

	var body struct {
		Reason string `json:"reason" binding:"required"`
		Notes  string `json:"notes"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "reason is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	operatorID, _ := c.Get("operator_id")
	opID, _ := operatorID.(string)

	transfer, err := h.svc.Reject(c.Request.Context(), transferID, opID, body.Reason, body.Notes)
	if err != nil {
		if errors.Is(err, models.ErrTransferNotReviewable) || errors.Is(err, models.ErrTransferNotFound) {
			c.JSON(http.StatusConflict, gin.H{
				"error": err.Error(),
				"code":  "INVALID_STATE_TRANSITION",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to reject deposit",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": transfer})
}

// GetAuditLog returns audit log entries, optionally filtered by transfer_id.
// GET /api/v1/operator/audit?transfer_id=<uuid>
func (h *Handler) GetAuditLog(c *gin.Context) {
	var transferID *uuid.UUID
	if raw := c.Query("transfer_id"); raw != "" {
		id, err := uuid.Parse(raw)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid transfer_id",
				"code":  "INVALID_INPUT",
			})
			return
		}
		transferID = &id
	}

	entries, err := h.svc.GetAuditLog(c.Request.Context(), transferID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve audit log",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	if entries == nil {
		entries = []AuditEntry{}
	}
	c.JSON(http.StatusOK, gin.H{"data": entries})
}

// parseTransferID extracts and validates the :id path parameter as a UUID.
func parseTransferID(c *gin.Context) (uuid.UUID, bool) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid transfer ID",
			"code":  "INVALID_INPUT",
		})
		return uuid.UUID{}, false
	}
	return id, true
}
