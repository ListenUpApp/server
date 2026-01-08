package sse

import (
	"log/slog"
	"sync"
	"time"
)

// RegistrationStatus represents the status of a pending registration.
type RegistrationStatus string

const (
	StatusPending  RegistrationStatus = "pending"
	StatusApproved RegistrationStatus = "approved"
	StatusDenied   RegistrationStatus = "denied"
)

// RegistrationStatusEvent is sent to pending users when their status changes.
type RegistrationStatusEvent struct {
	Status    RegistrationStatus `json:"status"`
	Timestamp time.Time          `json:"timestamp"`
}

// RegistrationSubscriber represents a pending user waiting for approval.
type RegistrationSubscriber struct {
	UserID    string
	EventChan chan RegistrationStatusEvent
	Done      chan struct{}
	CreatedAt time.Time
}

// RegistrationBroadcaster manages SSE connections for pending registrations.
// This is separate from the main SSE Manager because:
// 1. These connections are unauthenticated
// 2. They only need status change events
// 3. They're keyed by pending user ID, not session.
type RegistrationBroadcaster struct {
	subscribers map[string][]*RegistrationSubscriber // userId -> subscribers
	logger      *slog.Logger
	mu          sync.RWMutex
}

// NewRegistrationBroadcaster creates a new registration status broadcaster.
func NewRegistrationBroadcaster(logger *slog.Logger) *RegistrationBroadcaster {
	return &RegistrationBroadcaster{
		subscribers: make(map[string][]*RegistrationSubscriber),
		logger:      logger,
	}
}

// Subscribe creates a new subscriber for a pending user's status.
// Returns a subscriber with channels for events and done signal.
// The caller must call Unsubscribe when done to prevent leaks.
func (b *RegistrationBroadcaster) Subscribe(userID string) *RegistrationSubscriber {
	sub := &RegistrationSubscriber{
		UserID:    userID,
		EventChan: make(chan RegistrationStatusEvent, 10),
		Done:      make(chan struct{}),
		CreatedAt: time.Now(),
	}

	b.mu.Lock()
	b.subscribers[userID] = append(b.subscribers[userID], sub)
	totalSubs := len(b.subscribers[userID])
	b.mu.Unlock()

	b.logger.Debug("registration subscriber added",
		slog.String("user_id", userID),
		slog.Int("user_subscribers", totalSubs))

	return sub
}

// Unsubscribe removes a subscriber and closes its channels.
func (b *RegistrationBroadcaster) Unsubscribe(sub *RegistrationSubscriber) {
	b.mu.Lock()
	defer b.mu.Unlock()

	subs := b.subscribers[sub.UserID]
	for i, s := range subs {
		if s == sub {
			// Remove from slice
			b.subscribers[sub.UserID] = append(subs[:i], subs[i+1:]...)
			break
		}
	}

	// Clean up empty entries
	if len(b.subscribers[sub.UserID]) == 0 {
		delete(b.subscribers, sub.UserID)
	}

	// Close channels (safe to close multiple times via sync.Once if needed)
	close(sub.Done)
	close(sub.EventChan)

	b.logger.Debug("registration subscriber removed",
		slog.String("user_id", sub.UserID),
		slog.Duration("duration", time.Since(sub.CreatedAt)))
}

// NotifyApproved sends an approved status to all subscribers for a user.
func (b *RegistrationBroadcaster) NotifyApproved(userID string) {
	b.notify(userID, StatusApproved)
}

// NotifyDenied sends a denied status to all subscribers for a user.
func (b *RegistrationBroadcaster) NotifyDenied(userID string) {
	b.notify(userID, StatusDenied)
}

// notify sends a status event to all subscribers for a user.
func (b *RegistrationBroadcaster) notify(userID string, status RegistrationStatus) {
	event := RegistrationStatusEvent{
		Status:    status,
		Timestamp: time.Now(),
	}

	b.mu.RLock()
	subs := b.subscribers[userID]
	b.mu.RUnlock()

	if len(subs) == 0 {
		b.logger.Debug("no subscribers for registration status",
			slog.String("user_id", userID),
			slog.String("status", string(status)))
		return
	}

	var delivered, dropped int
	for _, sub := range subs {
		select {
		case sub.EventChan <- event:
			delivered++
		default:
			dropped++
		}
	}

	b.logger.Info("registration status broadcast",
		slog.String("user_id", userID),
		slog.String("status", string(status)),
		slog.Int("delivered", delivered),
		slog.Int("dropped", dropped))
}

// SubscriberCount returns the total number of active subscribers.
func (b *RegistrationBroadcaster) SubscriberCount() int {
	b.mu.RLock()
	defer b.mu.RUnlock()

	count := 0
	for _, subs := range b.subscribers {
		count += len(subs)
	}
	return count
}
