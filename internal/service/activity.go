package service

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/listenupapp/listenup-server/internal/color"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/id"
	"github.com/listenupapp/listenup-server/internal/sse"
	"github.com/listenupapp/listenup-server/internal/store"
)

// ActivityService manages social activity recording and retrieval.
type ActivityService struct {
	store      *store.Store
	sseManager *sse.Manager
	logger     *slog.Logger
}

// getUserAvatarInfo retrieves avatar type and value from user profile.
// Falls back to "auto" type if profile not found.
func (s *ActivityService) getUserAvatarInfo(ctx context.Context, userID string) (avatarType, avatarValue string) {
	profile, err := s.store.GetUserProfile(ctx, userID)
	if err != nil || profile == nil {
		return "auto", ""
	}
	return string(profile.AvatarType), profile.AvatarValue
}

// NewActivityService creates a new activity service.
func NewActivityService(store *store.Store, sseManager *sse.Manager, logger *slog.Logger) *ActivityService {
	return &ActivityService{
		store:      store,
		sseManager: sseManager,
		logger:     logger,
	}
}

// RecordBookStarted creates an activity when a user starts reading a book.
// isReread distinguishes first-time reads from re-reads after completion.
func (s *ActivityService) RecordBookStarted(ctx context.Context, userID, bookID string, isReread bool) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	activityID, err := id.Generate("act")
	if err != nil {
		return fmt.Errorf("generate activity ID: %w", err)
	}

	avatarType, avatarValue := s.getUserAvatarInfo(ctx, userID)

	activity := &domain.Activity{
		ID:              activityID,
		UserID:          userID,
		Type:            domain.ActivityStartedBook,
		CreatedAt:       time.Now(),
		UserDisplayName: user.Name(),
		UserAvatarColor: color.ForUser(userID),
		UserAvatarType:  avatarType,
		UserAvatarValue: avatarValue,
		BookID:          bookID,
		BookTitle:       book.Title,
		BookAuthorName:  getAuthorName(ctx, s.store, book),
		BookCoverPath:   getBookCoverPath(book),
		IsReread:        isReread,
	}

	if err := s.store.CreateActivity(ctx, activity); err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	s.broadcastActivity(activity)

	action := "started"
	if isReread {
		action = "re-reading"
	}
	s.logger.Info("activity recorded",
		"type", activity.Type,
		"action", action,
		"user_id", userID,
		"book_id", bookID,
	)

	return nil
}

// RecordBookFinished creates an activity when a user completes a book.
func (s *ActivityService) RecordBookFinished(ctx context.Context, userID, bookID string) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	activityID, err := id.Generate("act")
	if err != nil {
		return fmt.Errorf("generate activity ID: %w", err)
	}

	avatarType, avatarValue := s.getUserAvatarInfo(ctx, userID)

	activity := &domain.Activity{
		ID:              activityID,
		UserID:          userID,
		Type:            domain.ActivityFinishedBook,
		CreatedAt:       time.Now(),
		UserDisplayName: user.Name(),
		UserAvatarColor: color.ForUser(userID),
		UserAvatarType:  avatarType,
		UserAvatarValue: avatarValue,
		BookID:          bookID,
		BookTitle:       book.Title,
		BookAuthorName:  getAuthorName(ctx, s.store, book),
		BookCoverPath:   getBookCoverPath(book),
	}

	if err := s.store.CreateActivity(ctx, activity); err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	s.broadcastActivity(activity)

	s.logger.Info("activity recorded",
		"type", activity.Type,
		"user_id", userID,
		"book_id", bookID,
	)

	return nil
}

// RecordStreakMilestone creates an activity when a user hits a streak milestone.
func (s *ActivityService) RecordStreakMilestone(ctx context.Context, userID string, days int) error {
	if !domain.IsStreakMilestone(days) {
		return nil // Not a milestone, skip
	}

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	activityID, err := id.Generate("act")
	if err != nil {
		return fmt.Errorf("generate activity ID: %w", err)
	}

	avatarType, avatarValue := s.getUserAvatarInfo(ctx, userID)

	activity := &domain.Activity{
		ID:              activityID,
		UserID:          userID,
		Type:            domain.ActivityStreakMilestone,
		CreatedAt:       time.Now(),
		UserDisplayName: user.Name(),
		UserAvatarColor: color.ForUser(userID),
		UserAvatarType:  avatarType,
		UserAvatarValue: avatarValue,
		MilestoneValue:  days,
		MilestoneUnit:   "days",
	}

	if err := s.store.CreateActivity(ctx, activity); err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	s.broadcastActivity(activity)

	s.logger.Info("streak milestone recorded",
		"user_id", userID,
		"days", days,
	)

	return nil
}

// RecordListeningMilestone creates an activity when a user crosses a listening hours milestone.
func (s *ActivityService) RecordListeningMilestone(ctx context.Context, userID string, hours int) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	activityID, err := id.Generate("act")
	if err != nil {
		return fmt.Errorf("generate activity ID: %w", err)
	}

	avatarType, avatarValue := s.getUserAvatarInfo(ctx, userID)

	activity := &domain.Activity{
		ID:              activityID,
		UserID:          userID,
		Type:            domain.ActivityListeningMilestone,
		CreatedAt:       time.Now(),
		UserDisplayName: user.Name(),
		UserAvatarColor: color.ForUser(userID),
		UserAvatarType:  avatarType,
		UserAvatarValue: avatarValue,
		MilestoneValue:  hours,
		MilestoneUnit:   "hours",
	}

	if err := s.store.CreateActivity(ctx, activity); err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	s.broadcastActivity(activity)

	s.logger.Info("listening milestone recorded",
		"user_id", userID,
		"hours", hours,
	)

	return nil
}

// RecordLensCreated creates an activity when a user creates a lens.
func (s *ActivityService) RecordLensCreated(ctx context.Context, userID string, lens *domain.Lens) error {
	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	activityID, err := id.Generate("act")
	if err != nil {
		return fmt.Errorf("generate activity ID: %w", err)
	}

	avatarType, avatarValue := s.getUserAvatarInfo(ctx, userID)

	activity := &domain.Activity{
		ID:              activityID,
		UserID:          userID,
		Type:            domain.ActivityLensCreated,
		CreatedAt:       time.Now(),
		UserDisplayName: user.Name(),
		UserAvatarColor: color.ForUser(userID),
		UserAvatarType:  avatarType,
		UserAvatarValue: avatarValue,
		LensID:          lens.ID,
		LensName:        lens.Name,
	}

	if err := s.store.CreateActivity(ctx, activity); err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	s.broadcastActivity(activity)

	s.logger.Info("lens created activity recorded",
		"user_id", userID,
		"lens_id", lens.ID,
		"lens_name", lens.Name,
	)

	return nil
}

// RecordListeningSession creates an activity when a user completes a listening session.
// Only records if durationMs >= MinListeningSessionMs to avoid spam.
func (s *ActivityService) RecordListeningSession(ctx context.Context, userID, bookID string, durationMs int64) error {
	durationSec := durationMs / 1000

	s.logger.Info("evaluating listening session for activity",
		"user_id", userID,
		"book_id", bookID,
		"duration_ms", durationMs,
		"duration_sec", durationSec,
		"min_duration_ms", domain.MinListeningSessionMs,
	)

	if durationMs < domain.MinListeningSessionMs {
		s.logger.Info("listening session too short for activity",
			"user_id", userID,
			"book_id", bookID,
			"duration_sec", durationSec,
			"min_duration_sec", domain.MinListeningSessionMs/1000,
		)
		return nil // Not an error, just skip
	}

	durationMin := durationMs / 60000

	user, err := s.store.GetUser(ctx, userID)
	if err != nil {
		return fmt.Errorf("get user: %w", err)
	}

	book, err := s.store.GetBookNoAccessCheck(ctx, bookID)
	if err != nil {
		return fmt.Errorf("get book: %w", err)
	}

	activityID, err := id.Generate("act")
	if err != nil {
		return fmt.Errorf("generate activity ID: %w", err)
	}

	avatarType, avatarValue := s.getUserAvatarInfo(ctx, userID)

	activity := &domain.Activity{
		ID:              activityID,
		UserID:          userID,
		Type:            domain.ActivityListeningSession,
		CreatedAt:       time.Now(),
		UserDisplayName: user.Name(),
		UserAvatarColor: color.ForUser(userID),
		UserAvatarType:  avatarType,
		UserAvatarValue: avatarValue,
		BookID:          bookID,
		BookTitle:       book.Title,
		BookAuthorName:  getAuthorName(ctx, s.store, book),
		BookCoverPath:   getBookCoverPath(book),
		DurationMs:      durationMs,
	}

	if err := s.store.CreateActivity(ctx, activity); err != nil {
		return fmt.Errorf("create activity: %w", err)
	}

	s.broadcastActivity(activity)

	s.logger.Info("listening session activity recorded",
		"user_id", userID,
		"book_id", bookID,
		"duration_min", durationMin,
	)

	return nil
}

// GetFeed retrieves the global activity feed with ACL filtering.
// Only returns activities for books the viewing user can access.
func (s *ActivityService) GetFeed(ctx context.Context, viewingUserID string, limit int, before *time.Time) ([]*domain.Activity, error) {
	// Overfetch to account for ACL filtering
	overfetchLimit := limit * 3
	if overfetchLimit < 30 {
		overfetchLimit = 30
	}

	activities, err := s.store.GetActivitiesFeed(ctx, overfetchLimit, before)
	if err != nil {
		return nil, fmt.Errorf("fetching activities: %w", err)
	}

	// Filter activities based on book access
	filtered := make([]*domain.Activity, 0, limit)
	for _, activity := range activities {
		if len(filtered) >= limit {
			break
		}

		// Non-book activities are always visible
		if activity.BookID == "" {
			filtered = append(filtered, activity)
			continue
		}

		// Book activities require access check
		// GetBook returns ErrBookNotFound for both missing books and access denied
		_, err := s.store.GetBook(ctx, activity.BookID, viewingUserID)
		if err != nil {
			// Skip if not found or access denied
			if errors.Is(err, store.ErrBookNotFound) {
				continue
			}
			// Log other errors but continue
			s.logger.Debug("error checking book access for activity",
				"activity_id", activity.ID,
				"book_id", activity.BookID,
				"error", err,
			)
			continue
		}

		filtered = append(filtered, activity)
	}

	return filtered, nil
}

// GetUserActivities retrieves activities for a specific user.
func (s *ActivityService) GetUserActivities(ctx context.Context, userID string, limit int) ([]*domain.Activity, error) {
	return s.store.GetUserActivities(ctx, userID, limit)
}

// GetBookActivities retrieves activities for a specific book.
func (s *ActivityService) GetBookActivities(ctx context.Context, bookID string, limit int) ([]*domain.Activity, error) {
	return s.store.GetBookActivities(ctx, bookID, limit)
}

// broadcastActivity sends the activity to all connected SSE clients.
func (s *ActivityService) broadcastActivity(activity *domain.Activity) {
	if s.sseManager == nil {
		return
	}
	s.sseManager.Emit(sse.NewActivityEvent(activity))
}

// getBookCoverPath extracts cover path from a book.
func getBookCoverPath(book *domain.Book) string {
	if book.CoverImage != nil && book.CoverImage.Path != "" {
		return book.CoverImage.Path
	}
	return ""
}
