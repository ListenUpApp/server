package abs

import (
	"context"
	"log/slog"
	"testing"

	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// mockStore embeds the Store interface; only methods used in tests are implemented.
type mockStore struct {
	store.Store
	states map[string]*domain.PlaybackState
}

func newMockStore() *mockStore {
	return &mockStore{states: make(map[string]*domain.PlaybackState)}
}

func (m *mockStore) GetState(_ context.Context, userID, bookID string) (*domain.PlaybackState, error) {
	key := userID + ":" + bookID
	if s, ok := m.states[key]; ok {
		return s, nil
	}
	return nil, nil
}

func (m *mockStore) UpsertState(_ context.Context, state *domain.PlaybackState) error {
	key := state.UserID + ":" + state.BookID
	m.states[key] = state
	return nil
}

func TestApplyMediaProgressOverride(t *testing.T) {
	tests := []struct {
		name           string
		users          []User
		userMap        map[string]string
		bookMap        map[string]string
		existingStates map[string]*domain.PlaybackState
		wantOverrides  int
		wantFinished   map[string]bool  // key -> isFinished
		wantPositionMs map[string]int64 // key -> currentPositionMs
	}{
		{
			name: "finished book with session history gets override",
			users: []User{
				{
					ID: "abs-user-1", Username: "alice", Type: "user",
					Progress: []MediaProgress{
						{
							LibraryItemID: "abs-item-1",
							MediaItemType: "book",
							Progress:      1.0,
							CurrentTime:   36000, // 10 hours
							Duration:      36000,
							IsFinished:    true,
							FinishedAt:    1704067200000,
							LastUpdate:    1704067200000,
							StartedAt:     1704000000000,
						},
					},
				},
			},
			userMap: map[string]string{"abs-user-1": "lu-user-1"},
			bookMap: map[string]string{"abs-item-1": "lu-book-1"},
			existingStates: map[string]*domain.PlaybackState{
				"lu-user-1:lu-book-1": {
					UserID:            "lu-user-1",
					BookID:            "lu-book-1",
					CurrentPositionMs: 23040000, // 64% from event sums
					IsFinished:        false,
					TotalListenTimeMs: 23040000,
				},
			},
			wantOverrides:  1,
			wantFinished:   map[string]bool{"lu-user-1:lu-book-1": true},
			wantPositionMs: map[string]int64{"lu-user-1:lu-book-1": 36000000},
		},
		{
			name: "in-progress book gets correct position override",
			users: []User{
				{
					ID: "abs-user-1", Username: "bob", Type: "user",
					Progress: []MediaProgress{
						{
							LibraryItemID: "abs-item-2",
							MediaItemType: "book",
							Progress:      0.75,
							CurrentTime:   27000, // 7.5 hours
							Duration:      36000,
							IsFinished:    false,
							LastUpdate:    1704067200000,
							StartedAt:     1704000000000,
						},
					},
				},
			},
			userMap: map[string]string{"abs-user-1": "lu-user-1"},
			bookMap: map[string]string{"abs-item-2": "lu-book-2"},
			existingStates: map[string]*domain.PlaybackState{
				"lu-user-1:lu-book-2": {
					UserID:            "lu-user-1",
					BookID:            "lu-book-2",
					CurrentPositionMs: 20000000,
					IsFinished:        false,
				},
			},
			wantOverrides:  1,
			wantFinished:   map[string]bool{"lu-user-1:lu-book-2": false},
			wantPositionMs: map[string]int64{"lu-user-1:lu-book-2": 27000000},
		},
		{
			name: "zero progress and not finished gets skipped",
			users: []User{
				{
					ID: "abs-user-1", Username: "charlie", Type: "user",
					Progress: []MediaProgress{
						{
							LibraryItemID: "abs-item-3",
							MediaItemType: "book",
							Progress:      0,
							CurrentTime:   0,
							Duration:      36000,
							IsFinished:    false,
							LastUpdate:    1704067200000,
						},
					},
				},
			},
			userMap:        map[string]string{"abs-user-1": "lu-user-1"},
			bookMap:        map[string]string{"abs-item-3": "lu-book-3"},
			wantOverrides:  0,
			wantFinished:   map[string]bool{},
			wantPositionMs: map[string]int64{},
		},
		{
			name: "book not in bookMap gets skipped",
			users: []User{
				{
					ID: "abs-user-1", Username: "dave", Type: "user",
					Progress: []MediaProgress{
						{
							LibraryItemID: "abs-item-unknown",
							MediaItemType: "book",
							Progress:      0.5,
							CurrentTime:   18000,
							Duration:      36000,
							IsFinished:    false,
							LastUpdate:    1704067200000,
						},
					},
				},
			},
			userMap:        map[string]string{"abs-user-1": "lu-user-1"},
			bookMap:        map[string]string{}, // no mapping
			wantOverrides:  0,
			wantFinished:   map[string]bool{},
			wantPositionMs: map[string]int64{},
		},
		{
			name: "user not in userMap gets skipped",
			users: []User{
				{
					ID: "abs-user-unknown", Username: "eve", Type: "user",
					Progress: []MediaProgress{
						{
							LibraryItemID: "abs-item-1",
							MediaItemType: "book",
							Progress:      0.5,
							CurrentTime:   18000,
							Duration:      36000,
							IsFinished:    false,
							LastUpdate:    1704067200000,
						},
					},
				},
			},
			userMap:        map[string]string{}, // no mapping
			bookMap:        map[string]string{"abs-item-1": "lu-book-1"},
			wantOverrides:  0,
			wantFinished:   map[string]bool{},
			wantPositionMs: map[string]int64{},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			ms := newMockStore()
			// Seed existing states
			for k, v := range tt.existingStates {
				ms.states[k] = v
			}

			im := NewImporter(ms, nil, slog.Default())
			backup := &Backup{Users: tt.users}
			result := &ImportResult{}

			err := im.applyMediaProgressOverride(context.Background(), backup, tt.userMap, tt.bookMap, result)
			if err != nil {
				t.Fatalf("applyMediaProgressOverride() error = %v", err)
			}

			if result.ProgressOverridesApplied != tt.wantOverrides {
				t.Errorf("ProgressOverridesApplied = %d, want %d", result.ProgressOverridesApplied, tt.wantOverrides)
			}

			for key, wantFinished := range tt.wantFinished {
				state, ok := ms.states[key]
				if !ok {
					t.Errorf("expected state for %s, not found", key)
					continue
				}
				if state.IsFinished != wantFinished {
					t.Errorf("state[%s].IsFinished = %v, want %v", key, state.IsFinished, wantFinished)
				}
			}

			for key, wantPos := range tt.wantPositionMs {
				state, ok := ms.states[key]
				if !ok {
					t.Errorf("expected state for %s, not found", key)
					continue
				}
				if state.CurrentPositionMs != wantPos {
					t.Errorf("state[%s].CurrentPositionMs = %d, want %d", key, state.CurrentPositionMs, wantPos)
				}
			}
		})
	}
}
