package api

import (
	"context"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerAdminRepairRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "repairBookRelationships",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/repair/book-relationships",
		Summary:     "Repair book relationships",
		Description: "Rescans all books and rebuilds missing contributor and series links",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleRepairBookRelationships)
}

// RepairBookRelationshipsInput is the Huma input for the repair endpoint.
type RepairBookRelationshipsInput struct {
	Authorization string `header:"Authorization"`
}

// RepairBookRelationshipsResponse is the API response for book relationship repair.
type RepairBookRelationshipsResponse struct {
	BooksRepaired int `json:"books_repaired" doc:"Number of books whose relationships were repaired"`
}

// RepairBookRelationshipsOutput is the Huma output wrapper.
type RepairBookRelationshipsOutput struct {
	Body RepairBookRelationshipsResponse
}

func (s *Server) handleRepairBookRelationships(ctx context.Context, _ *RepairBookRelationshipsInput) (*RepairBookRelationshipsOutput, error) {
	if _, err := s.RequireAdmin(ctx); err != nil {
		return nil, err
	}

	repaired, err := s.services.Book.RepairBookRelationships(ctx)
	if err != nil {
		return nil, err
	}

	return &RepairBookRelationshipsOutput{
		Body: RepairBookRelationshipsResponse{BooksRepaired: repaired},
	}, nil
}
