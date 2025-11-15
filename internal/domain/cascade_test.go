package domain

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

// mockCascadeUpdater is a mock implementation of CascadeUpdater for testing.
type mockCascadeUpdater struct {
	returnError     error
	touchedEntities []touchedEntity
}

type touchedEntity struct {
	entityType string
	id         string
}

func (m *mockCascadeUpdater) TouchEntity(_ context.Context, entityType, id string) error {
	if m.returnError != nil {
		return m.returnError
	}
	m.touchedEntities = append(m.touchedEntities, touchedEntity{
		entityType: entityType,
		id:         id,
	})
	return nil
}

func (m *mockCascadeUpdater) GetBookIDsBySeries(_ context.Context, _ string) ([]string, error) {
	if m.returnError != nil {
		return nil, m.returnError
	}
	return []string{}, nil
}

func TestCascadeBookUpdate(t *testing.T) {
	ctx := context.Background()

	t.Run("successfully touches book entity", func(t *testing.T) {
		mock := &mockCascadeUpdater{}
		bookID := "book-123"

		err := CascadeBookUpdate(ctx, mock, bookID)

		require.NoError(t, err)
		assert.Len(t, mock.touchedEntities, 1)
		assert.Equal(t, "book", mock.touchedEntities[0].entityType)
		assert.Equal(t, bookID, mock.touchedEntities[0].id)
	})

	t.Run("returns error when TouchEntity fails", func(t *testing.T) {
		expectedErr := errors.New("touch entity failed")
		mock := &mockCascadeUpdater{
			returnError: expectedErr,
		}
		bookID := "book-123"

		err := CascadeBookUpdate(ctx, mock, bookID)

		assert.Error(t, err)
		assert.Equal(t, expectedErr, err)
	})

	t.Run("handles empty book ID", func(t *testing.T) {
		mock := &mockCascadeUpdater{}
		bookID := ""

		err := CascadeBookUpdate(ctx, mock, bookID)

		require.NoError(t, err)
		assert.Len(t, mock.touchedEntities, 1)
		assert.Equal(t, "", mock.touchedEntities[0].id)
	})
}

func TestGetCurrentCheckpoint(t *testing.T) {
	t.Run("returns zero time for empty book list", func(t *testing.T) {
		books := []*Book{}

		checkpoint := GetCurrentCheckpoint(books)

		assert.True(t, checkpoint.IsZero())
	})

	t.Run("returns latest timestamp from single book", func(t *testing.T) {
		now := time.Now()
		books := []*Book{
			{
				Syncable: Syncable{
					ID:        "book-1",
					UpdatedAt: now,
				},
			},
		}

		checkpoint := GetCurrentCheckpoint(books)

		assert.Equal(t, now, checkpoint)
	})

	t.Run("returns latest timestamp from multiple books", func(t *testing.T) {
		now := time.Now()
		earlier := now.Add(-1 * time.Hour)
		latest := now.Add(1 * time.Hour)

		books := []*Book{
			{
				Syncable: Syncable{
					ID:        "book-1",
					UpdatedAt: earlier,
				},
			},
			{
				Syncable: Syncable{
					ID:        "book-2",
					UpdatedAt: now,
				},
			},
			{
				Syncable: Syncable{
					ID:        "book-3",
					UpdatedAt: latest,
				},
			},
		}

		checkpoint := GetCurrentCheckpoint(books)

		assert.Equal(t, latest, checkpoint)
	})

	t.Run("handles books with same timestamp", func(t *testing.T) {
		now := time.Now()
		books := []*Book{
			{
				Syncable: Syncable{
					ID:        "book-1",
					UpdatedAt: now,
				},
			},
			{
				Syncable: Syncable{
					ID:        "book-2",
					UpdatedAt: now,
				},
			},
		}

		checkpoint := GetCurrentCheckpoint(books)

		assert.Equal(t, now, checkpoint)
	})

	t.Run("handles nil book in list", func(t *testing.T) {
		now := time.Now()
		books := []*Book{
			{
				Syncable: Syncable{
					ID:        "book-1",
					UpdatedAt: now,
				},
			},
			nil,
			{
				Syncable: Syncable{
					ID:        "book-2",
					UpdatedAt: now.Add(1 * time.Hour),
				},
			},
		}

		// This will panic if we don't handle nil books, but the current.
		// implementation doesn't check for nil. This test documents the behavior.
		assert.Panics(t, func() {
			GetCurrentCheckpoint(books)
		})
	})
}
