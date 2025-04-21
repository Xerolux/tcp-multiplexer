package multiplexer

import (
	"container/ring"
	"fmt"
	"log/slog"
	"math"
	"net"
	"sync"
	"time"
)

// connPool manages a pool of target connections
type connPool struct {
	config      *Config
	connections []*connectionState
	connRing    *ring.Ring // For round-robin connection selection
	connMutex   sync.RWMutex
	metrics     *metrics
	quit        chan struct{}
}

// newConnPool creates a new connection pool
func newConnPool(config Config, metrics *metrics, quit chan struct{}) *connPool {
	pool := &connPool{
		config:      &config,
		connections: make([]*connectionState, config.MaxConnections),
		metrics:     metrics,
		quit:        quit,
	}

	// Initialize connection states
	for i := 0; i < config.MaxConnections; i++ {
		pool.connections[i] = &connectionState{
			active: false,
			index:  i,
		}
	}

	// Initialize ring for round-robin selection
	pool.connRing = ring.New(config.MaxConnections)
	for i := 0; i < config.MaxConnections; i++ {
		pool.connRing.Value = i
		pool.connRing = pool.connRing.Next()
	}

	return pool
}

// start initializes all connections in the pool
func (p *connPool) start() {
	for i := 0; i < p.config.MaxConnections; i++ {
		p.createConnection(i)
	}

	// Start health check routine
	go p.healthCheckLoop()
}

// healthCheckLoop periodically checks connection health
func (p *connPool) healthCheckLoop() {
	ticker := time.NewTicker(p.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-p.quit:
			return
		case <-ticker.C:
			p.performHealthCheck()
		}
	}
}

// performHealthCheck checks all connections and recreates failed ones
func (p *connPool) performHealthCheck() {
	p.connMutex.RLock()
	connsToCheck := make([]int, 0, len(p.connections))

	// Build list of connections to check under read lock (minimizing lock time)
	for i, conn := range p.connections {
		if conn == nil || !conn.active {
			continue
		}

		// Only check idle connections or connections idle for too long
		if !conn.processing.Load() && time.Since(conn.lastUsed) > p.config.HealthCheckInterval {
			connsToCheck = append(connsToCheck, i)
		}
	}
	p.connMutex.RUnlock()

	// Check connections outside the lock
	for _, idx := range connsToCheck {
		p.connMutex.RLock()
		conn := p.connections[idx].conn
		p.connMutex.RUnlock()

		if conn == nil {
			continue
		}

		// Simple health check: set a short deadline and try to write
		if err := conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond)); err != nil {
			slog.Warn(fmt.Sprintf("Health check failed for connection %d: %v", idx, err))
			p.closeConnection(idx)
			go p.createConnection(idx)
		} else {
			// Reset deadline
			if err := conn.SetWriteDeadline(time.Time{}); err != nil {
				slog.Warn(fmt.Sprintf("Failed to reset deadline for connection %d: %v", idx, err))
			}
		}
	}
}

// getConnection finds the best available connection using improved selection algorithm
func (p *connPool) getConnection() (int, error) {
	// Try round-robin first (with minimal locking)
	p.connMutex.RLock()
	initial := p.connRing
	current := initial

	// Do one round to find an available connection
	for i := 0; i < p.config.MaxConnections; i++ {
		idx := current.Value.(int)
		current = current.Next()

		if p.connections[idx].active && !p.connections[idx].processing.Load() {
			// Move the ring forward for next selection
			p.connRing = current
			p.connMutex.RUnlock()
			return idx, nil
		}
	}
	p.connMutex.RUnlock()

	// If no active connection found, try to find the first inactive one to activate
	p.connMutex.RLock()
	for i, conn := range p.connections {
		if conn != nil && !conn.active {
			p.connMutex.RUnlock()

			// Try to create this connection
			if p.createConnection(i) {
				return i, nil
			}
			break
		}
	}
	p.connMutex.RUnlock()

	// Last resort: poll for an available connection
	maxWaitTime := 3 * time.Second
	pollInterval := 50 * time.Millisecond
	endTime := time.Now().Add(maxWaitTime)

	for time.Now().Before(endTime) {
		for i := 0; i < p.config.MaxConnections; i++ {
			p.connMutex.RLock()
			connExists := p.connections[i] != nil && p.connections[i].active
			p.connMutex.RUnlock()

			if connExists {
				// Try to acquire this connection atomically
				if p.connections[i].processing.CompareAndSwap(false, true) {
					p.connMutex.RLock()
					stillActive := p.connections[i].active
					p.connMutex.RUnlock()

					if stillActive {
						return i, nil
					}

					// Release if not actually active
					p.connections[i].processing.Store(false)
				}
			}
		}

		// Wait before polling again
		select {
		case <-p.quit:
			return -1, fmt.Errorf("connection pool is shutting down")
		case <-time.After(pollInterval):
			// Continue polling
		}
	}

	return -1, fmt.Errorf("no target connections available after %.1f seconds", maxWaitTime.Seconds())
}

// createConnection creates a new connection with optimized error handling
func (p *connPool) createConnection(index int) bool {
	if index < 0 || index >= len(p.connections) {
		return false
	}

	// Get fail count atomically
	p.connMutex.RLock()
	failCount := p.connections[index].failCount
	p.connMutex.RUnlock()

	// Calculate backoff with exponential increase
	backoff := p.config.ReconnectBackoff
	if failCount > 0 {
		maxBackoff := 30 * time.Second
		// Fix: Use math.Pow instead of bit shift for exponential calculation
		backoffSeconds := float64(p.config.ReconnectBackoff) * math.Pow(2, float64(failCount-1))
		if backoffSeconds > float64(maxBackoff) {
			backoffSeconds = float64(maxBackoff)
		}
		backoff = time.Duration(backoffSeconds)
	}

	slog.Info(fmt.Sprintf("Creating target connection %d (attempt #%d, backoff: %v)",
		index, failCount+1, backoff))

	p.metrics.reconnectionAttempts.Add(1)

	// Wait before attempting to connect (for non-first attempts)
	if failCount > 0 {
		select {
		case <-p.quit:
			return false
		case <-time.After(backoff):
			// Continue after backoff
		}
	}

	// Set deadline for connection establishment
	dialer := &net.Dialer{
		Timeout:   10 * time.Second,
		KeepAlive: 30 * time.Second,
	}

	// Try to establish connection
	conn, err := dialer.Dial("tcp", p.config.TargetServer)
	if err != nil {
		// Update connection state for next attempt
		p.connMutex.Lock()
		p.connections[index].failCount++
		p.connections[index].active = false
		if p.connections[index].conn != nil {
			// Ensure old connection is closed
			_ = p.connections[index].conn.Close()
			p.connections[index].conn = nil
		}
		p.connMutex.Unlock()

		slog.Error(fmt.Sprintf("Failed to connect to target server %s (attempt #%d): %v",
			p.config.TargetServer, failCount+1, err))

		// Schedule next attempt
		select {
		case <-p.quit:
			return false
		default:
			go p.createConnection(index)
			return false
		}
	}

	// Configure TCP connection for optimal performance
	if tcpConn, ok := conn.(*net.TCPConn); ok {
		_ = tcpConn.SetKeepAlive(true)
		_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
		_ = tcpConn.SetNoDelay(true) // Disable Nagle's algorithm for lower latency
	}

	slog.Info(fmt.Sprintf("New target connection %d: %v <-> %v",
		index, conn.LocalAddr(), conn.RemoteAddr()))

	// Initial delay if configured
	if p.config.InitialDelay > 0 {
		time.Sleep(p.config.InitialDelay)
	}

	// Update connection state
	p.connMutex.Lock()
	oldConn := p.connections[index].conn

	// Check if we're shutting down
	select {
	case <-p.quit:
		p.connMutex.Unlock()
		_ = conn.Close()
		return false
	default:
		// Close old connection if it exists
		if oldConn != nil {
			_ = oldConn.Close()
		}

		// Update connection state
		p.connections[index].conn = conn
		p.connections[index].active = true
		p.connections[index].lastUsed = time.Now()
		p.connections[index].failCount = 0
		p.connections[index].processing.Store(false)

		// Update metrics
		if oldConn == nil {
			p.metrics.activeConnections.Add(1)
		}

		p.connMutex.Unlock()
		return true
	}
}

// closeConnection closes and cleans up a connection
func (p *connPool) closeConnection(index int) {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	if index < 0 || index >= len(p.connections) || !p.connections[index].active {
		return
	}

	if p.connections[index].conn != nil {
		_ = p.connections[index].conn.Close()
		p.connections[index].conn = nil
	}

	if p.connections[index].active {
		p.metrics.activeConnections.Add(-1)
	}

	p.connections[index].active = false
}

// close shuts down all connections in the pool
func (p *connPool) close() {
	p.connMutex.Lock()
	defer p.connMutex.Unlock()

	for i, conn := range p.connections {
		if conn != nil && conn.conn != nil {
			_ = conn.conn.Close()
			p.connections[i].conn = nil
			p.connections[i].active = false
		}
	}
}
