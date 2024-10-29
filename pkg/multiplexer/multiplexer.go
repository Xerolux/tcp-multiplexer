package multiplexer

import (
	"github.com/davecgh/go-spew/spew"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/sirupsen/logrus"
	"io"
	"net"
	"sync"
	"time"
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

	Multiplexer struct {
		targetServer  string
		port          string
		messageReader message.Reader
		l             net.Listener
		quit          chan struct{}
		wg            *sync.WaitGroup
		requestQueue  chan *reqContainer
	}
)

const (
	Connection messageType = iota
	Disconnection
	Packet
)

// Sets the deadline for read/write operations
func deadline() time.Time {
	return time.Now().Add(60 * time.Second)
}

// New initializes a new instance of the Multiplexer
func New(targetServer, port string, messageReader message.Reader) Multiplexer {
	return Multiplexer{
		targetServer:  targetServer,
		port:          port,
		messageReader: messageReader,
		quit:          make(chan struct{}),
	}
}

// Start begins accepting client connections and handles multiplexing
func (mux *Multiplexer) Start() error {
	var err error
	mux.l, err = net.Listen("tcp", ":"+mux.port)
	if err != nil {
		return err
	}

	var wg sync.WaitGroup
	mux.wg = &wg

	requestQueue := make(chan *reqContainer, 32)
	mux.requestQueue = requestQueue

	// Start the target connection loop in a goroutine
	go func() {
		mux.targetConnLoop(requestQueue)
	}()

	count := 0
	for {
		conn, err := mux.l.Accept()
		if err != nil {
			select {
			case <-mux.quit:
				logrus.Info("no more connections will be accepted")
				return nil
			default:
				logrus.Error(err)
				continue
			}
		}
		count++
		logrus.Infof("Accepted connection #%d: %v <-> %v", count, conn.RemoteAddr(), conn.LocalAddr())

		wg.Add(1)
		go func() {
			mux.handleConnection(conn, requestQueue)
			wg.Done()
		}()
	}
}

// handleConnection handles a single client connection and forwards messages to the target server
func (mux *Multiplexer) handleConnection(conn net.Conn, sender chan<- *reqContainer) {
	defer func(c net.Conn) {
		logrus.Debugf("Closing client connection: %v", c.RemoteAddr())
		err := c.Close()
		sender <- &reqContainer{typ: Disconnection}
		if err != nil {
			logrus.Error(err)
		}
	}(conn)

	sender <- &reqContainer{typ: Connection}
	callback := make(chan *respContainer)

	for {
		err := conn.SetReadDeadline(deadline())
		if err != nil {
			logrus.Errorf("error setting read deadline: %v", err)
		}

		// Read message from the client
		msg, err := mux.messageReader.ReadMessage(conn)
		if err == io.EOF {
			logrus.Infof("Connection closed by client: %v <-> %v", conn.RemoteAddr(), conn.LocalAddr())
			break
		}
		if err != nil {
			logrus.Errorf("error reading message from client: %v", err)
			break
		}

		logrus.Debug("Message from client...\n%s", spew.Sdump(msg))

		// Enqueue message to the target connection loop
		sender <- &reqContainer{
			typ:     Packet,
			message: msg,
			sender:  callback,
		}

		// Wait for response from the target connection loop
		resp := <-callback
		if resp.err != nil {
			logrus.Errorf("failed to forward message, %v", resp.err)
			break
		}

		// Write response back to client
		err = conn.SetWriteDeadline(deadline())
		if err != nil {
			logrus.Errorf("error setting write deadline: %v", err)
		}
		_, err = conn.Write(resp.message)
		if err != nil {
			logrus.Errorf("error writing response to client: %v", err)
			break
		}
	}
}

// createTargetConn establishes a connection to the target server with retry logic
func (mux *Multiplexer) createTargetConn() net.Conn {
	for {
		logrus.Info("creating target connection")
		conn, err := net.DialTimeout("tcp", mux.targetServer, 30*time.Second)
		if err != nil {
			logrus.Errorf("failed to connect to target server %s, %v", mux.targetServer, err)
			time.Sleep(1 * time.Second) // Retry after a delay
			continue
		}
		logrus.Infof("Connected to target server: %v <-> %v", conn.LocalAddr(), conn.RemoteAddr())
		return conn
	}
}

// targetConnLoop manages the connection with the target server and processes the request queue
func (mux *Multiplexer) targetConnLoop(requestQueue <-chan *reqContainer) {
	var conn net.Conn
	clients := 0

	for container := range requestQueue {
		switch container.typ {
		case Connection:
			clients++
			logrus.Infof("Number of connected clients: %d", clients)
		case Disconnection:
			clients--
			logrus.Infof("Number of connected clients: %d", clients)
			if clients == 0 && conn != nil {
				logrus.Info("Closing target connection due to no clients")
				if err := conn.Close(); err != nil {
					logrus.Error(err)
				}
				conn = nil
			}
		case Packet:
			if conn == nil {
				conn = mux.createTargetConn()
			}

			err := conn.SetWriteDeadline(deadline())
			if err != nil {
				logrus.Errorf("error setting write deadline: %v", err)
			}

			_, err = conn.Write(container.message)
			if err != nil {
				container.sender <- &respContainer{err: err}
				logrus.Errorf("target connection write error: %v", err)

				if err := conn.Close(); err != nil {
					logrus.Error("error while closing target connection: %v", err)
				}
				conn = nil
				continue
			}

			err = conn.SetReadDeadline(deadline())
			if err != nil {
				logrus.Errorf("error setting read deadline: %v", err)
			}

			msg, err := mux.messageReader.ReadMessage(conn)
			container.sender <- &respContainer{message: msg, err: err}

			logrus.Debug("Message from target server...\n%s", spew.Sdump(msg))

			if err != nil {
				logrus.Errorf("error reading from target connection: %v", err)

				if err := conn.Close(); err != nil {
					logrus.Error("error while closing target connection: %v", err)
				}
				conn = nil
			}
		}
	}

	logrus.Info("target connection loop stopped gracefully")
}

// Close shuts down the multiplexer server and waits for active connections to close
func (mux *Multiplexer) Close() error {
	close(mux.quit)
	logrus.Info("closing multiplexer server...")
	err := mux.l.Close()
	if err != nil {
		return err
	}

	logrus.Debug("waiting for all connections to close")
	mux.wg.Wait()
	logrus.Info("all client connections closed")

	close(mux.requestQueue)

	logrus.Info("multiplexer server stopped gracefully")
	return nil
}
