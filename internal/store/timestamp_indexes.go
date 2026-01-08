package store

import (
	"fmt"
	"strings"
	"time"
)

// formatTimestampIndexKey creates a timestamp index key with sortable timestamp.
// We use a custom format with zero-padded nanoseconds to ensure lexicographic sorting works correctly.
// Format: {prefix}{YYYY-MM-DDTHH:MM:SS.NNNNNNNNNZ}:{entityType}:{entityID}.
// Example: idx:books:updated_at:2024-01-15T10:30:00.123456789Z:book:abc-123.
func formatTimestampIndexKey(prefix string, timestamp time.Time, entityType, entityID string) []byte {
	// Use custom format with fixed-width nanoseconds (always 9 digits).
	// This ensures proper lexicographic sorting of timestamps.
	timestampStr := timestamp.UTC().Format("2006-01-02T15:04:05") + fmt.Sprintf(".%09d", timestamp.Nanosecond()) + "Z"
	return fmt.Appendf(nil, "%s%s:%s:%s", prefix, timestampStr, entityType, entityID)
}

// parseTimestampIndexKey extracts entity type and ID from a timestamp index key.
// Returns entityType, entityID, and error.
func parseTimestampIndexKey(key []byte, expectedPrefix string) (entityType, entityID string, err error) {
	keyStr := string(key)
	if !strings.HasPrefix(keyStr, expectedPrefix) {
		return "", "", fmt.Errorf("invalid timestamp key: missing prefix %s", expectedPrefix)
	}

	remainder := strings.TrimPrefix(keyStr, expectedPrefix)

	// Timestamp format is fixed width: 2006-01-02T15:04:05.NNNNNNNNNZ = 30 characters.
	// This avoids issues with splitting on : which appears in the timestamp.
	const timestampLen = 30
	if len(remainder) < timestampLen+2 { // +2 for at least "::"
		return "", "", fmt.Errorf("invalid timestamp key format: %s", keyStr)
	}

	// Skip the timestamp and first colon.
	afterTimestamp := remainder[timestampLen+1:]

	// Now split the entityType:entityID part.
	parts := strings.SplitN(afterTimestamp, ":", 2)
	if len(parts) != 2 {
		return "", "", fmt.Errorf("invalid timestamp key format: %s", keyStr)
	}

	// parts[0] is the entity type (e.g., "book", "author", "series").
	// parts[1] is the entity ID (e.g., UUID).

	return parts[0], parts[1], nil
}
