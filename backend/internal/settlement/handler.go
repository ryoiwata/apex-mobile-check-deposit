package settlement

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
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
