package api

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) handleListABSImportUsers(ctx context.Context, input *ListABSImportUsersInput) (*ListABSImportUsersOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	filter := domain.MappingFilter(input.Filter)
	if filter == "" {
		filter = domain.MappingFilterAll
	}

	users, err := s.store.ListABSImportUsers(ctx, input.ID, filter)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list users", err)
	}

	resp := &ListABSImportUsersOutput{}
	resp.Body.Users = make([]ABSImportUserResponse, len(users))
	for i, u := range users {
		resp.Body.Users[i] = toABSImportUserResponse(u)
	}

	return resp, nil
}

func (s *Server) handleMapABSImportUser(ctx context.Context, input *MapABSImportUserInput) (*MapABSImportUserOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.ListenUpID == "" {
		return nil, huma.Error400BadRequest("listenup_id is required")
	}

	// Verify ListenUp user exists
	luUser, err := s.store.GetUser(ctx, input.Body.ListenUpID)
	if err != nil {
		return nil, huma.Error400BadRequest("ListenUp user not found")
	}

	// Resolve display info for the mapped user
	var luEmail, luDisplayName *string
	if luUser.Email != "" {
		luEmail = &luUser.Email
	}
	if luUser.DisplayName != "" {
		luDisplayName = &luUser.DisplayName
	}

	if err := s.store.UpdateABSImportUserMapping(ctx, input.ID, input.ABSUserID, &input.Body.ListenUpID, luEmail, luDisplayName); err != nil {
		return nil, huma.Error500InternalServerError("failed to update mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatusesForUser(ctx, input.ID, input.ABSUserID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	user, err := s.store.GetABSImportUser(ctx, input.ID, input.ABSUserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get user", err)
	}

	return &MapABSImportUserOutput{
		Body: toABSImportUserResponse(user),
	}, nil
}

func (s *Server) handleClearABSImportUserMapping(ctx context.Context, input *ClearABSImportUserMappingInput) (*ClearABSImportUserMappingOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.store.UpdateABSImportUserMapping(ctx, input.ID, input.ABSUserID, nil, nil, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to clear mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatusesForUser(ctx, input.ID, input.ABSUserID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	user, err := s.store.GetABSImportUser(ctx, input.ID, input.ABSUserID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get user", err)
	}

	return &ClearABSImportUserMappingOutput{
		Body: toABSImportUserResponse(user),
	}, nil
}

func toABSImportUserResponse(u *domain.ABSImportUser) ABSImportUserResponse {
	resp := ABSImportUserResponse{
		ABSUserID:     u.ABSUserID,
		ABSUsername:   u.ABSUsername,
		ABSEmail:      u.ABSEmail,
		SessionCount:  u.SessionCount,
		TotalListenMs: u.TotalListenMs,
		Confidence:    u.Confidence,
		MatchReason:   u.MatchReason,
		Suggestions:   u.Suggestions,
		IsMapped:      u.IsMapped(),
	}
	if u.ListenUpID != nil {
		resp.ListenUpID = *u.ListenUpID
	}
	if u.ListenUpEmail != nil {
		resp.ListenUpEmail = *u.ListenUpEmail
	}
	if u.ListenUpDisplayName != nil {
		resp.ListenUpDisplayName = *u.ListenUpDisplayName
	}
	return resp
}
