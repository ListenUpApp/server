package sqlite

import (
	"context"
	"time"

	"github.com/listenupapp/listenup-server/internal/sse"
)

// LogEvent persists an SSE event to the event log.
func (s *Store) LogEvent(ctx context.Context, eventType, payload, userID string) (int64, error) {
	result, err := s.db.ExecContext(ctx,
		"INSERT INTO sse_event_log (event_type, payload, user_id, created_at) VALUES (?, ?, ?, ?)",
		eventType, payload, nilIfEmpty(userID), time.Now().UTC().Format(time.RFC3339Nano),
	)
	if err != nil {
		return 0, err
	}
	return result.LastInsertId()
}

// ReplayEvents returns logged events newer than 'since' that the given user
// should see: broadcast events (user_id IS NULL) plus events targeted at this user.
func (s *Store) ReplayEvents(ctx context.Context, since time.Time, userID string) ([]sse.EventLogEntry, error) {
	sinceStr := since.UTC().Format(time.RFC3339Nano)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_type, payload, COALESCE(user_id, ''), created_at
		 FROM sse_event_log
		 WHERE created_at > ?
		   AND (user_id IS NULL OR user_id = ?)
		 ORDER BY id ASC`,
		sinceStr, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []sse.EventLogEntry
	for rows.Next() {
		var e sse.EventLogEntry
		var createdStr string
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload, &e.UserID, &createdStr); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// ReplayEventsSinceID returns logged events with id > lastEventID that the
// given user should see.
func (s *Store) ReplayEventsSinceID(ctx context.Context, lastEventID int64, userID string) ([]sse.EventLogEntry, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id, event_type, payload, COALESCE(user_id, ''), created_at
		 FROM sse_event_log
		 WHERE id > ?
		   AND (user_id IS NULL OR user_id = ?)
		 ORDER BY id ASC`,
		lastEventID, userID,
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entries []sse.EventLogEntry
	for rows.Next() {
		var e sse.EventLogEntry
		var createdStr string
		if err := rows.Scan(&e.ID, &e.EventType, &e.Payload, &e.UserID, &createdStr); err != nil {
			return nil, err
		}
		e.CreatedAt, _ = time.Parse(time.RFC3339Nano, createdStr)
		entries = append(entries, e)
	}
	return entries, rows.Err()
}

// CleanupEventLog deletes events older than the given duration.
func (s *Store) CleanupEventLog(ctx context.Context, maxAge time.Duration) (int64, error) {
	cutoff := time.Now().UTC().Add(-maxAge).Format(time.RFC3339Nano)
	result, err := s.db.ExecContext(ctx,
		"DELETE FROM sse_event_log WHERE created_at < ?",
		cutoff,
	)
	if err != nil {
		return 0, err
	}
	return result.RowsAffected()
}

// nilIfEmpty returns nil for empty strings, used for nullable user_id.
func nilIfEmpty(s string) any {
	if s == "" {
		return nil
	}
	return s
}
