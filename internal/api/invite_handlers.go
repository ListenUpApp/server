package api

import (
	"encoding/json/v2"
	"net/http"

	"github.com/go-chi/chi/v5"
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/http/response"
	"github.com/listenupapp/listenup-server/internal/service"
)

// Public Invite Handlers

// handleGetInviteDetails returns invite details by code.
// GET /api/v1/invites/{code}.
func (s *Server) handleGetInviteDetails(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := chi.URLParam(r, "code")

	if code == "" {
		response.BadRequest(w, "Invite code is required", s.logger)
		return
	}

	details, err := s.services.Invite.GetInviteDetails(ctx, code)
	if err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	response.Success(w, details, s.logger)
}

// handleClaimInvite claims an invite and creates a new user.
// POST /api/v1/invites/{code}/claim.
func (s *Server) handleClaimInvite(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	code := chi.URLParam(r, "code")

	if code == "" {
		response.BadRequest(w, "Invite code is required", s.logger)
		return
	}

	var req struct {
		Password   string          `json:"password"`
		DeviceInfo auth.DeviceInfo `json:"device_info"`
	}
	if err := json.UnmarshalRead(r.Body, &req); err != nil {
		response.BadRequest(w, "Invalid request body", s.logger)
		return
	}

	// Build claim request
	claimReq := service.ClaimInviteRequest{
		Code:       code,
		Password:   req.Password,
		DeviceInfo: req.DeviceInfo,
		IPAddress:  r.RemoteAddr,
	}

	authResp, err := s.services.Invite.ClaimInvite(ctx, claimReq)
	if err != nil {
		handleServiceError(w, err, s.logger)
		return
	}

	response.Created(w, authResp, s.logger)
}
