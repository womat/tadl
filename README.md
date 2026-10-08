# tadl

**Read the DL-Bus of a Technische Alternative UVR42 or UVR31 heating controller on a Raspberry Pi — and
see the temperatures live.**

[![CI](https://github.com/womat/tadl/actions/workflows/ci.yml/badge.svg)](https://github.com/womat/tadl/actions/workflows/ci.yml)
[![Release](https://img.shields.io/github/v/release/womat/tadl)](https://github.com/womat/tadl/releases/latest)
[![License: MIT](https://img.shields.io/badge/License-MIT-blue)](LICENSE)
[![Go](https://img.shields.io/github/go-mod/go-version/womat/tadl)](go.mod)
![Raspberry Pi](https://img.shields.io/badge/runs%20on-Raspberry%20Pi-C51A4A)

🇩🇪 [Deutsche Kurzfassung](README.de.md)

<p align="center">
  <picture>
    <source media="(prefers-color-scheme: dark)" srcset="docs/screenshots/web-ui-dark.png">
    <img src="docs/screenshots/web-ui.png" width="640" alt="tadl web page: four named temperatures with range bars and 15-minute trend, the outputs, and the state of the DL-Bus">
  </picture>
  &nbsp;
  <img src="docs/screenshots/web-ui-phone.png" width="180" alt="The same page on a phone">
</p>

> **Got a Pi and a UVR42 or UVR31?** The [Quick start](#quick-start) gets you from download to the
> live page; the [Wiring](#wiring) needs an optocoupler and a resistor.

The solar and heating controllers UVR42 and UVR31 of [Technische Alternative](https://www.ta.co.at/)
send their temperatures and output states on the **DL-Bus**, a two-wire data line originally meant
for TA's own data loggers. Connect that line through an optocoupler to a GPIO pin of a Raspberry Pi,
and tadl turns it into readings:

- the **temperatures** of all sensors and the **state of the outputs** (pumps, valves),
- published via **MQTT** on every significant change and at a fixed interval, ready for Node-RED,
  Home Assistant, ioBroker or openHAB,
- shown live on a **built-in web page**, with your own sensor names, a bar per temperature and its
  trend over the last 15 minutes,
- available through an **HTTPS REST API** for scripts and monitoring.

No cloud, no database, no runtime: a single binary, configured with one YAML file.

## Features

- **Decodes DL-Bus frames** of the **UVR42** and the **UVR31**
- **Any bit rate, no setting**: the Manchester decoder recovers the controller's clock from the
  signal and follows it, also when the controller's clock is off or drifts
- **Line polarity detected** at every frame, so an inverting optocoupler needs no setting
- **Web page**: temperatures with names, range bars and 15-minute trend, outputs, a receive LED,
  signal state, line polarity and the errors of the last 24 hours — readable on a phone, light and
  dark mode
- **MQTT** publishing at an interval plus a delta trigger (`minDeltaTemp`); a changed output is sent
  at once
- **HTTPS REST API** with API key, IP allowlist / blocklist
- **Hot reload** of the configuration via `SIGHUP`; a broken file is refused, the running
  configuration stays
- **Controller emulator** `dlbussim` for testing without a controller
- Release builds for every Raspberry Pi architecture, from the **Pi Zero (ARMv6)** to 64-bit systems

---

## Supported devices

| Controller | Device ID | Temperatures | Outputs | Bit rate | One frame every |
|------------|-----------|--------------|---------|----------|-----------------|
| UVR42      | `0x10`    | 4            | 2       | 50 Hz    | 2.32 s          |
| UVR31      | `0x30`    | 3            | 1       | 50 Hz    | 1.92 s          |

The protocol is described in Technische Alternative's
[DL-Bus protocol description v1.7](https://www.mikrocontroller.net/attachment/646125/DL-Bus_Protokoll_v1.7.pdf)
(German): Manchester code, a SYNC of 16 high bits, bytes with start and stop bit, LSB first, and the
frame layout of every controller. Temperatures are sent in 0.1 °C; a value outside −50 … 300 °C, such
as from a broken or disconnected sensor, is left out.

Other TA controllers (UVR1611, UVR61-3, ESR21, UVR64, …) use the same bus, and tadl reads their
signal at any bit rate, but it does not decode their frames: the web page then shows
*Signal, no valid frames*.

tadl runs on every Raspberry Pi with a 40-pin header under a current Raspberry Pi OS; it reads the
GPIO through the Linux GPIO character device.

---

## Quick start

**1. Download** the archive for your Pi from the [latest release](https://github.com/womat/tadl/releases/latest):

| Archive        | Raspberry Pi model                           |
|----------------|----------------------------------------------|
| `linux_armv6`  | Pi 1 and Zero (1st gen); also runs on every newer Pi |
| `linux_armv7`  | Pi 2 / 3 / 4 / 5 / Zero 2 W with a 32-bit OS |
| `linux_arm64`  | Pi 3 / 4 / 5 / 400 / Zero 2 W with a 64-bit OS |

```sh
VERSION=1.7.0 ARCH=arm64        # see the release page for the latest version
BASE=https://github.com/womat/tadl/releases/download/v$VERSION
curl -LO $BASE/tadl_${VERSION}_linux_$ARCH.tar.gz -LO $BASE/checksums.txt
sha256sum -c checksums.txt --ignore-missing
tar xzf tadl_${VERSION}_linux_$ARCH.tar.gz
```

**2. Install** binary, example configuration and a certificate:

```sh
sudo groupadd -r -f tadl
sudo useradd -r -s /usr/sbin/nologin -g tadl tadl
sudo usermod -aG gpio tadl
sudo mkdir -p /opt/tadl/{bin,etc}

sudo install -m 755 tadl /opt/tadl/bin/
sudo install -m 640 config/config.yaml /opt/tadl/etc/
sudo openssl req -x509 -nodes -newkey rsa:2048 -days 825 \
  -keyout /opt/tadl/etc/key.pem -out /opt/tadl/etc/cert.pem -subj "/CN=$(hostname)"
sudo chown -R tadl:tadl /opt/tadl
```

**3. Configure** `/opt/tadl/etc/config.yaml`: set `env: prod`, a random `apiKey`
(`openssl rand -hex 24`), the controller `type`, the GPIO pin and your MQTT broker (or
`connection: ""`) — see [Configuration](#configuration) and [Wiring](#wiring).

**4. Start** it as a service and open the firewall:

```sh
sudo tee /etc/systemd/system/tadl.service > /dev/null <<'EOF'
[Unit]
Description=tadl - data logger for the DL-Bus of Technische Alternative
After=network-online.target
Wants=network-online.target

[Service]
User=tadl
Group=tadl
Type=simple
ExecStart=/opt/tadl/bin/tadl
ExecReload=/bin/kill -HUP $MAINPID
Restart=on-failure
RestartSec=5

[Install]
WantedBy=multi-user.target
EOF

sudo systemctl daemon-reload
sudo systemctl enable --now tadl
sudo ufw allow 8443/tcp          # if ufw is active
journalctl -u tadl -n 20         # "Module started successfully"
```

**5. Open** `https://<your-pi>:8443/`, accept the self-signed certificate and enter the API key.

---

## Wiring

The DL-Bus is a two-wire line: the data line (**DL**) and ground. The controller drives the data
line with **12 V** and also powers TA's data loggers and DL sensors from it (at most about 40 mA).
**Never connect it straight to a GPIO pin**: the GPIO pins take **3.3 V at most**.

### With an optocoupler

An optocoupler, such as a **PC817** (also sold as LTV-817 or EL817), keeps the bus apart from the Pi
and converts the level. The bus drives the optocoupler's LED, and its transistor pulls the GPIO to
ground:

<p align="center">
  <img src="docs/wiring-optocoupler.svg" width="720" alt="PC817 optocoupler U1: the DL line of the controller (12 V) drives the LED (pin 1 anode, pin 2 cathode) through the reverse-polarity diode D1 and the 2.2 kΩ series resistor R1, the cathode goes to the ground of the bus; the transistor (pin 4 collector, pin 3 emitter) connects GPIO4 (header pin 7), pulled up to 3V3 (pin 1) by the 10 kΩ R2 or the internal pull-up, to GND (pin 9); a dashed line marks the galvanic isolation">
</p>

| Bus / Pi side | Connection |
|---------------|------------|
| DL (data line) | through a series resistor to the LED anode, PC817 pin 1 |
| Ground of the bus | LED cathode, PC817 pin 2 |
| GPIO (BCM, `dlbus.gpio`) | collector, PC817 pin 4 |
| GND of the Pi | emitter, PC817 pin 3 |
| 3V3 of the Pi | pull-up to the GPIO: an external resistor of about 10 kΩ, or `gpioTermination: pullup` |

The series resistor sets the LED current: about `(12 V − 1.2 V) / R`, so **2.2 kΩ** gives about
5 mA, which switches a PC817 reliably and takes little from the bus. Check the voltage on your bus
before you size the resistor; a diode such as a 1N4148 in series protects the LED against a swapped
connection. The bit rate of 50 Hz is far below what a PC817 can switch.

This stage **inverts** the signal: a high bus pulls the GPIO low. tadl detects the polarity at the
SYNC of every frame, so no setting is needed; the web page and `/health` show it as *inverted*. A
non-inverting interface works just as well.

An open-collector output like the PC817's needs a pull-up: either the resistor to 3V3 above, or
`dlbus.gpioTermination: pullup` for the Pi's internal one (about 50 kΩ). Keep `debounceTime: 0s`, see
[Configuration](#configuration).

### Test bench without a controller

[`dlbussim`](#testing-without-a-controller) drives a second GPIO pin of the same Pi with DL-Bus frames.
Wired through the same optocoupler, the test signal is inverted like a real bus:

<p align="center">
  <img src="docs/wiring-testbench.svg" width="720" alt="Test bench on one Raspberry Pi: dlbussim drives GPIO21 (pin 40), which drives the LED of the PC817 through the 470 Ω R1 to GND (pin 34); the transistor connects GPIO20 (pin 38), tadl's input, pulled up to 3V3 by the 10 kΩ R2 or the internal pull-up, to GND (pin 39)">
</p>

Run `dlbussim -gpio 21` and set tadl's `dlbus.gpio: 20`. A 3.3 V source like this may also be wired
straight to the input with a plain wire (GPIO21 to GPIO20); a real DL-Bus never.

---

## Web UI

Open `https://<host>:8443/` in a browser and enter the API key once; it is kept in that browser's
local storage only and sent as `X-API-Key` to `/health`. **Sign out** removes it, and a rejected key
brings the prompt back. The page refreshes every 3 s while the tab is visible and shows:

- the temperatures of the controller, each with a bar on its own range and the change over the
  last 15 minutes, and the outputs on or off. Names and ranges come from `datalogger.sensors`;
- whether data arrives: a receive LED flashes on every valid frame, and the signal line reads
  *Receiving*, *Signal, no valid frames* (wrong `datalogger.type`) or *No signal*;
- the line polarity and the number of errors in the last 24 hours, with the time of the last one.

The browser tab reads `tadl · <host>`, so several devices can be told apart. The page itself is
public and contains no data. It is a single file embedded in the binary, with no external scripts or
fonts, so it works without internet access; it follows the system's dark mode and works on a phone.

The 15-minute trend and the 24-hour error count are kept in memory: they start again after a
restart or a reload.

---

## Configuration

Default location: `/opt/tadl/etc/config.yaml`

- Environment variables are expanded inside the file, e.g. `apiKey: ${TADL_API_KEY}`. Both `${VAR}`
  and `$VAR` are expanded and an unset variable becomes empty, so a value that contains a literal `$`
  (an API key or a broker password) is changed — avoid `$` in such values.
- The configuration is validated on start and before every reload: `env` is `dev` or `prod`,
  `apiKey` is set, `logLevel` is known, the port is 1–65535, `publishInterval` is at least `1s`,
  `topicPrefix` is set when MQTT is enabled, `minDeltaTemp`, `debounceTime` and `bitClock` are not
  negative, the GPIO is between 2 and 27, `gpioTermination` and `type` are known, and every
  `datalogger.sensors` entry names a key of the configured controller.
- Keys that tadl does not know are ignored, so check the spelling of a setting that seems to have no
  effect.

```yaml
# =============================================================================
# app configuration
# =============================================================================

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
  # Supported types: uvr42 | uvr31
  type: uvr42
  # Optional: name sent as "device" in every telegram; default is the type.
  # name: solar

  # Optional: names and bar ranges for the web UI, keyed like the data
  # (uvr42: temperature1-4, out1-2; uvr31: temperature1-3, out1).
  # Without an entry the web UI shows T1, A1 ... and a range of -20 ... 150 °C.
  # The API (/data) and MQTT are not affected.
  # sensors:
  #   temperature1: { label: Collector, min: -20, max: 150 }
  #   temperature2: { label: Tank top, min: 0, max: 100 }
  #   temperature4: { label: Boiler room, min: -5, max: 25 }
  #   out1: { label: Solar pump }

# =============================================================================
# DL-Bus configuration
# =============================================================================
dlbus:
  # GPIO input pin for DL-Bus signal
  gpio: 4

  # Debounce period as Go duration string  (e.g. 5ms, 0 = disabled)
  # Keep 0: the kernel rounds the debounce up to its timer tick (4-10ms on a
  # Raspberry Pi) and then swallows the edges of short half-bits.
  debounceTime: 0s

  # GPIO line termination
  # Supported values: pullup | pulldown | none
  gpioTermination: none

  # DL-Bus bit clock in Hz, 0 = recover it from the signal (recommended).
  # With 0 tadl reads any controller whatever its bit rate (UVR31/UVR42: 50 Hz,
  # UVR1611/UVR61-3/ESR21: 488 Hz) and follows a controller whose clock is off
  # or drifts. A fixed value must match the controller within 25 %.
  # The line polarity needs no setting either: an optocoupler inverts the
  # signal, and tadl detects that at the SYNC of every frame.
  bitClock: 0

# =============================================================================
# MQTT configuration
# =============================================================================
mqtt:
  # Broker connection string (empty = MQTT disabled)
  # Format: tcp://host:port
  connection: "tcp://mqtt.example.com:1883"

  # Retain messages on the broker
  retained: false

  # MQTT topic prefix to publish measurements to
  topicPrefix: test/uvr42

  # Publish interval as Go duration string (e.g. 60s), at least 1s.
  # The current values are published at this interval in any case.
  publishInterval: 60s

  # Minimum temperature change in Kelvin since the last publish that triggers an
  # immediate publish. A changed output is always published at once.
  # 0 = temperatures are published only by interval (see publishInterval)
  minDeltaTemp: 0.5
```

### Reference

| Key | Type | Default | Description |
|-----|------|---------|-------------|
| `logLevel` | string | `info` | `debug`, `info`, `warn` (or `warning`), `error` |
| `logDestination` | string | `stdout` | `stdout`, `stderr` or a file path |
| `env` | string | `dev` | `dev` or `prod`; see [TLS certificate](#tls-certificate) |
| `webserver.listenHost` | string | `0.0.0.0` | Address the HTTPS server listens on |
| `webserver.listenPort` | int | `8443` | Port the HTTPS server listens on |
| `webserver.apiKey` | string | — | **Required.** Key for the protected endpoints, sent as `X-API-Key` |
| `webserver.keyFile` | string | — | TLS private key file |
| `webserver.certFile` | string | — | TLS certificate file |
| `webserver.blockedIPs` | list | empty | Addresses or networks (CIDR) that are refused |
| `webserver.allowedIPs` | list | empty | Addresses or networks (CIDR) that are allowed; empty allows all |
| `webserver.jwtSecret`, `webserver.jwtID` | string | empty | Optional: also accept a JWT as `Authorization: Bearer`, only when both are set. Not needed for the web page |
| `datalogger.type` | string | `uvr42` | `uvr42` or `uvr31` |
| `datalogger.name` | string | the type | Sent as `device` in every telegram, so a consumer can tell several controllers apart |
| `datalogger.sensors.<key>.label` | string | `T1` … / `A1` … | Name of a temperature or output on the web page |
| `datalogger.sensors.<key>.min`, `.max` | float | `-20`, `150` | Range of a temperature bar in °C, within −50 … 300; temperatures only |
| `dlbus.gpio` | int | — | **Required.** GPIO of the DL-Bus input, BCM numbering, 2–27 |
| `dlbus.debounceTime` | duration | `0s` | Kernel debounce of the input; keep `0s` |
| `dlbus.gpioTermination` | string | `pullup` | `pullup`, `pulldown` or `none`; the example file sets `none` |
| `dlbus.bitClock` | int | `0` | Bit clock in Hz; `0` recovers it from the signal |
| `mqtt.connection` | string | empty | Broker, e.g. `tcp://host:1883` or `tcp://user:password@host:1883`; empty disables MQTT |
| `mqtt.retained` | bool | `false` | Broker keeps the last message |
| `mqtt.topicPrefix` | string | — | Topic the values are published to; required with a broker |
| `mqtt.publishInterval` | duration | `10s` | Heartbeat, at least `1s` |
| `mqtt.minDeltaTemp` | float | `0` | Temperature change in Kelvin that triggers a publish; `0` disables the trigger |

The `<key>` of `datalogger.sensors` is the key of the value in `/data`: `temperature1` … `temperature4`,
`out1`, `out2` for a UVR42, `temperature1` … `temperature3`, `out1` for a UVR31. Sensors without an
entry keep the defaults. The broker password, if any, is never written to the log.

---

## MQTT publishing

### Telegram

The values of the controller are published as one JSON object on the topic `topicPrefix` itself
(QoS 0); `/data` returns the same object. A UVR42:

```
test/uvr42  {"device":"uvr42","out1":true,"out2":false,"temperature1":21.5,"temperature2":45.3,"temperature3":-7.2,"temperature4":0,"timestamp":"2026-10-08T14:32:05+02:00"}
```

| Key | Content |
|-----|---------|
| `device` | `datalogger.name`, by default the controller type (`uvr42`, `uvr31`) |
| `temperature1` … `temperature4` | Temperature in °C, 0.1 °C resolution. A sensor out of range (−50 … 300 °C) is left out |
| `out1`, `out2` | Output on (`true`) or off (`false`) |
| `timestamp` | Time the frame was received, RFC 3339 in local time with offset, whole seconds |

A UVR31 sends `temperature1` … `temperature3` and `out1`.

### When the values are published

- The **first frame** after a start or reload is published at once.
- Every frame (one every 2–3 seconds) is compared with the **last published** one, not with its
  predecessor, so a slow drift adds up. It is published at once when an output changed, a sensor
  went out of range or came back, or a temperature changed by at least `minDeltaTemp`.
- In any case the current values are published every `publishInterval` (the heartbeat).
- Nothing is published while there is no current frame: when the last one is older than three
  times `publishInterval`, but at least 30 s, the values count as stale. `/data` then answers 503.

While the broker is unreachable, tadl keeps reading and reconnects in the background. The outage
and its end are logged once each, not for every frame.

---

## REST API

| Method | Path       | Auth    | Description                                                 |
|--------|------------|---------|-------------------------------------------------------------|
| GET    | `/`        | —       | Web page, see [Web UI](#web-ui)                             |
| GET    | `/version` | —       | Application name and version                                |
| GET    | `/health`  | API Key | Runtime metrics, MQTT state, controller values, DL-Bus state and errors |
| GET    | `/data`    | API Key | Latest values of the controller; 503 without a current frame |

Authentication via the `X-API-Key` header. Errors are returned as `{"error": "..."}` with the HTTP
status (401 without a valid key, 503 from `/data` while there is no current frame).

```sh
# Latest values
curl -k -H "X-Api-Key: your-api-key" https://<your-pi>:8443/data

# Application version (no auth required)
curl -k https://<your-pi>:8443/version

# Health check
curl -k -H "X-Api-Key: your-api-key" https://<your-pi>:8443/health
```

`/health` reports, besides the runtime metrics (`app`, `appVersion`, `goVersion`, `hostname`, `os`,
`uptimeSeconds`, memory, goroutines, `timestamp`), the MQTT connection as `mqtt` (`connected`,
`disconnected` — also while reconnecting — or `disabled` without a broker), the controller and the
bus — everything the web page shows:

```json
"datalogger": {
  "type": "uvr42",
  "current": true,
  "lastFrame": "2026-10-08T14:32:05+02:00",
  "lastFrameAgeSeconds": 1.6,
  "temperatures": [
    { "key": "temperature1", "label": "Collector", "value": 118.6, "min": -20, "max": 150, "trend15m": 2.4 },
    { "key": "temperature3", "label": "Tank bottom", "value": null, "min": 0, "max": 100, "trend15m": null }
  ],
  "outputs": [
    { "key": "out1", "label": "Solar pump", "on": true }
  ]
},
"bus": {
  "signal": "receiving",
  "bitRateHz": 50,
  "invertedLine": true,
  "framesReceived": 48213,
  "rejectedFrames": 0,
  "droppedFrames": 0,
  "protocolErrors": 3,
  "droppedEdges": 0,
  "errors24h": { "rejectedFrames": 0, "droppedFrames": 0, "protocolErrors": 3, "droppedEdges": 0, "total": 3 },
  "lastError": "2026-10-08T12:18:05+02:00",
  "lastErrorAgeSeconds": 8040,
  "decoder": "Decoder state: decoding data, Frequency: 50.00 Hz, Buffer overflow count: 0, Resync count: 0"
}
```

| Field | Meaning |
|-------|---------|
| `temperatures[].value` | `null` while the sensor is out of range |
| `temperatures[].trend15m` | Change in Kelvin over the last 15 minutes; `null` until tadl has run that long |
| `bus.signal` | `receiving` (valid frames in the last 10 s), `noValidFrames` (frames arrive but none fits `datalogger.type`), `noSignal` |
| `bus.bitRateHz` | Bit clock the decoder follows; `null` while it searches for it |
| `bus.invertedLine` | The last SYNC arrived inverted, as behind an optocoupler |
| `rejectedFrames` | Frames of the wrong size or another device, or without a valid temperature |
| `droppedFrames` | Frames discarded because the reader was busy |
| `protocolErrors` | Frames discarded for a missing stop bit |
| `droppedEdges` | GPIO edges lost before decoding |

The counters in `bus` run since the start, `errors24h` sums the last 24 hours.

With the build tag `swagger`, the Swagger UI is served at `/swagger/`; it is meant for development
only.

---

## Command-line flags

| Flag        | Default                     | Description                                       |
|-------------|-----------------------------|---------------------------------------------------|
| `--config`  | `/opt/tadl/etc/config.yaml` | Path to the configuration file                    |
| `--debug`   | `false`                     | Enable debug logging to stdout (overrides config) |
| `--version` | `false`                     | Print the application version and exit            |
| `--about`   | `false`                     | Print application details and exit                |
| `--help`    | `false`                     | Print the help text and exit                      |

The config file path can also be set via the environment variable `CONFIG_FILE`; `--config` wins
over it.

```sh
tadl --config /etc/tadl/config.yaml
tadl --debug
CONFIG_FILE=/etc/tadl/config.yaml tadl
```

---

## TLS certificate

tadl serves HTTPS only. With `env: prod` it needs `certFile` and `keyFile`; a missing `certFile`
stops the start. The [Quick start](#quick-start) creates a self-signed pair valid for 825 days,
the maximum browsers accept.

With `env: dev`, tadl falls back to a self-signed certificate compiled into the binary when
`certFile` does not exist, and logs a warning. That certificate's private key ships inside every
published release archive, so it is public knowledge: anyone can extract it and impersonate an
instance running on the fallback. It exists so a fresh checkout starts
during development; never run a reachable device on it.

---

## Hot reload

Send `SIGHUP` to reload the configuration without restarting the process:

```sh
sudo systemctl reload tadl          # requires ExecReload in the unit, see Quick start
# or, independent of the unit file:
sudo systemctl kill -s HUP tadl
```

The new configuration is loaded and validated **before** anything is stopped. If it fails, the
reload is refused with `Config reload rejected, keeping the running configuration` in the log and
tadl keeps reading with its current settings — fix the file and reload again. A valid file restarts
tadl's components with the new settings: the GPIO line is reopened and the MQTT client reconnects,
within a few seconds. Some problems only show then, e.g. a missing TLS certificate or a port or GPIO
line in use: tadl logs `Start with the new configuration failed, continuing with the previous one`
and starts again with the settings it ran with before.

---

## Testing without a controller

`cmd/dlbussim` emulates a controller: it drives a GPIO pin with DL-Bus frames back to back,
Manchester-encoded, with values and timing set by flags. Connect that pin to tadl's input — directly,
since both sides are 3.3 V, or through the optocoupler a real bus would use — and point tadl's
`dlbus.gpio` at the input. Every release archive contains `dlbussim` next to `tadl`.

```sh
# on the Pi, e.g. GPIO21 drives an optocoupler whose output is GPIO20 (tadl: gpio 20)
./dlbussim -gpio 21 -temps 21.5,45.3,-7.2,0 -out1  # UVR42 at 50 Hz
./dlbussim -gpio 21 -offset 5                      # a controller whose clock is 5 % fast
./dlbussim -gpio 21 -bitClock 488 -frames 20       # 488 Hz, twenty frames
./dlbussim -type uvr31 -temps 20,30,40             # UVR31
```

| Flag | Default | Description |
|------|---------|-------------|
| `-gpio` | `21` | GPIO pin (BCM) to drive |
| `-type` | `uvr42` | Controller to emulate: `uvr42` or `uvr31` |
| `-bitClock` | `50` | Nominal bit clock in Hz |
| `-offset` | `0` | Deviation of the sender's clock in percent, e.g. `5` or `-5` |
| `-temps` | `21.5,45.3,-7.2,0` | Temperatures in °C, comma separated (UVR42: 4, UVR31: 3) |
| `-out1`, `-out2` | `false` | Outputs on (`-out2` UVR42 only) |
| `-frames` | `0` | Number of frames to send, `0` = until interrupted |

`GET /data` then shows the values sent. The tests in `pkg/dlbus` run the same chain without
hardware: golib's encoder, a virtual line (also inverted, with the clock 5 % off), the decoder
without a bit clock, the DL-Bus handler and the data logger.

---

## Security

- The REST API and the web page are served over HTTPS only. Use your own certificate and `env: prod`
  on every device, see [TLS certificate](#tls-certificate).
- Use a random `apiKey` (`openssl rand -hex 24`); everything except `/`, `/version` and the TLS
  handshake needs it. tadl only reads the bus: the API cannot change anything on the controller.
- Limit who can reach the port with `allowedIPs`, e.g. to your home network, and do not expose it
  to the internet.
- `mqtt.connection` may carry a broker password; keep the config file readable for the `tadl` user
  only (`640`, as in the Quick start) or pass the password in an environment variable.

Report vulnerabilities privately, see [SECURITY.md](SECURITY.md).

---

## Troubleshooting

**The web page shows *No signal***
No frames arrive. Check the wiring and the GPIO number in `dlbus.gpio`, and that the transistor side
of the optocoupler has a pull-up (`gpioTermination: pullup` or an external resistor). `--debug` logs
every GPIO edge; none at all means the input is silent.

**The web page shows *Signal, no valid frames***
The bus works, but the frames do not fit `datalogger.type` — another controller type, or a UVR42 set
up as `uvr31`. `--debug` logs why each frame was rejected.

**A temperature shows *sensor fault* and `/data` leaves it out**
The controller reports a value outside −50 … 300 °C, usually a sensor that is not connected or
broken. The log names the sensor and its value, at most once a minute.

**Service is `dead` immediately after start**
A configuration error; the process exits with code 1 and prints the reason on stdout:
`Failed to load config file` (YAML could not be parsed — durations such as `publishInterval` must be
Go duration strings like `60s`, not plain numbers) or `config validation failed`. Check with
`journalctl -u tadl -n 20`.

---

## Backup & restore

```sh
# Backup
sudo tar czf /tmp/tadl-backup.tar.gz /opt/tadl

# Restore
sudo tar xzf /tmp/tadl-backup.tar.gz -C /
sudo chown -R tadl:tadl /opt/tadl
sudo systemctl restart tadl
```

tadl keeps no data of its own; the backup holds the binary, the configuration and the certificate.

---

## Releases

Every release on the [releases page](https://github.com/womat/tadl/releases) carries archives for
all Raspberry Pi architectures with the binary, the controller emulator `dlbussim`,
`config/config.yaml`, `README.md` and `LICENSE`,
plus a `checksums.txt` and a changelog. Versions follow [semantic versioning](https://semver.org/);
a breaking change of the API, the telegram or the configuration raises the major version.

`tadl --version` reports the release a binary was built from. A local build reports something
like `1.7.0-3-g0c13781-dirty` instead, which is how the two are told apart on a device.

Building from source needs Go and `make`: clone the repository and run `make help` for the targets;
[`CLAUDE.md`](CLAUDE.md) describes the architecture, the tests and the release process.

---

## License

tadl is released under the MIT License - see [`LICENSE`](LICENSE) for the full text.

tadl is not affiliated with Technische Alternative. UVR42 and UVR31 are product names of
Technische Alternative RT GmbH.

### Third-party licenses

The source tree contains no third-party code, but a **compiled binary statically links** the
modules below. Their terms apply to anyone distributing that binary, not to the sources here.

| Module                                            | License                        |
|---------------------------------------------------|--------------------------------|
| `github.com/eclipse/paho.mqtt.golang`             | **EPL-2.0**, dual with EDL-1.0 |
| `github.com/womat/golib`                          | MIT                            |
| `github.com/warthog618/go-gpiocdev`               | MIT                            |
| `github.com/golang-jwt/jwt/v5`                    | MIT                            |
| `gopkg.in/yaml.v3`                                | MIT and Apache-2.0             |
| `github.com/gorilla/websocket`                    | BSD-3-Clause                   |
| `golang.org/x/net`, `sync`, `sys`                 | BSD-3-Clause                   |
| Swagger UI build only (`-tags swagger`):          |                                |
| `github.com/swaggo/swag`, `http-swagger`, `files` | MIT                            |
| `github.com/go-openapi/*`, `go.yaml.in/yaml/v3`   | Apache-2.0                     |
| `github.com/KyleBanks/depth`                      | MIT                            |
| `golang.org/x/mod`, `tools`                       | BSD-3-Clause                   |

All of these are permissive except the Eclipse Paho MQTT client, which is weak copyleft at file
level: if you hand out a built binary, the source of the EPL-covered parts has to remain available
(it is, at <https://github.com/eclipse-paho/paho.mqtt.golang>). Paho is dual-licensed, so the
BSD-style EDL-1.0 may be chosen instead. Neither obliges tadl itself to change its license.
