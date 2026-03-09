package main

import (
	"context"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/apex/mcd/internal/db"
)

func main() {
	// --- Required configuration ---
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

	// --- Connect to Postgres ---
	sqlDB, err := db.Connect(databaseURL)
	if err != nil {
		log.Fatalf("Failed to connect to postgres: %v", err)
	}
	defer sqlDB.Close()

	// --- Connect to Redis ---
	rdb, err := db.NewRedisClient(redisURL)
	if err != nil {
		log.Fatalf("Failed to connect to redis: %v", err)
	}
	defer rdb.Close()

	// --- Run migrations ---
	if err := db.RunMigrations(sqlDB); err != nil {
		log.Fatalf("Migrations failed: %v", err)
	}
	log.Println("Migrations completed successfully")

	// --- Configure Gin router ---
	r := gin.Default()
	r.Use(gin.Recovery())

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

	log.Printf("Starting server on :%s", port)
	if err := r.Run(":" + port); err != nil {
		log.Fatalf("Server failed to start: %v", err)
	}
}
