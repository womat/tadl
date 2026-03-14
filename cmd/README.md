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
