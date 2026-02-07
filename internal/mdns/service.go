// Package mdns provides mDNS/Zeroconf service advertisement for ListenUp server discovery.
//
// This implementation uses avahi's D-Bus API for robust service registration.
// Unlike spawning external processes or creating separate multicast sockets,
// D-Bus integration works WITH the system's mDNS infrastructure:
//
//   - Clean registration/deregistration lifecycle
//   - No orphaned processes or zombies
//   - No port conflicts with avahi-daemon
//   - Proper cleanup on server shutdown
//
// If avahi is unavailable (Docker, cloud environments), the server continues
// to function - users can always enter the server URL manually.
package mdns

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/godbus/dbus/v5"
	"github.com/holoplot/go-avahi"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	// ServiceType is the mDNS service type for ListenUp servers.
	ServiceType = "_listenup._tcp"

	// APIVersion is the current API version advertised in TXT records.
	APIVersion = "v1"

	// ServerVersion is the current server version advertised in TXT records.
	ServerVersion = "1.0.0"
)

// Service manages mDNS advertisement for the ListenUp server via avahi D-Bus.
type Service struct {
	conn       *dbus.Conn
	server     *avahi.Server
	entryGroup *avahi.EntryGroup
	logger     *slog.Logger
	mu         sync.Mutex
}

// NewService creates a new mDNS service.
func NewService(logger *slog.Logger) *Service {
	return &Service{
		logger: logger,
	}
}

// Start begins advertising the server via mDNS using avahi's D-Bus API.
//
// This connects to the system D-Bus, creates an avahi entry group, and
// registers the service. The service remains advertised until Stop() is called.
//
// Returns an error if avahi is unavailable (non-fatal for server operation).
func (s *Service) Start(instance *domain.Instance, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Clean up any existing registration first
	s.stopLocked()

	// Connect to system D-Bus
	conn, err := dbus.SystemBus()
	if err != nil {
		return fmt.Errorf("connect to system D-Bus: %w", err)
	}
	s.conn = conn

	// Create avahi server connection
	server, err := avahi.ServerNew(conn)
	if err != nil {
		s.conn.Close()
		s.conn = nil
		return fmt.Errorf("connect to avahi: %w", err)
	}
	s.server = server

	// Create entry group for our service
	entryGroup, err := server.EntryGroupNew()
	if err != nil {
		s.conn.Close()
		s.conn = nil
		s.server = nil
		return fmt.Errorf("create entry group: %w", err)
	}
	s.entryGroup = entryGroup

	// Get hostname for service name
	hostname, err := os.Hostname()
	if err != nil {
		hostname = "listenup-server"
	}

	// Build TXT records as [][]byte
	txtRecords := [][]byte{
		[]byte("id=" + instance.ID),
		[]byte("name=" + instance.Name),
		[]byte("version=" + ServerVersion),
		[]byte("api=" + APIVersion),
	}
	if instance.RemoteURL != "" {
		txtRecords = append(txtRecords, []byte("remote="+instance.RemoteURL))
	}

	// Register the service
	// Parameters: interface, protocol, flags, name, type, domain, host, port, txt
	err = entryGroup.AddService(
		avahi.InterfaceUnspec, // All interfaces
		avahi.ProtoUnspec,     // IPv4 and IPv6
		0,                     // No flags
		hostname,              // Service name (visible in discovery)
		ServiceType,           // _listenup._tcp
		"local",               // Domain
		"",                    // Host (empty = use avahi default)
		uint16(port),          // Port
		txtRecords,            // TXT records
	)
	if err != nil {
		s.cleanup()
		return fmt.Errorf("add service: %w", err)
	}

	// Commit to announce the service on the network
	if err := entryGroup.Commit(); err != nil {
		s.cleanup()
		return fmt.Errorf("commit entry group: %w", err)
	}

	s.logger.Info("mDNS advertisement started",
		"service", ServiceType,
		"port", port,
		"name", instance.Name,
		"id", instance.ID,
		"method", "avahi-dbus",
	)

	return nil
}

// Stop stops mDNS advertising and deregisters the service.
// Safe to call multiple times or if not started.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.stopLocked()
}

// stopLocked performs the actual stop. Caller must hold the mutex.
func (s *Service) stopLocked() {
	if s.entryGroup != nil || s.conn != nil {
		s.cleanup()
		s.logger.Info("mDNS advertisement stopped")
	}
}

// cleanup releases all avahi resources. Caller must hold the mutex.
func (s *Service) cleanup() {
	if s.entryGroup != nil && s.server != nil {
		// Free the entry group via server - this deregisters the service
		s.server.EntryGroupFree(s.entryGroup)
		s.entryGroup = nil
	}
	s.server = nil
	if s.conn != nil {
		_ = s.conn.Close()
		s.conn = nil
	}
}

// Running returns true if mDNS is currently advertising.
func (s *Service) Running() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.entryGroup != nil
}

// Refresh restarts the mDNS advertisement with updated instance data.
// This is used when instance settings (like RemoteURL) change at runtime.
// If mDNS is not currently running, this is a no-op.
func (s *Service) Refresh(instance *domain.Instance, port int) error {
	s.mu.Lock()
	wasRunning := s.entryGroup != nil
	if wasRunning {
		s.stopLocked()
	}
	s.mu.Unlock()

	if !wasRunning {
		return nil
	}

	s.logger.Info("Refreshing mDNS advertisement", "remote_url", instance.RemoteURL)
	return s.Start(instance, port)
}
