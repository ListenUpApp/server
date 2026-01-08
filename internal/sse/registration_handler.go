package sse

import (
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// RegistrationStatusHandler handles SSE connections for pending registration status.
// Endpoint: GET /api/v1/auth/registration-status/{user_id}/stream
//
// This is an unauthenticated endpoint - pending users don't have tokens yet.
// Security is maintained by:
// - User IDs are opaque UUIDs (can't enumerate)
// - Only binary status information is exposed (approved/denied)
// - No sensitive data flows through this channel.
type RegistrationStatusHandler struct {
	broadcaster *RegistrationBroadcaster
	logger      *slog.Logger
}

// NewRegistrationStatusHandler creates a new handler for registration status SSE.
func NewRegistrationStatusHandler(broadcaster *RegistrationBroadcaster, logger *slog.Logger) *RegistrationStatusHandler {
	return &RegistrationStatusHandler{
		broadcaster: broadcaster,
		logger:      logger,
	}
}

// ServeHTTP handles the SSE connection for a pending user's registration status.
// The userID must be provided via the request context (set by router).
func (h *RegistrationStatusHandler) ServeHTTP(w http.ResponseWriter, r *http.Request, userID string) {
	// Only accept GET requests.
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Check if request context is already canceled.
	if r.Context().Err() != nil {
		return
	}

	// Set SSE headers.
	w.Header().Set("Content-Type", "text/event-stream")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Connection", "keep-alive")
	w.Header().Set("X-Accel-Buffering", "no") // Disable nginx buffering

	// Use ResponseController for modern HTTP handling.
	rc := http.NewResponseController(w)

	// Flush headers immediately.
	if err := rc.Flush(); err != nil {
		h.logger.Error("failed to flush headers", slog.String("error", err.Error()))
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Subscribe to status updates for this user.
	sub := h.broadcaster.Subscribe(userID)
	defer h.broadcaster.Unsubscribe(sub)

	// Create logger with context.
	subLogger := h.logger.With(slog.String("user_id", userID))

	// Send initial connection message with current status (pending).
	if err := h.sendEvent(w, rc, "connected", map[string]any{
		"status":  StatusPending,
		"message": "Waiting for admin approval",
	}); err != nil {
		subLogger.Warn("failed to send initial message", slog.String("error", err.Error()))
		return
	}

	// Stream events until client disconnects or status changes.
	ctx := r.Context()

	// Send periodic heartbeat to keep connection alive.
	heartbeatTicker := time.NewTicker(30 * time.Second)
	defer heartbeatTicker.Stop()

	for {
		select {
		case event := <-sub.EventChan:
			// Status changed - send event and close connection.
			if err := h.sendEvent(w, rc, "status", event); err != nil {
				subLogger.Info("client disconnected during status send")
				return
			}
			// After sending final status, close the connection.
			// The client got what they were waiting for.
			subLogger.Info("status sent, closing connection",
				slog.String("status", string(event.Status)))
			return

		case <-heartbeatTicker.C:
			// Send heartbeat to keep connection alive.
			if err := h.sendEvent(w, rc, "heartbeat", map[string]any{
				"server_time": time.Now().UTC().Format(time.RFC3339),
			}); err != nil {
				subLogger.Info("client disconnected during heartbeat")
				return
			}

		case <-sub.Done:
			// Broadcaster closed this subscriber.
			subLogger.Info("subscriber closed by broadcaster")
			return

		case <-ctx.Done():
			// Client disconnected.
			subLogger.Info("client context canceled")
			return
		}
	}
}

// sendEvent writes an SSE event to the response writer.
func (h *RegistrationStatusHandler) sendEvent(w http.ResponseWriter, rc *http.ResponseController, eventType string, data any) error {
	jsonData, err := json.Marshal(data)
	if err != nil {
		return fmt.Errorf("marshal event data: %w", err)
	}

	if _, err := fmt.Fprintf(w, "event: %s\n", eventType); err != nil {
		return err
	}

	if _, err := fmt.Fprintf(w, "data: %s\n\n", jsonData); err != nil {
		return err
	}

	if err := rc.Flush(); err != nil {
		return err
	}

	// Set write deadline for keepalive.
	if err := rc.SetWriteDeadline(time.Now().Add(60 * time.Second)); err != nil {
		h.logger.Debug("failed to set write deadline", slog.String("error", err.Error()))
	}

	return nil
}
