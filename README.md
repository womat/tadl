# tadl

**Datalogger for DL-Bus of Technische Alternative**

Reads sensor data from UVR31/UVR42 heating controllers via the DL-Bus protocol on a Raspberry Pi,
exposes the data via a secured HTTPS REST API, and publishes it to an MQTT broker.

---

## Features

- Decodes DL-Bus frames from **UVR31** and **UVR42** controllers
- Publishes measurements to an **MQTT broker** (configurable interval + delta trigger)
- Exposes a secured **HTTPS REST API** (API key or JWT authentication)
- **IP allowlist / blocklist** support
- **Hot-reload** of configuration via `SIGHUP`
- Embedded self-signed TLS certificate for development (no setup required)
- Optional **Swagger UI** (build tag `swagger`, dev only)

---

## Project overview

- HTTPS REST API for relay control and health checks
- Config-driven relay registration from `config/config.yaml`
- Graceful shutdown and `SIGHUP`-based reloads
- Optional Swagger UI via the `swagger` build tag

---

## Where to start

- Runtime, API, build, deploy, and Swagger usage: [`cmd/README.md`](cmd/README.md)
- Example configuration: [`config/config.yaml`](config/config.yaml)
- Swagger generation script: [`docs/generate.sh`](docs/generate.sh)

---

## Supported Devices

| Device | ID   | Temperatures | Outputs |
|--------|------|--------------|---------|
| UVR42  | 0x10 | 4            | 2       |
| UVR31  | 0x30 | 3            | 1       |

---

## API Endpoints

| Method | Path       | Auth    | Description                        |
|--------|------------|---------|------------------------------------|
| GET    | `/version` | —       | Application name and version       |
| GET    | `/health`  | API Key | Runtime metrics (memory, uptime …) |
| GET    | `/data`    | API Key | Latest sensor readings             |

Authentication via the `X-API-Key` header.

### Examples

```sh
# Latest sensor data
curl -k -H "X-Api-Key: your-api-key" https://localhost:8443/data

# Application version (no auth required)
curl -k https://localhost:8443/version

# Health check
curl -k -H "X-Api-Key: your-api-key" https://localhost:8443/health
```

---

## Command-Line Flags

| Flag        | Default                     | Description                                       |
|-------------|-----------------------------|---------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml` | Path to the configuration file                    |
| `--debug`   | `false`                     | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                     | Print the application version and exit            |
| `--about`   | `false`                     | Print application details and exit                |
| `--help`    | `false`                     | Print this help message and exit                  |

The config file path can also be set via the environment variable `CONFIG_FILE`.

```sh
tadl --config /etc/tadl/config.yaml
tadl --debug
tadl --version
CONFIG_FILE=/etc/tadl/config.yaml tadl
```

---

## Configuration

Default location: `/opt/tadl/etc/config.yaml`
Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.


```yaml
# logLevel defines the minimum log level.
# Messages with at least this level are logged.
# Allowed values: debug | info | warn | error
logLevel: info

# logDestination defines where logs are written to.
# Supported values: stdout | stderr | /path/to/logfile
logDestination: stdout

# environment: dev | prod
env: dev

# =============================================================================
# Webserver configuration (HTTPS)
# =============================================================================
webserver:
  # Host address the HTTPS server listens on (0.0.0.0 = all interfaces)
  listenHost: 0.0.0.0

  # Port the HTTPS server listens on
  listenPort: 8443

  # Global API key for protected endpoints
  apiKey: changeme!

  # TLS private key file
  keyFile: /opt/tadl/etc/key.pem

  # TLS certificate file
  certFile: /opt/tadl/etc/cert.pem

  # Blocked IP addresses or networks (empty = none blocked)
  # Examples: 192.168.0.1, 192.168.0.0/16, 10.0.0.0/8
  blockedIPs: [ ]
  #  - 192.168.0.1
  #  - 192.168.0.0/16

  # Allowed IP addresses or networks (empty = all allowed)
  # Note: ::1 is the IPv6 loopback address
  # Examples: 127.0.0.1, ::1, 192.168.0.0/16
  allowedIPs: [ ]
  #  - 127.0.0.1
  #  - ::1
  #  - 192.168.0.0/16

# =============================================================================
# Datalogger configuration
# =============================================================================
datalogger:
  # Supported types: uvr42
  type: uvr42


# =============================================================================
# DL-Bus configuration
# =============================================================================
dlbus:
  # GPIO input pin for DL-Bus signal
  gpio: 4

  # Debounce period in milliseconds (0 = disabled)
  bounceTime: 0

  # GPIO line termination
  # Supported values: pullup | pulldown | none
  gpioTermination: none

  # DL-Bus bit clock frequency in Hz (used for timing), 0 = auto-detect
  bitClock: 50

# =============================================================================
# MQTT configuration
# =============================================================================
mqtt:
  # Broker connection string (empty = MQTT disabled)
  # Format: tcp://host:port
  connection: "tcp://localhost:1883"

  # Retain messages on the broker
  retained: false

  # MQTT topic prefix to publish measurements to
  topicPrefix: test/uvr42

  # Publish interval in seconds
  # 0 = publish only on change (see minDeltaTemp)
  publishInterval: 60

  # Minimum temperature delta in Celsius to trigger a publish
  # 0 = publish only by interval (see publishInterval)
  minDeltaTemp: 0.5
```

---

## TLS Certificate

For development the application falls back to an embedded self-signed certificate automatically.
For production, generate your own:

```sh
openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout /opt/tadl/etc/key.pem \
  -out    /opt/tadl/etc/cert.pem \
  -days 825 \
  -subj "/C=AT/ST=Vienna/L=Vienna/O=MyOrg/CN=localhost"
```

**Subject fields:**

| Field           | Example             | Description                                  |
|-----------------|---------------------|----------------------------------------------|
| `/C`            | `AT`                | Country code (2 letters)                     |
| `/ST`           | `Vienna`            | State or province (optional)                 |
| `/L`            | `Vienna`            | City (optional)                              |
| `/O`            | `MyCompany`         | Organization (optional)                      |
| `/OU`           | `DEV`               | Organizational unit (optional)               |
| `/CN`           | `localhost`         | **Common Name — your domain or `localhost`** |
| `/emailAddress` | `admin@example.com` | E-mail address (optional)                    |

> **Note:** Browsers enforce a maximum certificate validity of 825 days. Use `-days 365` for production-like setups.

---

## Installation

### 1. Create system user and directories

```sh
sudo groupadd -f tadl
sudo useradd -r -s /usr/sbin/nologin -g tadl tadl
sudo usermod -aG gpio tadl
sudo mkdir -p /opt/tadl/{bin,etc,data}
sudo chown -R tadl:tadl /opt/tadl
```

### 2. Copy files

```sh
sudo cp tadl /opt/tadl/bin/
sudo cp config.yaml /opt/tadl/etc/
sudo cp cert.pem key.pem /opt/tadl/etc/
sudo chown -R tadl:tadl /opt/tadl
```

### 3. Create systemd service

```sh
sudo tee /etc/systemd/system/tadl.service > /dev/null <<'EOF'
[Unit]
Description=tadl - Datalogger for DL-Bus of Technische Alternative
After=network.target

[Service]
User=tadl
Group=tadl
Type=simple
ExecStart=/opt/tadl/bin/tadl --config /opt/tadl/etc/config.yaml
Restart=on-failure

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable tadl
sudo systemctl start tadl
sudo systemctl status tadl
```

### 4. View logs

```sh
journalctl -u tadl -n 50 -f
```

---

## Build

```sh
# Raspberry Pi 4/5 (64-bit OS)
make build_arm64

# Raspberry Pi 2/3/4 (32-bit OS)
make build_arm7

# Raspberry Pi 1 / Zero (32-bit OS)
make build_arm6

# Build with Swagger UI (dev only)
make build_arm64_dev

# Build and deploy to Raspberry Pi via SCP
make deploy
```

---

## Hot-Reload

Send `SIGHUP` to reload the configuration without restarting the process:

```sh
sudo systemctl reload tadl
# or
kill -HUP $(pidof tadl)
```

---

## Firewall

```sh
# Allow the configured port (default 8443)
sudo ufw allow 8443/tcp
sudo ufw status
```

---

## Backup & Restore

```sh
# Backup
sudo tar czf /tmp/tadl-backup.tar.gz /opt/tadl

# Restore
sudo tar xzf /tmp/tadl-backup.tar.gz -C /
sudo chown -R tadl:tadl /opt/tadl
sudo systemctl restart tadl
```

---

# License

MIT