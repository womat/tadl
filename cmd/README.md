# tadl

**tadl** reads the DL-Bus of a Technische Alternative UVR42 or UVR31 heating controller on a
Raspberry Pi GPIO pin, publishes the temperatures and outputs via MQTT, serves them through an HTTPS
REST API and shows them on a built-in web page (`https://<host>:<listenPort>/`).

---

## Usage

```text
tadl [--config FILE] [--debug] [--version] [--about] [--help]
```

---

## Command-line Flags

| Flag        | Default                     | Description                                                         |
|-------------|-----------------------------|---------------------------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml` | Path to the configuration file                                      |
| `--debug`   | `false`                     | Enable debug logging to stdout (overrides log settings from config) |
| `--version` | `false`                     | Print the application version and exit                              |
| `--about`   | `false`                     | Print application details and exit                                  |
| `--help`    | `false`                     | Print this help message and exit                                    |

The config file path can also be set via the environment variable `CONFIG_FILE`; `--config` wins
over it.

`--version` prints the semantic version the binary was built from. It is injected from the Git tag
at build time, so an official release reports a plain `1.7.0`, while a local development build
reports a descriptive fallback such as `1.7.0-5-g0c13781-dirty` or `dev`. `--about` additionally
shows the build date and commit.

**Examples:**

```bash
tadl --config /etc/tadl/config.yaml
tadl --debug
tadl --version
CONFIG_FILE=/etc/tadl/config.yaml tadl
```

`SIGHUP` reloads the configuration; a file that does not load or validate is refused and the
running configuration stays.

---

## Configuration

The configuration file is a YAML file. By default it is loaded from `/opt/tadl/etc/config.yaml`.

Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`.

Every key is documented in the example configuration (`config/config.yaml`) and in the project
README: <https://github.com/womat/tadl#configuration>.
