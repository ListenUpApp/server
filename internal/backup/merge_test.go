package backup_test

import (
	"context"
	"log/slog"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/listenupapp/listenup-server/internal/backup"
	"github.com/listenupapp/listenup-server/internal/domain"
	"github.com/listenupapp/listenup-server/internal/store"
)

// TestMergeMode_KeepLocal tests merge mode with keep_local strategy.
func TestMergeMode_KeepLocal(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	older := now.Add(-time.Hour)

	// Create user in source
	sourceUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-1",
			CreatedAt: older,
			UpdatedAt: older,
		},
		Email:       "source@test.com",
		DisplayName: "Source User",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, sourceStore.Users.Create(ctx, sourceUser.ID, sourceUser))

	// Create instance (required for backup)
	_, err := sourceStore.CreateInstance(ctx)
	require.NoError(t, err)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{})
	require.NoError(t, err)

	// Create destination store with different user data
	destDir, err := os.MkdirTemp("", "merge_test")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destStore, err := store.New(filepath.Join(destDir, "dest.db"), nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	// Create user with same ID but different data in dest
	destUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-1",
			CreatedAt: now,
			UpdatedAt: now, // Newer than source
		},
		Email:       "dest@test.com",
		DisplayName: "Dest User",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, destStore.Users.Create(ctx, destUser.ID, destUser))

	// Restore with keep_local strategy
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	restoreSvc := backup.NewRestoreService(destStore, destDir, logger)

	restoreResult, err := restoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode:          backup.RestoreModeMerge,
		MergeStrategy: backup.MergeKeepLocal,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, restoreResult.Skipped["users"]) // Should skip

	// Verify local data kept
	user, err := destStore.Users.Get(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "dest@test.com", user.Email) // Local email kept
	assert.Equal(t, "Dest User", user.DisplayName)
}

// TestMergeMode_KeepBackup tests merge mode with keep_backup strategy.
func TestMergeMode_KeepBackup(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create user in source
	sourceUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:       "source@test.com",
		DisplayName: "Source User",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, sourceStore.Users.Create(ctx, sourceUser.ID, sourceUser))

	// Create instance
	_, err := sourceStore.CreateInstance(ctx)
	require.NoError(t, err)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{})
	require.NoError(t, err)

	// Create destination store with different user data
	destDir, err := os.MkdirTemp("", "merge_test")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destStore, err := store.New(filepath.Join(destDir, "dest.db"), nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	// Create user with same ID but different data
	destUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-1",
			CreatedAt: now,
			UpdatedAt: now,
		},
		Email:       "dest@test.com",
		DisplayName: "Dest User",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, destStore.Users.Create(ctx, destUser.ID, destUser))

	// Restore with keep_backup strategy
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	restoreSvc := backup.NewRestoreService(destStore, destDir, logger)

	restoreResult, err := restoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode:          backup.RestoreModeMerge,
		MergeStrategy: backup.MergeKeepBackup,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, restoreResult.Imported["users"]) // Should import

	// Verify backup data used
	user, err := destStore.Users.Get(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "source@test.com", user.Email) // Backup email used
	assert.Equal(t, "Source User", user.DisplayName)
}

// TestMergeMode_Newest tests merge mode with newest strategy.
func TestMergeMode_Newest(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	baseTime := time.Now()
	olderTime := baseTime.Add(-2 * time.Hour)
	newerTime := baseTime.Add(-1 * time.Hour)

	// Create user in source with older timestamp
	sourceUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-1",
			CreatedAt: olderTime,
			UpdatedAt: olderTime, // Older
		},
		Email:       "source@test.com",
		DisplayName: "Source User (Older)",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, sourceStore.Users.Create(ctx, sourceUser.ID, sourceUser))

	// Create instance
	_, err := sourceStore.CreateInstance(ctx)
	require.NoError(t, err)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{})
	require.NoError(t, err)

	// Create destination store with newer user data
	destDir, err := os.MkdirTemp("", "merge_test")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destStore, err := store.New(filepath.Join(destDir, "dest.db"), nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	// Create user with same ID but newer timestamp
	destUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-1",
			CreatedAt: baseTime,
			UpdatedAt: newerTime, // Newer than source
		},
		Email:       "dest@test.com",
		DisplayName: "Dest User (Newer)",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, destStore.Users.Create(ctx, destUser.ID, destUser))

	// Restore with newest strategy
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	restoreSvc := backup.NewRestoreService(destStore, destDir, logger)

	restoreResult, err := restoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode:          backup.RestoreModeMerge,
		MergeStrategy: backup.MergeNewest,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, restoreResult.Skipped["users"]) // Should skip (local is newer)

	// Verify newer (local) data kept
	user, err := destStore.Users.Get(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "dest@test.com", user.Email)
	assert.Equal(t, "Dest User (Newer)", user.DisplayName)
}

// TestMergeMode_NewEntitiesAdded tests that new entities from backup are added.
func TestMergeMode_NewEntitiesAdded(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()

	// Create two users in source
	user1 := &domain.User{
		Syncable:    domain.Syncable{ID: "user-1", CreatedAt: now, UpdatedAt: now},
		Email:       "user1@test.com",
		DisplayName: "User One",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	user2 := &domain.User{
		Syncable:    domain.Syncable{ID: "user-2", CreatedAt: now, UpdatedAt: now},
		Email:       "user2@test.com",
		DisplayName: "User Two",
		Role:        domain.RoleMember,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, sourceStore.Users.Create(ctx, user1.ID, user1))
	require.NoError(t, sourceStore.Users.Create(ctx, user2.ID, user2))

	// Create instance
	_, err := sourceStore.CreateInstance(ctx)
	require.NoError(t, err)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{})
	require.NoError(t, err)

	// Create destination store with only user-1
	destDir, err := os.MkdirTemp("", "merge_test")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destStore, err := store.New(filepath.Join(destDir, "dest.db"), nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	// Only create user-1 in dest
	destUser1 := &domain.User{
		Syncable:    domain.Syncable{ID: "user-1", CreatedAt: now, UpdatedAt: now},
		Email:       "dest1@test.com",
		DisplayName: "Dest User One",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, destStore.Users.Create(ctx, destUser1.ID, destUser1))

	// Restore with keep_local strategy
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	restoreSvc := backup.NewRestoreService(destStore, destDir, logger)

	restoreResult, err := restoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode:          backup.RestoreModeMerge,
		MergeStrategy: backup.MergeKeepLocal,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, restoreResult.Imported["users"]) // user-2 added
	assert.Equal(t, 1, restoreResult.Skipped["users"])  // user-1 skipped

	// Verify user-1 kept local data
	u1, err := destStore.Users.Get(ctx, "user-1")
	require.NoError(t, err)
	assert.Equal(t, "dest1@test.com", u1.Email)

	// Verify user-2 was added from backup
	u2, err := destStore.Users.Get(ctx, "user-2")
	require.NoError(t, err)
	assert.Equal(t, "user2@test.com", u2.Email)
	assert.Equal(t, "User Two", u2.DisplayName)
}

// TestMergeMode_SoftDeletedSkipped tests that soft-deleted entities are skipped in merge mode.
func TestMergeMode_SoftDeletedSkipped(t *testing.T) {
	sourceStore, backupSvc, _, _, cleanup := testSetup(t)
	defer cleanup()

	ctx := context.Background()
	now := time.Now()
	deletedAt := now.Add(-time.Hour)

	// Create soft-deleted user in source
	deletedUser := &domain.User{
		Syncable: domain.Syncable{
			ID:        "user-deleted",
			CreatedAt: now,
			UpdatedAt: now,
			DeletedAt: &deletedAt, // Soft deleted
		},
		Email:       "deleted@test.com",
		DisplayName: "Deleted User",
		IsRoot:      true,
		Role:        domain.RoleAdmin,
		Status:      domain.UserStatusActive,
	}
	require.NoError(t, sourceStore.Users.Create(ctx, deletedUser.ID, deletedUser))

	// Create instance
	_, err := sourceStore.CreateInstance(ctx)
	require.NoError(t, err)

	// Create backup
	result, err := backupSvc.Create(ctx, backup.BackupOptions{})
	require.NoError(t, err)

	// Create empty destination store
	destDir, err := os.MkdirTemp("", "merge_test")
	require.NoError(t, err)
	defer os.RemoveAll(destDir)

	destStore, err := store.New(filepath.Join(destDir, "dest.db"), nil, store.NewNoopEmitter())
	require.NoError(t, err)
	defer destStore.Close()

	// Restore with merge mode
	logger := slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelWarn}))
	restoreSvc := backup.NewRestoreService(destStore, destDir, logger)

	restoreResult, err := restoreSvc.Restore(ctx, result.Path, backup.RestoreOptions{
		Mode:          backup.RestoreModeMerge,
		MergeStrategy: backup.MergeKeepBackup,
	})
	require.NoError(t, err)
	assert.Equal(t, 1, restoreResult.Skipped["users"]) // Soft-deleted should be skipped

	// Verify user was not imported
	_, err = destStore.Users.Get(ctx, "user-deleted")
	assert.Error(t, err) // Should not exist
}
