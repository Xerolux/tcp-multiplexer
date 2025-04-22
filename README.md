# Modbus TCP Proxy

## Overview
This project provides a Modbus TCP Proxy service that enables the management of Modbus communication over TCP. It supports dynamic configuration, robust logging, and is compatible with Debian 12 and Ubuntu 24. The setup is fully managed using a `Makefile`, which simplifies building, installing, running, updating, and packaging.

## Features
- Supports Modbus-TCP communication
- Dynamic YAML-based configuration
- Persistent connection to the Modbus server
- Automatic reconnection and robust error handling
- Systemd service integration
- Flexible logging (console and file)
- Easy install/update via `make`
- Package support: `.deb` and Docker
- CI/CD automation via GitHub Actions
- Security scanning (Codacy, TruffleHog)
- Automatic dependency updates

## System Requirements
- **Operating System:** Debian 12 or Ubuntu 24
- **Python:** 3.10 or newer (tested with 3.10, 3.11)

## Installation with Make
All project operations are handled through a `Makefile`.

### 💡 Common Commands:
```bash
make install          # First-time setup
make update           # Pull latest version + update dependencies
make restart          # Restart the systemd service
make logs             # Show live logs
make backup-config    # Backup the config file
make uninstall        # Remove everything except config
make help             # Show all available commands
```

## Configuration
Create your configuration at:
```bash
/etc/Modbus-Tcp-Proxy/config.yaml
```

### Example:
```yaml
Proxy:
  ServerHost: "0.0.0.0"
  ServerPort: 502
  AllowedIPs:
    - "192.168.1.10"
    - "192.168.2.0/24"
  MaxConnections: 50

ModbusServer:
  ModbusServerHost: "192.168.1.100"
  ModbusServerPort: 502
  ConnectionTimeout: 10
  DelayAfterConnection: 0.5
  MaxRetries: 5
  MaxBackoff: 30.0

Logging:
  Enable: true
  LogFile: "/var/log/modbus_proxy.log"
  LogLevel: "INFO"
```

### Parameters
- **Proxy:** Listen address and port for incoming clients
  - `AllowedIPs`: List of allowed IPs or CIDR ranges
  - `MaxConnections`: Maximum concurrent connections
- **ModbusServer:** Target Modbus server connection parameters
  - `MaxRetries`: Retry attempts on connection failure
  - `MaxBackoff`: Maximum backoff time (seconds)
- **Logging:** Logging control and log level

## Service Management (Systemd)
The `make install` command sets up a systemd service named `modbus_proxy.service`. You can manage it using:
```bash
sudo systemctl start modbus_proxy.service
sudo systemctl stop modbus_proxy.service
sudo systemctl restart modbus_proxy.service
sudo systemctl enable modbus_proxy.service
sudo systemctl status modbus_proxy.service
```

## Logs
```bash
sudo tail -f /var/log/modbus_proxy.log
# Or use:
make logs
```

## Docker Usage
A `Dockerfile` is provided to run the proxy in a container.

### Build the image:
```bash
docker build -t modbus-proxy .
```

### Run the container:
```bash
docker run -d \
  --name modbus-proxy \
  -p 502:502 \
  -v /your/config/path/config.yaml:/etc/Modbus-Tcp-Proxy/config.yaml \
  modbus-proxy
```

### Docker Compose:
A `docker-compose.yml` file is included for easy deployment:

```bash
docker-compose up -d
```

## .deb Package
This project includes support for packaging into a `.deb` file, which is automatically built by GitHub Actions.

### Structure:
```
debian/
├── DEBIAN/control
├── opt/Modbus-Tcp-Proxy/...
└── etc/Modbus-Tcp-Proxy/config.yaml
```

### Build .deb package manually:
```bash
dpkg-deb --build debian build/modbus-tcp-proxy.deb
```

### Install:
```bash
sudo dpkg -i build/modbus-tcp-proxy.deb
```

## Development & Contributing

### GitHub Actions
The project uses several GitHub Actions workflows for CI/CD:
- **Build**: Builds Docker image and .deb package on push
- **Pylint**: Lints Python code (Python 3.10, 3.11)
- **Security Scans**: Codacy and TruffleHog for security analysis
- **Auto-merge**: Automatically merges Dependabot PRs
- **Dependency Review**: Checks for vulnerable dependencies
- **Documentation**: Builds and deploys mdBook documentation

### Contributing
Please see [CONTRIBUTING.md](CONTRIBUTING.md) for guidelines on how to contribute to this project. All contributors must adhere to our [Code of Conduct](CODE_OF_CONDUCT.md).

### Security
For security issues, please refer to our [Security Policy](SECURITY.md). Do not disclose security issues publicly without following the reporting procedure.

## Documentation
Project documentation is built using mdBook and automatically deployed to GitHub Pages. The documentation source is in the `src/` directory.

## Libraries Used
- `pymodbus>=3.8.0` – Modbus TCP communication
- `PyYAML>=6.0` – YAML config loader
- `cerberus>=1.3.4` – Configuration schema validation
- Built-in: `logging`, `queue`, `socket`, `threading`

Install manually with:
```bash
pip install -r requirements.txt
```

## Technical Details
- The proxy uses a persistent socket to the Modbus server and handles multiple client connections
- Automatic reconnection with exponential backoff ensures high availability
- Thread-safe queue and thread pool handle incoming requests efficiently
- CIDR notation support for IP filtering
- Comprehensive logging for monitoring and troubleshooting

## Like the Project?

If you'd like to support this integration or show your appreciation, you can:

<a href="https://www.buymeacoffee.com/xerolux" target="_blank"><img src="https://cdn.buymeacoffee.com/buttons/v2/default-yellow.png" alt="Buy Me A Coffee" style="height: 60px !important;width: 217px !important;"></a>

## License
MIT License – see [LICENSE](LICENSE) file.

## Support
For questions or issues, open a [GitHub Issue](https://github.com/Xerolux/Modbus-Tcp-Proxy/issues).

Current Version: 0.0.4
