package httpapi

import (
	"database/sql"
	"encoding/json"
	"net/http"

	"go-practice/internal/categories"
	"go-practice/internal/config"
	"go-practice/internal/items"

	"github.com/redis/go-redis/v9"
)

func NewHandler(settings config.Settings) http.Handler {
	return NewHandlerWithServices(settings, nil, nil)
}

func NewHandlerWithCategoryService(settings config.Settings, categoryService *categories.Service) http.Handler {
	return NewHandlerWithServices(settings, categoryService, nil)
}

type AuthDependencies struct {
	Database *sql.DB
	Redis    *redis.Client
}

func NewHandlerWithServices(settings config.Settings, categoryService *categories.Service, itemService *items.Service, authDependencies ...AuthDependencies) http.Handler {
	mux := http.NewServeMux()
	var dependencies AuthDependencies
	if len(authDependencies) > 0 {
		dependencies = authDependencies[0]
	}
	auth := newAuthManager(dependencies.Database, dependencies.Redis, settings.Environment != "local" && settings.Environment != "development", settings.OAuth)
	mux.HandleFunc("/", homeHandler(settings))
	mux.HandleFunc("/health", healthHandler)
	mux.HandleFunc("/ready", readinessHandler(settings))
	mux.HandleFunc("/static/categories.css", categoryStylesheetHandler)
	mux.HandleFunc("/static/items.css", itemStylesheetHandler)
	auth.registerRoutes(mux)
	if settings.StorageDriver == "local" {
		mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(settings.UploadDir))))
	}
	categoryHandler := newCategoryHandler(categoryService)
	categoryHandler.registerRoutes(mux)
	itemHandler := newItemHandler(itemService, categoryService)
	itemHandler.registerRoutes(mux)
	return loggingMiddleware(auth.middleware(mux))
}

func homeHandler(settings config.Settings) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		writeJSON(response, http.StatusOK, map[string]string{
			"service":     "go-practice",
			"environment": settings.Environment,
			"status":      "running",
		})
	}
}

func healthHandler(response http.ResponseWriter, request *http.Request) {
	writeJSON(response, http.StatusOK, map[string]string{"status": "healthy"})
}

func readinessHandler(settings config.Settings) http.HandlerFunc {
	return func(response http.ResponseWriter, request *http.Request) {
		status := "ready"
		if settings.DatabaseURL == "" || settings.RedisURL == "" {
			status = "configuration-pending"
		}
		writeJSON(response, http.StatusOK, map[string]string{"status": status})
	}
}

func loggingMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(response http.ResponseWriter, request *http.Request) {
		next.ServeHTTP(response, request)
	})
}

func writeJSON(response http.ResponseWriter, status int, payload map[string]string) {
	response.Header().Set("Content-Type", "application/json")
	response.WriteHeader(status)
	_ = json.NewEncoder(response).Encode(payload)
}
