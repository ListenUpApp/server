package api

import (
	"net/http"

	"github.com/danielgtaylor/huma/v2"
)

// registerAdminABSImportRoutes wires up the admin ABS import endpoints. The
// handler implementations live in sibling files split by resource:
//   - admin_abs_import_handlers_imports.go  (imports CRUD + shared helpers)
//   - admin_abs_import_handlers_users.go    (user mapping)
//   - admin_abs_import_handlers_books.go    (book mapping)
//   - admin_abs_import_handlers_sessions.go (session mapping/skip/import)
//   - admin_abs_import_analysis.go          (background runImportAnalysis)
//   - admin_abs_import_types.go             (request/response DTOs)
func (s *Server) registerAdminABSImportRoutes() {
	// Import management
	huma.Register(s.api, huma.Operation{
		OperationID: "createABSImport",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/imports",
		Summary:     "Create ABS import",
		Description: "Creates a new persistent ABS import from an uploaded backup (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleCreateABSImport)

	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImports",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports",
		Summary:     "List ABS imports",
		Description: "Lists all ABS imports with status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImports)

	huma.Register(s.api, huma.Operation{
		OperationID: "getABSImport",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}",
		Summary:     "Get ABS import",
		Description: "Gets details of a single ABS import (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetABSImport)

	huma.Register(s.api, huma.Operation{
		OperationID: "deleteABSImport",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/abs/imports/{id}",
		Summary:     "Delete ABS import",
		Description: "Deletes an ABS import and all its data (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleDeleteABSImport)

	// User mapping
	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImportUsers",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}/users",
		Summary:     "List ABS import users",
		Description: "Lists users in an ABS import with mapping status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImportUsers)

	huma.Register(s.api, huma.Operation{
		OperationID: "mapABSImportUser",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/abs/imports/{id}/users/{absUserId}",
		Summary:     "Map ABS user",
		Description: "Maps an ABS user to a ListenUp user (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMapABSImportUser)

	huma.Register(s.api, huma.Operation{
		OperationID: "clearABSImportUserMapping",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/abs/imports/{id}/users/{absUserId}",
		Summary:     "Clear ABS user mapping",
		Description: "Clears the mapping for an ABS user (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleClearABSImportUserMapping)

	// Book mapping
	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImportBooks",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}/books",
		Summary:     "List ABS import books",
		Description: "Lists books in an ABS import with mapping status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImportBooks)

	huma.Register(s.api, huma.Operation{
		OperationID: "mapABSImportBook",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/abs/imports/{id}/books/{absMediaId}",
		Summary:     "Map ABS book",
		Description: "Maps an ABS book to a ListenUp book (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleMapABSImportBook)

	huma.Register(s.api, huma.Operation{
		OperationID: "clearABSImportBookMapping",
		Method:      http.MethodDelete,
		Path:        "/api/v1/admin/abs/imports/{id}/books/{absMediaId}",
		Summary:     "Clear ABS book mapping",
		Description: "Clears the mapping for an ABS book (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleClearABSImportBookMapping)

	// Session management
	huma.Register(s.api, huma.Operation{
		OperationID: "listABSImportSessions",
		Method:      http.MethodGet,
		Path:        "/api/v1/admin/abs/imports/{id}/sessions",
		Summary:     "List ABS import sessions",
		Description: "Lists sessions in an ABS import with status (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleListABSImportSessions)

	huma.Register(s.api, huma.Operation{
		OperationID: "importABSSessions",
		Method:      http.MethodPost,
		Path:        "/api/v1/admin/abs/imports/{id}/sessions/import",
		Summary:     "Import ready sessions",
		Description: "Imports all ready sessions from an ABS import (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleImportABSSessions)

	huma.Register(s.api, huma.Operation{
		OperationID: "skipABSSession",
		Method:      http.MethodPut,
		Path:        "/api/v1/admin/abs/imports/{id}/sessions/{sessionId}/skip",
		Summary:     "Skip ABS session",
		Description: "Marks an ABS session as skipped (admin only)",
		Tags:        []string{"Admin", "ABS Import"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleSkipABSSession)
}
