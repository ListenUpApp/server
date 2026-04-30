package api

import (
	"context"
	"log/slog"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) handleListABSImportBooks(ctx context.Context, input *ListABSImportBooksInput) (*ListABSImportBooksOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	filter := domain.MappingFilter(input.Filter)
	if filter == "" {
		filter = domain.MappingFilterAll
	}

	books, err := s.store.ListABSImportBooks(ctx, input.ID, filter)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list books", err)
	}

	resp := &ListABSImportBooksOutput{}
	resp.Body.Books = make([]ABSImportBookResponse, len(books))
	for i, b := range books {
		resp.Body.Books[i] = toABSImportBookResponse(b)
	}

	return resp, nil
}

func (s *Server) handleMapABSImportBook(ctx context.Context, input *MapABSImportBookInput) (*MapABSImportBookOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if input.Body.ListenUpID == "" {
		return nil, huma.Error400BadRequest("listenup_id is required")
	}

	// Verify ListenUp book exists (pass empty userID for admin access)
	luBook, err := s.store.GetBook(ctx, input.Body.ListenUpID, "")
	if err != nil {
		return nil, huma.Error400BadRequest("ListenUp book not found")
	}

	// Resolve display info for the mapped book
	var luTitle *string
	if luBook.Title != "" {
		luTitle = &luBook.Title
	}
	luAuthor := (*string)(nil) // Contributors are separate entities; author display TBD

	if err := s.store.UpdateABSImportBookMapping(ctx, input.ID, input.ABSMediaID, &input.Body.ListenUpID, luTitle, luAuthor); err != nil {
		return nil, huma.Error500InternalServerError("failed to update mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatusesForBook(ctx, input.ID, input.ABSMediaID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	book, err := s.store.GetABSImportBook(ctx, input.ID, input.ABSMediaID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get book", err)
	}

	return &MapABSImportBookOutput{
		Body: toABSImportBookResponse(book),
	}, nil
}

func (s *Server) handleClearABSImportBookMapping(ctx context.Context, input *ClearABSImportBookMappingInput) (*ClearABSImportBookMappingOutput, error) {
	_, err := s.RequireAdmin(ctx)
	if err != nil {
		return nil, err
	}

	if err := s.store.UpdateABSImportBookMapping(ctx, input.ID, input.ABSMediaID, nil, nil, nil); err != nil {
		return nil, huma.Error500InternalServerError("failed to clear mapping", err)
	}

	// Recalculate session statuses
	if err := s.store.RecalculateSessionStatusesForBook(ctx, input.ID, input.ABSMediaID); err != nil {
		s.logger.Error("failed to recalculate sessions", slog.String("error", err.Error()))
	}

	// Update import stats
	s.updateImportStats(ctx, input.ID)

	book, err := s.store.GetABSImportBook(ctx, input.ID, input.ABSMediaID)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to get book", err)
	}

	return &ClearABSImportBookMappingOutput{
		Body: toABSImportBookResponse(book),
	}, nil
}

func toABSImportBookResponse(b *domain.ABSImportBook) ABSImportBookResponse {
	resp := ABSImportBookResponse{
		ABSMediaID:    b.ABSMediaID,
		ABSTitle:      b.ABSTitle,
		ABSAuthor:     b.ABSAuthor,
		ABSDurationMs: b.ABSDurationMs,
		SessionCount:  b.SessionCount,
		Confidence:    b.Confidence,
		MatchReason:   b.MatchReason,
		Suggestions:   b.Suggestions,
		IsMapped:      b.IsMapped(),
	}
	if b.ListenUpID != nil {
		resp.ListenUpID = *b.ListenUpID
	}
	if b.ListenUpTitle != nil {
		resp.ListenUpTitle = *b.ListenUpTitle
	}
	if b.ListenUpAuthor != nil {
		resp.ListenUpAuthor = *b.ListenUpAuthor
	}
	return resp
}
