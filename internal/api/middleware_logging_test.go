package api

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestStructuredLogger_IncludesUserID(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mw := StructuredLogger(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/books", nil)
	// Inject userID into context the same way authMiddleware does.
	req = req.WithContext(setUserID(req.Context(), "user-abc"))
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	logLine := buf.String()
	if !strings.Contains(logLine, `"user_id":"user-abc"`) {
		t.Fatalf("expected log to contain user_id=user-abc, got:\n%s", logLine)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(logLine), &got); err != nil {
		t.Fatalf("log line is not valid JSON: %v\n%s", err, logLine)
	}
}

func TestStructuredLogger_NoUserIDForUnauthenticated(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	logger := slog.New(slog.NewJSONHandler(&buf, &slog.HandlerOptions{Level: slog.LevelDebug}))

	mw := StructuredLogger(logger)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/health", nil)
	rec := httptest.NewRecorder()

	handler.ServeHTTP(rec, req)

	if strings.Contains(buf.String(), "user_id") {
		t.Fatalf("expected log to not contain user_id, got:\n%s", buf.String())
	}

	if _, err := GetUserID(context.Background()); err == nil {
		t.Fatal("GetUserID on empty context should error")
	}
}
