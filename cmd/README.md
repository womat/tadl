# tadl

**Datalogger for DL-Bus of Technische Alternative**

Reads sensor data from UVR31/UVR42 heating controllers via the DL-Bus protocol on a Raspberry Pi,
exposes the data via a secured HTTPS REST API, and publishes it to an MQTT broker.
---

## Command-Line Flags

| Flag        | Default                        | Description                                                         |
|-------------|--------------------------------|---------------------------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml`    | Path to the configuration file                                      |
| `--debug`   | `false`                        | Enable debug logging to stdout (overrides log settings from config) |
| `--version` | `false`                        | Print the application version and exit                              |
| `--about`   | `false`                        | Print application details and exit                                  |
| `--help`    | `false`                        | Print this help message and exit                                    |

The config file path can also be set via the environment variable `CONFIG_FILE`.

```sh
tadl --config /etc/tadl/config.yaml
tadl --debug
tadl --version
CONFIG_FILE=/etc/tadl/config.yaml tadl
```

---

## Configuration

The configuration file is a YAML file. By default it is loaded from `/opt/tadl/etc/config.yaml`.
Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.

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
  connection: "tcp://broker.example.com:1883"  # empty = MQTT disabled
  retained: false
  topicPrefix: home/uvr42
  publishInterval: 60    # seconds
  minDeltaTemp: 0.5      # minimum °C change to trigger publish
```

---

