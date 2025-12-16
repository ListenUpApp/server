package api

import (
	"embed"
	"html/template"
	"net/http"

	"github.com/go-chi/chi/v5"
)

//go:embed templates/*.html templates/*.json
var templates embed.FS

// handleAssetLinks serves the Android App Links verification file.
// GET /.well-known/assetlinks.json
func (s *Server) handleAssetLinks(w http.ResponseWriter, _ *http.Request) {
	data, err := templates.ReadFile("templates/assetlinks.json")
	if err != nil {
		s.logger.Error("Failed to read assetlinks.json", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	w.Write(data)
}

// joinPageData contains data for the invite landing page template.
type joinPageData struct {
	Valid      bool
	Claimed    bool
	Expired    bool
	Name       string
	Email      string
	ServerName string
	InviterName string
	ServerURL  string
	Code       string
}

// handleJoinPage serves the invite landing page.
// GET /join/{code}
func (s *Server) handleJoinPage(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := chi.URLParam(r, "code")

	if code == "" {
		http.Error(w, "Invite code is required", http.StatusBadRequest)
		return
	}

	// Get invite details
	details, err := s.services.Invite.GetInviteDetails(ctx, code)

	// Build template data
	data := joinPageData{
		Code:      code,
		ServerURL: getServerURL(r),
	}

	if err != nil {
		// Invalid invite - show error state
		data.Valid = false
	} else {
		data.Valid = details.Valid
		data.Name = details.Name
		data.Email = details.Email
		data.ServerName = details.ServerName
		data.InviterName = details.InvitedBy

		// Determine specific invalid reason
		if !details.Valid {
			// Check if the invite exists but is claimed/expired
			// The service returns Valid=false for both cases
			data.Claimed = true // Default assumption
		}
	}

	// Parse and execute template
	tmpl, err := template.ParseFS(templates, "templates/join.html")
	if err != nil {
		s.logger.Error("Failed to parse join template", "error", err)
		http.Error(w, "Internal Server Error", http.StatusInternalServerError)
		return
	}

	w.Header().Set("Content-Type", "text/html; charset=utf-8")
	if err := tmpl.Execute(w, data); err != nil {
		s.logger.Error("Failed to execute join template", "error", err)
	}
}

// getServerURL extracts the server URL from the request.
func getServerURL(r *http.Request) string {
	scheme := "https"
	if r.TLS == nil {
		// Check for X-Forwarded-Proto header (common with reverse proxies)
		if proto := r.Header.Get("X-Forwarded-Proto"); proto != "" {
			scheme = proto
		} else {
			scheme = "http"
		}
	}

	host := r.Host
	if forwardedHost := r.Header.Get("X-Forwarded-Host"); forwardedHost != "" {
		host = forwardedHost
	}

	return scheme + "://" + host
}
