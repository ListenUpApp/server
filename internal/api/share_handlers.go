package api

import (
	"context"
	"errors"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
	domainerrors "github.com/listenupapp/listenup-server/internal/errors"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

func (s *Server) registerShareRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "shareCollection",
		Method:      http.MethodPost,
		Path:        "/api/v1/collections/{id}/shares",
		Summary:     "Share collection",
		Description: "Shares a collection with another user",
		Tags:        []string{"Sharing"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleShareCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "listCollectionShares",
		Method:      http.MethodGet,
		Path:        "/api/v1/collections/{id}/shares",
		Summary:     "List collection shares",
		Description: "Lists all shares for a collection",
		Tags:        []string{"Sharing"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListCollectionShares)

	huma.Register(s.api, huma.Operation{
		OperationID: "getShare",
		Method:      http.MethodGet,
		Path:        "/api/v1/shares/{id}",
		Summary:     "Get share",
		Description: "Returns a share by ID",
		Tags:        []string{"Sharing"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetShare)

	huma.Register(s.api, huma.Operation{
		OperationID: "updateShare",
		Method:      http.MethodPatch,
		Path:        "/api/v1/shares/{id}",
		Summary:     "Update share",
		Description: "Updates share permission",
		Tags:        []string{"Sharing"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUpdateShare)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteShare",
		Method:      http.MethodDelete,
		Path:        "/api/v1/shares/{id}",
		Summary:     "Delete share",
		Description: "Removes a share",
		Tags:        []string{"Sharing"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteShare)

	huma.Register(s.api, huma.Operation{
		OperationID: "listSharedWithMe",
		Method:      http.MethodGet,
		Path:        "/api/v1/shares/received",
		Summary:     "List shared with me",
		Description: "Lists collections shared with the current user",
		Tags:        []string{"Sharing"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListSharedWithMe)
}

// === DTOs ===

// ShareCollectionRequest is the request body for sharing a collection.
type ShareCollectionRequest struct {
	UserID     string `json:"user_id" validate:"required" doc:"User ID to share with"`
	Permission string `json:"permission" validate:"required,oneof=read write" doc:"Permission level"`
}

// ShareCollectionInput wraps the share collection request for Huma.
type ShareCollectionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
	Body          ShareCollectionRequest
}

// ShareResponse contains share data in API responses.
type ShareResponse struct {
	ID               string    `json:"id" doc:"Share ID"`
	CollectionID     string    `json:"collection_id" doc:"Collection ID"`
	SharedWithUserID string    `json:"shared_with_user_id" doc:"User ID shared with"`
	SharedByUserID   string    `json:"shared_by_user_id" doc:"User ID who shared"`
	Permission       string    `json:"permission" doc:"Permission level"`
	CreatedAt        time.Time `json:"created_at" doc:"Creation time"`
}

// ShareOutput wraps the share response for Huma.
type ShareOutput struct {
	Body ShareResponse
}

// ListCollectionSharesInput contains parameters for listing collection shares.
type ListCollectionSharesInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Collection ID"`
}

// ListSharesResponse contains a list of shares.
type ListSharesResponse struct {
	Shares []ShareResponse `json:"shares" doc:"List of shares"`
}

// ListSharesOutput wraps the list shares response for Huma.
type ListSharesOutput struct {
	Body ListSharesResponse
}

// GetShareInput contains parameters for getting a share.
type GetShareInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Share ID"`
}

// UpdateShareRequest is the request body for updating a share.
type UpdateShareRequest struct {
	Permission string `json:"permission" validate:"required,oneof=read write" doc:"New permission level"`
}

// UpdateShareInput wraps the update share request for Huma.
type UpdateShareInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Share ID"`
	Body          UpdateShareRequest
}

// DeleteShareInput contains parameters for deleting a share.
type DeleteShareInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Share ID"`
}

// ListSharedWithMeInput contains parameters for listing shares with the current user.
type ListSharedWithMeInput struct {
	Authorization string `header:"Authorization"`
}

// === Handlers ===

func (s *Server) handleShareCollection(ctx context.Context, input *ShareCollectionInput) (*ShareOutput, error) {
	user, err := s.RequireUser(ctx)
	if err != nil {
		return nil, err
	}
	if !user.CanShare() {
		return nil, domainerrors.Forbidden("Share permission required")
	}

	userID := user.ID

	var permission domain.SharePermission
	switch input.Body.Permission {
	case "read":
		permission = domain.PermissionRead
	case "write":
		permission = domain.PermissionWrite
	}

	share, err := s.services.Sharing.ShareCollection(ctx, userID, input.ID, input.Body.UserID, permission)
	if err != nil {
		return nil, err
	}

	// Emit book.created events to the shared user for all books in the collection
	go s.emitBooksForShare(context.Background(), input.ID, input.Body.UserID, true)

	return &ShareOutput{Body: mapShareResponse(share)}, nil
}

func (s *Server) handleListCollectionShares(ctx context.Context, input *ListCollectionSharesInput) (*ListSharesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shares, err := s.services.Sharing.ListCollectionShares(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	resp := MapSlice(shares, mapShareResponse)

	return &ListSharesOutput{Body: ListSharesResponse{Shares: resp}}, nil
}

func (s *Server) handleGetShare(ctx context.Context, input *GetShareInput) (*ShareOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	share, err := s.services.Sharing.GetShare(ctx, userID, input.ID)
	if err != nil {
		return nil, err
	}

	return &ShareOutput{Body: mapShareResponse(share)}, nil
}

func (s *Server) handleUpdateShare(ctx context.Context, input *UpdateShareInput) (*ShareOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	var permission domain.SharePermission
	switch input.Body.Permission {
	case "read":
		permission = domain.PermissionRead
	case "write":
		permission = domain.PermissionWrite
	}

	share, err := s.services.Sharing.UpdateSharePermission(ctx, userID, input.ID, permission)
	if err != nil {
		return nil, err
	}

	return &ShareOutput{Body: mapShareResponse(share)}, nil
}

func (s *Server) handleDeleteShare(ctx context.Context, input *DeleteShareInput) (*MessageOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	// Get the share before deleting to know which user and collection to notify
	share, err := s.store.GetShare(ctx, input.ID)
	if err != nil {
		return nil, err
	}

	if err := s.services.Sharing.UnshareCollection(ctx, userID, input.ID); err != nil {
		return nil, err
	}

	// Emit book.deleted events to the previously shared user for all books in the collection
	go s.emitBooksForShare(context.Background(), share.CollectionID, share.SharedWithUserID, false)

	return &MessageOutput{Body: MessageResponse{Message: "Share removed"}}, nil
}

func (s *Server) handleListSharedWithMe(ctx context.Context, _ *ListSharedWithMeInput) (*ListSharesOutput, error) {
	userID, err := GetUserID(ctx)
	if err != nil {
		return nil, err
	}

	shares, err := s.services.Sharing.ListSharedWithMe(ctx, userID)
	if err != nil {
		return nil, err
	}

	resp := MapSlice(shares, mapShareResponse)

	return &ListSharesOutput{Body: ListSharesResponse{Shares: resp}}, nil
}

// === Mappers ===

func mapShareResponse(s *domain.CollectionShare) ShareResponse {
	return ShareResponse{
		ID:               s.ID,
		CollectionID:     s.CollectionID,
		SharedWithUserID: s.SharedWithUserID,
		SharedByUserID:   s.SharedByUserID,
		Permission:       s.Permission.String(),
		CreatedAt:        s.CreatedAt,
	}
}

// emitBooksForShare emits events for all books in a collection to a specific user.
// Used when sharing/unsharing to notify the affected user.
// isCreated=true emits book.created events, isCreated=false emits book.deleted events.
func (s *Server) emitBooksForShare(ctx context.Context, collectionID, userID string, isCreated bool) {
	// Get the collection to find the owner and book IDs
	coll, err := s.store.AdminGetCollection(ctx, collectionID)
	if err != nil {
		if !errors.Is(err, store.ErrCollectionNotFound) {
			s.logger.Error("failed to get collection for share notification", "collection_id", collectionID, "error", err)
		}
		return
	}

	// Emit events for each book to the specific user
	for _, bookID := range coll.BookIDs {
		if isCreated {
			// For book.created, we need the full enriched book data
			book, err := s.store.GetBook(ctx, bookID, coll.OwnerID)
			if err != nil {
				s.logger.Error("failed to get book for share notification", "book_id", bookID, "error", err)
				continue
			}

			enrichedBook, err := s.store.EnrichBook(ctx, book)
			if err != nil {
				s.logger.Error("failed to enrich book for share notification", "book_id", bookID, "error", err)
				continue
			}

			event := sse.NewBookCreatedEvent(enrichedBook)
			s.sseManager.EmitToUser(userID, event)
		} else {
			// For book.deleted, we just need the book ID
			event := sse.NewBookDeletedEvent(bookID, time.Now())
			s.sseManager.EmitToUser(userID, event)
		}
	}
}
