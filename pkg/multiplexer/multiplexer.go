package multiplexer

import (
	"fmt"
	"io"
	"log/slog"
	"math"
	"net"
	"sync"
	"sync/atomic"
	"time"

	"github.com/Xerolux/tcp-multiplexer/pkg/message"
)

type (
	messageType int

	reqContainer struct {
		typ     messageType
		message []byte
		sender  chan<- *respContainer
	}

	respContainer struct {
		message []byte
		err     error
	}

	// connectionState tracks the state of a target connection
	connectionState struct {
		conn       net.Conn
		active     bool
		lastUsed   time.Time
		failCount  int
		processing atomic.Bool
	}

	Multiplexer struct {
		config        Config
		messageReader message.Reader
		l             net.Listener
		quit          chan struct{}
		wg            *sync.WaitGroup
		requestQueue  chan *reqContainer
		connMutex     sync.RWMutex
		connections   []*connectionState
		metrics       *metrics
	}

	// metrics tracks multiplexer statistics
	metrics struct {
		activeConnections   atomic.Int32
		totalRequests       atomic.Int64
		failedRequests      atomic.Int64
		reconnectionAttempts atomic.Int64
		processingTime      atomic.Int64
		requestCount        atomic.Int64
	}
)

const (
	Connection messageType = iota
	Disconnection
	Packet
)

// New creates a multiplexer with default config
func New(targetServer, port string, messageReader message.Reader, delay time.Duration, timeout time.Duration) Multiplexer {
	config := Config{
		TargetServer:        targetServer,
		Port:                port,
		InitialDelay:        delay,
		Timeout:             timeout,
		MaxConnections:      1,
		ReconnectBackoff:    time.Second,
		HealthCheckInterval: 30 * time.Second,
		QueueSize:           32,
	}

	return NewWithConfig(config, messageReader)
}

// NewWithConfig creates a multiplexer with detailed configuration
func NewWithConfig(config Config, messageReader message.Reader) Multiplexer {
	if config.MaxConnections < 1 {
		config.MaxConnections = 1
	}

	return Multiplexer{
		config:        config,
		messageReader: messageReader,
		quit:          make(chan struct{}),
		metrics:       &metrics{},
	}
}

func (mux *Multiplexer) deadline() time.Time {
	return time.Now().Add(mux.config.Timeout)
}

func (mux *Multiplexer) Start() error {
	var err error
	mux.l, err = net.Listen("tcp", ":"+mux.config.Port)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	mux.wg = &wg

	// Initialize request queue
	requestQueue := make(chan *reqContainer, mux.config.QueueSize)
	mux.requestQueue = requestQueue

	// Initialize connection pool
	mux.connections = make([]*connectionState, mux.config.MaxConnections)
	for i := 0; i < mux.config.MaxConnections; i++ {
		mux.connections[i] = &connectionState{
			active: false,
		}
	}

	// Start connection management and health check routines
	go mux.startConnectionPool()
	go mux.healthCheckLoop()
	go mux.logMetricsLoop()

	// Start target connection processing loop
	go mux.targetConnRoutine()

	count := 0
L:
	for {
		conn, err := mux.l.Accept()
		if err != nil {
			select {
			case <-mux.quit:
				slog.Info("no more connections will be accepted")
				return nil
			default:
				slog.Error(err.Error())
				goto L
			}
		}
		count++
		slog.Info(fmt.Sprintf("#%d: %v <-> %v", count, conn.RemoteAddr(), conn.LocalAddr()))

		wg.Add(1)
		go func() {
			mux.handleConnection(conn, requestQueue)
			wg.Done()
		}()
	}
}

// startConnectionPool initializes the connection pool
func (mux *Multiplexer) startConnectionPool() {
	for i := 0; i < mux.config.MaxConnections; i++ {
		mux.createTargetConn(i)
	}
}

// healthCheckLoop periodically checks all connections for health
func (mux *Multiplexer) healthCheckLoop() {
	ticker := time.NewTicker(mux.config.HealthCheckInterval)
	defer ticker.Stop()

	for {
		select {
		case <-mux.quit:
			return
		case <-ticker.C:
			mux.performHealthCheck()
		}
	}
}

// logMetricsLoop periodically logs performance metrics
func (mux *Multiplexer) logMetricsLoop() {
	ticker := time.NewTicker(60 * time.Second)
	defer ticker.Stop()

	for {
		select {
		case <-mux.quit:
			return
		case <-ticker.C:
			activeConns := mux.metrics.activeConnections.Load()
			totalReqs := mux.metrics.totalRequests.Load()
			failedReqs := mux.metrics.failedRequests.Load()
			reconnects := mux.metrics.reconnectionAttempts.Load()
			
			avgTime := float64(0)
			reqCount := mux.metrics.requestCount.Load()
			if reqCount > 0 {
				avgTime = float64(mux.metrics.processingTime.Load()) / float64(reqCount) / float64(time.Millisecond)
			}
			
			slog.Info(fmt.Sprintf("Metrics - Active connections: %d, Total requests: %d, Failed: %d, Reconnects: %d, Avg processing time: %.2fms", 
				activeConns, totalReqs, failedReqs, reconnects, avgTime))
		}
	}
}

// performHealthCheck checks all connections and recreates failed ones
func (mux *Multiplexer) performHealthCheck() {
	mux.connMutex.Lock()
	defer mux.connMutex.Unlock()

	for i, connState := range mux.connections {
		if connState == nil || !connState.active {
			continue
		}

		// If connection is idle for too long, test it with a ping
		if time.Since(connState.lastUsed) > mux.config.HealthCheckInterval*2 {
			// Only check if not currently processing a request
			if !connState.processing.Load() {
				// Simple TCP check - set a short deadline and check if writable
				err := connState.conn.SetWriteDeadline(time.Now().Add(500 * time.Millisecond))
				if err != nil {
					slog.Warn(fmt.Sprintf("Health check failed for connection %d: %v", i, err))
					mux.closeConnection(i)
					// Recreate connection asynchronously to not block health check
					go mux.createTargetConn(i)
				}
			}
		}
	}
}

// handleConnection handles a client connection
func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer) {
	defer func(c net.Conn) {
		slog.Debug(fmt.Sprintf("Closing client connection: %v", c.RemoteAddr()))
		err := c.Close()
		sender <- &reqContainer{typ: Disconnection}
		if err != nil {
			slog.Error(err.Error())
		}
	}(conn)

	sender <- &reqContainer{typ: Connection}
	callback := make(chan *respContainer, 1) // Buffer to prevent deadlocks
	defer close(callback)                    // Ensure channel is closed

	for {
		err := conn.SetReadDeadline(mux.deadline())
		if err != nil {
			slog.Error(fmt.Sprintf("error setting read deadline: %v", err))
		}
		
		msg, err := mux.messageReader.ReadMessage(conn)
		if err == io.EOF {
			slog.Info(fmt.Sprintf("closed: %v <-> %v", conn.RemoteAddr(), conn.LocalAddr()))
			break
		}
		if err != nil {
			slog.Error(err.Error())
			break // Intentionally breaks the outer for loop to stop processing on error
		}

		slog.Debug(fmt.Sprintf("Message from client...\n%x\n", msg))
		mux.metrics.totalRequests.Add(1)

		// Record start time for metrics
		startTime := time.Now()

		// enqueue request msg to target conn loop
		sender <- &reqContainer{
			typ:     Packet,
			message: msg,
			sender:  callback,
		}

		// get response from target conn loop
		resp := <-callback
		
		// Update metrics
		processingTime := time.Since(startTime)
		mux.metrics.processingTime.Add(int64(processingTime))
		mux.metrics.requestCount.Add(1)
		
		if resp.err != nil {
			mux.metrics.failedRequests.Add(1)
			slog.Error(fmt.Sprintf("failed to forward message, %v", resp.err))
			break
		}

		// write back
		err = conn.SetWriteDeadline(mux.deadline())
		if err != nil {
			slog.Error(fmt.Sprintf("error setting write deadline: %v", err))
		}
		_, err = conn.Write(resp.message)
		if err != nil {
			slog.Error(err.Error())
			break // Intentionally breaks the outer for loop to stop processing on write error
		}
	}
}

// findAvailableConnection finds an available connection or returns -1
func (mux *Multiplexer) findAvailableConnection() int {
	mux.connMutex.RLock()
	defer mux.connMutex.RUnlock()
	
	for i, connState := range mux.connections {
		if connState != nil && connState.active && !connState.processing.Load() {
			return i
		}
	}
	
	// If all connections are busy but we have inactive ones, return first inactive
	for i, connState := range mux.connections {
		if connState != nil && !connState.active {
			return i
		}
	}
	
	return -1
}

// markConnectionBusy marks a connection as busy (processing)
func (mux *Multiplexer) markConnectionBusy(index int) bool {
	mux.connMutex.RLock()
	defer mux.connMutex.RUnlock()
	
	if index < 0 || index >= len(mux.connections) || 
	   mux.connections[index] == nil || 
	   !mux.connections[index].active {
		return false
	}
	
	// Try to atomically switch from not-processing to processing
	return mux.connections[index].processing.CompareAndSwap(false, true)
}

// markConnectionFree marks a connection as available
func (mux *Multiplexer) markConnectionFree(index int) {
	mux.connMutex.RLock()
	defer mux.connMutex.RUnlock()
	
	if index < 0 || index >= len(mux.connections) || mux.connections[index] == nil {
		return
	}
	
	mux.connections[index].processing.Store(false)
	mux.connections[index].lastUsed = time.Now()
}

// closeConnection closes a specific connection
func (mux *Multiplexer) closeConnection(index int) {
	if index < 0 || index >= len(mux.connections) || 
	   mux.connections[index] == nil || 
	   !mux.connections[index].active {
		return
	}
	
	conn := mux.connections[index].conn
	if conn != nil {
		err := conn.Close()
		if err != nil {
			slog.Error(fmt.Sprintf("error closing connection %d: %v", index, err))
		}
	}
	
	mux.connections[index].active = false
	mux.connections[index].conn = nil
	mux.metrics.activeConnections.Add(-1)
}

// createTargetConn creates a new target connection with exponential backoff
func (mux *Multiplexer) createTargetConn(index int) bool {
	if index < 0 || index >= mux.config.MaxConnections {
		slog.Error(fmt.Sprintf("invalid connection index: %d", index))
		return false
	}

	// Get fail count from existing connection state
	failCount := 0
	if mux.connections[index] != nil {
		failCount = mux.connections[index].failCount
	}

	// Calculate backoff with exponential increase and jitter
	backoff := mux.config.ReconnectBackoff
	if failCount > 0 {
		// Exponential backoff with a cap
		maxBackoff := 30 * time.Second
		backoff = time.Duration(math.Min(
			float64(mux.config.ReconnectBackoff) * math.Pow(1.5, float64(failCount)),
			float64(maxBackoff),
		))
	}

	slog.Info(fmt.Sprintf("creating target connection %d (attempt #%d, backoff: %v)", 
		index, failCount+1, backoff))
	
	mux.metrics.reconnectionAttempts.Add(1)

	// Wait before attempting to connect
	if failCount > 0 {
		time.Sleep(backoff)
	}

	// Try to establish connection
	conn, err := net.DialTimeout("tcp", mux.config.TargetServer, 30*time.Second)
	if err != nil {
		failCount++
		
		// Update connection state for next attempt
		mux.connMutex.Lock()
		if mux.connections[index] == nil {
			mux.connections[index] = &connectionState{}
		}
		mux.connections[index].failCount = failCount
		mux.connections[index].active = false
		mux.connMutex.Unlock()
		
		slog.Error(fmt.Sprintf("failed to connect to target server %s (attempt #%d): %v", 
			mux.config.TargetServer, failCount, err))
		
		// Schedule next attempt if not shutting down
		select {
		case <-mux.quit:
			return false
		default:
			go mux.createTargetConn(index)
			return false
		}
	}

	// Connection successful
	slog.Info(fmt.Sprintf("new target connection %d: %v <-> %v", 
		index, conn.LocalAddr(), conn.RemoteAddr()))

	if mux.config.InitialDelay > 0 {
		slog.Info(fmt.Sprintf("waiting %s, before using new target connection %d", 
			mux.config.InitialDelay, index))
		time.Sleep(mux.config.InitialDelay)
	}

	// Update connection state
	mux.connMutex.Lock()
	defer mux.connMutex.Unlock()
	
	// Check if multiplexer is shutting down
	select {
	case <-mux.quit:
		if err := conn.Close(); err != nil {
			slog.Error(fmt.Sprintf("error closing connection in createTargetConn: %v", err))
		}
		return false
	default:
		// Update connection state
		mux.connections[index] = &connectionState{
			conn:      conn,
			active:    true,
			lastUsed:  time.Now(),
			failCount: 0,
		}
		mux.metrics.activeConnections.Add(1)
		return true
	}
}

// targetConnRoutine handles all requests to target servers
func (mux *Multiplexer) targetConnRoutine() {
	clients := int32(0)

	for container := range mux.requestQueue {
		switch container.typ {
		case Connection:
			newCount := atomic.AddInt32(&clients, 1)
			slog.Info(fmt.Sprintf("Connected clients: %d", newCount))
			continue
		case Disconnection:
			newCount := atomic.AddInt32(&clients, -1)
			slog.Info(fmt.Sprintf("Connected clients: %d", newCount))
			continue
		case Packet:
			// Process packet
			// Ineffektives break entfernt
		}

		// Find an available connection
		connIndex := mux.findAvailableConnection()
		
		// If no connection available, create one or wait
		if connIndex == -1 {
			slog.Debug("No connection available, waiting...")
			// Wait and retry a few times before failing
			for retry := 0; retry < 3; retry++ {
				time.Sleep(100 * time.Millisecond)
				connIndex = mux.findAvailableConnection()
				if connIndex != -1 {
					break
				}
			}
			
			// If still no connection, return error
			if connIndex == -1 {
				container.sender <- &respContainer{
					err: fmt.Errorf("no target connections available"),
				}
				continue
			}
		}
		
		// Mark connection as busy
		if !mux.markConnectionBusy(connIndex) {
			slog.Debug(fmt.Sprintf("Failed to acquire connection %d, retrying", connIndex))
			// Try again with another connection
			container.sender <- &respContainer{
				err: fmt.Errorf("failed to acquire target connection"),
			}
			continue
		}
		
		// Process the request
		var err error
		var msg []byte
		
		func() {
			// Always mark connection as free when done
			defer mux.markConnectionFree(connIndex)
			
			// Get the connection
			mux.connMutex.RLock()
			conn := mux.connections[connIndex].conn
			mux.connMutex.RUnlock()
			
			if conn == nil {
				err = fmt.Errorf("connection is nil")
				return
			}
			
			// Write the request
			err = conn.SetWriteDeadline(mux.deadline())
			if err != nil {
				slog.Error(fmt.Sprintf("error setting write deadline: %v", err))
				return
			}
			
			_, err = conn.Write(container.message)
			if err != nil {
				slog.Error(fmt.Sprintf("error writing to target connection %d: %v", connIndex, err))
				// Mark connection for renewal
				go func() {
					mux.connMutex.Lock()
					defer mux.connMutex.Unlock()
					mux.closeConnection(connIndex)
					go mux.createTargetConn(connIndex)
				}()
				return
			}
			
			// Read the response
			err = conn.SetReadDeadline(mux.deadline())
			if err != nil {
				slog.Error(fmt.Sprintf("error setting read deadline: %v", err))
				return
			}
			
			msg, err = mux.messageReader.ReadMessage(conn)
			if err != nil {
				slog.Error(fmt.Sprintf("error reading from target connection %d: %v", connIndex, err))
				// Mark connection for renewal
				go func() {
					mux.connMutex.Lock()
					defer mux.connMutex.Unlock()
					mux.closeConnection(connIndex)
					go mux.createTargetConn(connIndex)
				}()
				return
			}
			
			slog.Debug(fmt.Sprintf("Message from target server %d...\n%x\n", connIndex, msg))
		}()
		
		// Send the response or error
		container.sender <- &respContainer{
			message: msg,
			err:     err,
		}
	}

	slog.Info("target connection routine stopped gracefully")
}

// Close gracefully shuts down the multiplexer
func (mux *Multiplexer) Close() error {
	// Signal shutdown
	close(mux.quit)
	slog.Info("closing server...")
	
	// Close listener to stop accepting new connections
	err := mux.l.Close()
	if err != nil {
		return err
	}

	// Wait for all client connections to close
	slog.Debug("wait all incoming connections closed")
	mux.wg.Wait()
	slog.Info("incoming connections closed")

	// Close all target connections
	mux.connMutex.Lock()
	for i, connState := range mux.connections {
		if connState != nil && connState.active && connState.conn != nil {
			err := connState.conn.Close()
			if err != nil {
				slog.Error(fmt.Sprintf("error closing target connection %d: %v", i, err))
			}
		}
	}
	mux.connMutex.Unlock()

	// Stop request processing
	close(mux.requestQueue)

	slog.Info("multiplexer server stopped gracefully")
	return nil
}
