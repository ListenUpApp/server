// Package dto provides request and response types for the ListenUp API.
// These types are used by huma to generate OpenAPI documentation and perform validation.
package dto

// ListResponse is a generic paginated list response.
type ListResponse[T any] struct {
	Items    []T  `json:"items" doc:"List of items"`
	Total    int  `json:"total" doc:"Total count across all pages"`
	Page     int  `json:"page" doc:"Current page number"`
	PageSize int  `json:"page_size" doc:"Items per page"`
	HasMore  bool `json:"has_more" doc:"Whether more pages exist"`
}

// PaginationParams defines common pagination query parameters.
type PaginationParams struct {
	Page     int `query:"page" default:"1" minimum:"1" doc:"Page number"`
	PageSize int `query:"page_size" default:"20" minimum:"1" maximum:"100" doc:"Items per page"`
}

// SortParams defines common sorting query parameters.
type SortParams struct {
	SortBy    string `query:"sort_by" default:"created_at" doc:"Field to sort by"`
	SortOrder string `query:"sort_order" enum:"asc,desc" default:"desc" doc:"Sort direction"`
}

// IDParam is a path parameter for resource IDs.
type IDParam struct {
	ID string `path:"id" doc:"Resource identifier"`
}

// MessageResponse is a simple success message response.
type MessageResponse struct {
	Message string `json:"message" doc:"Success message"`
}

// MessageOutput wraps a message response for huma.
type MessageOutput struct {
	Body MessageResponse
}
