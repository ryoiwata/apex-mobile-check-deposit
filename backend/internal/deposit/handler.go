package deposit

import (
	"errors"
	"net/http"
	"os"
	"path/filepath"
	"strconv"

	"github.com/apex/mcd/internal/models"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// Config holds deposit handler configuration.
type Config struct {
	ImageStorageDir string
	ReturnFeeCents  int64
}

// Handler handles HTTP requests for deposit operations.
type Handler struct {
	svc *Service
	cfg Config
}

// NewHandler creates a deposit Handler.
func NewHandler(svc *Service, cfg Config) *Handler {
	return &Handler{svc: svc, cfg: cfg}
}

// transferDetailResponse wraps a Transfer with its state history for the GetByID response.
type transferDetailResponse struct {
	*models.Transfer
	StateHistory []models.StateTransition `json:"state_history"`
}

// Submit handles POST /api/v1/deposits.
// Parses multipart form, saves images, runs full deposit pipeline.
func (h *Handler) Submit(c *gin.Context) {
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

	req := &SubmitRequest{
		TransferID:          transferID,
		AccountID:           accountID,
		AmountCents:         amountCents,
		DeclaredAmountCents: amountCents, // declared == submitted for MVP
		FrontImageRef:       frontPath,
		BackImageRef:        backPath,
	}

	transfer, err := h.svc.Submit(c.Request.Context(), req)
	if err != nil {
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

// Return handles POST /api/v1/operator/deposits/:id/return.
// Body: { "return_reason": "insufficient_funds", "bank_reference": "RET-001" }
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
		ReturnReason  string `json:"return_reason"`
		BankReference string `json:"bank_reference"`
	}
	if err := c.ShouldBindJSON(&body); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid request body",
			"code":  "INVALID_INPUT",
		})
		return
	}

	if body.ReturnReason == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "return_reason is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	transfer, err := h.svc.ProcessReturn(c.Request.Context(), id, body.ReturnReason, h.cfg.ReturnFeeCents)
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

	c.JSON(http.StatusOK, gin.H{"data": transfer})
}
