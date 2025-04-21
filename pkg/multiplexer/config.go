package multiplexer

import (
	"time"
)

// Config holds all configuration options for the multiplexer
type Config struct {
	// TargetServer is the address of the target server to connect to
	TargetServer string
	
	// Port is the local port to listen on
	Port string
	
	// InitialDelay specifies the delay after establishing a new connection
	// before it can be used
	InitialDelay time.Duration
	
	// Timeout specifies the read/write timeout for connections
	Timeout time.Duration
	
	// MaxConnections specifies the maximum number of target connections
	// to maintain in the connection pool
	MaxConnections int
	
	// ReconnectBackoff specifies the initial backoff time for reconnection attempts
	// This will increase exponentially with failed attempts
	ReconnectBackoff time.Duration
	
	// HealthCheckInterval specifies how often to check connection health
	HealthCheckInterval time.Duration
	
	// QueueSize specifies the size of the request queue
	QueueSize int
	
	// MaxRequestSize limits the size of incoming requests
	MaxRequestSize int
	
	// MaxResponseSize limits the size of responses from the target
	MaxResponseSize int
	
	// IdleTimeout specifies how long a connection can remain idle
	// before it's closed
	IdleTimeout time.Duration
}

// DefaultConfig returns the default configuration
func DefaultConfig() Config {
	return Config{
		TargetServer:        "127.0.0.1:1234",
		Port:                "8000",
		InitialDelay:        0,
		Timeout:             60 * time.Second,
		MaxConnections:      1,
		ReconnectBackoff:    time.Second,
		HealthCheckInterval: 30 * time.Second,
		QueueSize:           32,
		MaxRequestSize:      1 << 20, // 1 MB
		MaxResponseSize:     1 << 20, // 1 MB
		IdleTimeout:         5 * time.Minute,
	}
}
