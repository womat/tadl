# 🚀 tadl Datalogger for DL-Bus of Technische Alternative

---

## Features

- Exposes data via HTTPS REST API (with API key or JWT authentication)
- Publishes data to an MQTT broker
- IP allowlist / blocklist support

---

## API

### Get meter data

```sh
curl -k -H "X-Api-Key: your-api-key" https://localhost:8443/data
```

### Get application version

```sh
curl -k https://localhost:8443/version
```

### Health check

```sh
curl -k -H "X-Api-Key: your-api-key" https://localhost:8443/health
```

---

## TLS Certificate

Generate a self-signed certificate for development:

```sh
openssl req -x509 -nodes -newkey rsa:2048 \
  -keyout /opt/tadl/etc/key.pem \
  -out /opt/tadl/etc/cert.pem \
  -days 825 \
  -subj "/C=AT/ST=Vienna/L=Vienna/O=MyCompany/OU=DEV/CN=localhost"
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

### 2. Copy binary and configuration

```sh
sudo cp tadl /opt/tadl/bin/
sudo cp config.yaml /opt/tadl/etc/
sudo cp cert.pem key.pem /opt/tadl/etc/
sudo chown -R tadl:tadl /opt/tadl
```

### 3. Create systemd service

```sh
SERVICE_PATH="/etc/systemd/system/tadl.service"
sudo tee "$SERVICE_PATH" > /dev/null <<'EOF'
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
sudo systemctl enable tadl.service
sudo systemctl start tadl
sudo systemctl status tadl
```

### 4. View logs

```sh
journalctl -u tadl -n 50 -f
```

---

## Firewall (optional)

If ufw is active, allow the configured port:

```sh
# Check which port tadl is listening on
sudo netstat -tulpn | grep tadl

# Allow the port (replace 8443 with your configured port)
sudo ufw allow 8443/tcp
sudo ufw status
```

---

## Backup & Restore

### Backup

```sh
sudo tar czvf /tmp/tadl-backup.tar.gz /opt/tadl
```

### Restore

```sh
sudo tar xzvf /tmp/tadl-backup.tar.gz -C /
sudo chown -R tadl:tadl /opt/tadl
sudo systemctl restart tadl
```

---

## Command Line Flags

| Flag        | Default                     | Description                                       |
|-------------|-----------------------------|---------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml` | Path to the config file                           |
| `--debug`   | `false`                     | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                     | Print the app version and exit                    |
| `--about`   | `false`                     | Print app details and exit                        |
| `--help`    | `false`                     | Print a help message and exit                     |

The config file path can also be set via the environment variable `CONFIG_FILE`.

---

## Configuration

The configuration file is located at `/opt/tadl/etc/config.yaml`. See the included `config.yaml` for all available
options and documentation.


---

## License

MIT