package message

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"io"
)

// ISO8583MessageReader reads messages in ISO 8583 format.
// It expects a 2-byte header indicating the length of the ISO 8583 message.
type ISO8583MessageReader struct{}

// Name returns the protocol name associated with this reader.
func (I ISO8583MessageReader) Name() string {
	return "iso8583"
}

// ReadMessage reads an ISO 8583 message from the connection.
// The message is expected to include a 2-byte header indicating the message length.
func (I ISO8583MessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// Step 1: Read the 2-byte header to determine the message length
	header := make([]byte, 2)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("failed to read header: %w", err)
	}

	// Step 2: Convert header to uint16 to get the length of the ISO 8583 message
	var length uint16
	if err := binary.Read(bytes.NewReader(header), binary.BigEndian, &length); err != nil {
		return nil, fmt.Errorf("failed to parse message length: %w", err)
	}

	// Step 3: Read the ISO 8583 message based on the length specified in the header
	isoMsg := make([]byte, length)
	if _, err := io.ReadFull(conn, isoMsg); err != nil {
		return nil, fmt.Errorf("failed to read ISO 8583 message: %w", err)
	}

	// Return the complete message (header + ISO 8583 message)
	return append(header, isoMsg...), nil
}
