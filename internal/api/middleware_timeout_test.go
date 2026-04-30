package api

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5/middleware"
)

func TestRequestTimeout_FiresOnSlowHandler(t *testing.T) {
	t.Parallel()

	mw := middleware.Timeout(50 * time.Millisecond)
	handler := mw(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		select {
		case <-time.After(200 * time.Millisecond):
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("late"))
		case <-r.Context().Done():
			return
		}
	}))

	req := httptest.NewRequest(http.MethodGet, "/api/v1/slow", nil)
	rec := httptest.NewRecorder()

	start := time.Now()
	handler.ServeHTTP(rec, req)
	elapsed := time.Since(start)

	if elapsed > 150*time.Millisecond {
		t.Fatalf("handler took %v, expected to abort within ~50ms", elapsed)
	}
	if rec.Code != http.StatusServiceUnavailable && rec.Code != http.StatusGatewayTimeout {
		t.Fatalf("expected 503 or 504, got %d", rec.Code)
	}
}
