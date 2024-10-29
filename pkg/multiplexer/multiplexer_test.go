package multiplexer

import (
	"bufio"
	"fmt"
	"github.com/ingmarstein/tcp-multiplexer/pkg/message"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"io"
	"net"
	"os"
	"sync"
	"testing"
	"time"
)

func init() {
	logrus.SetLevel(logrus.InfoLevel)
}

// handleErr prints the error and exits if the error is non-nil
func handleErr(err error) {
	if err != nil {
		fmt.Println(err)
		os.Exit(2)
	}
}

// client simulates a TCP client that sends messages and verifies echo responses
func client(t *testing.T, server string, clientIndex int) {
	conn, err := net.Dial("tcp", server)
	handleErr(err)
	defer conn.Close()

	for i := 0; i < 10; i++ {
		// Send an echo message
		echo := []byte(fmt.Sprintf("client %d counter %d\n", clientIndex, i))
		_, err = conn.Write(echo)
		handleErr(err)

		// Read echo response
		echoReply, err := message.EchoMessageReader{}.ReadMessage(conn)
		handleErr(err)

		// Assert that the response matches the sent message
		assert.Equal(t, echo, echoReply)
	}

	fmt.Printf("Client %d: connection closed\n", clientIndex)
}

// handleConnection handles incoming connections by echoing received messages
func handleConnection(conn net.Conn) {
	defer func(c net.Conn) {
		err := c.Close()
		if err != nil {
			fmt.Println(err)
		}
	}(conn)

	reader := bufio.NewReader(conn)
	for {
		// Read message until newline
		data, err := reader.ReadBytes('\n')
		if err == io.EOF {
			fmt.Println("Connection closed by client")
			break
		}
		if err != nil {
			fmt.Println("Error reading from connection:", err)
			break
		}

		// Echo the received message back to the client
		_, err = conn.Write(data)
		if err != nil {
			fmt.Println("Error writing to connection:", err)
			break
		}
	}
}

// TestMultiplexer_Start tests the multiplexer server with multiple concurrent clients
func TestMultiplexer_Start(t *testing.T) {
	// Create a listener to simulate the target server
	listener, err := net.Listen("tcp", ":0")
	handleErr(err)
	defer listener.Close()

	// Handle incoming connections to the simulated target server
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				fmt.Println("Error accepting connection:", err)
				break
			}
			go handleConnection(conn)
		}
	}()

	// Start the multiplexer
	muxServerPort := "1235"
	muxServer := "127.0.0.1:" + muxServerPort
	mux := New(listener.Addr().String(), muxServerPort, message.EchoMessageReader{})

	go func() {
		err := mux.Start()
		assert.NoError(t, err)
	}()

	// Allow server setup time
	time.Sleep(time.Second)

	// Start multiple clients and test echo functionality
	var wg sync.WaitGroup
	clientCount := 2
	wg.Add(clientCount)
	for i := 1; i <= clientCount; i++ {
		go func(clientIndex int) {
			client(t, muxServer, clientIndex)
			wg.Done()
		}(i)
	}

	// Wait for all clients to finish
	wg.Wait()

	// Allow cleanup time before shutting down the multiplexer
	time.Sleep(time.Second)

	// Close the multiplexer server
	err = mux.Close()
	assert.NoError(t, err)
}
