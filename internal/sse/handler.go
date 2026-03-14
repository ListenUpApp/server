package sse

import (
	"context"
	"encoding/json/v2"
	"fmt"
	"log/slog"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/listenupapp/listenup-server/internal/domain"
)

// TokenVerifier verifies access tokens and returns the associated user.
// This interface breaks the import cycle between sse and service packages.
type TokenVerifier interface {
	VerifyAccessToken(ctx context.Context, token string) (*domain.User, error)
}

// Handler handles SSE connections at GET /api/v1/sync/stream.
type Handler struct {
	manager       *Manager
	logger        *slog.Logger
	tokenVerifier TokenVerifier
	eventLogger   EventLogger
}

// NewHandler creates a new SSE Handler.
func NewHandler(manager *Manager, logger *slog.Logger, tokenVerifier TokenVerifier, eventLogger EventLogger) *Handler {
	return &Handler{
		manager:       manager,
		logger:        logger,
		tokenVerifier: tokenVerifier,
		eventLogger:   eventLogger,
	}
}

// extractBearerToken extracts the token from a Bearer authorization header.
func extractBearerToken(authHeader string) string {
	parts := strings.SplitN(authHeader, " ", 2)
	if len(parts) != 2 || parts[0] != "Bearer" {
		return ""
	}
	return parts[1]
}

// ServeHTTP handles the SSE connection.
func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only accept GET requests.
	if r.Method != http.MethodGet {
		http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
		return
	}

	// Authenticate the request.
	authHeader := r.Header.Get("Authorization")
	token := extractBearerToken(authHeader)
	if token == "" {
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
		return
	}

	user, err := h.tokenVerifier.VerifyAccessToken(r.Context(), token)
	if err != nil {
		h.logger.Debug("SSE auth failed", slog.String("error", err.Error()))
		http.Error(w, "Unauthorized", http.StatusUnauthorized)
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

	// SSE connections are long-lived. Disable the server's WriteTimeout immediately
	// to prevent premature connection closure. The heartbeat mechanism (30s) handles
	// keepalive, and we set per-write deadlines (60s) in sendEvent().
	// Use a very long deadline (24 hours) instead of 0 for compatibility.
	if err := rc.SetWriteDeadline(time.Now().Add(24 * time.Hour)); err != nil {
		h.logger.Debug("failed to disable write timeout for SSE", slog.String("error", err.Error()))
	}

	// Flush headers immediately.
	if err := rc.Flush(); err != nil {
		h.logger.Error("failed to flush headers", slog.String("error", err.Error()))
		http.Error(w, "Streaming not supported", http.StatusInternalServerError)
		return
	}

	// Register client with authenticated user info.
	client, err := h.manager.Connect(user.ID, user.IsAdmin())
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

	// Replay missed events if client provides Last-Event-ID or ?since= parameter.
	if h.eventLogger != nil {
		h.replayMissedEvents(w, rc, r, client, clientLogger)
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

// replayMissedEvents replays logged events to a reconnecting client.
func (h *Handler) replayMissedEvents(w http.ResponseWriter, rc *http.ResponseController, r *http.Request, client *Client, logger *slog.Logger) {
	var entries []EventLogEntry
	var err error

	// Check Last-Event-ID header first (standard SSE reconnect mechanism).
	if lastID := r.Header.Get("Last-Event-ID"); lastID != "" {
		id, parseErr := strconv.ParseInt(lastID, 10, 64)
		if parseErr == nil {
			entries, err = h.eventLogger.ReplayEventsSinceID(r.Context(), id, client.UserID)
		}
	} else if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		// Fall back to ?since= query parameter.
		since, parseErr := time.Parse(time.RFC3339, sinceStr)
		if parseErr == nil {
			entries, err = h.eventLogger.ReplayEvents(r.Context(), since, client.UserID)
		} else {
			// Try RFC3339Nano.
			since, parseErr = time.Parse(time.RFC3339Nano, sinceStr)
			if parseErr == nil {
				entries, err = h.eventLogger.ReplayEvents(r.Context(), since, client.UserID)
			}
		}
	}

	if err != nil {
		logger.Error("failed to replay events", slog.String("error", err.Error()))
		return
	}

	if len(entries) > 0 {
		logger.Info("replaying missed events", slog.Int("count", len(entries)))
		for _, entry := range entries {
			if err := h.sendRawEvent(w, rc, entry.EventType, entry.ID, entry.Payload); err != nil {
				logger.Warn("failed to replay event", slog.String("error", err.Error()))
				return
			}
		}
	}
}

// sendRawEvent writes a pre-serialized SSE event with an id field.
func (h *Handler) sendRawEvent(w http.ResponseWriter, rc *http.ResponseController, eventType string, id int64, jsonPayload string) error {
	if _, err := fmt.Fprintf(w, "id: %d\nevent: %s\ndata: %s\n\n", id, eventType, jsonPayload); err != nil {
		return err
	}
	if err := rc.Flush(); err != nil {
		return err
	}
	if err := rc.SetWriteDeadline(time.Now().Add(60 * time.Second)); err != nil {
		h.logger.Debug("failed to set write deadline", slog.String("error", err.Error()))
	}
	return nil
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
