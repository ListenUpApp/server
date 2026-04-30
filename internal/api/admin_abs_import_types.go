package api

// === DTOs for persistent imports ===

// CreateABSImportRequest is the request for creating a persistent import.
type CreateABSImportRequest struct {
	BackupPath string `json:"backup_path" doc:"Path to uploaded .audiobookshelf backup file"`
	Name       string `json:"name,omitempty" doc:"Optional friendly name for this import"`
}

type CreateABSImportInput struct {
	Authorization string `header:"Authorization"`
	Body          CreateABSImportRequest
}

// ABSImportResponse represents an ABS import.
type ABSImportResponse struct {
	ID               string `json:"id" doc:"Import ID"`
	Name             string `json:"name" doc:"Import name"`
	BackupPath       string `json:"backup_path" doc:"Path to the backup file"`
	Status           string `json:"status" doc:"Import status: active, completed, archived"`
	CreatedAt        string `json:"created_at" doc:"When the import was created"`
	UpdatedAt        string `json:"updated_at" doc:"When the import was last updated"`
	TotalUsers       int    `json:"total_users" doc:"Total users in backup"`
	TotalBooks       int    `json:"total_books" doc:"Total books in backup"`
	TotalSessions    int    `json:"total_sessions" doc:"Total sessions in backup"`
	UsersMapped      int    `json:"users_mapped" doc:"Users with confirmed mappings"`
	BooksMapped      int    `json:"books_mapped" doc:"Books with confirmed mappings"`
	SessionsImported int    `json:"sessions_imported" doc:"Sessions already imported"`
}

type CreateABSImportOutput struct {
	Body ABSImportResponse
}

type ListABSImportsInput struct {
	Authorization string `header:"Authorization"`
}

type ListABSImportsOutput struct {
	Body struct {
		Imports []ABSImportResponse `json:"imports" doc:"List of ABS imports"`
	}
}

type GetABSImportInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
}

type GetABSImportOutput struct {
	Body ABSImportResponse
}

type DeleteABSImportInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
}

type DeleteABSImportOutput struct {
	Body struct {
		Message string `json:"message" doc:"Success message"`
	}
}

// User mapping DTOs

type ListABSImportUsersInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	Filter        string `query:"filter" doc:"Filter: all, mapped, unmapped" default:"all"`
}

type ABSImportUserResponse struct {
	ABSUserID           string   `json:"abs_user_id" doc:"ABS user ID"`
	ABSUsername         string   `json:"abs_username" doc:"ABS username"`
	ABSEmail            string   `json:"abs_email,omitempty" doc:"ABS email"`
	ListenUpID          string   `json:"listenup_id,omitempty" doc:"Mapped ListenUp user ID"`
	ListenUpEmail       string   `json:"listenup_email,omitempty" doc:"Mapped ListenUp user email"`
	ListenUpDisplayName string   `json:"listenup_display_name,omitempty" doc:"Mapped ListenUp user display name"`
	SessionCount        int      `json:"session_count" doc:"Number of sessions for this user"`
	TotalListenMs       int64    `json:"total_listen_ms" doc:"Total listening time in milliseconds"`
	Confidence          string   `json:"confidence" doc:"Match confidence"`
	MatchReason         string   `json:"match_reason,omitempty" doc:"Why matched"`
	Suggestions         []string `json:"suggestions,omitempty" doc:"Suggested ListenUp user IDs"`
	IsMapped            bool     `json:"is_mapped" doc:"Whether this user is mapped"`
}

type ListABSImportUsersOutput struct {
	Body struct {
		Users []ABSImportUserResponse `json:"users" doc:"List of ABS users"`
	}
}

type MapABSImportUserInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSUserID     string `path:"absUserId" doc:"ABS user ID"`
	Body          struct {
		ListenUpID string `json:"listenup_id" doc:"ListenUp user ID to map to"`
	}
}

type MapABSImportUserOutput struct {
	Body ABSImportUserResponse
}

type ClearABSImportUserMappingInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSUserID     string `path:"absUserId" doc:"ABS user ID"`
}

type ClearABSImportUserMappingOutput struct {
	Body ABSImportUserResponse
}

// Book mapping DTOs

type ListABSImportBooksInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	Filter        string `query:"filter" doc:"Filter: all, mapped, unmapped" default:"all"`
}

type ABSImportBookResponse struct {
	ABSMediaID     string   `json:"abs_media_id" doc:"ABS media ID"`
	ABSTitle       string   `json:"abs_title" doc:"ABS book title"`
	ABSAuthor      string   `json:"abs_author,omitempty" doc:"ABS author"`
	ABSDurationMs  int64    `json:"abs_duration_ms" doc:"ABS duration in milliseconds"`
	ListenUpID     string   `json:"listenup_id,omitempty" doc:"Mapped ListenUp book ID"`
	ListenUpTitle  string   `json:"listenup_title,omitempty" doc:"Mapped ListenUp book title"`
	ListenUpAuthor string   `json:"listenup_author,omitempty" doc:"Mapped ListenUp book author (first contributor)"`
	SessionCount   int      `json:"session_count" doc:"Number of sessions for this book"`
	Confidence     string   `json:"confidence" doc:"Match confidence"`
	MatchReason    string   `json:"match_reason,omitempty" doc:"Why matched"`
	Suggestions    []string `json:"suggestions,omitempty" doc:"Suggested ListenUp book IDs"`
	IsMapped       bool     `json:"is_mapped" doc:"Whether this book is mapped"`
}

type ListABSImportBooksOutput struct {
	Body struct {
		Books []ABSImportBookResponse `json:"books" doc:"List of ABS books"`
	}
}

type MapABSImportBookInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSMediaID    string `path:"absMediaId" doc:"ABS media ID"`
	Body          struct {
		ListenUpID string `json:"listenup_id" doc:"ListenUp book ID to map to"`
	}
}

type MapABSImportBookOutput struct {
	Body ABSImportBookResponse
}

type ClearABSImportBookMappingInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	ABSMediaID    string `path:"absMediaId" doc:"ABS media ID"`
}

type ClearABSImportBookMappingOutput struct {
	Body ABSImportBookResponse
}

// Session DTOs

type ListABSImportSessionsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	Status        string `query:"status" doc:"Filter: all, pending, ready, imported, skipped" default:"all"`
}

type ABSImportSessionResponse struct {
	ABSSessionID  string `json:"abs_session_id" doc:"ABS session ID"`
	ABSUserID     string `json:"abs_user_id" doc:"ABS user ID"`
	ABSMediaID    string `json:"abs_media_id" doc:"ABS media ID"`
	StartTime     string `json:"start_time" doc:"Session start time"`
	Duration      int64  `json:"duration" doc:"Session duration in milliseconds"`
	StartPosition int64  `json:"start_position" doc:"Start position in milliseconds"`
	EndPosition   int64  `json:"end_position" doc:"End position in milliseconds"`
	Status        string `json:"status" doc:"Import status"`
	ImportedAt    string `json:"imported_at,omitempty" doc:"When imported"`
	SkipReason    string `json:"skip_reason,omitempty" doc:"Why skipped"`
}

type ListABSImportSessionsOutput struct {
	Body struct {
		Sessions []ABSImportSessionResponse `json:"sessions" doc:"List of sessions"`
		Summary  struct {
			Total    int `json:"total" doc:"Total sessions"`
			Pending  int `json:"pending" doc:"Pending sessions"`
			Ready    int `json:"ready" doc:"Ready to import"`
			Imported int `json:"imported" doc:"Already imported"`
			Skipped  int `json:"skipped" doc:"Skipped sessions"`
		} `json:"summary" doc:"Session counts by status"`
	}
}

type ImportABSSessionsInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
}

type ImportABSSessionsOutput struct {
	Body struct {
		SessionsImported       int    `json:"sessions_imported" doc:"Sessions successfully imported"`
		SessionsFailed         int    `json:"sessions_failed" doc:"Sessions that failed to import"`
		EventsCreated          int    `json:"events_created" doc:"Listening events created"`
		ProgressRebuilt        int    `json:"progress_rebuilt" doc:"User+book progress records rebuilt"`
		ProgressFailed         int    `json:"progress_failed" doc:"Progress rebuilds that failed"`
		ABSProgressUnmatched   int    `json:"abs_progress_unmatched" doc:"Books where ABS progress could not be matched (finished status may be incorrect)"`
		ReadingSessionsCreated int    `json:"reading_sessions_created" doc:"BookReadingSession records created for readers section"`
		ReadingSessionsSkipped int    `json:"reading_sessions_skipped" doc:"Sessions skipped (already existed)"`
		Duration               string `json:"duration" doc:"Import duration"`
	}
}

type SkipABSSessionInput struct {
	Authorization string `header:"Authorization"`
	ID            string `path:"id" doc:"Import ID"`
	SessionID     string `path:"sessionId" doc:"ABS session ID"`
	Body          struct {
		Reason string `json:"reason,omitempty" doc:"Why skipping this session"`
	}
}

type SkipABSSessionOutput struct {
	Body ABSImportSessionResponse
}
