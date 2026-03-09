package funding

import (
	"context"
	"crypto/sha256"
	"fmt"
	"log"
	"time"

	"github.com/apex/mcd/internal/models"
	"github.com/redis/go-redis/v9"
)

const (
	// MaxDepositAmountCents is the maximum allowed deposit: $5,000.
	MaxDepositAmountCents = int64(500000)
	// DupeTTL is the Redis TTL for duplicate check hashes: 90 days.
	DupeTTL = 90 * 24 * time.Hour
)

// applyDepositLimit rejects amounts over $5,000.
func applyDepositLimit(amountCents int64) error {
	if amountCents > MaxDepositAmountCents {
		return fmt.Errorf("%w: %d cents exceeds limit of %d",
			models.ErrDepositOverLimit, amountCents, MaxDepositAmountCents)
	}
	return nil
}

// applyDuplicateCheck stores a hash of (routing+account+amount+serial) in Redis
// with 90-day TTL. Returns ErrDuplicateDeposit if hash already exists.
// Uses SETNX semantics — only sets if not exists.
// Gracefully degrades if Redis is unavailable.
func applyDuplicateCheck(ctx context.Context, rdb *redis.Client,
	routing, account, serial string, amountCents int64) error {

	raw := fmt.Sprintf("%s:%s:%d:%s", routing, account, amountCents, serial)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	key := "dupe:check:" + hash

	set, err := rdb.SetNX(ctx, key, "1", DupeTTL).Result()
	if err != nil {
		// Redis down: log warning but continue (graceful degradation)
		log.Printf("funding: duplicate check unavailable (Redis error): %v", err)
		return nil
	}
	if !set {
		return fmt.Errorf("%w: check hash %s already exists", models.ErrDuplicateDeposit, hash[:8])
	}
	return nil
}

// applyContributionType returns the contribution type based on account type.
// Returns "INDIVIDUAL" for retirement accounts, "" for all others.
func applyContributionType(accountType string) string {
	if accountType == "retirement" {
		return "INDIVIDUAL"
	}
	return ""
}
