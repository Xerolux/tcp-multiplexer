package message

import (
	"fmt"
	"io"
	"strconv"
)

// MPUMessageReader reads messages in MPU Switch format (ISO 8583) with a 4-byte ASCII header
// specifying the length of the ISO 8583 message.
type MPUMessageReader struct{}

// Name returns the protocol name associated with this reader.
func (M MPUMessageReader) Name() string {
	return "mpu"
}

// ReadMessage reads a message from the connection in the MPU format.
// The message header is a 4-byte ASCII string representing the length of the message.
func (M MPUMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// Step 1: Read the 4-byte header to determine message length
	header := make([]byte, 4)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Convert header to a string and parse it as an integer to get the message length
	length, err := strconv.Atoi(string(header))
	if err != nil {
		return nil, fmt.Errorf("invalid message length in header: %w", err)
	}

	// Step 2: Read the ISO 8583 message based on the length specified in the header
	isoMsg := make([]byte, length)
	if _, err = io.ReadFull(conn, isoMsg); err != nil {
		return nil, fmt.Errorf("failed to read ISO 8583 message: %w", err)
	}

	// Return the full message (header + ISO 8583 message)
	return append(header, isoMsg...), nil
}
