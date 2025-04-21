package multiplexer

import (
	"fmt"
	"io"
	"log/slog"
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
		index      int
		bytesSent  atomic.Int64
		bytesRecv  atomic.Int64
	}

	Multiplexer struct {
		config        Config
		messageReader message.Reader
		l             net.Listener
		quit          chan struct{}
		wg            *sync.WaitGroup
		requestQueue  chan *reqContainer
		connPool      *connPool
		metrics       *metrics
	}

	// metrics tracks multiplexer statistics
	metrics struct {
		activeConnections    atomic.Int32
		totalRequests        atomic.Int64
		failedRequests       atomic.Int64
		reconnectionAttempts atomic.Int64
		processingTime       atomic.Int64
		requestCount         atomic.Int64
		activeClients        atomic.Int32
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

	quit := make(chan struct{})
	metrics := &metrics{}

	return Multiplexer{
		config:        config,
		messageReader: messageReader,
		quit:          quit,
		metrics:       metrics,
		connPool:      newConnPool(config, metrics, quit),
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

	// Start connection pool
	mux.connPool.start()

	// Start metrics logging
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

		// Optimize TCP connection settings for clients
		if tcpConn, ok := conn.(*net.TCPConn); ok {
			_ = tcpConn.SetKeepAlive(true)
			_ = tcpConn.SetKeepAlivePeriod(30 * time.Second)
			_ = tcpConn.SetNoDelay(true) // Disable Nagle's algorithm
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
			activeClients := mux.metrics.activeClients.Load()

			avgTime := float64(0)
			reqCount := mux.metrics.requestCount.Load()
			if reqCount > 0 {
				avgTime = float64(mux.metrics.processingTime.Load()) / float64(reqCount) / float64(time.Millisecond)
			}

			slog.Info(fmt.Sprintf("Metrics - Active connections: %d, Active clients: %d, Total requests: %d, Failed: %d, Reconnects: %d, Avg processing time: %.2fms",
				activeConns, activeClients, totalReqs, failedReqs, reconnects, avgTime))
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

// targetConnRoutine handles all requests to target servers
func (mux *Multiplexer) targetConnRoutine() {
	for container := range mux.requestQueue {
		switch container.typ {
		case Connection:
			newCount := mux.metrics.activeClients.Add(1)
			slog.Info(fmt.Sprintf("Connected clients: %d", newCount))
			continue
		case Disconnection:
			newCount := mux.metrics.activeClients.Add(-1)
			slog.Info(fmt.Sprintf("Connected clients: %d", newCount))
			continue
		case Packet:
			// Process packet - handled below
		}

		// Find an available connection using the connection pool
		connIndex, err := mux.connPool.getConnection()

		// If no connection available, return error
		if err != nil {
			container.sender <- &respContainer{
				err: fmt.Errorf("no target connections available: %w", err),
			}
			continue
		}

		// Process the request
		var respMsg []byte

		func() {
			// Always mark connection as free when done
			defer mux.connPool.connections[connIndex].processing.Store(false)

			// Get the connection
			mux.connPool.connMutex.RLock()
			conn := mux.connPool.connections[connIndex].conn
			mux.connPool.connMutex.RUnlock()

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

			n, writeErr := conn.Write(container.message)
			if writeErr != nil {
				err = writeErr
				slog.Error(fmt.Sprintf("error writing to target connection %d: %v", connIndex, err))
				// Mark connection for renewal
				go func() {
					mux.connPool.closeConnection(connIndex)
					go mux.connPool.createConnection(connIndex)
				}()
				return
			}

			// Track bytes sent
			mux.connPool.connections[connIndex].bytesSent.Add(int64(n))

			// Read the response
			err = conn.SetReadDeadline(mux.deadline())
			if err != nil {
				slog.Error(fmt.Sprintf("error setting read deadline: %v", err))
				return
			}

			respMsg, err = mux.messageReader.ReadMessage(conn)
			if err != nil {
				slog.Error(fmt.Sprintf("error reading from target connection %d: %v", connIndex, err))
				// Mark connection for renewal
				go func() {
					mux.connPool.closeConnection(connIndex)
					go mux.connPool.createConnection(connIndex)
				}()
				return
			}

			// Track bytes received
			mux.connPool.connections[connIndex].bytesRecv.Add(int64(len(respMsg)))

			// Update last used time
			mux.connPool.connections[connIndex].lastUsed = time.Now()

			slog.Debug(fmt.Sprintf("Message from target server %d...\n%x\n", connIndex, respMsg))
		}()

		// Send the response or error
		container.sender <- &respContainer{
			message: respMsg,
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
	mux.connPool.close()

	// Stop request processing
	close(mux.requestQueue)

	slog.Info("multiplexer server stopped gracefully")
	return nil
}
