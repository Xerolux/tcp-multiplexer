package message

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

// https://tools.ietf.org/html/rfc2616
// refer /usr/local/Cellar/go/1.16.3/libexec/src/net/http/request.go:1021 readRequest

// HTTPMessageReader for the HTTP protocol
type HTTPMessageReader struct {
	// No embedded bufPool anymore
}

// bufferPool is a global pool instance instead of as part of the structure
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 4096)
		return &buf
	},
}

func (H HTTPMessageReader) Name() string {
	return "http"
}

const (
	headerKeyContentLength    = "Content-Length"
	headerKeyContentType      = "Content-Type"
	headerKeyTransferEncoding = "Transfer-Encoding"
	headerFormContentType     = "multipart/form-data"
	CRLF                      = "\r\n"
	boundaryPrefix            = "boundary="
	maxHeaderSize             = 1 << 20 // 1 MB
	maxBodySize               = 1 << 26 // 64 MB
)

// getBuffer retrieves a buffer from the pool
func getBuffer() *[]byte {
	return bufferPool.Get().(*[]byte)
}

// releaseBuffer returns a buffer to the pool
func releaseBuffer(buf *[]byte) {
	// Clear buffer data for security reasons
	for i := range *buf {
		(*buf)[i] = 0
	}
	bufferPool.Put(buf)
}

// support HTTP1 plaintext
// DO NOT Support:
// 1. https
// 2. websocket
func (H HTTPMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// Use buffered reader for better performance
	bufReader, ok := conn.(*bufio.Reader)
	if !ok {
		bufReader = bufio.NewReader(conn)
	}
	
	tp := textproto.NewReader(bufReader)
	
	// Read the request/response line and headers
	message, err := readMessageHeaders(tp)
	if err != nil {
		return nil, fmt.Errorf("failed to read headers: %w", err)
	}
	
	// Determine content reading strategy based on headers
	body, err := readMessageBody(tp, message.headers)
	if err != nil {
		return nil, fmt.Errorf("failed to read body: %w", err)
	}
	
	// Combine the message parts
	if body != nil {
		if _, err := message.buffer.Write(body); err != nil {
			return nil, fmt.Errorf("failed to write body to buffer: %w", err)
		}
	}
	
	return message.buffer.Bytes(), nil
}

// httpMessage represents the components of an HTTP message
type httpMessage struct {
	startLine string
	headers   textproto.MIMEHeader
	buffer    *bytes.Buffer
}

// readMessageHeaders reads the HTTP start line and headers
func readMessageHeaders(tp *textproto.Reader) (*httpMessage, error) {
	// Read the start line
	startLine, err := tp.ReadLine()
	if err != nil {
		return nil, err
	}
	
	if len(startLine) == 0 {
		return nil, errors.New("empty HTTP start line")
	}
	
	slog.Debug(startLine)

	// Read headers with size limit to prevent attack vectors
	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}
	
	if len(headers) > maxHeaderSize {
		return nil, errors.New("HTTP headers too large")
	}
	
	slog.Debug(fmt.Sprintf("%v", headers))

	// Prepare for response
	var resultBuffer bytes.Buffer
	
	// Write start line and headers to result buffer
	if _, err := resultBuffer.WriteString(startLine); err != nil {
		return nil, fmt.Errorf("failed to write start line: %w", err)
	}
	
	if _, err := resultBuffer.WriteString(CRLF); err != nil {
		return nil, fmt.Errorf("failed to write CRLF: %w", err)
	}
	
	for k, vv := range headers {
		for _, v := range vv {
			if _, err := resultBuffer.WriteString(k); err != nil {
				return nil, fmt.Errorf("failed to write header key: %w", err)
			}
			if _, err := resultBuffer.WriteString(": "); err != nil {
				return nil, fmt.Errorf("failed to write header separator: %w", err)
			}
			if _, err := resultBuffer.WriteString(v); err != nil {
				return nil, fmt.Errorf("failed to write header value: %w", err)
			}
			if _, err := resultBuffer.WriteString(CRLF); err != nil {
				return nil, fmt.Errorf("failed to write header CRLF: %w", err)
			}
		}
	}
	
	if _, err := resultBuffer.WriteString(CRLF); err != nil {
		return nil, fmt.Errorf("failed to write final CRLF: %w", err)
	}

	return &httpMessage{
		startLine: startLine,
		headers:   headers,
		buffer:    &resultBuffer,
	}, nil
}

// readMessageBody reads the HTTP message body based on the headers
func readMessageBody(tp *textproto.Reader, headers textproto.MIMEHeader) ([]byte, error) {
	// First check for Transfer-Encoding (takes precedence over Content-Length)
	if transferEncoding, ok := headers[headerKeyTransferEncoding]; ok && len(transferEncoding) > 0 {
		if strings.ToLower(transferEncoding[0]) == "chunked" {
			var bodyBuffer bytes.Buffer
			err := readChunkedBody(tp.R, &bodyBuffer)
			if err != nil {
				return nil, fmt.Errorf("chunked read error: %w", err)
			}
			return bodyBuffer.Bytes(), nil
		}
	}

	// Check for multipart form data
	if contentType, ok := headers[headerKeyContentType]; ok && len(contentType) > 0 {
		contentTypeParts := strings.Split(contentType[0], ";")
		if len(contentTypeParts) > 0 && strings.TrimSpace(contentTypeParts[0]) == headerFormContentType {
			return readMultipartFormBody(tp.R, contentTypeParts)
		}
	}

	// Read using Content-Length if specified
	if contentLengthStr, ok := headers[headerKeyContentLength]; ok && len(contentLengthStr) > 0 {
		return readContentLengthBody(tp.R, contentLengthStr[0])
	}

	// No body
	return nil, nil
}

// readMultipartFormBody reads multipart form data body
func readMultipartFormBody(r io.Reader, contentTypeParts []string) ([]byte, error) {
	boundary := ""
	for _, part := range contentTypeParts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, boundaryPrefix) {
			boundary = strings.TrimPrefix(part, boundaryPrefix)
			break
		}
	}
	
	if boundary == "" {
		return nil, errors.New("missing boundary in multipart/form-data")
	}
	
	boundaryEnd := "--" + boundary + "--"
	scanner := bufio.NewScanner(r)
	var bodyBuffer bytes.Buffer
	
	for scanner.Scan() {
		line := scanner.Bytes()
		if _, err := bodyBuffer.Write(line); err != nil {
			return nil, fmt.Errorf("failed to write multipart line: %w", err)
		}
		
		if _, err := bodyBuffer.WriteString(CRLF); err != nil {
			return nil, fmt.Errorf("failed to write multipart CRLF: %w", err)
		}
		
		if string(line) == boundaryEnd {
			break
		}
	}
	
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error scanning multipart body: %w", err)
	}
	
	return bodyBuffer.Bytes(), nil
}

// readContentLengthBody reads body using Content-Length header
func readContentLengthBody(r io.Reader, contentLengthStr string) ([]byte, error) {
	contentLength, err := strconv.Atoi(contentLengthStr)
	if err != nil {
		return nil, fmt.Errorf("invalid Content-Length: %s", contentLengthStr)
	}
	
	if contentLength < 0 {
		return nil, errors.New("negative Content-Length")
	}
	
	if contentLength > maxBodySize {
		return nil, errors.New("HTTP body too large")
	}
	
	if contentLength == 0 {
		return nil, nil
	}
	
	// Get buffer from pool
	buf := getBuffer()
	defer releaseBuffer(buf)
	
	// Ensure buffer has enough capacity
	if cap(*buf) < contentLength {
		*buf = make([]byte, contentLength)
	} else {
		// Use existing buffer but slice to correct length
		*buf = (*buf)[:contentLength]
	}
	
	// Read directly into buffer
	_, err = io.ReadFull(r, *buf)
	if err != nil {
		return nil, fmt.Errorf("failed to read content body: %w", err)
	}
	
	// Return a copy of the buffer content
	bodyContent := make([]byte, contentLength)
	copy(bodyContent, *buf)
	
	return bodyContent, nil
}

// readChunkedBody reads HTTP chunked encoding
func readChunkedBody(r io.Reader, buffer *bytes.Buffer) error {
	br := bufio.NewReader(r)
	totalSize := 0
	
	for {
		// Read chunk size line
		line, err := br.ReadString('\n')
		if err != nil {
			return fmt.Errorf("failed to read chunk size: %w", err)
		}
		
		// Write the chunk header to output
		if _, err := buffer.WriteString(line); err != nil {
			return fmt.Errorf("failed to write chunk header: %w", err)
		}
		
		// Parse chunk size (in hex)
		chunkSize, err := strconv.ParseInt(strings.TrimSpace(line), 16, 64)
		if err != nil {
			return fmt.Errorf("invalid chunk size: %s", line)
		}
		
		// Zero-sized chunk means end of content
		if chunkSize == 0 {
			// Read the final CRLF
			finalCRLF, err := br.ReadString('\n')
			if err != nil {
				return fmt.Errorf("failed to read final CRLF: %w", err)
			}
			
			if _, err := buffer.WriteString(finalCRLF); err != nil {
				return fmt.Errorf("failed to write final CRLF: %w", err)
			}
			
			return nil
		}
		
		// Check size constraints
		totalSize += int(chunkSize)
		if totalSize > maxBodySize {
			return errors.New("HTTP chunked body too large")
		}
		
		// Read the chunk data
		chunk := make([]byte, chunkSize)
		if _, err = io.ReadFull(br, chunk); err != nil {
			return fmt.Errorf("failed to read chunk data: %w", err)
		}
		
		// Write chunk to output
		if _, err := buffer.Write(chunk); err != nil {
			return fmt.Errorf("failed to write chunk data: %w", err)
		}
		
		// Read the CRLF after chunk
		crlf := make([]byte, 2)
		if _, err = io.ReadFull(br, crlf); err != nil {
			return fmt.Errorf("failed to read chunk CRLF: %w", err)
		}
		
		if _, err := buffer.Write(crlf); err != nil {
			return fmt.Errorf("failed to write chunk CRLF: %w", err)
		}
	}
}
