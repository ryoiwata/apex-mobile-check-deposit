package settlement

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

// chicagoLoc is loaded once at package init so we don't re-load it on every request.
var chicagoLoc *time.Location

func init() {
	var err error
	chicagoLoc, err = time.LoadLocation("America/Chicago")
	if err != nil {
		// Fallback: use a fixed UTC-6 offset (CST). Should not happen if tzdata is embedded.
		chicagoLoc = time.FixedZone("CST", -6*3600)
	}
}

// Handler exposes settlement service methods as HTTP endpoints.
type Handler struct {
	svc *Service
}

// NewHandler creates a settlement Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc}
}

// Trigger runs EOD settlement for the given batch date.
// POST /api/v1/operator/settlement/trigger
//
// Body (optional): {"batch_date": "2026-03-09"}
// If batch_date is omitted, defaults to today (UTC).
func (h *Handler) Trigger(c *gin.Context) {
	var body struct {
		BatchDate string `json:"batch_date"`
	}
	// Ignore bind error — batch_date is optional.
	_ = c.ShouldBindJSON(&body)

	var batchDate time.Time
	if body.BatchDate != "" {
		var err error
		// Parse in CT so "2026-03-09" means March 9 business day in Chicago time.
		// time.Parse would produce UTC midnight, which converts to the previous CT evening.
		batchDate, err = time.ParseInLocation("2006-01-02", body.BatchDate, chicagoLoc)
		if err != nil {
			c.JSON(http.StatusBadRequest, gin.H{
				"error": "invalid batch_date format, expected YYYY-MM-DD",
				"code":  "INVALID_INPUT",
			})
			return
		}
	} else {
		// Default to today's date in CT so the cutoff resolves to the correct business day.
		now := time.Now().In(chicagoLoc)
		y, m, d := now.Date()
		batchDate = time.Date(y, m, d, 12, 0, 0, 0, chicagoLoc)
	}

	batch, err := h.svc.RunSettlement(c.Request.Context(), batchDate)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "settlement run failed: " + err.Error(),
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": batch})
}

// ListBatches returns all settlement batches.
// GET /api/v1/settlement/batches
func (h *Handler) ListBatches(c *gin.Context) {
	batches, err := h.svc.ListBatches(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to list settlement batches",
			"code":  "INTERNAL_ERROR",
		})
		return
	}
	if batches == nil {
		batches = []Batch{}
	}
	c.JSON(http.StatusOK, gin.H{"data": batches})
}

// GetBatch returns a single settlement batch with its deposits.
// GET /api/v1/settlement/batches/:id
func (h *Handler) GetBatch(c *gin.Context) {
	batchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid batch_id",
			"code":  "INVALID_INPUT",
		})
		return
	}

	detail, err := h.svc.GetBatchWithDeposits(c.Request.Context(), batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
			"code":  "NOT_FOUND",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": detail})
}

// GetPreview returns a preview of which FundsPosted deposits will be included
// in the next settlement batch vs. rolled to the next business day.
// GET /api/v1/settlement/preview
func (h *Handler) GetPreview(c *gin.Context) {
	preview, err := h.svc.GetSettlementPreview(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get settlement preview: " + err.Error(),
			"code":  "INTERNAL_ERROR",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": preview})
}

// GetEODStatus returns the current EOD cutoff status and pending deposit count.
// GET /api/v1/settlement/eod-status
func (h *Handler) GetEODStatus(c *gin.Context) {
	status, err := h.svc.GetEODStatus(c.Request.Context())
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to get EOD status",
			"code":  "INTERNAL_ERROR",
		})
		return
	}
	c.JSON(http.StatusOK, gin.H{"data": status})
}

// GetFileContents returns the settlement file contents as JSON for inline viewing.
// GET /api/v1/settlement/batches/:id/file
func (h *Handler) GetFileContents(c *gin.Context) {
	batchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid batch_id",
			"code":  "INVALID_INPUT",
		})
		return
	}

	batch, err := h.svc.getBatch(c.Request.Context(), batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
			"code":  "NOT_FOUND",
		})
		return
	}

	if batch.FilePath == nil || *batch.FilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "settlement file not found for this batch",
			"code":  "NOT_FOUND",
		})
		return
	}

	fileData, err := os.ReadFile(*batch.FilePath)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": fmt.Sprintf("failed to read settlement file: %v", err),
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	var contents map[string]any
	if err := json.Unmarshal(fileData, &contents); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to parse settlement file",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, contents)
}

// DownloadFile serves the settlement file as a downloadable attachment.
// GET /api/v1/settlement/batches/:id/download
func (h *Handler) DownloadFile(c *gin.Context) {
	batchID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid batch_id",
			"code":  "INVALID_INPUT",
		})
		return
	}

	batch, err := h.svc.getBatch(c.Request.Context(), batchID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"error": err.Error(),
			"code":  "NOT_FOUND",
		})
		return
	}

	if batch.FilePath == nil || *batch.FilePath == "" {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "settlement file not found for this batch",
			"code":  "NOT_FOUND",
		})
		return
	}

	if _, err := os.Stat(*batch.FilePath); os.IsNotExist(err) {
		c.JSON(http.StatusNotFound, gin.H{
			"error": "settlement file not found on disk",
			"code":  "NOT_FOUND",
		})
		return
	}

	filename := fmt.Sprintf("settlement_batch_%s.json", batchID)
	c.Header("Content-Description", "File Transfer")
	c.Header("Content-Disposition", fmt.Sprintf("attachment; filename=%s", filename))
	c.Header("Content-Type", "application/json")
	c.File(*batch.FilePath)
}

// Retry re-attempts bank submission for a batch in retry_pending state.
// POST /api/v1/operator/settlement/retry/:batch_id
func (h *Handler) Retry(c *gin.Context) {
	batchID, err := uuid.Parse(c.Param("batch_id"))
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid batch_id",
			"code":  "INVALID_INPUT",
		})
		return
	}

	batch, err := h.svc.RetryBatch(c.Request.Context(), batchID)
	if err != nil {
		c.JSON(http.StatusUnprocessableEntity, gin.H{
			"error": err.Error(),
			"code":  "SETTLEMENT_RETRY_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": batch})
}
