package sse

import (
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// Handler handles SSE connections at GET /api/v1/sync/stream.
type Handler struct {
	manager *Manager
	logger  *slog.Logger
}

// NewHandler creates a new SSE Handler.
func NewHandler(manager *Manager, logger *slog.Logger) *Handler {
	return &Handler{
		manager: manager,
		logger:  logger,
	}
}

// ServeHTTP handles the SSE connection.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests.
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if request context is already canceled (early client disconnect).
	if r.Context().Err() != nil {
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Use ResponseController for modern HTTP handling (Go 1.20+).
	rc := http.NewResponseController(w)

	// Flush headers immediately.
	if err := rc.Flush(); err != nil {
		h.logger.Error("failed to flush headers", slog.String("error", err.Error()))
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Register client.
	client, err := h.manager.Connect()
	if err != nil {
		h.logger.Error("failed to register SSE client", slog.String("error", err.Error()))
		http.Error(w, "Failed to establish connection", http.StatusInternalServerError)
		return
	}
	defer h.manager.Disconnect(client.ID)

	// Create logger with client context.
	clientLogger := h.logger.With(slog.String("client_id", client.ID))

	// Send initial connection message.
	if err := h.sendEvent(w, rc, "connected", map[string]string{
		"client_id": client.ID,
		"message":   "SSE connection established",
	}); err != nil {
		clientLogger.Warn("failed to send initial connection message", slog.String("error", err.Error()))
		return
	}

	// Stream events until client disconnects.
	ctx := r.Context()

	// Send periodic heartbeat to keep connection alive (every 30 seconds).
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case event := <-client.EventChan:
			if err := h.sendEvent(w, rc, string(event.Type), event); err != nil {
				// Client disconnect is normal, not an error condition.
				clientLogger.Info("client disconnected during send")
				return
			}

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive.
			heartbeat := NewHeartbeatEvent()
			if err := h.sendEvent(w, rc, string(heartbeat.Type), heartbeat); err != nil {
				clientLogger.Info("client disconnected during heartbeat")
				return
			}

		case <-client.Done:
			// Manager closed this client (server shutdown).
			clientLogger.Info("client closed by manager")
			return

		case <-ctx.Done():
			// Client disconnected.
			clientLogger.Info("client context canceled")
			return
		}
	}
}

// sendEvent writes an SSE event to the response writer using modern json/v2.
func (h *Handler) sendEvent(w http.ResponseWriter, rc *http.ResponseController, eventType string, data any) error {
	// Marshal data to JSON using json/v2.
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	// Write SSE format:
	// event: <type>.
	// data: <json>.
	// (blank line).

	if _, err := fmt.Fprintf(w, "event: %s\n", eventType); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", jsonData); err != nil {
		return err
	}

	// Flush immediately so client receives the event.
	if err := rc.Flush(); err != nil {
		return err
	}

	// Set write deadline for keepalive (prevents hung connections).
	// Reset after each successful write.
	if err := rc.SetWriteDeadline(time.Now().Add(60 * time.Second)); err != nil {
		// SetWriteDeadline may not be supported by all ResponseWriters.
		// Log but don't fail the request.
		h.logger.Debug("failed to set write deadline", slog.String("error", err.Error()))
	}

	return nil
}
