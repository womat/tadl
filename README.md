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

## Supported Devices

| Device | ID   | Temperatures | Outputs |
|--------|------|-------------|---------|
| UVR42  | 0x10 | 4           | 2       |
| UVR31  | 0x30 | 3           | 1       |

---

## API Endpoints

| Method | Path       | Auth     | Description                        |
|--------|------------|----------|------------------------------------|
| GET    | `/version` | —        | Application name and version       |
| GET    | `/health`  | API Key  | Runtime metrics (memory, uptime …) |
| GET    | `/data`    | API Key  | Latest sensor readings             |

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

| Flag        | Default                        | Description                                       |
|-------------|--------------------------------|---------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml`    | Path to the configuration file                    |
| `--debug`   | `false`                        | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                        | Print the application version and exit            |
| `--about`   | `false`                        | Print application details and exit                |
| `--help`    | `false`                        | Print this help message and exit                  |

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

```yaml
# Log level: debug | info | warn | error
logLevel: info

# Log destination: stdout | stderr | /path/to/logfile
logDestination: stdout

# Environment: dev | prod
env: dev

webserver:
  listenHost: 0.0.0.0
  listenPort: 8443
  apiKey: changeme!
  keyFile: /opt/tadl/etc/key.pem
  certFile: /opt/tadl/etc/cert.pem
  blockedIPs: []
  allowedIPs: []

datalogger:
  # Supported types: uvr42 | uvr31
  type: uvr42

dlbus:
  gpio: 4                  # BCM GPIO pin number
  bounceTime: 0            # Debounce in ms (0 = disabled)
  gpioTermination: pullup  # pullup | pulldown | none
  bitClock: 50             # Bit clock in Hz (0 = auto-detect)

mqtt:
  connection: "tcp://broker.example.com:1883"  # empty = disabled
  retained: false
  topicPrefix: home/uvr42
  publishInterval: 60      # seconds
  minDeltaTemp: 0.5        # minimum °C change to trigger publish
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
sudo cp tadl              /opt/tadl/bin/
sudo cp config.yaml       /opt/tadl/etc/
sudo cp cert.pem key.pem  /opt/tadl/etc/
sudo chown -R tadl:tadl   /opt/tadl
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

## License

GNU General Public License v3.0 — see [LICENSE](LICENSE) for details.