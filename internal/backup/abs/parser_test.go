package abs

import (
	"os"
	"testing"
	"time"
)

func TestParseRealBackup(t *testing.T) {
	// Use actual backup file for integration test
	backupPath := "/home/simonh/listenUp/backups/uploads/abs-upload-1768064161252570495.audiobookshelf"

	if _, err := os.Stat(backupPath); os.IsNotExist(err) {
		t.Skip("Test backup file not found - skipping integration test")
	}

	start := time.Now()
	t.Logf("Starting parse of %s", backupPath)

	backup, err := Parse(backupPath)
	if err != nil {
		t.Fatalf("Parse failed: %v", err)
	}

	duration := time.Since(start)
	t.Logf("Parse completed in %v", duration)

	// Verify we got data
	t.Logf("Results:")
	t.Logf("  Users: %d", len(backup.Users))
	t.Logf("  Libraries: %d", len(backup.Libraries))
	t.Logf("  Items: %d", len(backup.Items))
	t.Logf("  Authors: %d", len(backup.Authors))
	t.Logf("  Series: %d", len(backup.Series))
	t.Logf("  Sessions: %d", len(backup.Sessions))

	// Basic sanity checks
	if len(backup.Users) == 0 {
		t.Error("Expected at least one user")
	}
	if len(backup.Items) == 0 {
		t.Error("Expected at least one library item")
	}

	// Performance check - should complete in under 10 seconds for reasonable backup
	if duration > 10*time.Second {
		t.Errorf("Parse took too long: %v (expected < 10s)", duration)
	}

	t.Logf("Summary: %s", backup.Summary())
}
