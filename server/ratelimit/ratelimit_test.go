package ratelimit

import (
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"rbf-api/applog"
)

func TestMiddlewareBlocksWhenLimitExceeded(t *testing.T) {
	logger := applog.NewForWriter(io.Discard)
	middleware := New(Config{
		Logger:            logger,
		Route:             "/install-index.ps1",
		RequestsPerMinute: 1,
		Burst:             1,
	})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstRequest := httptest.NewRequest(http.MethodGet, "http://example.com/install-index.ps1", nil)
	firstRequest.RemoteAddr = "203.0.113.10:1000"
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, firstRequest)

	if firstResponse.Code != http.StatusOK {
		t.Fatalf("expected first request to pass, got %d", firstResponse.Code)
	}

	secondRequest := httptest.NewRequest(http.MethodGet, "http://example.com/install-index.ps1", nil)
	secondRequest.RemoteAddr = "203.0.113.10:1000"
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)

	if secondResponse.Code != http.StatusTooManyRequests {
		t.Fatalf("expected second request to be rate limited, got %d", secondResponse.Code)
	}

	if secondResponse.Header().Get("Retry-After") == "" {
		t.Fatal("expected Retry-After header to be set")
	}
}

func TestMiddlewareSeparatesClientsByIP(t *testing.T) {
	logger := applog.NewForWriter(io.Discard)
	middleware := New(Config{
		Logger:            logger,
		Route:             "/index",
		RequestsPerMinute: 1,
		Burst:             1,
	})

	handler := middleware(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	firstRequest := httptest.NewRequest(http.MethodPost, "http://example.com/index", nil)
	firstRequest.RemoteAddr = "203.0.113.10:1000"
	firstResponse := httptest.NewRecorder()
	handler.ServeHTTP(firstResponse, firstRequest)

	secondRequest := httptest.NewRequest(http.MethodPost, "http://example.com/index", nil)
	secondRequest.RemoteAddr = "203.0.113.11:1000"
	secondResponse := httptest.NewRecorder()
	handler.ServeHTTP(secondResponse, secondRequest)

	if firstResponse.Code != http.StatusOK {
		t.Fatalf("expected first IP to pass, got %d", firstResponse.Code)
	}

	if secondResponse.Code != http.StatusOK {
		t.Fatalf("expected second IP to have its own bucket, got %d", secondResponse.Code)
	}
}
