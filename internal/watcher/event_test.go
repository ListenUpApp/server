package watcher

import (
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
)

func TestEventType_String(t *testing.T) {
	tests := []struct {
		eventType EventType
		want      string
	}{
		{EventAdded, "added"},
		{EventModified, "modified"},
		{EventRemoved, "removed"},
		{EventMoved, "moved"},
	}

	for _, tt := range tests {
		t.Run(tt.want, func(t *testing.T) {
			got := tt.eventType.String()
			assert.Equal(t, tt.want, got)
		})
	}
}

func TestEvent_Creation(t *testing.T) {
	now := time.Now()
	event := Event{
		Type:    EventAdded,
		Path:    "/test/file.m4b",
		Inode:   12345,
		Size:    1024,
		ModTime: now,
	}

	assert.Equal(t, EventAdded, event.Type)
	assert.Equal(t, "/test/file.m4b", event.Path)
	assert.Equal(t, uint64(12345), event.Inode)
	assert.Equal(t, int64(1024), event.Size)
	assert.Equal(t, now, event.ModTime)
}

func TestEvent_MoveEvent(t *testing.T) {
	event := Event{
		Type:    EventMoved,
		Path:    "/new/path.m4b",
		OldPath: "/old/path.m4b",
		Inode:   12345,
	}

	assert.Equal(t, EventMoved, event.Type)
	assert.Equal(t, "/new/path.m4b", event.Path)
	assert.Equal(t, "/old/path.m4b", event.OldPath)
	assert.Equal(t, uint64(12345), event.Inode)
}
