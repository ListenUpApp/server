package api

import (
	"context"
	"fmt"
	"net/http"

	"github.com/danielgtaylor/huma/v2"
	"github.com/listenupapp/listenup-server/internal/domain"
)

func (s *Server) registerSocialRoutes() {
	huma.Register(s.api, huma.Operation{
		OperationID: "getLeaderboard",
		Method:      http.MethodGet,
		Path:        "/api/v1/social/leaderboard",
		Summary:     "Get leaderboard",
		Description: "Returns the community leaderboard for the specified category and period",
		Tags:        []string{"Social"},
		Security:    []map[string][]string{{"bearer": {}}},
	}, s.handleGetLeaderboard)
}

// === DTOs ===

// GetLeaderboardInput contains parameters for getting the leaderboard.
type GetLeaderboardInput struct {
	Authorization string `header:"Authorization"`
	Period        string `query:"period" enum:"week,month,year,all" default:"week" doc:"Time period for leaderboard"`
	Category      string `query:"category" enum:"time,books,streak" default:"time" doc:"Ranking category"`
	Limit         int    `query:"limit" doc:"Max entries (default 10, max 50)"`
}

// LeaderboardEntryResponse represents a single leaderboard entry.
type LeaderboardEntryResponse struct {
	Rank          int     `json:"rank" doc:"Position in leaderboard"`
	UserID        string  `json:"user_id" doc:"User ID"`
	DisplayName   string  `json:"display_name" doc:"User display name"`
	AvatarURL     *string `json:"avatar_url,omitempty" doc:"Avatar URL if available"`
	Value         int64   `json:"value" doc:"Numeric value (time ms, count, or days)"`
	ValueLabel    string  `json:"value_label" doc:"Human-readable value"`
	IsCurrentUser bool    `json:"is_current_user" doc:"Whether this is the requesting user"`
}

// CommunityStatsResponse contains aggregate community statistics.
type CommunityStatsResponse struct {
	TotalTimeMs      int64   `json:"total_time_ms" doc:"Community total listening time"`
	TotalTimeLabel   string  `json:"total_time_label" doc:"Human-readable total time"`
	TotalBooks       int     `json:"total_books" doc:"Community total books finished"`
	AverageStreak    float64 `json:"average_streak" doc:"Average streak days across users"`
	ActiveUsersCount int     `json:"active_users_count" doc:"Number of users with activity"`
}

// LeaderboardResponse contains the full leaderboard data.
type LeaderboardResponse struct {
	Category       string                     `json:"category" doc:"Ranking category"`
	Period         string                     `json:"period" doc:"Time period"`
	Entries        []LeaderboardEntryResponse `json:"entries" doc:"Ranked entries"`
	CommunityStats CommunityStatsResponse     `json:"community_stats" doc:"Aggregate stats"`
}

// LeaderboardOutput wraps the leaderboard response for Huma.
type LeaderboardOutput struct {
	Body LeaderboardResponse
}

// === Handlers ===

func (s *Server) handleGetLeaderboard(ctx context.Context, input *GetLeaderboardInput) (*LeaderboardOutput, error) {
	userID, err := s.authenticateRequest(ctx, input.Authorization)
	if err != nil {
		return nil, err
	}

	// Parse and validate period
	period := domain.StatsPeriod(input.Period)
	if !period.Valid() {
		period = domain.StatsPeriodWeek
	}

	// Parse and validate category
	category := domain.LeaderboardCategory(input.Category)
	if !category.Valid() {
		category = domain.LeaderboardCategoryTime
	}

	// Get leaderboard from service
	leaderboard, err := s.services.Social.GetLeaderboard(ctx, userID, period, category, input.Limit)
	if err != nil {
		return nil, err
	}

	// Convert entries to response format
	entries := make([]LeaderboardEntryResponse, len(leaderboard.Entries))
	for i, e := range leaderboard.Entries {
		entries[i] = LeaderboardEntryResponse{
			Rank:          e.Rank,
			UserID:        e.UserID,
			DisplayName:   e.DisplayName,
			AvatarURL:     e.AvatarURL,
			Value:         e.Value,
			ValueLabel:    e.ValueLabel,
			IsCurrentUser: e.IsCurrentUser,
		}
	}

	// Format community total time
	totalMinutes := leaderboard.CommunityTotalTimeMs / 60_000
	hours := totalMinutes / 60
	minutes := totalMinutes % 60
	var totalTimeLabel string
	switch {
	case hours == 0:
		totalTimeLabel = formatMinutes(minutes)
	case minutes == 0:
		totalTimeLabel = formatHours(hours)
	default:
		totalTimeLabel = formatHoursMinutes(hours, minutes)
	}

	return &LeaderboardOutput{
		Body: LeaderboardResponse{
			Category: string(leaderboard.Category),
			Period:   string(leaderboard.Period),
			Entries:  entries,
			CommunityStats: CommunityStatsResponse{
				TotalTimeMs:      leaderboard.CommunityTotalTimeMs,
				TotalTimeLabel:   totalTimeLabel,
				TotalBooks:       leaderboard.CommunityTotalBooks,
				AverageStreak:    leaderboard.CommunityAverageStreak,
				ActiveUsersCount: leaderboard.TotalUsers,
			},
		},
	}, nil
}

func formatMinutes(m int64) string {
	if m == 1 {
		return "1 minute"
	}
	return fmt.Sprintf("%d minutes", m)
}

func formatHours(h int64) string {
	if h == 1 {
		return "1 hour"
	}
	return fmt.Sprintf("%d hours", h)
}

func formatHoursMinutes(h, m int64) string {
	return fmt.Sprintf("%dh %dm", h, m)
}
