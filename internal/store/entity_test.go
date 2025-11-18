package store_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/listenupapp/listenup-server/internal/store"
	"github.com/stretchr/testify/require"
)

type TestEntity struct {
	ID    string `json:"id"`
	Name  string `json:"name"`
	Email string `json:"email"`
}

func setupTestStore(t *testing.T) (*store.Store, func()) {
	t.Helper()

	tmpDir, err := os.MkdirTemp("", "entity-test-*")
	require.NoError(t, err)

	dbPath := filepath.Join(tmpDir, "test.db")
	s, err := store.New(dbPath, nil, store.NewNoopEmitter())
	require.NoError(t, err)

	cleanup := func() {
		_ = s.Close()
		_ = os.RemoveAll(tmpDir)
	}

	return s, cleanup
}

func TestEntity_Create_Success(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := entity.Create(context.Background(), "1", testData)
	require.NoError(t, err)

	// Verify we can retrieve it
	retrieved, err := entity.Get(context.Background(), "1")
	require.NoError(t, err)
	require.Equal(t, testData.ID, retrieved.ID)
	require.Equal(t, testData.Name, retrieved.Name)
	require.Equal(t, testData.Email, retrieved.Email)
}

func TestEntity_Create_AlreadyExists(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Create first time
	err := entity.Create(context.Background(), "1", testData)
	require.NoError(t, err)

	// Try to create again
	err = entity.Create(context.Background(), "1", testData)
	require.Error(t, err)
	require.ErrorIs(t, err, store.ErrAlreadyExists)
}

func TestEntity_Get_Success(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := entity.Create(context.Background(), "1", testData)
	require.NoError(t, err)

	retrieved, err := entity.Get(context.Background(), "1")
	require.NoError(t, err)
	require.NotNil(t, retrieved)
	require.Equal(t, testData.ID, retrieved.ID)
	require.Equal(t, testData.Name, retrieved.Name)
	require.Equal(t, testData.Email, retrieved.Email)
}

func TestEntity_Get_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	retrieved, err := entity.Get(context.Background(), "nonexistent")
	require.Error(t, err)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, retrieved)
}

func TestEntity_Update_Success(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := entity.Create(context.Background(), "1", testData)
	require.NoError(t, err)

	// Update the entity
	updatedData := &TestEntity{
		ID:    "1",
		Name:  "Jane Doe",
		Email: "jane@example.com",
	}

	err = entity.Update(context.Background(), "1", updatedData)
	require.NoError(t, err)

	// Verify the update
	retrieved, err := entity.Get(context.Background(), "1")
	require.NoError(t, err)
	require.Equal(t, updatedData.Name, retrieved.Name)
	require.Equal(t, updatedData.Email, retrieved.Email)
}

func TestEntity_Update_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := entity.Update(context.Background(), "nonexistent", testData)
	require.Error(t, err)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestEntity_Delete_Success(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	err := entity.Create(context.Background(), "1", testData)
	require.NoError(t, err)

	// Delete the entity
	err = entity.Delete(context.Background(), "1")
	require.NoError(t, err)

	// Verify it's gone
	retrieved, err := entity.Get(context.Background(), "1")
	require.Error(t, err)
	require.ErrorIs(t, err, store.ErrNotFound)
	require.Nil(t, retrieved)
}

func TestEntity_Delete_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	// Delete should be idempotent - no error if not exists
	err := entity.Delete(context.Background(), "nonexistent")
	require.NoError(t, err)
}

func TestEntity_ContextCancellation(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Test Create with cancelled context
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	err := entity.Create(ctx, "1", testData)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	// Test Get with cancelled context
	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	_, err = entity.Get(ctx, "1")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	// Test Update with cancelled context
	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	err = entity.Update(ctx, "1", testData)
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)

	// Test Delete with cancelled context
	ctx, cancel = context.WithCancel(context.Background())
	cancel()

	err = entity.Delete(ctx, "1")
	require.Error(t, err)
	require.ErrorIs(t, err, context.Canceled)
}

func TestEntity_ContextTimeout(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")

	testData := &TestEntity{
		ID:    "1",
		Name:  "John Doe",
		Email: "john@example.com",
	}

	// Test with expired context
	ctx, cancel := context.WithTimeout(context.Background(), 1*time.Nanosecond)
	defer cancel()
	time.Sleep(2 * time.Nanosecond)

	err := entity.Create(ctx, "1", testData)
	require.Error(t, err)
	require.ErrorIs(t, err, context.DeadlineExceeded)
}

func TestEntity_WithIndex(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:").
		WithIndex("email", func(e *TestEntity) []string {
			return []string{e.Email}
		})

	ctx := context.Background()

	testEntity := &TestEntity{
		ID:    "test_123",
		Name:  "Test",
		Email: "test@example.com",
	}

	err := entity.Create(ctx, "test_123", testEntity)
	require.NoError(t, err)

	// Retrieve by index
	retrieved, err := entity.GetByIndex(ctx, "email", "test@example.com")
	require.NoError(t, err)
	require.Equal(t, testEntity.ID, retrieved.ID)
}

func TestEntity_GetByIndex_NotFound(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:").
		WithIndex("email", func(e *TestEntity) []string {
			return []string{e.Email}
		})

	ctx := context.Background()

	_, err := entity.GetByIndex(ctx, "email", "nonexistent@example.com")
	require.Error(t, err)
	require.ErrorIs(t, err, store.ErrNotFound)
}

func TestEntity_IndexConflict(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:").
		WithIndex("email", func(e *TestEntity) []string {
			return []string{e.Email}
		})

	ctx := context.Background()

	first := &TestEntity{
		ID:    "test_1",
		Name:  "First",
		Email: "same@example.com",
	}

	err := entity.Create(ctx, "test_1", first)
	require.NoError(t, err)

	// Try to create another with same email
	second := &TestEntity{
		ID:    "test_2",
		Name:  "Second",
		Email: "same@example.com",
	}

	err = entity.Create(ctx, "test_2", second)
	require.Error(t, err)
	require.Contains(t, err.Error(), "email")
}

func TestEntity_List(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")
	ctx := context.Background()

	// Create multiple entities
	for i := 1; i <= 5; i++ {
		testEntity := &TestEntity{
			ID:    fmt.Sprintf("test_%d", i),
			Name:  fmt.Sprintf("Test %d", i),
			Email: fmt.Sprintf("test%d@example.com", i),
		}
		err := entity.Create(ctx, testEntity.ID, testEntity)
		require.NoError(t, err)
	}

	// Iterate over all
	var count int
	for retrieved, err := range entity.List(ctx) {
		require.NoError(t, err)
		require.NotEmpty(t, retrieved.ID)
		count++
	}

	require.Equal(t, 5, count)
}

func TestEntity_List_EarlyTermination(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")
	ctx := context.Background()

	// Create multiple entities
	for i := 1; i <= 10; i++ {
		testEntity := &TestEntity{
			ID:    fmt.Sprintf("test_%d", i),
			Name:  fmt.Sprintf("Test %d", i),
			Email: fmt.Sprintf("test%d@example.com", i),
		}
		err := entity.Create(ctx, testEntity.ID, testEntity)
		require.NoError(t, err)
	}

	// Stop after 3 items
	var count int
	for retrieved, err := range entity.List(ctx) {
		require.NoError(t, err)
		require.NotEmpty(t, retrieved.ID)
		count++
		if count == 3 {
			break
		}
	}

	require.Equal(t, 3, count)
}

func TestEntity_List_ContextCancellation(t *testing.T) {
	s, cleanup := setupTestStore(t)
	defer cleanup()

	entity := store.NewEntity[TestEntity](s, "test:")
	ctx, cancel := context.WithCancel(context.Background())

	// Create entities
	for i := 1; i <= 5; i++ {
		testEntity := &TestEntity{
			ID:    fmt.Sprintf("test_%d", i),
			Name:  fmt.Sprintf("Test %d", i),
			Email: fmt.Sprintf("test%d@example.com", i),
		}
		err := entity.Create(context.Background(), testEntity.ID, testEntity)
		require.NoError(t, err)
	}

	// Cancel context during iteration
	var count int
	for retrieved, err := range entity.List(ctx) {
		count++
		if count == 2 {
			cancel()
			continue
		}
		if err != nil {
			require.Equal(t, context.Canceled, err)
			break
		}
		require.NotNil(t, retrieved)
	}
}
