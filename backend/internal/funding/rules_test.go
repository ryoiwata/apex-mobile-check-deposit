package funding

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/redis/go-redis/v9"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/apex/mcd/internal/models"
)

// getTestDB opens a Postgres connection using DATABASE_URL env var.
// Skips the test if Postgres is not reachable.
func getTestDB(t *testing.T) *sql.DB {
	t.Helper()
	dsn := os.Getenv("DATABASE_URL")
	if dsn == "" {
		dsn = "postgres://mcd:mcd@localhost:5432/mcd?sslmode=disable"
	}
	db, err := sql.Open("postgres", dsn)
	if err != nil {
		t.Skipf("skipping: cannot open postgres: %v", err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := db.PingContext(ctx); err != nil {
		db.Close()
		t.Skipf("skipping: postgres not reachable: %v", err)
	}
	return db
}

// getTestRedis opens a Redis connection using REDIS_URL env var.
// Skips the test if Redis is not reachable.
func getTestRedis(t *testing.T) *redis.Client {
	t.Helper()
	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		redisURL = "redis://localhost:6379"
	}
	opts, err := redis.ParseURL(redisURL)
	if err != nil {
		t.Skipf("skipping: cannot parse redis URL: %v", err)
	}
	rdb := redis.NewClient(opts)
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if err := rdb.Ping(ctx).Err(); err != nil {
		rdb.Close()
		t.Skipf("skipping: redis not reachable: %v", err)
	}
	return rdb
}

// dupeKey returns the Redis key for a given check hash tuple.
// Mirrors the logic in applyDuplicateCheck so tests can clean up after themselves.
func dupeKey(routing, account, serial string, amountCents int64) string {
	raw := fmt.Sprintf("%s:%s:%d:%s", routing, account, amountCents, serial)
	hash := fmt.Sprintf("%x", sha256.Sum256([]byte(raw)))
	return "dupe:check:" + hash
}

// --- Pure unit tests (no external dependencies) ---

func TestDepositLimit_UnderLimit(t *testing.T) {
	err := applyDepositLimit(100000)
	assert.NoError(t, err)
}

func TestDepositLimit_AtLimit_500000(t *testing.T) {
	err := applyDepositLimit(500000)
	assert.NoError(t, err)
}

func TestDepositLimit_OverLimit_500001(t *testing.T) {
	err := applyDepositLimit(500001)
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrDepositOverLimit),
		"expected ErrDepositOverLimit, got: %v", err)
}

func TestContributionType_Retirement_Individual(t *testing.T) {
	ct := applyContributionType("retirement")
	assert.Equal(t, "INDIVIDUAL", ct)
}

func TestContributionType_Individual_Empty(t *testing.T) {
	ct := applyContributionType("individual")
	assert.Equal(t, "", ct)
}

// --- Integration tests (need Redis) ---

func TestDuplicateCheck_FirstDeposit_Allowed(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	// Use a unique routing number per test run to avoid cross-test contamination.
	routing := fmt.Sprintf("TEST-%d", time.Now().UnixNano())
	account := "111222333"
	serial := "0001"
	amount := int64(100000)

	key := dupeKey(routing, account, serial, amount)
	t.Cleanup(func() { rdb.Del(context.Background(), key) })

	err := applyDuplicateCheck(context.Background(), rdb, routing, account, serial, amount)
	assert.NoError(t, err)
}

func TestDuplicateCheck_SecondDeposit_Rejected(t *testing.T) {
	rdb := getTestRedis(t)
	defer rdb.Close()

	routing := fmt.Sprintf("DUP-%d", time.Now().UnixNano())
	account := "987654321"
	serial := "0042"
	amount := int64(50000)

	key := dupeKey(routing, account, serial, amount)
	t.Cleanup(func() { rdb.Del(context.Background(), key) })

	// First deposit: should succeed.
	err := applyDuplicateCheck(context.Background(), rdb, routing, account, serial, amount)
	require.NoError(t, err)

	// Second deposit with identical data: should be rejected.
	err = applyDuplicateCheck(context.Background(), rdb, routing, account, serial, amount)
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrDuplicateDeposit),
		"expected ErrDuplicateDeposit, got: %v", err)
}

// --- Integration tests (need Postgres) ---

func TestAccountResolver_Active_ReturnsOmnibus(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	resolver := NewAccountResolver(db)
	acct, err := resolver.Resolve(context.Background(), "ACC-SOFI-1006")
	require.NoError(t, err)
	assert.Equal(t, "OMNI-SOFI-001", acct.OmnibusAccountID)
	assert.Equal(t, "active", acct.Status)
}

func TestAccountResolver_NotFound(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	resolver := NewAccountResolver(db)
	_, err := resolver.Resolve(context.Background(), "ACC-NONEXISTENT")
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrAccountNotFound),
		"expected ErrAccountNotFound, got: %v", err)
}

func TestAccountResolver_Suspended_Ineligible(t *testing.T) {
	db := getTestDB(t)
	defer db.Close()

	suspendedID := fmt.Sprintf("ACC-TEST-SUSPENDED-%d", time.Now().UnixNano())
	_, err := db.ExecContext(context.Background(), `
		INSERT INTO accounts (id, correspondent_id, account_type, status)
		VALUES ($1, 'CORR-SOFI', 'individual', 'suspended')`,
		suspendedID)
	require.NoError(t, err)
	t.Cleanup(func() {
		db.ExecContext(context.Background(), `DELETE FROM accounts WHERE id = $1`, suspendedID)
	})

	resolver := NewAccountResolver(db)
	_, err = resolver.Resolve(context.Background(), suspendedID)
	require.Error(t, err)
	assert.True(t, errors.Is(err, models.ErrAccountIneligible),
		"expected ErrAccountIneligible, got: %v", err)
}
