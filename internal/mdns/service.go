// Package mdns provides mDNS/Zeroconf service advertisement for ListenUp server discovery.
package mdns

import (
	"fmt"
	"log/slog"
	"os"
	"sync"

	"github.com/hashicorp/mdns"
	"github.com/listenupapp/listenup-server/internal/domain"
)

const (
	// ServiceType is the mDNS service type for ListenUp servers.
	ServiceType = "_listenup._tcp"

	// APIVersion is the current API version advertised in TXT records.
	APIVersion = "v1"

	// ServerVersion is the current server version advertised in TXT records.
	// TODO: Extract to a shared version package.
	ServerVersion = "1.0.0"
)

// Service manages mDNS advertisement for the ListenUp server.
// It allows local network discovery of the server without manual configuration.
type Service struct {
	server *mdns.Server
	logger *slog.Logger
	mu     sync.Mutex
}

// NewService creates a new mDNS service.
func NewService(logger *slog.Logger) *Service {
	return &Service{
		logger: logger,
	}
}

// Start begins advertising the server via mDNS.
// It should be called after the HTTP server is running.
//
// Parameters:
//   - instance: Server instance containing ID, name, and URLs
//   - port: The HTTP server port
//
// Returns an error if mDNS advertisement fails to start.
// Errors are typically non-fatal (e.g., multicast not supported in Docker).
func (s *Service) Start(instance *domain.Instance, port int) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Stop existing server if running (for restart scenarios)
	if s.server != nil {
		_ = s.server.Shutdown()
		s.server = nil
	}

	// Get hostname for mDNS instance name
	host, err := os.Hostname()
	if err != nil {
		host = "listenup-server"
	}

	// Build TXT records with server metadata
	txtRecords := []string{
		fmt.Sprintf("id=%s", instance.ID),
		fmt.Sprintf("name=%s", instance.Name),
		fmt.Sprintf("version=%s", ServerVersion),
		fmt.Sprintf("api=%s", APIVersion),
	}

	// Only include remote URL if configured
	if instance.RemoteURL != "" {
		txtRecords = append(txtRecords, fmt.Sprintf("remote=%s", instance.RemoteURL))
	}

	// Create mDNS service configuration
	service, err := mdns.NewMDNSService(
		host,        // Instance name (hostname)
		ServiceType, // Service type (_listenup._tcp)
		"",          // Domain (empty = .local)
		"",          // Host (empty = use system hostname)
		port,        // Port
		nil,         // IPs (nil = all interfaces)
		txtRecords,  // TXT records
	)
	if err != nil {
		return fmt.Errorf("create mDNS service: %w", err)
	}

	// Create and start mDNS server
	server, err := mdns.NewServer(&mdns.Config{
		Zone: service,
	})
	if err != nil {
		return fmt.Errorf("start mDNS server: %w", err)
	}

	s.server = server

	s.logger.Info("mDNS advertisement started",
		"service", ServiceType,
		"port", port,
		"name", instance.Name,
		"id", instance.ID,
	)

	return nil
}

// Stop stops mDNS advertising.
// Safe to call multiple times or if not started.
func (s *Service) Stop() {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.server != nil {
		_ = s.server.Shutdown()
		s.server = nil
		s.logger.Info("mDNS advertisement stopped")
	}
}
