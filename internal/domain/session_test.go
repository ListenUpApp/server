package domain

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewBookReadingSession(t *testing.T) {
	session := NewBookReadingSession("session-123", "user-456", "book-789")

	require.NotNil(t, session)
	assert.Equal(t, "session-123", session.ID)
	assert.Equal(t, "user-456", session.UserID)
	assert.Equal(t, "book-789", session.BookID)
	assert.False(t, session.StartedAt.IsZero())
	assert.Nil(t, session.FinishedAt)
	assert.False(t, session.IsCompleted)
	assert.Equal(t, 0.0, session.FinalProgress)
	assert.Equal(t, int64(0), session.ListenTimeMs)
	assert.False(t, session.CreatedAt.IsZero())
	assert.False(t, session.UpdatedAt.IsZero())
	assert.True(t, session.IsActive())
}

func TestBookReadingSession_IsActive(t *testing.T) {
	tests := []struct {
		name       string
		finishedAt *time.Time
		want       bool
	}{
		{
			name:       "active session - no finish time",
			finishedAt: nil,
			want:       true,
		},
		{
			name: "inactive session - has finish time",
			finishedAt: func() *time.Time {
				t := time.Now()
				return &t
			}(),
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			session := &BookReadingSession{
				FinishedAt: tt.finishedAt,
			}
			assert.Equal(t, tt.want, session.IsActive())
		})
	}
}

func TestBookReadingSession_IsStale(t *testing.T) {
	now := time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC)

	tests := []struct {
		name       string
		session    *BookReadingSession
		checkTime  time.Time
		wantStale  bool
	}{
		{
			name: "active session - recently updated",
			session: &BookReadingSession{
				FinishedAt: nil,
				UpdatedAt:  now.Add(-30 * 24 * time.Hour), // 1 month ago
			},
			checkTime: now,
			wantStale: false,
		},
		{
			name: "active session - stale (7 months old)",
			session: &BookReadingSession{
				FinishedAt: nil,
				UpdatedAt:  now.Add(-7 * 30 * 24 * time.Hour), // 7 months ago
			},
			checkTime: now,
			wantStale: true,
		},
		{
			name: "active session - exactly 6 months (not stale)",
			session: &BookReadingSession{
				FinishedAt: nil,
				UpdatedAt:  now.Add(-6 * 30 * 24 * time.Hour),
			},
			checkTime: now,
			wantStale: false,
		},
		{
			name: "inactive session - recently finished",
			session: &BookReadingSession{
				FinishedAt: func() *time.Time {
					t := now.Add(-30 * 24 * time.Hour) // 1 month ago
					return &t
				}(),
			},
			checkTime: now,
			wantStale: false,
		},
		{
			name: "inactive session - stale (7 months since finish)",
			session: &BookReadingSession{
				FinishedAt: func() *time.Time {
					t := now.Add(-7 * 30 * 24 * time.Hour) // 7 months ago
					return &t
				}(),
			},
			checkTime: now,
			wantStale: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			assert.Equal(t, tt.wantStale, tt.session.IsStale(tt.checkTime))
		})
	}
}

func TestBookReadingSession_MarkCompleted(t *testing.T) {
	session := NewBookReadingSession("session-123", "user-456", "book-789")
	beforeMark := time.Now()

	session.MarkCompleted(0.99, 3600000) // 99%, 1 hour

	assert.NotNil(t, session.FinishedAt)
	assert.True(t, session.FinishedAt.After(beforeMark) || session.FinishedAt.Equal(beforeMark))
	assert.True(t, session.IsCompleted)
	assert.Equal(t, 0.99, session.FinalProgress)
	assert.Equal(t, int64(3600000), session.ListenTimeMs)
	assert.False(t, session.IsActive())
	assert.True(t, session.UpdatedAt.After(beforeMark) || session.UpdatedAt.Equal(beforeMark))
}

func TestBookReadingSession_MarkAbandoned(t *testing.T) {
	session := NewBookReadingSession("session-123", "user-456", "book-789")
	beforeMark := time.Now()

	session.MarkAbandoned(0.45, 1800000) // 45%, 30 min

	assert.NotNil(t, session.FinishedAt)
	assert.True(t, session.FinishedAt.After(beforeMark) || session.FinishedAt.Equal(beforeMark))
	assert.False(t, session.IsCompleted)
	assert.Equal(t, 0.45, session.FinalProgress)
	assert.Equal(t, int64(1800000), session.ListenTimeMs)
	assert.False(t, session.IsActive())
	assert.True(t, session.UpdatedAt.After(beforeMark) || session.UpdatedAt.Equal(beforeMark))
}

func TestBookReadingSession_UpdateProgress(t *testing.T) {
	session := NewBookReadingSession("session-123", "user-456", "book-789")
	originalUpdatedAt := session.UpdatedAt

	// Wait a tiny bit to ensure time difference
	time.Sleep(2 * time.Millisecond)

	session.UpdateProgress(1800000) // 30 minutes

	assert.Equal(t, int64(1800000), session.ListenTimeMs)
	assert.True(t, session.UpdatedAt.After(originalUpdatedAt))
	assert.True(t, session.IsActive()) // Still active
}

func TestBookReadingSession_UpdateProgress_Accumulates(t *testing.T) {
	session := NewBookReadingSession("session-123", "user-456", "book-789")

	session.UpdateProgress(1800000) // 30 min
	assert.Equal(t, int64(1800000), session.ListenTimeMs)

	session.UpdateProgress(3600000) // Update to 1 hour
	assert.Equal(t, int64(3600000), session.ListenTimeMs)
}
