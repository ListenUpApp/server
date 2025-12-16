package api

import (
	"encoding/json/v2"
	"net/http"
	"strings"

	"github.com/listenupapp/listenup-server/internal/auth"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
)

// handleSetup creates the first (root) user and completes server setup.
// POST /api/v1/auth/setup.
func (s *Server) handleSetup(w http.ResponseWriter, r *http.Request) {
	var req service.SetupRequest
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Validate required fields
	if req.Email == "" || req.Password == "" {
		response.BadRequest(w, "Email and password are required", s.logger)
		return
	}

	// Setup server with root user
	authResp, err := s.services.Auth.Setup(r.Context(), req)
	if err != nil {
		s.logger.Error("Setup failed", "error", err)

		// Check for specific domain errors
		if domainerrors.Is(err, domainerrors.ErrAlreadyConfigured) {
			response.Conflict(w, "Server is already set up", s.logger)
			return
		}
		if domainerrors.Is(err, domainerrors.ErrAlreadyExists) {
			response.Conflict(w, "Email already in use", s.logger)
			return
		}
		if domainerrors.Is(err, domainerrors.ErrValidation) {
			response.BadRequest(w, err.Error(), s.logger)
			return
		}

		response.InternalError(w, "Failed to complete setup", s.logger)
		return
	}

	response.Success(w, authResp, s.logger)
}

// handleLogin authenticates a user and creates a session.
// POST /api/v1/auth/login.
func (s *Server) handleLogin(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		Email      string          `json:"email"`
		Password   string          `json:"password"`
		DeviceInfo auth.DeviceInfo `json:"device_info"`
	}

	if err := json.UnmarshalRead(r.Body, &reqBody); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Validate required fields
	if reqBody.Email == "" || reqBody.Password == "" {
		response.BadRequest(w, "Email and password are required", s.logger)
		return
	}

	// Build login request with IP address
	loginReq := service.LoginRequest{
		Email:      reqBody.Email,
		Password:   reqBody.Password,
		DeviceInfo: reqBody.DeviceInfo,
		IPAddress:  getIPAddress(r),
	}

	// Authenticate
	authResp, err := s.services.Auth.Login(r.Context(), loginReq)
	if err != nil {
		s.logger.Warn("Login failed",
			"email", reqBody.Email,
			"error", err,
		)

		// Check for specific domain errors
		if domainerrors.Is(err, domainerrors.ErrInvalidCredentials) {
			response.Unauthorized(w, "Invalid email or password", s.logger)
			return
		}
		if domainerrors.Is(err, domainerrors.ErrValidation) {
			response.BadRequest(w, err.Error(), s.logger)
			return
		}

		response.InternalError(w, "Login failed", s.logger)
		return
	}

	response.Success(w, authResp, s.logger)
}

// handleRefresh generates new tokens using a refresh token.
// POST /api/v1/auth/refresh.
func (s *Server) handleRefresh(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		RefreshToken string          `json:"refresh_token"`
		DeviceInfo   auth.DeviceInfo `json:"device_info"` // Optional
	}

	if err := json.UnmarshalRead(r.Body, &reqBody); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if reqBody.RefreshToken == "" {
		response.BadRequest(w, "Refresh token is required", s.logger)
		return
	}

	// Build refresh request
	refreshReq := service.RefreshRequest{
		RefreshToken: reqBody.RefreshToken,
		DeviceInfo:   reqBody.DeviceInfo,
		IPAddress:    getIPAddress(r),
	}

	// Refresh tokens
	authResp, err := s.services.Auth.RefreshTokens(r.Context(), refreshReq)
	if err != nil {
		if domainerrors.Is(err, domainerrors.ErrTokenExpired) {
			response.Unauthorized(w, "Invalid or expired refresh token", s.logger)
			return
		}

		s.logger.Error("Token refresh failed", "error", err)
		response.InternalError(w, "Failed to refresh tokens", s.logger)
		return
	}

	response.Success(w, authResp, s.logger)
}

// handleLogout revokes the current session.
// POST /api/v1/auth/logout.
func (s *Server) handleLogout(w http.ResponseWriter, r *http.Request) {
	var reqBody struct {
		SessionID string `json:"session_id"`
	}

	if err := json.UnmarshalRead(r.Body, &reqBody); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	if reqBody.SessionID == "" {
		response.BadRequest(w, "Session ID is required", s.logger)
		return
	}

	// Logout
	if err := s.services.Auth.Logout(r.Context(), reqBody.SessionID); err != nil {
		s.logger.Error("Logout failed",
			"session_id", reqBody.SessionID,
			"error", err,
		)
		response.InternalError(w, "Failed to logout", s.logger)
		return
	}

	response.Success(w, map[string]string{
		"message": "Logged out successfully",
	}, s.logger)
}

// handleGetCurrentUser returns the authenticated user's information.
// GET /api/v1/users/me.
func (s *Server) handleGetCurrentUser(w http.ResponseWriter, r *http.Request) {
	userID := getUserID(r.Context())
	if userID == "" {
		response.Unauthorized(w, "Not authenticated", s.logger)
		return
	}

	user, err := s.store.GetUser(r.Context(), userID)
	if err != nil {
		s.logger.Error("Failed to get current user",
			"user_id", userID,
			"error", err,
		)
		response.InternalError(w, "Failed to get user", s.logger)
		return
	}

	response.Success(w, user, s.logger)
}

// getIPAddress extracts the client IP address from the request.
// Checks X-Forwarded-For and X-Real-IP headers before falling back to RemoteAddr.
func getIPAddress(r *http.Request) string {
	// Check X-Forwarded-For (may contain multiple IPs, first is client)
	if xff := r.Header.Get("X-Forwarded-For"); xff != "" {
		ips := strings.Split(xff, ",")
		if len(ips) > 0 {
			return strings.TrimSpace(ips[0])
		}
	}

	// Check X-Real-IP
	if xri := r.Header.Get("X-Real-IP"); xri != "" {
		return xri
	}

	// Fall back to RemoteAddr
	ip := r.RemoteAddr
	// Strip port if present
	if idx := strings.LastIndex(ip, ":"); idx != -1 {
		ip = ip[:idx]
	}
	return ip
}
