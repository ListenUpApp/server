package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/auth"
	"github.com/listenupapp/listenup-server/internal/service"
)

func (s *Server) registerInviteRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getInviteDetails",
		Method:      http.MethodGet,
		Path:        "/api/v1/invites/{code}",
		Summary:     "Get invite details",
		Description: "Returns details about an invite code",
		Tags:        []string{"Invites"},
	}, s.handleGetInviteDetails)

	huma.Register(s.api, huma.Operation{
		OperationID: "claimInvite",
		Method:      http.MethodPost,
		Path:        "/api/v1/invites/{code}/claim",
		Summary:     "Claim invite",
		Description: "Claims an invite code to create a new user account",
		Tags:        []string{"Invites"},
	}, s.handleClaimInvite)
}

type InviteCodeParam struct {
	Code string `path:"code" doc:"Invite code"`
}

type InviteDetailsResponse struct {
	Name       string `json:"name" doc:"Invitee name"`
	Email      string `json:"email" doc:"Invitee email"`
	ServerName string `json:"server_name" doc:"Server name"`
	InvitedBy  string `json:"invited_by,omitempty" doc:"Inviter name"`
	Valid      bool   `json:"valid" doc:"Whether invite is valid"`
}

type InviteDetailsOutput struct {
	Body InviteDetailsResponse
}

type ClaimInviteRequest struct {
	Password   string     `json:"password" validate:"required,min=8,max=1024" doc:"New user password"`
	DeviceInfo DeviceInfo `json:"device_info,omitempty" doc:"Device information"`
}

type ClaimInviteInput struct {
	Code string `path:"code" doc:"Invite code to claim"`
	Body ClaimInviteRequest
}

func (s *Server) handleGetInviteDetails(ctx context.Context, input *InviteCodeParam) (*InviteDetailsOutput, error) {
	details, err := s.services.Invite.GetInviteDetails(ctx, input.Code)
	if err != nil {
		return nil, err
	}

	return &InviteDetailsOutput{
		Body: InviteDetailsResponse{
			Name:       details.Name,
			Email:      details.Email,
			ServerName: details.ServerName,
			InvitedBy:  details.InvitedBy,
			Valid:      details.Valid,
		},
	}, nil
}

func (s *Server) handleClaimInvite(ctx context.Context, input *ClaimInviteInput) (*AuthOutput, error) {
	req := service.ClaimInviteRequest{
		Code:     input.Code,
		Password: input.Body.Password,
		DeviceInfo: auth.DeviceInfo{
			DeviceType:      input.Body.DeviceInfo.DeviceType,
			Platform:        input.Body.DeviceInfo.Platform,
			PlatformVersion: input.Body.DeviceInfo.PlatformVersion,
			ClientName:      input.Body.DeviceInfo.ClientName,
			ClientVersion:   input.Body.DeviceInfo.ClientVersion,
			ClientBuild:     input.Body.DeviceInfo.ClientBuild,
			DeviceName:      input.Body.DeviceInfo.DeviceName,
			DeviceModel:     input.Body.DeviceInfo.DeviceModel,
		},
	}

	resp, err := s.services.Invite.ClaimInvite(ctx, req)
	if err != nil {
		return nil, err
	}

	return &AuthOutput{Body: mapAuthResponse(resp)}, nil
}
