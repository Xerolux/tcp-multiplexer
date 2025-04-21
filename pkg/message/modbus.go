package message

import (
	"encoding/binary"
	"fmt"
	"io"
	"sync"
)

const (
	maxTCPFrameLength = 260 // Maximum Modbus TCP frame length
	mbapHeaderLength  = 6   // Length of Modbus TCP header
	maxPDULength      = 253 // Maximum Modbus PDU length
)

// Buffer pool for Modbus message handling - Store slice pointers to avoid allocations
var modbusBufferPool = sync.Pool{
	New: func() interface{} {
		// Pre-allocate maximum possible Modbus TCP frame size
		buf := make([]byte, maxTCPFrameLength)
		return &buf // Return pointer to avoid allocations
	},
}

type ModbusMessageReader struct{}

func (m ModbusMessageReader) Name() string {
	return "modbus"
}

func (m ModbusMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// Get buffer from pool (as pointer to avoid allocations)
	bufPtr := modbusBufferPool.Get().(*[]byte)
	defer modbusBufferPool.Put(bufPtr) // Return pointer to pool

	buffer := *bufPtr // Dereference for easier use

	// Read the MBAP header (first 6 bytes)
	if _, err := io.ReadFull(conn, buffer[:mbapHeaderLength]); err != nil {
		return nil, fmt.Errorf("error reading Modbus header: %w", err)
	}

	// Extract length field (bytes 4-5)
	pduLength := int(binary.BigEndian.Uint16(buffer[4:6]))

	// Validate length
	if pduLength <= 0 {
		return nil, fmt.Errorf("invalid Modbus PDU length: %d", pduLength)
	}

	if pduLength > maxPDULength {
		return nil, fmt.Errorf("PDU length too large: %d (max: %d)", pduLength, maxPDULength)
	}

	// Calculate full message length
	messageLength := mbapHeaderLength + pduLength

	// Read the PDU
	if _, err := io.ReadFull(conn, buffer[mbapHeaderLength:messageLength]); err != nil {
		return nil, fmt.Errorf("error reading Modbus PDU: %w", err)
	}

	// Create a copy of the message to return (since we'll reuse the buffer)
	result := make([]byte, messageLength)
	copy(result, buffer[:messageLength])

	return result, nil
}

// Helper functions for common Modbus operations

// ParseModbusFunction extracts the function code from a Modbus message
func ParseModbusFunction(msg []byte) (byte, error) {
	if len(msg) < mbapHeaderLength+1 {
		return 0, fmt.Errorf("message too short to contain function code")
	}
	return msg[mbapHeaderLength], nil
}

// IsModbusException checks if a Modbus response is an exception
func IsModbusException(msg []byte) bool {
	if len(msg) < mbapHeaderLength+2 {
		return false
	}

	// Exception responses have high bit set in function code
	functionCode := msg[mbapHeaderLength]
	return (functionCode & 0x80) != 0
}

// GetModbusExceptionCode extracts exception code if present
func GetModbusExceptionCode(msg []byte) (byte, error) {
	if !IsModbusException(msg) {
		return 0, fmt.Errorf("not an exception response")
	}

	if len(msg) < mbapHeaderLength+2 {
		return 0, fmt.Errorf("message too short to contain exception code")
	}

	return msg[mbapHeaderLength+1], nil
}

// CreateModbusRequest creates a properly formatted Modbus request
func CreateModbusRequest(transactionID uint16, unitID byte, functionCode byte, data []byte) []byte {
	// Calculate PDU length (function code + data length + unit ID)
	pduLength := 1 + len(data) + 1

	// Create message buffer
	msg := make([]byte, mbapHeaderLength+pduLength)

	// Set transaction ID (bytes 0-1)
	binary.BigEndian.PutUint16(msg[0:2], transactionID)

	// Set protocol ID (bytes 2-3) - always 0 for Modbus TCP
	binary.BigEndian.PutUint16(msg[2:4], 0)

	// Set length field (bytes 4-5)
	binary.BigEndian.PutUint16(msg[4:6], uint16(pduLength))

	// Set unit ID (byte 6)
	msg[6] = unitID

	// Set function code (byte 7)
	msg[7] = functionCode

	// Copy data (starting at byte 8)
	if len(data) > 0 {
		copy(msg[8:], data)
	}

	return msg
}
