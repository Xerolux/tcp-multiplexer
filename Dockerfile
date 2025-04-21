# Build stage
FROM golang:1.24-alpine AS builder

# Install build dependencies
RUN apk add --no-cache git make ca-certificates

# Set working directory
WORKDIR /app

# Copy go modules
COPY go.mod go.sum ./

# Download dependencies
RUN go mod download

# Copy source code
COPY . .

# Build the application
RUN CGO_ENABLED=0 go build -ldflags="-s -w" -o tcp-multiplexer .

# Run security scan on the binary but don't fail the build
RUN wget -qO- https://raw.githubusercontent.com/securego/gosec/master/install.sh | sh && \
    echo "Running security scan (issues will be reported but won't fail the build):" && \
    ./bin/gosec ./... || echo "Security issues found - review the output above"

# Final stage
FROM scratch

# Copy CA certificates for HTTPS
COPY --from=builder /etc/ssl/certs/ca-certificates.crt /etc/ssl/certs/

# Copy binary from builder
COPY --from=builder /app/tcp-multiplexer /

# Set user to non-root (nobody)
USER 65534:65534

# Expose port
EXPOSE 8000

# Run application
ENTRYPOINT ["/tcp-multiplexer"]
CMD ["server", "-p", "echo"]

# Add labels according to OCI image spec
LABEL org.opencontainers.image.title="TCP Multiplexer" \
      org.opencontainers.image.description="Multiplex multiple connections into a single TCP connection" \
      org.opencontainers.image.source="https://github.com/Xerolux/tcp-multiplexer" \
      org.opencontainers.image.licenses="MIT"
