package api

import (
	"context"
	"net/http"
	"time"

	"github.com/danielgtaylor/huma/v2"
)

func (s *Server) registerAdminInboxRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "listInboxBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/inbox",
		Summary:     "List inbox books",
		Description: "Lists all books in the inbox staging area (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListInboxBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "releaseInboxBooks",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/inbox/release",
		Summary:     "Release inbox books",
		Description: "Releases books from inbox to library (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleReleaseInboxBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "stageInboxCollection",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/inbox/{bookId}/stage",
		Summary:     "Stage collection assignment",
		Description: "Adds a collection to a book's staged assignments (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleStageInboxCollection)

	huma.Register(s.api, huma.Operation{
		OperationID: "unstageInboxCollection",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/inbox/{bookId}/stage/{collectionId}",
		Summary:     "Remove staged collection",
		Description: "Removes a collection from a book's staged assignments (admin only)",
		Tags:        []string{"Admin"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleUnstageInboxCollection)
}

// === DTOs ===

// CollectionRef is a reference to a collection with name for display.
type CollectionRef struct {
	ID   string `json:"id" doc:"Collection ID"`
	Name string `json:"name" doc:"Collection name"`
}

// InboxBookResponse is the API response for a book in the inbox.
type InboxBookResponse struct {
	ID                  string          `json:"id" doc:"Book ID"`
	Title               string          `json:"title" doc:"Book title"`
	Author              string          `json:"author,omitempty" doc:"Primary author"`
	CoverURL            string          `json:"cover_url,omitempty" doc:"Cover image URL"`
	Duration            int64           `json:"duration" doc:"Total duration in milliseconds"`
	StagedCollectionIDs []string        `json:"staged_collection_ids" doc:"Collection IDs to assign on release"`
	StagedCollections   []CollectionRef `json:"staged_collections" doc:"Staged collections with names"`
	ScannedAt           time.Time       `json:"scanned_at" doc:"When the book was scanned"`
}

// ListInboxBooksInput is the Huma input for listing inbox books.
type ListInboxBooksInput struct {
	Authorization string `header:"Authorization"`
}

// ListInboxBooksResponse is the API response for listing inbox books.
type ListInboxBooksResponse struct {
	Books []InboxBookResponse `json:"books" doc:"List of inbox books"`
	Total int                 `json:"total" doc:"Total count"`
}

// ListInboxBooksOutput is the Huma output wrapper for listing inbox books.
type ListInboxBooksOutput struct {
	Body ListInboxBooksResponse
}

// ReleaseInboxBooksRequest is the request body for releasing books.
type ReleaseInboxBooksRequest struct {
	BookIDs []string `json:"book_ids" validate:"required,min=1" doc:"Book IDs to release"`
}

// ReleaseInboxBooksInput is the Huma input for releasing inbox books.
type ReleaseInboxBooksInput struct {
	Authorization string `header:"Authorization"`
	Body          ReleaseInboxBooksRequest
}

// ReleaseInboxBooksResponse is the API response for releasing inbox books.
type ReleaseInboxBooksResponse struct {
	Released      int `json:"released" doc:"Number of books released"`
	Public        int `json:"public" doc:"Number of books made public (no collections)"`
	ToCollections int `json:"to_collections" doc:"Number of collection assignments made"`
}

// ReleaseInboxBooksOutput is the Huma output wrapper for releasing inbox books.
type ReleaseInboxBooksOutput struct {
	Body ReleaseInboxBooksResponse
}

// StageInboxCollectionRequest is the request body for staging a collection.
type StageInboxCollectionRequest struct {
	CollectionID string `json:"collection_id" validate:"required" doc:"Collection ID to stage"`
}

// StageInboxCollectionInput is the Huma input for staging a collection.
type StageInboxCollectionInput struct {
	Authorization string `header:"Authorization"`
	BookID        string `path:"bookId" doc:"Book ID"`
	Body          StageInboxCollectionRequest
}

// StageInboxCollectionOutput is the Huma output wrapper for staging a collection.
type StageInboxCollectionOutput struct {
	// Empty response on success
}

// UnstageInboxCollectionInput is the Huma input for unstaging a collection.
type UnstageInboxCollectionInput struct {
	Authorization string `header:"Authorization"`
	BookID        string `path:"bookId" doc:"Book ID"`
	CollectionID  string `path:"collectionId" doc:"Collection ID to unstage"`
}

// UnstageInboxCollectionOutput is the Huma output wrapper for unstaging a collection.
type UnstageInboxCollectionOutput struct {
	// Empty response on success
}

// === Handlers ===

func (s *Server) handleListInboxBooks(ctx context.Context, input *ListInboxBooksInput) (*ListInboxBooksOutput, error) {
	_, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	books, err := s.services.Inbox.ListBooks(ctx)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to list inbox books", err)
	}

	// Pre-fetch all contributors and collections to avoid N+1 queries
	contributorIDs := make(map[string]struct{})
	collectionIDs := make(map[string]struct{})

	for _, book := range books {
		for _, c := range book.Contributors {
			contributorIDs[c.ContributorID] = struct{}{}
		}
		for _, collID := range book.StagedCollectionIDs {
			collectionIDs[collID] = struct{}{}
		}
	}

	// Batch fetch contributors
	contributorMap := make(map[string]string) // id -> name
	for id := range contributorIDs {
		if contributor, err := s.store.GetContributor(ctx, id); err == nil {
			contributorMap[id] = contributor.Name
		}
	}

	// Batch fetch collections
	collectionMap := make(map[string]CollectionRef) // id -> ref
	for id := range collectionIDs {
		if coll, err := s.store.AdminGetCollection(ctx, id); err == nil {
			collectionMap[id] = CollectionRef{ID: coll.ID, Name: coll.Name}
		}
	}

	// Build response using pre-fetched data
	responses := make([]InboxBookResponse, 0, len(books))
	for _, book := range books {
		// Get author from pre-fetched contributors
		author := ""
		for _, c := range book.Contributors {
			for _, role := range c.Roles {
				if role.String() == "author" {
					if name, ok := contributorMap[c.ContributorID]; ok {
						author = name
					}
					break
				}
			}
			if author != "" {
				break
			}
		}

		// Build staged collections from pre-fetched data
		stagedCollections := make([]CollectionRef, 0, len(book.StagedCollectionIDs))
		for _, collID := range book.StagedCollectionIDs {
			if ref, ok := collectionMap[collID]; ok {
				stagedCollections = append(stagedCollections, ref)
			}
		}

		// Build cover URL
		coverURL := ""
		if book.CoverImage != nil {
			coverURL = "/api/v1/books/" + book.ID + "/cover"
		}

		// Ensure staged IDs is never nil (Go's JSON encodes nil as null, not [])
		stagedIDs := book.StagedCollectionIDs
		if stagedIDs == nil {
			stagedIDs = []string{}
		}

		responses = append(responses, InboxBookResponse{
			ID:                  book.ID,
			Title:               book.Title,
			Author:              author,
			CoverURL:            coverURL,
			Duration:            book.TotalDuration,
			StagedCollectionIDs: stagedIDs,
			StagedCollections:   stagedCollections,
			ScannedAt:           book.ScannedAt,
		})
	}

	return &ListInboxBooksOutput{
		Body: ListInboxBooksResponse{
			Books: responses,
			Total: len(responses),
		},
	}, nil
}

func (s *Server) handleReleaseInboxBooks(ctx context.Context, input *ReleaseInboxBooksInput) (*ReleaseInboxBooksOutput, error) {
	_, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	result, err := s.services.Inbox.ReleaseBooks(ctx, input.Body.BookIDs)
	if err != nil {
		return nil, huma.Error500InternalServerError("failed to release books", err)
	}

	return &ReleaseInboxBooksOutput{
		Body: ReleaseInboxBooksResponse{
			Released:      result.Released,
			Public:        result.Public,
			ToCollections: result.ToCollections,
		},
	}, nil
}

func (s *Server) handleStageInboxCollection(ctx context.Context, input *StageInboxCollectionInput) (*StageInboxCollectionOutput, error) {
	_, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Inbox.StageCollection(ctx, input.BookID, input.Body.CollectionID); err != nil {
		return nil, huma.Error500InternalServerError("failed to stage collection", err)
	}

	return &StageInboxCollectionOutput{}, nil
}

func (s *Server) handleUnstageInboxCollection(ctx context.Context, input *UnstageInboxCollectionInput) (*UnstageInboxCollectionOutput, error) {
	_, err := s.authenticateAndRequireAdmin(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	if err := s.services.Inbox.UnstageCollection(ctx, input.BookID, input.CollectionID); err != nil {
		return nil, huma.Error500InternalServerError("failed to unstage collection", err)
	}

	return &UnstageInboxCollectionOutput{}, nil
}
