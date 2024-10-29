package message

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"github.com/davecgh/go-spew/spew"
	"github.com/sirupsen/logrus"
	"io"
	"net/textproto"
	"strconv"
	"strings"
)

const (
	headerKeyContentLength = "Content-Length"
	headerKeyContentType   = "Content-Type"
	headerFormContentType  = "multipart/form-data"
	CRLF                   = "\r\n"
	boundaryPrefix         = "boundary="
)

// HTTPMessageReader reads HTTP messages in plain text format
// Does not support HTTPS or WebSocket protocols
type HTTPMessageReader struct{}

// Name returns the protocol name
func (H HTTPMessageReader) Name() string {
	return "http"
}

// ReadMessage reads and parses an HTTP message from the connection
func (H HTTPMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	tp := textproto.NewReader(bufio.NewReader(conn))

	// Read the start line (e.g., "GET / HTTP/1.1")
	startLine, err := tp.ReadLine()
	if err != nil {
		return nil, fmt.Errorf("failed to read start line: %w", err)
	}
	logrus.Debug("Start Line: ", startLine)

	// Read headers
	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, fmt.Errorf("failed to read headers: %w", err)
	}
	logrus.Debug("Headers: ", headers)

	// Initialize body buffer
	var body []byte

	// Check for multipart/form-data content type
	isFormContentType := false
	if vv, ok := headers[headerKeyContentType]; ok {
		contentType := strings.TrimSpace(strings.Split(vv[0], ";")[0])
		if contentType == headerFormContentType {
			isFormContentType = true
			body, err = H.readMultipartFormBody(tp, vv[0])
			if err != nil {
				return nil, err
			}
		}
	}

	// Handle single-resource bodies using Content-Length
	if !isFormContentType {
		if vv, ok := headers[headerKeyContentLength]; ok {
			contentLength, err := strconv.Atoi(vv[0])
			if err != nil {
				return nil, fmt.Errorf("invalid Content-Length: %w", err)
			}
			body, err = H.readBodyWithLength(tp.R, contentLength)
			if err != nil {
				return nil, err
			}
		}
	}

	// TODO: Handle Transfer-Encoding (e.g., chunked)
	// Refer to https://developer.mozilla.org/en-US/docs/Web/HTTP/Headers/Transfer-Encoding

	// Dump the entire HTTP message into a byte slice
	msg := dumpHTTPMessage(startLine, headers, body)

	// Log the complete message in debug mode
	if logrus.GetLevel() == logrus.DebugLevel {
		spew.Dump(msg)
	}

	return msg, nil
}

// readMultipartFormBody reads multipart/form-data content with boundary parsing
func (H HTTPMessageReader) readMultipartFormBody(tp *textproto.Reader, contentTypeHeader string) ([]byte, error) {
	// Parse the boundary from Content-Type header
	parts := strings.Split(contentTypeHeader, ";")
	if len(parts) < 2 {
		return nil, errors.New("missing boundary in Content-Type")
	}
	boundary := strings.TrimPrefix(strings.TrimSpace(parts[1]), boundaryPrefix)
	if boundary == "" {
		return nil, errors.New("boundary not specified in Content-Type")
	}

	// Prepare to scan for the boundary
	var body bytes.Buffer
	scanner := bufio.NewScanner(tp.R)
	lastBoundary := "--" + boundary + "--"

	for scanner.Scan() {
		line := scanner.Bytes()
		body.Write(line)
		body.WriteString(CRLF)
		if string(line) == lastBoundary {
			break
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading multipart body: %w", err)
	}

	return body.Bytes(), nil
}

// readBodyWithLength reads a fixed-length body based on Content-Length header
func (H HTTPMessageReader) readBodyWithLength(conn io.Reader, contentLength int) ([]byte, error) {
	body := make([]byte, contentLength)
	if _, err := io.ReadFull(conn, body); err != nil {
		return nil, fmt.Errorf("failed to read body with length %d: %w", contentLength, err)
	}
	return body, nil
}

// dumpHTTPMessage formats the HTTP message for output as a byte slice
func dumpHTTPMessage(startLine string, headers textproto.MIMEHeader, body []byte) []byte {
	var b bytes.Buffer
	b.WriteString(startLine)
	b.WriteString(CRLF)
	for key, values := range headers {
		for _, value := range values {
			b.WriteString(key)
			b.WriteString(": ")
			b.WriteString(value)
			b.WriteString(CRLF)
		}
	}
	b.WriteString(CRLF)
	if len(body) > 0 {
		b.Write(body)
	}
	return b.Bytes()
}
