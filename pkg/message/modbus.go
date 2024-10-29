package message

import (
	"encoding/binary"
	"errors"
	"fmt"
	"io"
)

const (
	maxTCPFrameLength = 260 // Maximum allowed frame length for Modbus TCP
	mbapHeaderLength  = 6    // Length of the MBAP header for Modbus TCP
)

// ModbusMessageReader reads Modbus messages from a TCP connection
type ModbusMessageReader struct{}

// Name returns the name of the protocol
func (m ModbusMessageReader) Name() string {
	return "modbus"
}

// ReadMessage reads a single Modbus message from the given connection
func (m ModbusMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// Step 1: Read the MBAP header
	header := make([]byte, mbapHeaderLength)
	if _, err := io.ReadFull(conn, header); err != nil {
		return nil, fmt.Errorf("failed to read MBAP header: %w", err)
	}

	// Step 2: Validate MBAP header and determine the number of additional bytes to read
	bytesNeeded, err := validateMBAPHeader(header)
	if err != nil {
		return nil, err
	}

	// Step 3: Ensure the total frame length does not exceed the maximum allowed length
	if bytesNeeded+mbapHeaderLength > maxTCPFrameLength {
		return nil, fmt.Errorf("protocol error: frame length %d exceeds max allowed (%d)", bytesNeeded+mbapHeaderLength, maxTCPFrameLength)
	}

	// Step 4: Read the PDU (Protocol Data Unit) based on the determined length
	rxbuf := make([]byte, bytesNeeded)
	if _, err = io.ReadFull(conn, rxbuf); err != nil {
		return nil, fmt.Errorf("failed to read PDU: %w", err)
	}

	// Return the full Modbus message (MBAP header + PDU)
	return append(header, rxbuf...), nil
}

// validateMBAPHeader checks the MBAP header and returns the PDU length if valid
func validateMBAPHeader(header []byte) (int, error) {
	if len(header) != mbapHeaderLength {
		return 0, errors.New("invalid MBAP header length")
	}

	// Extract the length field from the MBAP header
	bytesNeeded := int(binary.BigEndian.Uint16(header[4:6]))

	// Check for illegal MBAP length
	if bytesNeeded <= 0 {
		return 0, fmt.Errorf("protocol error: illegal MBAP length (%d)", bytesNeeded)
	}

	return bytesNeeded, nil
}
