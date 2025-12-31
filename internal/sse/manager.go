package sse

import (
	"context"
	"iter"
	"log/slog"
	"sync"
	"time"

	"github.com/listenupapp/listenup-server/internal/id"
)

// Client represents a connected SSE client.
type Client struct {
	ConnectedAt time.Time
	EventChan   chan Event
	Done        chan struct{}
	ID          string
	// Filtering fields - events are filtered in broadcast() to only deliver
	// events matching these criteria. Empty string means "receive all".
	UserID  string
	IsAdmin bool
}

// Manager manages SSE connections and broadcasts events.
type Manager struct {
	clients           map[string]*Client
	events            chan Event
	logger            *slog.Logger
	wg                sync.WaitGroup
	heartbeatInterval time.Duration
	mu                sync.RWMutex

	// Shutdown state - protected by shutdownMu
	shutdownMu sync.RWMutex
	shutdown   bool
}

// NewManager creates a new SSE Manager.
func NewManager(logger *slog.Logger) *Manager {
	return &Manager{
		clients:           make(map[string]*Client),
		events:            make(chan Event, 1000), // Buffer 1000 events
		logger:            logger,
		heartbeatInterval: 30 * time.Second,
	}
}

// Start begins the event broadcasting loop.
// This should be called once at server startup in a goroutine.
func (m *Manager) Start(ctx context.Context) {
	m.wg.Add(1)
	defer m.wg.Done()

	m.logger.Info("SSE manager starting")

	// Start heartbeat ticker.
	heartbeatTicker := time.NewTicker(m.heartbeatInterval)
	defer heartbeatTicker.Stop()

	for {
		select {
		case event := <-m.events:
			m.broadcast(event)

		case <-heartbeatTicker.C:
			// Send heartbeat to all clients.
			m.broadcast(NewHeartbeatEvent())

		case <-ctx.Done():
			m.logger.Info("SSE manager stopping")
			m.closeAllClients()
			return
		}
	}
}

// Shutdown gracefully shuts down the manager.
// It stops accepting new events, drains remaining events, and closes all clients.
func (m *Manager) Shutdown(ctx context.Context) error {
	m.logger.Info("SSE manager shutdown initiated")

	// Mark as shutdown to prevent new events from being accepted.
	// This must happen BEFORE closing the channel to prevent race.
	m.shutdownMu.Lock()
	m.shutdown = true
	m.shutdownMu.Unlock()

	// Close events channel to stop accepting new events.
	close(m.events)

	// Drain remaining events with context timeout.
	done := make(chan struct{})
	go func() {
		for event := range m.events {
			m.broadcast(event)
		}
		close(done)
	}()

	select {
	case <-done:
		m.logger.Info("SSE events drained successfully")
	case <-ctx.Done():
		m.logger.Warn("SSE event drain timeout, some events may be lost")
	}

	// Wait for broadcast goroutine to exit.
	m.wg.Wait()

	m.logger.Info("SSE manager shutdown complete")
	return nil
}

// isAdminOnlyEvent returns true if the event should only be sent to admin users.
func isAdminOnlyEvent(eventType EventType) bool {
	switch eventType {
	case EventCollectionCreated,
		EventCollectionUpdated,
		EventCollectionDeleted,
		EventCollectionBookAdded,
		EventCollectionBookRemoved,
		EventUserPending,
		EventUserApproved,
		EventScanStarted,
		EventScanComplete,
		EventInboxBookAdded,
		EventInboxBookReleased:
		return true
	default:
		return false
	}
}

// broadcast sends an event to connected clients, filtered by user/collection.
func (m *Manager) broadcast(event Event) {
	var delivered, dropped, filtered int

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		// Filter admin-only events to admin users
		if isAdminOnlyEvent(event.Type) && !client.IsAdmin {
			filtered++
			continue
		}

		// Filter by user when event is user-specific.
		// Empty event.UserID means broadcast to all users.
		if event.UserID != "" && client.UserID != "" && event.UserID != client.UserID {
			filtered++
			continue
		}

		// Non-blocking send (drop if client is slow/stuck).
		select {
		case client.EventChan <- event:
			delivered++
		default:
			dropped++
			m.logger.Warn("dropped event for slow client",
				slog.String("client_id", client.ID),
				slog.String("event_type", string(event.Type)))
		}
	}

	if event.Type != EventHeartbeat {
		m.logger.Debug("event broadcast",
			slog.String("event_type", string(event.Type)),
			slog.Group("stats",
				slog.Int("delivered", delivered),
				slog.Int("filtered", filtered),
				slog.Int("dropped", dropped)))
	}
}

// Connect registers a new SSE client and returns the client object.
// The userID is used to filter events - only events matching this user
// will be delivered to this client. Empty string means "all".
// The isAdmin flag indicates whether this client has admin privileges.
func (m *Manager) Connect(userID string, isAdmin bool) (*Client, error) {
	clientID, err := id.Generate("sse")
	if err != nil {
		return nil, err
	}

	client := &Client{
		ID:          clientID,
		UserID:      userID,
		IsAdmin:     isAdmin,
		EventChan:   make(chan Event, 100), // Buffer 100 events per client
		Done:        make(chan struct{}),
		ConnectedAt: time.Now(),
	}

	m.mu.Lock()
	m.clients[client.ID] = client
	totalClients := len(m.clients)
	m.mu.Unlock()

	m.logger.Info("SSE client connected",
		slog.String("client_id", clientID),
		slog.String("user_id", userID),
		slog.Bool("is_admin", isAdmin),
		slog.Int("total_clients", totalClients))
	return client, nil
}

// Disconnect removes a client and closes its channels.
func (m *Manager) Disconnect(clientID string) {
	m.mu.Lock()
	client, ok := m.clients[clientID]
	if !ok {
		m.mu.Unlock()
		return
	}
	delete(m.clients, clientID)
	totalClients := len(m.clients)
	m.mu.Unlock()

	close(client.Done)
	close(client.EventChan)

	m.logger.Info("SSE client disconnected",
		slog.String("client_id", clientID),
		slog.Duration("duration", time.Since(client.ConnectedAt)),
		slog.Int("total_clients", totalClients))
}

// Emit queues an event for broadcasting to clients.
// This implements the store.EventEmitter interface.
// Events are filtered by UserID/CollectionID in broadcast().
func (m *Manager) Emit(event any) {
	// Check shutdown state first to prevent panic on closed channel
	m.shutdownMu.RLock()
	isShutdown := m.shutdown
	m.shutdownMu.RUnlock()

	if isShutdown {
		// Silently drop events after shutdown - this is expected during shutdown
		return
	}

	// Type assert to Event - this is safe because store only emits Event types.
	evt, ok := event.(Event)
	if !ok {
		m.logger.Error("invalid event type emitted",
			slog.String("type", "unknown"))
		return
	}

	select {
	case m.events <- evt:
		// Event queued for broadcast.
	default:
		// Event channel full, log and drop.
		// This should rarely happen with a 1000-event buffer.
		// May occur during initial library scans with many rapid changes.
		m.logger.Error("SSE event channel full, dropping event",
			slog.String("event_type", string(evt.Type)))
	}
}

// EmitToUser queues an event for a specific user only.
func (m *Manager) EmitToUser(userID string, event Event) {
	event.UserID = userID
	m.Emit(event)
}

// EmitToNonMembers sends an event to all connected users who are NOT in the given user ID set.
// Used for notifying users who gained or lost access to a book.
func (m *Manager) EmitToNonMembers(memberUserIDs map[string]bool, event Event) {
	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		if client.IsAdmin {
			continue // Admins get collection events, not synthetic book events
		}
		if memberUserIDs[client.UserID] {
			continue // User has access, skip
		}

		select {
		case client.EventChan <- event:
		default:
			m.logger.Warn("dropped targeted event for slow client",
				slog.String("client_id", client.ID),
				slog.String("event_type", string(event.Type)))
		}
	}
}

// Clients returns an iterator over all connected clients.
// This uses Go 1.23+ iter.Seq for idiomatic iteration.
func (m *Manager) Clients() iter.Seq[*Client] {
	return func(yield func(*Client) bool) {
		m.mu.RLock()
		defer m.mu.RUnlock()

		for _, client := range m.clients {
			if !yield(client) {
				return
			}
		}
	}
}

// ClientCount returns the number of connected clients.
func (m *Manager) ClientCount() int {
	m.mu.RLock()
	defer m.mu.RUnlock()
	return len(m.clients)
}

// closeAllClients closes all client connections (used during shutdown).
func (m *Manager) closeAllClients() {
	m.mu.Lock()
	defer m.mu.Unlock()

	for _, client := range m.clients {
		close(client.Done)
		close(client.EventChan)
	}
	m.clients = make(map[string]*Client) // Clear the map

	m.logger.Info("all SSE clients disconnected")
}
