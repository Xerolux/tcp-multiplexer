package message

import "io"

// Reader defines an interface for reading messages for specific application protocols
type Reader interface {
	ReadMessage(conn io.Reader) ([]byte, error) // Reads a message from the connection
	Name() string                               // Returns the name of the protocol
}

// Readers is a map of available protocol readers, keyed by protocol name
var Readers map[string]Reader

// init initializes the Readers map and registers all available protocol readers
func init() {
	Readers = make(map[string]Reader)
	registerReaders(
		&EchoMessageReader{},
		&HTTPMessageReader{},
		&ISO8583MessageReader{},
		&MPUMessageReader{},
		&ModbusMessageReader{},
	)
}

// registerReaders adds protocol readers to the Readers map
func registerReaders(readers ...Reader) {
	for _, msgReader := range readers {
		Readers[msgReader.Name()] = msgReader
	}
}
