package notification

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

// Handler exposes notification endpoints.
type Handler struct {
	repo *Repo
}

// NewHandler creates a notification Handler.
func NewHandler(repo *Repo) *Handler {
	return &Handler{repo: repo}
}

// List handles GET /api/v1/notifications?account_id=...
// Returns all notifications for the account plus the unread count.
func (h *Handler) List(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "account_id is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	notifications, err := h.repo.GetByAccount(c.Request.Context(), accountID, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve notifications",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	if notifications == nil {
		notifications = []Notification{}
	}

	unreadCount := 0
	for _, n := range notifications {
		if !n.Read {
			unreadCount++
		}
	}

	c.JSON(http.StatusOK, gin.H{
		"notifications": notifications,
		"unread_count":  unreadCount,
	})
}

// UnreadCount handles GET /api/v1/notifications/unread-count?account_id=...
func (h *Handler) UnreadCount(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "account_id is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	count, err := h.repo.GetUnreadCount(c.Request.Context(), accountID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to retrieve unread count",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"unread_count": count})
}

// MarkRead handles POST /api/v1/notifications/:id/read
func (h *Handler) MarkRead(c *gin.Context) {
	notifID := c.Param("id")
	if notifID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "notification ID is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	if err := h.repo.MarkRead(c.Request.Context(), notifID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to mark notification as read",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"id": notifID, "read": true}})
}

// MarkAllRead handles POST /api/v1/notifications/read-all?account_id=...
func (h *Handler) MarkAllRead(c *gin.Context) {
	accountID := c.Query("account_id")
	if accountID == "" {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "account_id is required",
			"code":  "INVALID_INPUT",
		})
		return
	}

	if err := h.repo.MarkAllRead(c.Request.Context(), accountID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"error": "failed to mark all notifications as read",
			"code":  "INTERNAL_ERROR",
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{"data": gin.H{"account_id": accountID, "read": true}})
}
