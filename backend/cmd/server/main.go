package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/apex/mcd/internal/db"
	"github.com/apex/mcd/internal/deposit"
	"github.com/apex/mcd/internal/funding"
	"github.com/apex/mcd/internal/ledger"
	"github.com/apex/mcd/internal/middleware"
	"github.com/apex/mcd/internal/notification"
	"github.com/apex/mcd/internal/operator"
	"github.com/apex/mcd/internal/settlement"
	"github.com/apex/mcd/internal/state"
	"github.com/apex/mcd/internal/vendor"
)

// Config holds all server configuration loaded from environment variables.
type Config struct {
	DatabaseURL            string
	RedisURL               string
	ServerPort             string
	ImageStorageDir        string
	SettlementOutputDir    string
	ReturnFeeCents         int64
	InvestorToken          string
	OperatorToken          string
	MaxSettlementRetries   int
}

func loadConfig() Config {
	databaseURL := os.Getenv("DATABASE_URL")
	if databaseURL == "" {
		log.Fatal("DATABASE_URL environment variable is required")
	}

	redisURL := os.Getenv("REDIS_URL")
	if redisURL == "" {
		log.Fatal("REDIS_URL environment variable is required")
	}

	port := os.Getenv("SERVER_PORT")
	if port == "" {
		port = "8080"
	}

	imageDir := os.Getenv("IMAGE_STORAGE_DIR")
	if imageDir == "" {
		imageDir = "/data/images"
	}

	settlementDir := os.Getenv("SETTLEMENT_OUTPUT_DIR")
	if settlementDir == "" {
		settlementDir = "/output/settlement"
	}

	returnFeeCents := int64(3000) // $30.00 default
	if v := os.Getenv("RETURN_FEE_CENTS"); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			returnFeeCents = n
		}
	}

	investorToken := os.Getenv("INVESTOR_TOKEN")
	if investorToken == "" {
		investorToken = "tok_investor_test"
	}

	operatorToken := os.Getenv("OPERATOR_TOKEN")
	if operatorToken == "" {
		operatorToken = "tok_operator_test"
	}

	maxSettlementRetries := settlement.DefaultMaxRetries
	if v := os.Getenv("MAX_SETTLEMENT_RETRIES"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			maxSettlementRetries = n
		}
	}

	return Config{
		DatabaseURL:          databaseURL,
		RedisURL:             redisURL,
		ServerPort:           port,
		ImageStorageDir:      imageDir,
		SettlementOutputDir:  settlementDir,
		ReturnFeeCents:       returnFeeCents,
		InvestorToken:        investorToken,
		OperatorToken:        operatorToken,
		MaxSettlementRetries: maxSettlementRetries,
	}
}

func main() {
	cfg := loadConfig()

	// --- Connect to Postgres ---
	sqlDB, err := db.Connect(cfg.DatabaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer sqlDB.Close()

	// --- Connect to Redis ---
	rdb, err := db.NewRedisClient(cfg.RedisURL)
	if err != nil {
		log.Fatalf("Failed to connect to redis: %v", err)
	}
	defer rdb.Close()

	// --- Run migrations ---
	if err := db.RunMigrations(sqlDB); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}
	log.Println("Migrations completed successfully")

	// --- Create data directories ---
	if err := os.MkdirAll(cfg.ImageStorageDir, 0755); err != nil {
		log.Fatalf("Failed to create image storage directory: %v", err)
	}
	if err := os.MkdirAll(cfg.SettlementOutputDir, 0755); err != nil {
		log.Fatalf("Failed to create settlement output directory: %v", err)
	}

	// --- Wire up services ---
	machine := state.New(sqlDB)
	vendorSvc := vendor.NewStub()
	fundingSvc := funding.NewService(sqlDB, rdb)
	ledgerSvc := ledger.NewService(sqlDB)
	depositSvc := deposit.NewService(sqlDB, machine, vendorSvc, fundingSvc, ledgerSvc)
	operatorSvc := operator.NewService(sqlDB, machine, ledgerSvc, fundingSvc)
	settlementSvc := settlement.NewService(sqlDB, machine, cfg.SettlementOutputDir)
	settlementSvc.SetMaxRetries(cfg.MaxSettlementRetries)
	notifRepo := notification.NewRepo(sqlDB)

	// --- Create handlers ---
	depositHandler := deposit.NewHandler(depositSvc, deposit.Config{
		ImageStorageDir: cfg.ImageStorageDir,
		ReturnFeeCents:  cfg.ReturnFeeCents,
	}, notifRepo)
	operatorHandler := operator.NewHandler(operatorSvc, notifRepo)
	settlementHandler := settlement.NewHandler(settlementSvc)
	ledgerHandler := ledger.NewHandler(ledgerSvc)
	notifHandler := notification.NewHandler(notifRepo)

	// --- Configure Gin router ---
	r := gin.Default()
	r.Use(gin.Recovery())

	// CORS for local development (frontend on :5173)
	r.Use(func(c *gin.Context) {
		c.Header("Access-Control-Allow-Origin", "*")
		c.Header("Access-Control-Allow-Methods", "GET, POST, PUT, PATCH, DELETE, OPTIONS")
		c.Header("Access-Control-Allow-Headers", "Content-Type, Authorization, X-Operator-ID")
		if c.Request.Method == "OPTIONS" {
			c.AbortWithStatus(http.StatusNoContent)
			return
		}
		c.Next()
	})

	// Health check
	r.GET("/health", func(c *gin.Context) {
		ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
		defer cancel()

		postgresStatus := "connected"
		if err := sqlDB.PingContext(ctx); err != nil {
			postgresStatus = "disconnected"
		}

		redisStatus := "connected"
		if err := rdb.Ping(ctx).Err(); err != nil {
			redisStatus = "disconnected"
		}

		status := "ok"
		httpStatus := http.StatusOK
		if postgresStatus != "connected" || redisStatus != "connected" {
			status = "degraded"
			httpStatus = http.StatusServiceUnavailable
		}

		c.JSON(httpStatus, gin.H{
			"status":    status,
			"postgres":  postgresStatus,
			"redis":     redisStatus,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Public endpoints — no auth required
	r.GET("/api/v1/returns/reasons", depositHandler.GetReturnReasons)
	// Image serving — no auth required (UUIDs are not guessable; browser <img> tags can't send headers)
	r.GET("/api/v1/deposits/:id/images/:side", depositHandler.ServeImage)

	// Investor routes (require Bearer token)
	inv := r.Group("/api/v1")
	inv.Use(middleware.InvestorAuth(cfg.InvestorToken))
	{
		inv.POST("/deposits", middleware.RateLimit(rdb, 10), depositHandler.Submit)
		inv.GET("/deposits", depositHandler.List)
		inv.GET("/deposits/:id", depositHandler.GetByID)
		inv.GET("/ledger/:account_id", ledgerHandler.GetByAccount)
		// Notification endpoints — investor-scoped
		inv.GET("/notifications", notifHandler.List)
		inv.GET("/notifications/unread-count", notifHandler.UnreadCount)
		inv.POST("/notifications/:id/read", notifHandler.MarkRead)
		inv.POST("/notifications/read-all", notifHandler.MarkAllRead)
	}

	// Operator routes (require X-Operator-ID header)
	ops := r.Group("/api/v1/operator")
	ops.Use(middleware.OperatorAuth())
	{
		ops.GET("/queue", operatorHandler.GetQueue)
		ops.POST("/deposits/:id/approve", operatorHandler.Approve)
		ops.POST("/deposits/:id/reject", operatorHandler.Reject)
		ops.PATCH("/deposits/:id/contribution-type", operatorHandler.OverrideContributionType)
		ops.GET("/audit", operatorHandler.GetAuditLog)
		// Return endpoint lives here — only operators can trigger returns
		ops.POST("/deposits/:id/return", depositHandler.Return)
		// Settlement trigger and retry
		ops.POST("/settlement/trigger", settlementHandler.Trigger)
		ops.POST("/settlement/retry/:batch_id", settlementHandler.Retry)
	}

	// Settlement read endpoints (operator auth)
	settle := r.Group("/api/v1/settlement")
	settle.Use(middleware.OperatorAuth())
	{
		settle.GET("/batches", settlementHandler.ListBatches)
		settle.GET("/batches/:id", settlementHandler.GetBatch)
		settle.GET("/eod-status", settlementHandler.GetEODStatus)
	}

	// Admin endpoints (operator auth)
	admin := r.Group("/api/v1/admin")
	admin.Use(middleware.OperatorAuth())
	{
		admin.GET("/deposits/:id/trace", depositHandler.GetTrace)
		admin.GET("/deposits", depositHandler.List)
	}

	log.Printf("Starting server on :%s", cfg.ServerPort)
	if err := r.Run(":" + cfg.ServerPort); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
