package main

import (
	"context"
	"database/sql"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"

	"go-practice/internal/categories"
	"go-practice/internal/config"
	"go-practice/internal/httpapi"
	"go-practice/internal/items"
	"go-practice/internal/storage"

	"github.com/aws/aws-sdk-go-v2/service/s3"
	_ "github.com/go-sql-driver/mysql"
	"github.com/redis/go-redis/v9"
)

func main() {
	logger := log.New(os.Stdout, "go-practice ", log.LstdFlags)
	settings := config.Load()
	var objectStorage storage.Storage
	switch settings.StorageDriver {
	case "local":
		objectStorage = storage.NewLocal(settings.UploadDir)
	case "s3":
		cfg, err := config.LoadAWS()
		if err != nil {
			logger.Fatalf("S3 configuration failed: %v", err)
		}
		objectStorage = storage.NewS3(s3.NewFromConfig(cfg), settings.S3Bucket)
	default:
		logger.Fatalf("unsupported storage driver: %s", settings.StorageDriver)
	}
	var database *sql.DB
	if settings.HasDatabase() {
		var err error
		database, err = sql.Open("mysql", settings.DatabaseURL)
		if err != nil {
			logger.Fatalf("database connection setup failed: %v", err)
		}
		defer database.Close()
	}
	redisOptions, err := redis.ParseURL(settings.RedisURL)
	if err != nil {
		logger.Fatalf("Redis configuration failed: %v", err)
	}
	redisClient := redis.NewClient(redisOptions)
	if err := redisClient.Ping(context.Background()).Err(); err != nil {
		logger.Fatalf("Redis connection failed: %v", err)
	}
	defer redisClient.Close()
	server := &http.Server{
		Addr: ":" + settings.Port,
		Handler: httpapi.NewHandlerWithServices(
			settings,
			categories.NewService(categories.NewSQLRepository(database)),
			items.NewService(
				items.NewSQLItemRepository(database),
				items.NewSQLImageRepository(database),
				objectStorage,
				items.NewSQLCategoryRepository(database),
			),
			httpapi.AuthDependencies{Database: database, Redis: redisClient},
		),
		ReadHeaderTimeout: settings.HTTP.ReadHeaderTimeout,
		ReadTimeout:       settings.HTTP.ReadTimeout,
		WriteTimeout:      settings.HTTP.WriteTimeout,
		IdleTimeout:       settings.HTTP.IdleTimeout,
	}

	shutdown := make(chan os.Signal, 1)
	signal.Notify(shutdown, syscall.SIGINT, syscall.SIGTERM)
	go func() {
		<-shutdown
		ctx, cancel := context.WithTimeout(context.Background(), settings.HTTP.ShutdownTimeout)
		defer cancel()
		if err := server.Shutdown(ctx); err != nil {
			logger.Printf("HTTP server shutdown failed: %v", err)
		}
	}()

	logger.Printf("HTTP server started: addr=%s environment=%s", server.Addr, settings.Environment)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		logger.Printf("HTTP server stopped: %v", err)
		os.Exit(1)
	}
	logger.Print("HTTP server stopped")
}
