package audible

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func loadFixture(t *testing.T, name string) []byte {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", name))
	if err != nil {
		t.Fatalf("failed to load fixture %s: %v", name, err)
	}
	return data
}

func newTestClient(t *testing.T, handler http.HandlerFunc) (*Client, *httptest.Server) {
	t.Helper()
	server := httptest.NewServer(handler)

	client := New(slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError})))
	// Override HTTP client to use test server
	client.http = server.Client()

	return client, server
}

func TestClient_Search(t *testing.T) {
	fixture := loadFixture(t, "search_response.json")

	tests := []struct {
		name       string
		response   []byte
		statusCode int
		wantCount  int
		wantErr    error
	}{
		{
			name:       "successful search",
			response:   fixture,
			statusCode: http.StatusOK,
			wantCount:  2,
		},
		{
			name:       "empty results",
			response:   []byte(`{"products": []}`),
			statusCode: http.StatusOK,
			wantCount:  0,
		},
		{
			name:       "rate limited",
			statusCode: http.StatusTooManyRequests,
			wantErr:    ErrRateLimited,
		},
		{
			name:       "server error",
			statusCode: http.StatusInternalServerError,
			wantErr:    ErrServer,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			handler := func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				if tt.response != nil {
					w.Write(tt.response)
				}
			}

			client, server := newTestClient(t, handler)
			defer server.Close()
			defer client.Close()

			results, err := client.searchWithHost(context.Background(), server.URL, SearchParams{Keywords: "test"})

			if tt.wantErr != nil {
				if err == nil {
					t.Errorf("expected error %v, got nil", tt.wantErr)
					return
				}
				// Check if error wraps expected error
				var audErr *Error
				if errors.As(err, &audErr) {
					if !errors.Is(audErr.Err, tt.wantErr) {
						t.Errorf("expected wrapped error %v, got %v", tt.wantErr, audErr.Err)
					}
				}
				return
			}

			if err != nil {
				t.Errorf("unexpected error: %v", err)
				return
			}

			if len(results) != tt.wantCount {
				t.Errorf("got %d results, want %d", len(results), tt.wantCount)
			}
		})
	}
}

func TestClient_Search_ParsesNarrators(t *testing.T) {
	fixture := loadFixture(t, "search_response.json")

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(fixture)
	}

	client, server := newTestClient(t, handler)
	defer server.Close()
	defer client.Close()

	results, err := client.searchWithHost(context.Background(), server.URL, SearchParams{Keywords: "sanderson"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(results) < 1 {
		t.Fatal("expected at least 1 result")
	}

	// First result should have 2 narrators
	if len(results[0].Narrators) != 2 {
		t.Errorf("expected 2 narrators, got %d", len(results[0].Narrators))
	}

	if results[0].Narrators[0].Name != "Michael Kramer" {
		t.Errorf("expected narrator 'Michael Kramer', got %q", results[0].Narrators[0].Name)
	}
}

func TestClient_GetBook(t *testing.T) {
	fixture := loadFixture(t, "book_response.json")

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(fixture)
	}

	client, server := newTestClient(t, handler)
	defer server.Close()
	defer client.Close()

	book, err := client.getBookWithHost(context.Background(), server.URL, "B08G9PRS1K")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify core fields
	if book.ASIN != "B08G9PRS1K" {
		t.Errorf("expected ASIN 'B08G9PRS1K', got %q", book.ASIN)
	}
	if book.Title != "The Way of Kings" {
		t.Errorf("expected title 'The Way of Kings', got %q", book.Title)
	}

	// Verify narrators (the key gap we're filling)
	if len(book.Narrators) != 2 {
		t.Errorf("expected 2 narrators, got %d", len(book.Narrators))
	}

	// Verify HTML was stripped from description
	if book.Description == "" {
		t.Error("description should not be empty")
	}
	if len(book.Description) > 0 && book.Description[0] == '<' {
		t.Error("description should have HTML stripped")
	}

	// Verify cover URL prefers 1024
	if book.CoverURL != "https://m.media-amazon.com/images/I/91KzZWpgmyL._SL1024_.jpg" {
		t.Errorf("expected 1024px cover URL, got %q", book.CoverURL)
	}

	// Verify rating
	if book.Rating < 4.7 || book.Rating > 4.9 {
		t.Errorf("expected rating ~4.8, got %f", book.Rating)
	}

	// Verify genres extracted
	if len(book.Genres) == 0 {
		t.Error("expected genres to be extracted")
	}
}

func TestClient_GetBook_NotFound(t *testing.T) {
	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}

	client, server := newTestClient(t, handler)
	defer server.Close()
	defer client.Close()

	_, err := client.getBookWithHost(context.Background(), server.URL, "B000000000")
	if err == nil {
		t.Fatal("expected error for not found")
	}

	var audErr *Error
	if !errors.As(err, &audErr) {
		t.Fatalf("expected *Error, got %T", err)
	}
	if !errors.Is(audErr.Err, ErrNotFound) {
		t.Errorf("expected ErrNotFound, got %v", audErr.Err)
	}
}

func TestClient_GetChapters(t *testing.T) {
	fixture := loadFixture(t, "chapters_response.json")

	handler := func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write(fixture)
	}

	client, server := newTestClient(t, handler)
	defer server.Close()
	defer client.Close()

	chapters, err := client.getChaptersWithHost(context.Background(), server.URL, "B08G9PRS1K")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	if len(chapters) != 4 {
		t.Errorf("expected 4 chapters, got %d", len(chapters))
	}

	// Verify first chapter
	if chapters[0].Title != "Opening Credits" {
		t.Errorf("expected first chapter 'Opening Credits', got %q", chapters[0].Title)
	}
	if chapters[0].StartMs != 0 {
		t.Errorf("expected StartMs 0, got %d", chapters[0].StartMs)
	}
	if chapters[0].DurationMs != 45000 {
		t.Errorf("expected DurationMs 45000, got %d", chapters[0].DurationMs)
	}

	// Verify chapter ordering (StartMs should be ascending)
	for i := 1; i < len(chapters); i++ {
		if chapters[i].StartMs <= chapters[i-1].StartMs {
			t.Errorf("chapters not in order: chapter %d StartMs %d <= chapter %d StartMs %d",
				i, chapters[i].StartMs, i-1, chapters[i-1].StartMs)
		}
	}
}

func TestValidateASIN(t *testing.T) {
	tests := []struct {
		asin  string
		valid bool
	}{
		{"B08G9PRS1K", true},
		{"B000000000", true},
		{"0123456789", true},
		{"ABCDEFGHIJ", true},
		{"B08G9PRS1", false},   // Too short
		{"B08G9PRS1KK", false}, // Too long
		{"B08G9PRS1k", false},  // Lowercase
		{"", false},
		{"B08G-PRS1K", false}, // Invalid character
	}

	for _, tt := range tests {
		t.Run(tt.asin, func(t *testing.T) {
			got := ValidateASIN(tt.asin)
			if got != tt.valid {
				t.Errorf("ValidateASIN(%q) = %v, want %v", tt.asin, got, tt.valid)
			}
		})
	}
}

func TestRegion_Host(t *testing.T) {
	tests := []struct {
		region Region
		host   string
	}{
		{RegionUS, "api.audible.com"},
		{RegionUK, "api.audible.co.uk"},
		{RegionDE, "api.audible.de"},
		{RegionFR, "api.audible.fr"},
		{RegionAU, "api.audible.com.au"},
		{RegionCA, "api.audible.ca"},
		{RegionJP, "api.audible.co.jp"},
		{RegionIT, "api.audible.it"},
		{RegionIN, "api.audible.in"},
		{RegionES, "api.audible.es"},
		{Region("invalid"), "api.audible.com"}, // Default to US
	}

	for _, tt := range tests {
		t.Run(string(tt.region), func(t *testing.T) {
			got := tt.region.Host()
			if got != tt.host {
				t.Errorf("Region(%q).Host() = %q, want %q", tt.region, got, tt.host)
			}
		})
	}
}

func TestRegion_Valid(t *testing.T) {
	tests := []struct {
		region Region
		valid  bool
	}{
		{RegionUS, true},
		{RegionUK, true},
		{RegionDE, true},
		{RegionFR, true},
		{RegionAU, true},
		{RegionCA, true},
		{RegionJP, true},
		{RegionIT, true},
		{RegionIN, true},
		{RegionES, true},
		{Region("invalid"), false},
		{Region(""), false},
	}

	for _, tt := range tests {
		t.Run(string(tt.region), func(t *testing.T) {
			got := tt.region.Valid()
			if got != tt.valid {
				t.Errorf("Region(%q).Valid() = %v, want %v", tt.region, got, tt.valid)
			}
		})
	}
}

func TestClient_ContextCancellation(t *testing.T) {
	// Slow handler
	handler := func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(5 * time.Second)
		w.WriteHeader(http.StatusOK)
	}

	client, server := newTestClient(t, handler)
	defer server.Close()
	defer client.Close()

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	_, err := client.searchWithHost(ctx, server.URL, SearchParams{Keywords: "test"})
	if err == nil {
		t.Error("expected error due to context cancellation")
	}
}

func TestError_Error(t *testing.T) {
	tests := []struct {
		name string
		err  *Error
		want string
	}{
		{
			name: "with ASIN",
			err: &Error{
				Op:     "getBook",
				Region: RegionUS,
				ASIN:   "B08G9PRS1K",
				Err:    ErrNotFound,
			},
			want: "audible getBook [us/B08G9PRS1K]: audible: not found",
		},
		{
			name: "without ASIN",
			err: &Error{
				Op:     "search",
				Region: RegionUK,
				Err:    ErrRateLimited,
			},
			want: "audible search [uk]: audible: rate limited by server",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := tt.err.Error()
			if got != tt.want {
				t.Errorf("Error() = %q, want %q", got, tt.want)
			}
		})
	}
}

func TestError_Unwrap(t *testing.T) {
	err := &Error{
		Op:     "getBook",
		Region: RegionUS,
		ASIN:   "B08G9PRS1K",
		Err:    ErrNotFound,
	}

	if !errors.Is(err, ErrNotFound) {
		t.Error("expected errors.Is to work with Unwrap")
	}
}
