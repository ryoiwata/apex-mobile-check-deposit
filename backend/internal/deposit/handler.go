package deposit

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"github.com/apex/mcd/internal/funding"
	"github.com/apex/mcd/internal/models"
	"github.com/apex/mcd/internal/notification"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// redactAccountID masks all but the last 4 characters of an account ID.
// "ACC-SOFI-1006" → "****1006"
func redactAccountID(id string) string {
	if len(id) <= 4 {
		return "****"
	}
	return "****" + id[len(id)-4:]
}

// Config holds deposit handler configuration.
type Config struct {
	ImageStorageDir string
	ReturnFeeCents  int64
}

// Handler handles HTTP requests for deposit operations.
type Handler struct {
	svc       *Service
	cfg       Config
	notifRepo *notification.Repo
}

// NewHandler creates a deposit Handler.
func NewHandler(svc *Service, cfg Config, notifRepo *notification.Repo) *Handler {
	return &Handler{svc: svc, cfg: cfg, notifRepo: notifRepo}
}

// transferDetailResponse wraps a Transfer with its state history for the GetByID response.
type transferDetailResponse struct {
	*models.Transfer
	StateHistory []models.StateTransition `json:"state_history"`
}

// Submit handles POST /api/v1/deposits.
// Parses multipart form, saves images, runs full deposit pipeline.
func (h *Handler) Submit(c *gin.Context) {
	// Defensive session check — belt-and-suspenders if middleware is misconfigured.
	if auth, _ := c.Get("investor_authenticated"); auth != true {
		c.JSON(http.StatusUnauthorized, gin.H{
			"error":      "session_invalid",
			"error_type": "authentication",
			"message":    "Investor session could not be verified. Please sign in again.",
			"action":     "re_authenticate",
		})
		return
	}

	if err := c.Request.ParseMultipartForm(10 << 20); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "failed to parse multipart form",
			"code":  "INVALID_INPUT",
		})
		return
	}

	accountID := c.PostForm("account_id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "account_id is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	amountStr := c.PostForm("amount_cents")
	if amountStr == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "amount_cents is required",
			"code":  "INVALID_INPUT",
		})
		return
	}
	amountCents, err := strconv.ParseInt(amountStr, 10, 64)
	if err != nil || amountCents <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "amount_cents must be a positive integer",
			"code":  "INVALID_INPUT",
		})
		return
	}
	if amountCents > 500000 {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error":   "deposit amount exceeds maximum limit of $5,000",
			"code":    "DEPOSIT_OVER_LIMIT",
			"details": gin.H{"max_amount_cents": 500000, "submitted_amount_cents": amountCents},
		})
		return
	}

	frontFile, err := c.FormFile("front_image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "front_image is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	backFile, err := c.FormFile("back_image")
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "back_image is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	// Generate transfer ID here so image directory matches the transfer ID
	transferID := uuid.New()
	dir := filepath.Join(h.cfg.ImageStorageDir, transferID.String())
	if err := os.MkdirAll(dir, 0755); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to create image storage directory",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	frontPath := filepath.Join(dir, "front.png")
	if err := c.SaveUploadedFile(frontFile, frontPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save front image",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	backPath := filepath.Join(dir, "back.png")
	if err := c.SaveUploadedFile(backFile, backPath); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to save back image",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	var simulatedOCRCents int64
	if ocrStr := c.PostForm("simulated_ocr_amount_cents"); ocrStr != "" {
		if v, err := strconv.ParseInt(ocrStr, 10, 64); err == nil && v > 0 {
			simulatedOCRCents = v
		}
	}

	req := &SubmitRequest{
		TransferID:              transferID,
		AccountID:               accountID,
		AmountCents:             amountCents,
		DeclaredAmountCents:     amountCents, // declared == submitted for MVP
		FrontImageRef:           frontPath,
		BackImageRef:            backPath,
		VendorScenario:          c.PostForm("vendor_scenario"),          // optional; empty → stub fallback
		SimulatedOCRAmountCents: simulatedOCRCents,
		CreatedAtOverride:       c.PostForm("created_at_override"),      // demo-only; empty → actual time
	}

	transfer, err := h.svc.Submit(c.Request.Context(), req)
	if err != nil {
		// Account does not exist — hard gate, 422 with structured error.
		if errors.Is(err, models.ErrAccountNotFound) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":   "account_not_found",
				"message": fmt.Sprintf("Account %s does not exist. Please select a valid account.", redactAccountID(req.AccountID)),
				"action":  "select_valid_account",
			})
			return
		}
		// Collect-all business rule violations → 422 with full violations array.
		var cae *funding.CollectAllError
		if errors.As(err, &cae) {
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":      "business_rules_failed",
				"violations": cae.Violations,
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": err.Error(),
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusCreated, gin.H{"data": transfer})
}

// GetByID handles GET /api/v1/deposits/:id.
// Returns the transfer with full state history.
func (h *Handler) GetByID(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid transfer ID",
			"code":  "INVALID_INPUT",
		})
		return
	}

	transfer, history, err := h.svc.GetByID(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, models.ErrTransferNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "transfer not found",
				"code":  "TRANSFER_NOT_FOUND",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve transfer",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	if history == nil {
		history = []models.StateTransition{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": transferDetailResponse{
			Transfer:     transfer,
			StateHistory: history,
		},
	})
}

// List handles GET /api/v1/deposits.
// Supports optional query params: status, account_id, limit, offset.
func (h *Handler) List(c *gin.Context) {
	status := c.Query("status")
	accountID := c.Query("account_id")

	limit := 50
	if l := c.Query("limit"); l != "" {
		if n, err := strconv.Atoi(l); err == nil && n > 0 {
			limit = n
		}
	}

	offset := 0
	if o := c.Query("offset"); o != "" {
		if n, err := strconv.Atoi(o); err == nil && n >= 0 {
			offset = n
		}
	}

	transfers, total, err := h.svc.List(c.Request.Context(), status, accountID, limit, offset)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list transfers",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	if transfers == nil {
		transfers = []*models.Transfer{}
	}

	c.JSON(http.StatusOK, gin.H{
		"data": transfers,
		"pagination": gin.H{
			"limit":  limit,
			"offset": offset,
			"total":  total,
		},
	})
}

// ServeImage handles GET /api/v1/deposits/:id/images/:side.
// Serves the uploaded check image from disk.
func (h *Handler) ServeImage(c *gin.Context) {
	id := c.Param("id")
	side := c.Param("side")

	if side != "front" && side != "back" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "side must be 'front' or 'back'",
			"code":  "INVALID_INPUT",
		})
		return
	}

	imgPath := filepath.Join(h.cfg.ImageStorageDir, id, side+".png")
	if _, err := os.Stat(imgPath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "image not found",
			"code":  "TRANSFER_NOT_FOUND",
		})
		return
	}

	c.File(imgPath)
}

// GetTrace handles GET /api/v1/admin/deposits/:id/trace.
// Returns the full lifecycle trace: transfer + state history + audit log + ledger + notifications.
func (h *Handler) GetTrace(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid transfer ID",
			"code":  "INVALID_INPUT",
		})
		return
	}

	trace, err := h.svc.GetTrace(c.Request.Context(), id)
	if err != nil {
		if errors.Is(err, models.ErrTransferNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "transfer not found",
				"code":  "TRANSFER_NOT_FOUND",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve deposit trace",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": trace})
}

// GetReturnReasons handles GET /api/v1/returns/reasons.
// Returns the canonical list of supported check return reason codes.
func (h *Handler) GetReturnReasons(c *gin.Context) {
	c.JSON(http.StatusOK, models.ReturnReasons)
}

// Return handles POST /api/v1/operator/deposits/:id/return.
//
// Body: { "reason_code": "insufficient_funds", "bank_reference": "RET-001", "notes": "..." }
// The "return_reason" field is also accepted for backward compatibility with existing scripts.
// If bank_reference is omitted, one is auto-generated.
func (h *Handler) Return(c *gin.Context) {
	id, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid transfer ID",
			"code":  "INVALID_INPUT",
		})
		return
	}

	var body struct {
		ReasonCode    string `json:"reason_code"`
		BankReference string `json:"bank_reference"`
		Notes         string `json:"notes"`
		// backward-compat: accept old "return_reason" field from existing scripts
		ReturnReason string `json:"return_reason"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
			"code":  "INVALID_INPUT",
		})
		return
	}

	// Prefer reason_code; fall back to return_reason for backward compatibility.
	reasonCode := body.ReasonCode
	if reasonCode == "" {
		reasonCode = body.ReturnReason
	}
	if reasonCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "reason_code is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	// Validate the reason code against the known list.
	// Only reject unknown codes when reason_code (not the legacy return_reason) was provided.
	returnReason := models.ValidReturnReasonCode(reasonCode)
	if returnReason == nil {
		if body.ReasonCode != "" {
			validCodes := make([]string, len(models.ReturnReasons))
			for i, r := range models.ReturnReasons {
				validCodes[i] = r.Code
			}
			c.JSON(http.StatusUnprocessableEntity, gin.H{
				"error":       fmt.Sprintf("unknown reason_code: %q", reasonCode),
				"code":        "INVALID_INPUT",
				"valid_codes": validCodes,
			})
			return
		}
		// Legacy return_reason free-text: wrap in a synthetic reason object.
		returnReason = &models.ReturnReason{Code: reasonCode, Label: reasonCode, Description: ""}
	}

	// Auto-generate a bank reference if none was provided.
	bankRef := body.BankReference
	if bankRef == "" {
		bankRef = fmt.Sprintf("RET-%s-%s", time.Now().UTC().Format("20060102"), uuid.New().String()[:8])
	}

	transfer, err := h.svc.ProcessReturn(c.Request.Context(), id, reasonCode, h.cfg.ReturnFeeCents)
	if err != nil {
		if errors.Is(err, models.ErrTransferNotReturnable) {
			c.JSON(http.StatusConflict, gin.H{
				"error": "transfer must be in completed state to be returned",
				"code":  "INVALID_STATE_TRANSITION",
			})
			return
		}
		if errors.Is(err, models.ErrTransferNotFound) {
			c.JSON(http.StatusNotFound, gin.H{
				"error": "transfer not found",
				"code":  "TRANSFER_NOT_FOUND",
			})
			return
		}
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to process return",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	// Best-effort notification — do not fail the request if this errors.
	investorNotified := false
	meta, _ := json.Marshal(map[string]any{
		"amount_cents":   transfer.AmountCents,
		"fee_cents":      h.cfg.ReturnFeeCents,
		"reason_code":    returnReason.Code,
		"reason_label":   returnReason.Label,
		"bank_reference": bankRef,
		"can_resubmit":   true,
	})
	if notifErr := h.notifRepo.Create(c.Request.Context(), &notification.Notification{
		AccountID:  transfer.AccountID,
		TransferID: transfer.ID.String(),
		Type:       "returned",
		Title:      "Check Returned — Fee Applied",
		Message: fmt.Sprintf(
			"Your check deposit of %s was returned by the bank. Reason: %s. A %s return fee has been deducted from your account. You may submit a new deposit with a different check.",
			notification.FormatCents(transfer.AmountCents),
			returnReason.Label,
			notification.FormatCents(h.cfg.ReturnFeeCents),
		),
		Metadata: meta,
	}); notifErr == nil {
		investorNotified = true
	}

	c.JSON(http.StatusOK, gin.H{
		"data": gin.H{
			"transfer_id":  transfer.ID,
			"status":       transfer.Status,
			"amount_cents": transfer.AmountCents,
			"return_reason": returnReason,
			"bank_reference": bankRef,
			"reversal": gin.H{
				"original_amount_cents": transfer.AmountCents,
				"fee_cents":             h.cfg.ReturnFeeCents,
				"total_debited_cents":   transfer.AmountCents + h.cfg.ReturnFeeCents,
			},
			"ledger_entries_created": 2,
			"investor_notified":      investorNotified,
		},
	})
}
