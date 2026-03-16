package funding

import (
	"context"
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
	"github.com/apex/mcd/internal/vendor"
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
// Mirrors the logic in dupeRedisKey so tests can clean up after themselves.
func dupeKey(routing, account, serial string, amountCents int64) string {
	return dupeRedisKey(routing, account, serial, amountCents)
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

// --- Contribution cap unit tests ---

func TestContributionCap_Retirement_UnderCap(t *testing.T) {
	err := applyContributionCap("retirement", MaxDepositAmountCents)
	assert.NoError(t, err)
}

func TestContributionCap_Retirement_OverCap(t *testing.T) {
	err := applyContributionCap("retirement", MaxRetirementContributionCents+1)
	require.Error(t, err)
}

func TestContributionCap_Individual_AlwaysPasses(t *testing.T) {
	err := applyContributionCap("individual", MaxRetirementContributionCents+100000)
	assert.NoError(t, err, "non-retirement accounts should never hit contribution cap")
}

// --- Collect-all (ApplyRules) integration tests (need Postgres + Redis) ---

func TestFundingFlow_CollectAll_DepositLimitAndDuplicate(t *testing.T) {
	db := getTestDB(t)
	rdb := getTestRedis(t)
	defer db.Close()
	defer rdb.Close()

	svc := NewService(db, rdb)

	// Pre-seed the duplicate key so the duplicate_check rule fires.
	routing, account, serial := fmt.Sprintf("COLL-%d", time.Now().UnixNano()), "111222333", "0099"
	amount := int64(600000) // also over $5,000 limit
	key := dupeKey(routing, account, serial, amount)
	rdb.Set(context.Background(), key, "1", DupeTTL)
	t.Cleanup(func() { rdb.Del(context.Background(), key) })

	transfer := &mockTransfer{AccountID: "ACC-SOFI-1006", AmountCents: amount}
	vendorResp := mockVendorResp(routing, account, serial)

	_, err := svc.ApplyRules(context.Background(), transfer.toModel(), vendorResp)
	require.Error(t, err)

	var cae *CollectAllError
	require.True(t, errors.As(err, &cae), "expected CollectAllError, got %T: %v", err, err)
	assert.Equal(t, 2, len(cae.Violations), "expected exactly 2 violations (over_limit + duplicate_funding)")

	codes := make(map[string]bool)
	for _, v := range cae.Violations {
		codes[v.Code] = true
	}
	assert.True(t, codes["over_limit"], "expected over_limit violation")
	assert.True(t, codes["duplicate_funding"], "expected duplicate_funding violation")
}

func TestFundingFlow_CollectAll_SingleViolation_OverLimit(t *testing.T) {
	db := getTestDB(t)
	rdb := getTestRedis(t)
	defer db.Close()
	defer rdb.Close()

	svc := NewService(db, rdb)

	transfer := &mockTransfer{AccountID: "ACC-SOFI-1006", AmountCents: MaxDepositAmountCents + 1}
	// Use unique MICR data so no pre-existing duplicate
	routing := fmt.Sprintf("SINGLE-%d", time.Now().UnixNano())
	vendorResp := mockVendorResp(routing, "999888777", "0001")

	_, err := svc.ApplyRules(context.Background(), transfer.toModel(), vendorResp)
	require.Error(t, err)

	var cae *CollectAllError
	require.True(t, errors.As(err, &cae))
	require.Equal(t, 1, len(cae.Violations))
	assert.Equal(t, "over_limit", cae.Violations[0].Code)
}

func TestFundingFlow_CollectAll_AllPass_NoViolations(t *testing.T) {
	db := getTestDB(t)
	rdb := getTestRedis(t)
	defer db.Close()
	defer rdb.Close()

	svc := NewService(db, rdb)

	routing := fmt.Sprintf("PASS-%d", time.Now().UnixNano())
	amount := int64(100000)
	key := dupeKey(routing, "555444333", "0001", amount)
	t.Cleanup(func() { rdb.Del(context.Background(), key) })

	transfer := &mockTransfer{AccountID: "ACC-SOFI-1006", AmountCents: amount}
	vendorResp := mockVendorResp(routing, "555444333", "0001")

	result, err := svc.ApplyRules(context.Background(), transfer.toModel(), vendorResp)
	require.NoError(t, err)
	assert.True(t, result.RulesPassed)
	assert.NotEmpty(t, result.OmnibusAccountID)
}

// mockTransfer is a minimal stand-in so we don't need a full DB row.
type mockTransfer struct {
	AccountID   string
	AmountCents int64
}

func (m *mockTransfer) toModel() *models.Transfer {
	return &models.Transfer{
		AccountID:   m.AccountID,
		AmountCents: m.AmountCents,
	}
}

// mockVendorResp builds a minimal vendor response with MICR data for duplicate checks.
func mockVendorResp(routing, account, serial string) *vendor.Response {
	return &vendor.Response{
		Status:    "pass",
		IQAResult: "pass",
		MICRData: &vendor.MICRData{
			RoutingNumber: routing,
			AccountNumber: account,
			CheckSerial:   serial,
			Confidence:    0.99,
		},
	}
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
