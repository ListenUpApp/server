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
	// SECURITY TODO: Currently broadcasting to all clients regardless of user/collection
	// Issue: Any connected client receives all library events
	// Risk: Medium - self-hosted single-user deployments are fine, multi-user is not
	// When we fix, filter events by:
	UserID     string
	Collection string
}

// Manager manages SSE connections and broadcasts events.
type Manager struct {
	clients           map[string]*Client
	events            chan Event
	logger            *slog.Logger
	wg                sync.WaitGroup
	heartbeatInterval time.Duration
	mu                sync.RWMutex
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

// broadcast sends an event to all connected clients.
func (m *Manager) broadcast(event Event) {
	var delivered, dropped int

	m.mu.RLock()
	defer m.mu.RUnlock()

	for _, client := range m.clients {
		// TODO: Filter by user/collection when multi-user is implemented
		// if event.UserID != "" && event.UserID != client.UserID {.
		//     continue.
		// }

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
				slog.Int("dropped", dropped)))
	}
}

// Connect registers a new SSE client and returns the client object.
func (m *Manager) Connect() (*Client, error) {
	clientID, err := id.Generate("sse")
	if err != nil {
		return nil, err
	}

	client := &Client{
		ID:          clientID,
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

// Emit queues an event for broadcasting to all clients.
// This implements the store.EventEmitter interface.
func (m *Manager) Emit(event any) {
	// Type assert to Event - this is safe because store only emits Event types.
	evt, ok := event.(Event)
	if !ok {
		m.logger.Error("invalid event type emitted",
			slog.String("type", "unknown"))
		return
	}

	// SECURITY TODO: Currently broadcasting to ALL clients.
	// This is acceptable for single-library, single-user deployments.
	// Must add filtering before multi-user support.
	// See: Client.UserID TODO above.

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
