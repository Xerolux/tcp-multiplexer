package message

import (
	"bufio"
	"io"
)

// EchoMessageReader is a reader for the Echo protocol as defined in RFC 862.
// It reads messages that are newline ('\n') terminated.
type EchoMessageReader struct{}

// Name returns the name of the protocol for this reader.
func (e EchoMessageReader) Name() string {
	return "echo"
}

// ReadMessage reads a message from the connection until a newline ('\n') is encountered.
// It returns the message as a byte slice or an error if reading fails.
func (e EchoMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	reader := bufio.NewReader(conn)
	
	// Read bytes until a newline ('\n') character is encountered
	message, err := reader.ReadBytes('\n')
	if err != nil {
		return nil, err // Return error if reading fails
	}
	
	return message, nil // Return the message as a byte slice
}
