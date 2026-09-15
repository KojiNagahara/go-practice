package httpapi

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"go-practice/internal/config"
)

func TestHealthHandler(t *testing.T) {
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/health", nil)

	NewHandler(config.Settings{}).ServeHTTP(record, request)

	if record.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, record.Code)
	}
	if record.Body.String() != "{\"status\":\"healthy\"}\n" {
		t.Fatalf("unexpected response: %s", record.Body.String())
	}
}

func TestReadinessRequiresDatabaseAndRedisConfiguration(t *testing.T) {
	record := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodGet, "/ready", nil)

	NewHandler(config.Settings{}).ServeHTTP(record, request)

	if record.Code != http.StatusOK {
		t.Fatalf("expected status %d, got %d", http.StatusOK, record.Code)
	}
	if record.Body.String() != "{\"status\":\"configuration-pending\"}\n" {
		t.Fatalf("unexpected response: %s", record.Body.String())
	}
}
