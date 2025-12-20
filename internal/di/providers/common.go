package providers

import "time"

const (
	// shutdownTimeout is the maximum time to wait for graceful shutdown of services.
	shutdownTimeout = 30 * time.Second
)
