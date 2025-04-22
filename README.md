# TCP-Multiplexer

[![Go](https://github.com/Xerolux/tcp-multiplexer/actions/workflows/go.yml/badge.svg)](https://github.com/Xerolux/tcp-multiplexer/actions/workflows/go.yml)
[![Release](https://github.com/Xerolux/tcp-multiplexer/actions/workflows/release.yml/badge.svg)](https://github.com/Xerolux/tcp-multiplexer/actions/workflows/release.yml)
[![CodeQL](https://github.com/Xerolux/tcp-multiplexer/actions/workflows/codeql-analysis.yml/badge.svg)](https://github.com/Xerolux/tcp-multiplexer/actions/workflows/codeql-analysis.yml)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](https://opensource.org/licenses/MIT)
[![Latest Release](https://img.shields.io/github/v/release/Xerolux/tcp-multiplexer?include_prereleases&sort=semver)](https://github.com/Xerolux/tcp-multiplexer/releases/latest)

TCP-Multiplexer is a powerful tool that bundles multiple client connections through a single TCP connection to a target server. It is ideal for scenarios where the target server supports only a limited number of simultaneous TCP connections.

A typical use case is connecting multiple Modbus/TCP clients to solar inverters, which often only support a single TCP connection.

## Table of Contents

- [Architecture](#architecture)
- [How It Works](#how-it-works)
- [Supported Protocols](#supported-protocols)
- [Installation](#installation)
- [Configuration](#configuration)
- [Systemd Service](#systemd-service)
- [Usage Examples](#usage-examples)
- [Performance Optimization](#performance-optimization)
- [Troubleshooting](#troubleshooting)
- [Contributing](#contributing)
- [License](#license)
- [Sponsorship](#sponsorship)

## Architecture

```
┌──────────┐
│ ┌────────┴──┐      ┌─────────────────┐      ┌───────────────┐
│ │           ├─────►│                 │      │               │
│ │ client(s) │      │ tcp-multiplexer ├─────►│ target server │
└─┤           ├─────►│                 │      │               │
  └───────────┘      └─────────────────┘      └───────────────┘


─────► TCP connection
```

Unlike a reverse proxy, the TCP connection between `tcp-multiplexer` and the target server is reused for all client connections. The current implementation also supports a connection pool for higher throughput.

## How It Works

TCP-Multiplexer uses a request-response pattern to coordinate communication:

1. Client establishes a TCP connection to the multiplexer
2. Multiplexer receives messages from the client
3. Multiplexer forwards the request to the target server
4. Multiplexer receives the response from the target server
5. Multiplexer returns the response back to the client

The enhanced version offers:
- Connection pooling for higher throughput
- Automatic recovery of failed connections
- Connection health monitoring
- Configurable timeouts and buffer sizes
- Round-robin load balancing with the connection pool

## Supported Protocols

TCP-Multiplexer supports various application protocols:

1. **echo**: Newline-terminated messages (`\n`)
2. **http**: HTTP/1.1 without HTTPS or WebSocket
3. **iso8583**: ISO-8583 messages with 2-byte length header
4. **modbus**: Modbus/TCP protocol
5. **mpu**: MPU Switch Format for ISO-8583

```bash
$ ./tcp-multiplexer list
* iso8583
* echo
* http
* modbus
* mpu

usage for example: ./tcp-multiplexer server -p echo
```

## Installation

### Option 1: Pre-compiled Binaries

Download the latest version from the [Releases page](https://github.com/Xerolux/tcp-multiplexer/releases) for your operating system and architecture:

```bash
# Example for Linux amd64
wget https://github.com/Xerolux/tcp-multiplexer/releases/latest/download/tcp-multiplexer_linux_amd64.tar.gz
tar -xzf tcp-multiplexer_linux_amd64.tar.gz
chmod +x tcp-multiplexer
sudo mv tcp-multiplexer /usr/local/bin/
```

### Option 2: Install with Go

```bash
go install github.com/Xerolux/tcp-multiplexer@latest
```

Or compile from source:

```bash
git clone https://github.com/Xerolux/tcp-multiplexer.git
cd tcp-multiplexer
make build
```

### Option 3: Docker Container

```bash
# With Docker Hub
docker pull xerolux/tcp-multiplexer:latest

# With GitHub Container Registry
docker pull ghcr.io/xerolux/tcp-multiplexer:latest
```

## Configuration

### Basic Configuration

```bash
tcp-multiplexer server -p <protocol> -t <target-server> -l <port>
```

| Parameter | Description | Default Value |
|-----------|-------------|---------------|
| `-p, --applicationProtocol` | Protocol to use (echo/http/iso8583/modbus/mpu) | echo |
| `-t, --targetServer` | Address of the target server in host:port format | 127.0.0.1:1234 |
| `-l, --listen` | Local port for the multiplexer to listen on | 8000 |
| `--timeout` | Connection timeout in seconds | 60 |
| `--delay` | Delay after establishing connection | 0 |
| `-v, --verbose` | Enable verbose logging | false |
| `-d, --debug` | Enable debug logging | false |

### Advanced Configuration

The enhanced version supports additional parameters:

| Parameter | Description | Default Value |
|-----------|-------------|---------------|
| `--max-connections` | Maximum number of simultaneous connections to the target server | 1 |
| `--reconnect-backoff` | Initial wait time between connection attempts | 1s |
| `--health-check-interval` | Interval for connection health checks | 30s |
| `--queue-size` | Size of the request queue | 32 |

## Systemd Service

For automatic startup of TCP-Multiplexer on Linux systems with systemd, you can set up a systemd service.

### Create Service File

Create the file `/etc/systemd/system/tcp-multiplexer.service`:

```bash
sudo nano /etc/systemd/system/tcp-multiplexer.service
```

Add the following content (adjust parameters for your environment):

```ini
[Unit]
Description=TCP Multiplexer Service
Documentation=https://github.com/Xerolux/tcp-multiplexer
After=network.target
Wants=network-online.target

[Service]
Type=simple
User=tcp-multiplexer
Group=tcp-multiplexer
ExecStart=/usr/local/bin/tcp-multiplexer server -p modbus -t 192.168.1.22:1502 -l 5020 --max-connections 2
Restart=on-failure
RestartSec=5
StandardOutput=journal
StandardError=journal
SyslogIdentifier=tcp-multiplexer
# Hardening options
ProtectSystem=full
PrivateTmp=true
NoNewPrivileges=true

[Install]
WantedBy=multi-user.target
```

### Create User for the Service

```bash
sudo useradd -r -s /bin/false tcp-multiplexer
```

### Enable and Start Service

```bash
# Reload systemd configuration
sudo systemctl daemon-reload

# Enable service to start on boot
sudo systemctl enable tcp-multiplexer.service

# Start the service
sudo systemctl start tcp-multiplexer.service

# Check status
sudo systemctl status tcp-multiplexer.service
```

### Manage Service

```bash
# Stop service
sudo systemctl stop tcp-multiplexer.service

# Restart service
sudo systemctl restart tcp-multiplexer.service

# View logs
sudo journalctl -u tcp-multiplexer.service

# Follow logs in real-time
sudo journalctl -u tcp-multiplexer.service -f
```

### Adjust Configuration

To change the service configuration, edit the service file and restart:

```bash
sudo nano /etc/systemd/system/tcp-multiplexer.service
sudo systemctl daemon-reload
sudo systemctl restart tcp-multiplexer.service
```

## Usage Examples

### Example: Modbus Proxy for Solar Inverters

```bash
# Via command line
tcp-multiplexer server -p modbus -t 192.168.1.22:1502 -l 5020 -v --max-connections 2

# With Docker
docker run -p 5020:5020 ghcr.io/xerolux/tcp-multiplexer server -t 192.168.1.22:1502 -l 5020 -p modbus -v
```

### Example: HTTP Proxy with Connection Pool

```bash
tcp-multiplexer server -p http -t backend.example.com:80 -l 8080 --max-connections 5 --health-check-interval 60s
```

### Docker Compose

Use the included `compose.yml` file as a template:

```yaml
services:
  modbus-proxy:
    image: ghcr.io/xerolux/tcp-multiplexer
    container_name: modbus_proxy
    ports:
      - "5020:5020"
    command: [ "server", "-t", "192.168.1.22:1502", "-l", "5020", "-p", "modbus", "-v", "--max-connections", "2" ]
    restart: unless-stopped
```

## Testing

### Echo Server Test

1. Start an echo server (listening on port 1234)

```bash
$ go run example/echo-server/main.go
1: 127.0.0.1:1234 <-> 127.0.0.1:58088
```

2. Start the TCP-Multiplexer (listening on port 8000)

```bash
$ ./tcp-multiplexer server -p echo -t 127.0.0.1:1234 -l 8000
INFO[2021-05-09T02:06:40+08:00] creating target connection
INFO[2021-05-09T02:06:40+08:00] new target connection: 127.0.0.1:58088 <-> 127.0.0.1:1234
INFO[2021-05-09T02:07:57+08:00] #1: 127.0.0.1:58342 <-> 127.0.0.1:8000
```

3. Test with a client

```bash
$ nc 127.0.0.1 8000
hello
hello
^C
$ nc 127.0.0.1 8000
world
world
```

## Performance Optimization

To achieve the best performance:

1. **Adjust Connection Pool**: Increase `--max-connections` for higher throughput, especially with many concurrent clients.

2. **Configure Health Monitoring**: Adjust `--health-check-interval` based on your network stability.

3. **Optimize Queue Sizes**: Increase `--queue-size` for applications with bursty traffic.

4. **Connection Timeouts**: Ensure `--timeout` is long enough to complete legitimate operations but short enough to detect hanging connections.

5. **Container Limits**: When using with Docker, set appropriate CPU and memory constraints.

## Troubleshooting

### Common Issues

1. **Connection Failures to Target Server**:
   - Check network connectivity to the target server
   - Verify the target server is listening on the specified port
   - Increase `--verbose` for more detailed logs

2. **Timeouts During Communication**:
   - Increase timeout value with `--timeout`
   - Check network latency to the target server

3. **High CPU or Memory Usage**:
   - Reduce `--max-connections` or `--queue-size`
   - Verify the target server can handle the traffic volume

4. **Inconsistent Responses**:
   - Ensure the correct protocol is selected with `-p`
   - Verify the target server supports the expected protocol

### Logging and Debugging

Enable verbose logging for better troubleshooting:

```bash
tcp-multiplexer server -p modbus -t 192.168.1.22:1502 -l 5020 -v -d
```

## Contributing

Contributions are welcome! Please open issues or pull requests for improvements or bug fixes.

## License

This project is licensed under the MIT License - see the [LICENSE](LICENSE) file for details.

## Sponsorship

If you find this project useful, please consider supporting its development:

[![GitHub Sponsor](https://img.shields.io/github/sponsors/xerolux?logo=github&style=flat-square&color=blue)](https://github.com/sponsors/xerolux)
[![Ko-Fi](https://img.shields.io/badge/Ko--fi-xerolux-blue?logo=ko-fi&style=flat-square)](https://ko-fi.com/xerolux)
[![Patreon](https://img.shields.io/badge/Patreon-Xerolux-red?logo=patreon&style=flat-square)](https://patreon.com/Xerolux)
[![Buy Me A Coffee](https://img.shields.io/badge/Buy%20Me%20A%20Coffee-xerolux-yellow?logo=buy-me-a-coffee&style=flat-square)](https://www.buymeacoffee.com/xerolux)
