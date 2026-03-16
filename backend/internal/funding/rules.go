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
	// MaxRetirementContributionCents is the per-transaction cap for retirement accounts: $6,000.
	MaxRetirementContributionCents = int64(600000)
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

// applyContributionCap checks whether the deposit exceeds the account type's contribution cap.
// Only enforced for retirement accounts (IRA-style per-transaction cap of $6,000).
func applyContributionCap(accountType string, amountCents int64) error {
	if accountType == "retirement" && amountCents > MaxRetirementContributionCents {
		return fmt.Errorf("retirement account deposit exceeds per-transaction cap of $%.2f",
			float64(MaxRetirementContributionCents)/100)
	}
	return nil
}

// checkDuplicateExists returns true if the check hash already exists in Redis.
// Non-destructive: does not set the key. Used during collect-all rule evaluation.
func checkDuplicateExists(ctx context.Context, rdb *redis.Client,
	routing, account, serial string, amountCents int64) bool {

	key := dupeRedisKey(routing, account, serial, amountCents)
	exists, err := rdb.Exists(ctx, key).Result()
	if err != nil {
		// Redis down: graceful degradation — treat as no duplicate
		log.Printf("funding: duplicate check unavailable (Redis error): %v", err)
		return false
	}
	return exists > 0
}

// markCheckDeposited sets the check hash in Redis with DupeTTL.
// Should only be called after all rules pass to avoid false positives on rejected deposits.
func markCheckDeposited(ctx context.Context, rdb *redis.Client,
	routing, account, serial string, amountCents int64) {

	key := dupeRedisKey(routing, account, serial, amountCents)
	if _, err := rdb.SetNX(ctx, key, "1", DupeTTL).Result(); err != nil {
		log.Printf("funding: marking check deposited failed (Redis error): %v", err)
	}
}

// applyDuplicateCheck stores a hash of (routing+account+amount+serial) in Redis
// with 90-day TTL. Returns ErrDuplicateDeposit if hash already exists.
// Uses SETNX semantics — only sets if not exists.
// Gracefully degrades if Redis is unavailable.
// Kept for backward compatibility with existing tests.
func applyDuplicateCheck(ctx context.Context, rdb *redis.Client,
	routing, account, serial string, amountCents int64) error {

	key := dupeRedisKey(routing, account, serial, amountCents)
	set, err := rdb.SetNX(ctx, key, "1", DupeTTL).Result()
	if err != nil {
		// Redis down: log warning but continue (graceful degradation)
		log.Printf("funding: duplicate check unavailable (Redis error): %v", err)
		return nil
	}
	if !set {
		return fmt.Errorf("%w: check hash %s already exists", models.ErrDuplicateDeposit, key[len(key)-8:])
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

// dupeRedisKey builds the Redis key for a check hash tuple.
func dupeRedisKey(routing, account, serial string, amountCents int64) string {
	raw := fmt.Sprintf("%s:%s:%d:%s", routing, account, amountCents, serial)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	return "dupe:check:" + hash
}
