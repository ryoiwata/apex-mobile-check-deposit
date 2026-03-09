package middleware

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/redis/go-redis/v9"
)

// RateLimit enforces max N requests per account_id per minute using Redis INCR.
// Key: "ratelimit:{account_id}:{minute_unix}". TTL: 90 seconds.
// Returns 429 if limit exceeded. Gracefully skips if Redis is unavailable.
func RateLimit(rdb *redis.Client, maxPerMinute int) gin.HandlerFunc {
	return func(c *gin.Context) {
		accountID := c.PostForm("account_id")
		if accountID == "" {
			c.Next()
			return
		}

		key := fmt.Sprintf("ratelimit:%s:%d", accountID, time.Now().Unix()/60)
		ctx := context.Background()

		count, err := rdb.Incr(ctx, key).Result()
		if err != nil {
			suffix := accountID
			if len(accountID) > 4 {
				suffix = accountID[len(accountID)-4:]
			}
			log.Printf("ratelimit: redis unavailable for account ...%s, skipping: %v", suffix, err)
			c.Next()
			return
		}

		if err := rdb.Expire(ctx, key, 90*time.Second).Err(); err != nil {
			log.Printf("ratelimit: failed to set expiry for key %s: %v", key, err)
		}

		if count > int64(maxPerMinute) {
			c.AbortWithStatusJSON(http.StatusTooManyRequests, gin.H{
				"error": "rate limit exceeded",
				"code":  "RATE_LIMITED",
			})
			return
		}

		c.Next()
	}
}
