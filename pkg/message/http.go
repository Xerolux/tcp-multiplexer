package message

import (
	"bufio"
	"bytes"
	"errors"
	"fmt"
	"io"
	"net/textproto"
	"strconv"
	"strings"
	"sync"
)

const (
	headerKeyContentLength    = "Content-Length"
	headerKeyTransferEncoding = "Transfer-Encoding"
	headerKeyContentType      = "Content-Type"
	headerFormContentType     = "multipart/form-data"
	CRLF                      = "\r\n"
	boundaryPrefix            = "boundary="
	maxHeaderSize             = 1 << 20 // 1 MB
	maxBodySize               = 1 << 26 // 64 MB
)

// HTTPMessageReader for the HTTP protocol
type HTTPMessageReader struct{}

// Pre-allocate a global buffer pool with fixed-size buffers
var bufferPool = sync.Pool{
	New: func() interface{} {
		buf := make([]byte, 8192) // Increased initial buffer size for better performance
		return &buf
	},
}

func (H HTTPMessageReader) Name() string {
	return "http"
}

// ReadMessage reads and parses an HTTP message from the connection
func (H HTTPMessageReader) ReadMessage(conn io.Reader) ([]byte, error) {
	// Optimize by using existing bufio.Reader if possible
	bufReader, ok := conn.(*bufio.Reader)
	if !ok {
		bufReader = bufio.NewReaderSize(conn, 16384) // Use larger buffer for better performance
	}

	tp := textproto.NewReader(bufReader)

	// Read start line
	startLine, err := tp.ReadLine()
	if err != nil {
		return nil, err
	}

	if len(startLine) == 0 {
		return nil, errors.New("empty HTTP start line")
	}

	// Read headers more efficiently
	headers, err := tp.ReadMIMEHeader()
	if err != nil {
		return nil, err
	}

	if len(headers) > maxHeaderSize {
		return nil, errors.New("HTTP headers too large")
	}

	// Get buffer from pool
	resultBuf := getBuffer()
	defer releaseBuffer(resultBuf)

	// Use bytes.Buffer with pre-allocated capacity for better performance
	var finalBuf bytes.Buffer
	finalBuf.Grow(len(startLine) + 2 + estimateHeadersSize(headers) + 1024) // Pre-allocate with extra padding

	// Write start line and headers
	finalBuf.WriteString(startLine)
	finalBuf.WriteString(CRLF)

	for k, vv := range headers {
		for _, v := range vv {
			finalBuf.WriteString(k)
			finalBuf.WriteString(": ")
			finalBuf.WriteString(v)
			finalBuf.WriteString(CRLF)
		}
	}

	finalBuf.WriteString(CRLF)

	// Read body based on headers
	if err := readBody(bufReader, headers, &finalBuf, resultBuf); err != nil {
		return nil, err
	}

	return finalBuf.Bytes(), nil
}

// estimateHeadersSize estimates the serialized size of headers to pre-allocate buffer
func estimateHeadersSize(headers textproto.MIMEHeader) int {
	size := 0
	for k, vv := range headers {
		for _, v := range vv {
			// Key + ": " + value + CRLF
			size += len(k) + 2 + len(v) + 2
		}
	}
	return size + 2 // +2 for the final CRLF
}

// readBody reads the HTTP body based on headers and appends it to the buffer
func readBody(reader *bufio.Reader, headers textproto.MIMEHeader, dst *bytes.Buffer, buf *[]byte) error {
	// Check for Transfer-Encoding first
	if te, ok := headers[textproto.CanonicalMIMEHeaderKey(headerKeyTransferEncoding)]; ok && len(te) > 0 {
		if strings.ToLower(te[0]) == "chunked" {
			return readChunkedBody(reader, dst)
		}
	}

	// Then check for Content-Length
	if cl, ok := headers[textproto.CanonicalMIMEHeaderKey(headerKeyContentLength)]; ok && len(cl) > 0 {
		contentLength, err := strconv.Atoi(cl[0])
		if err != nil {
			return fmt.Errorf("invalid Content-Length: %s", cl[0])
		}

		if contentLength < 0 {
			return errors.New("negative Content-Length")
		}

		if contentLength > maxBodySize {
			return errors.New("HTTP body too large")
		}

		if contentLength == 0 {
			return nil
		}

		// Ensure buffer has enough capacity
		if cap(*buf) < contentLength {
			// If buffer is too small, allocate a new one
			newBuf := make([]byte, contentLength)
			*buf = newBuf
		} else {
			// Reuse existing buffer with correct length
			*buf = (*buf)[:contentLength]
		}

		// Read directly into buffer
		_, err = io.ReadFull(reader, *buf)
		if err != nil {
			return err
		}

		// Append to destination buffer
		dst.Write((*buf)[:contentLength])
		return nil
	}

	// Check for multipart form data
	if ct, ok := headers[textproto.CanonicalMIMEHeaderKey(headerKeyContentType)]; ok && len(ct) > 0 {
		if strings.Contains(strings.ToLower(ct[0]), headerFormContentType) {
			return readMultipartFormBody(reader, ct[0], dst)
		}
	}

	// No body indicators found
	return nil
}

// readChunkedBody handles chunked transfer encoding efficiently
func readChunkedBody(reader *bufio.Reader, dst *bytes.Buffer) error {
	totalSize := 0

	for {
		// Read chunk size line
		line, err := reader.ReadString('\n')
		if err != nil {
			return err
		}

		dst.WriteString(line)

		// Parse chunk size (in hex)
		chunkSize, err := strconv.ParseInt(strings.TrimSpace(line), 16, 64)
		if err != nil {
			return fmt.Errorf("invalid chunk size: %s", line)
		}

		// Zero-sized chunk means end of content
		if chunkSize == 0 {
			// Read the final CRLF
			finalCRLF, err := reader.ReadString('\n')
			if err != nil {
				return err
			}

			dst.WriteString(finalCRLF)
			return nil
		}

		// Check size constraints
		totalSize += int(chunkSize)
		if totalSize > maxBodySize {
			return errors.New("HTTP chunked body too large")
		}

		// Create small buffer for limited reads
		// Check if chunkSize is within the range of the int type
		if chunkSize < int64(^uint(0)>>1)*-1 || chunkSize > int64(^uint(0)>>1) {
			return fmt.Errorf("chunk size out of range for int type: %d", chunkSize)
		}

		bufSize := int(chunkSize)
		if bufSize > 8192 {
			bufSize = 8192
		}

		// Read the chunk data in smaller pieces if necessary
		remaining := int(chunkSize)
		for remaining > 0 {
			readSize := remaining
			if readSize > bufSize {
				readSize = bufSize
			}

			chunk := make([]byte, readSize)
			if _, err = io.ReadFull(reader, chunk); err != nil {
				return err
			}

			dst.Write(chunk)
			remaining -= readSize
		}

		// Read the CRLF after chunk
		crlf := make([]byte, 2)
		if _, err = io.ReadFull(reader, crlf); err != nil {
			return err
		}

		dst.Write(crlf)
	}
}

// readMultipartFormBody handles multipart form data more efficiently
func readMultipartFormBody(reader *bufio.Reader, contentType string, dst *bytes.Buffer) error {
	boundary := ""
	parts := strings.Split(contentType, ";")

	for _, part := range parts[1:] {
		part = strings.TrimSpace(part)
		if strings.HasPrefix(part, boundaryPrefix) {
			boundary = strings.TrimPrefix(part, boundaryPrefix)
			boundary = strings.Trim(boundary, "\"")
			break
		}
	}

	if boundary == "" {
		return errors.New("missing boundary in multipart/form-data")
	}

	boundaryEnd := "--" + boundary + "--"
	scanner := bufio.NewScanner(reader)
	scanner.Buffer(make([]byte, 1024*64), 1024*1024)

	for scanner.Scan() {
		line := scanner.Bytes()
		dst.Write(line)
		dst.WriteString(CRLF)

		if string(line) == boundaryEnd {
			break
		}
	}

	return scanner.Err()
}

// getBuffer retrieves a buffer from the pool
func getBuffer() *[]byte {
	return bufferPool.Get().(*[]byte)
}

// releaseBuffer returns a buffer to the pool
func releaseBuffer(buf *[]byte) {
	// Clear sensitive data
	for i := range *buf {
		(*buf)[i] = 0
	}
	bufferPool.Put(buf)
}
