# CLAUDE.md

This file provides guidance to Claude Code (claude.ai/code) when working with code in this repository.

## Project

`tadl` is a Go daemon that reads the DL-Bus of Technische Alternative heating controllers (UVR42, UVR31) from a Raspberry Pi GPIO pin, decodes the Manchester-coded frames, publishes the values to MQTT, and serves a TLS REST API plus an embedded web page. Single binary, cross-compiled for the Pi. `cmd/dlbussim` is a second binary that emulates a controller on a GPIO output for tests without hardware.

## Build / develop

**The GPIO dependency (`warthog618/go-gpiocdev` via `womat/gpio/rpi`) only compiles for Linux.** On macOS `go build ./...` and `go vet ./...` fail with `undefined: uapi.*`. Always set the target explicitly when checking code locally:

```sh
GOOS=linux GOARCH=arm64 go vet ./...
GOOS=linux GOARCH=arm64 go build ./...
```

```sh
make build_arm64       # Pi 3/4/5/Zero2, 64-bit OS
make build_arm7        # Pi 2/3/4/Zero2, 32-bit OS
make build_arm6        # Pi 1 / Zero, 32-bit OS
make build_arm64_dev   # + Swagger UI (-tags swagger)
make build_sim_arm64   # the controller emulator cmd/dlbussim
make deploy            # build locally, then scp to $(PI_USER)@$(PI_HOST):$(PI_PATH)
make deploy_sim        # the same for dlbussim
make deploy_release TAG=vX.Y.Z   # download a published release, verify, scp
make release TAG=vX.Y.Z          # tag main and push, see Releases
make lint              # gofmt, go fix -diff, go mod tidy -diff, vet, golangci-lint (.golangci.yml), govulncheck for PI_ARCH, with and without -tags swagger; works on macOS
make clean
make help              # the authoritative list of targets
```

`ensure_dev_certs` (a prerequisite of every tadl build target) generates `app/certs/dev_{cert,key}.pem` if missing; these are `//go:embed`-ed and gitignored (`*.pem`), so a fresh clone must build via `make`, not bare `go build`.

`PI_USER`/`PI_HOST`/`PI_PATH` default to placeholders (`pi`, `raspberrypi`, `.`); the real host name is deliberately not in this public repo. The actual device comes from environment variables (set once for all projects; they win over the `?=` defaults) or, project-specific, from `Makefile.local` (gitignored, pulled in via `-include`); command-line values override both, e.g. `make deploy PI_HOST=my-pi`. `PI_PATH` defaults to `.`, the login directory, so it is correct for any user name.

Two ways onto a Pi, deliberately kept apart: `make deploy` builds locally and is the development loop (its binary reports a `-dirty` version, which is how you tell it apart on the device); `make deploy_release TAG=vX.Y.Z` downloads the published archive via `gh`, verifies the checksum and copies that.

### Tests

Tests need no hardware: `pkg/dlbus/e2e_test.go` runs the Manchester encoder of `womat/manchester` over a virtual line (also inverted, with the clock 5 % off) into the decoder without a bit clock, the DL-Bus handler and the data logger; the other packages test pure logic. They still import the Linux-only GPIO backend, so they compile on Linux only. Run them with `make test` (`go test -race ./...`), on macOS in a container:

```sh
docker run --rm -v "$PWD":/src -w /src golang:1.27 make test
```

Keep logic testable without a clock or hardware: `collector.Handler` and `errwindow.Window` take their time from a `now` func field that tests replace; `signalState`, `parseDecoderInfo`, `edgeRate` and `decoderStuck` in `app/bus.go` are pure.

### Releases

**There is one branch, `main`: work is committed to it and a release is a tag on it.** `make release TAG=vX.Y.Z` refuses to run from any other branch, with a dirty tree, or when `main` and `origin/main` differ; `.github/workflows/release.yml` re-checks that the tagged commit is on `main`, so a hand-made `git tag` cannot bypass it. Use a short-lived feature branch for work that must not land on `main` yet.

Versioning is SemVer and the Git tag is the single source of truth. `VERSION` (`app/app.go`), `buildDate` and `buildCommit` (`cmd/main.go`) are injected via `-ldflags` — never edit them in source. The Makefile derives `VERSION` from `git describe --tags`; GoReleaser uses the tag itself. `make release TAG=vX.Y.Z` verifies the tag shape and a clean tree, then tags and pushes; `.github/workflows/release.yml` runs `goreleaser release --clean`, which builds `tadl` and `dlbussim` for linux arm64/armv7/armv6, packs both into one archive per architecture and publishes a GitHub release with checksums and a grouped changelog.

Two things to keep in mind when touching `.goreleaser.yaml`: its `before` hook must keep running `make ensure_dev_certs` (GoReleaser calls `go build` directly, so the `//go:embed`-ed dev certs would otherwise be missing), and archives must keep shipping `README.md` — it carries the third-party license overview, and the statically linked Paho MQTT client is EPL-2.0. Validate changes with `goreleaser check` and `goreleaser release --snapshot --clean`.

`.github/workflows/ci.yml` runs on every push/PR against `main`, weekly (Monday 05:17 UTC) and by hand; `setup-go` takes the Go version from `go.mod` with `check-latest`, so the weekly run catches a toolchain advisory without a commit. It has a `test` job (`make test` with the race detector) and a build matrix over `PI_ARCH` arm6/arm7/arm64 that runs `make lint` and `make build_<arch> build_<arch>_dev build_sim_<arch>`. `release.yml` runs `make test` and `make lint` (default `PI_ARCH`, arm64) on the tagged commit before GoReleaser publishes. The jobs set no `GOOS`/`GOARCH` of their own, or `make lint` would build the analysers for arm. govulncheck and golangci-lint are pinned in the Makefile only; `.github/dependabot.yml` updates actions (`chore(ci)`) and Go modules (`chore(deps)`, Swagger in a group of its own) weekly, but not those `go install` pins — raise them by hand.

golangci-lint runs with the default linters; every exclusion in `.golangci.yml` is a decision with a why-comment (errcheck is off in `_test.go`, and the deferred `Close` of the golib logger and the GPIO pin is listed in `exclude-functions` instead of switching errcheck off). Fix a new finding rather than widening an exclusion. Like vet, it needs `GOOS=linux`, which `make lint` sets. Check the workflow files for the exact jobs when you change them.

### Swagger

Swagger UI is behind the `swagger` build tag (`app/swagger.go` vs `app/swagger_stub.go`, both defining `registerSwaggerRoute`). Regenerate `docs/` from the annotations after changing API handlers:

```sh
docs/generate.sh   # must run from the project root; needs swaggo/swag installed
```

### Screenshots

The README screenshots are rendered from the real `app/ui/index.html` with a mocked `/health` in headless Chromium. Re-run after visible UI changes:

```sh
docker run --rm -v "$PWD":/src -w /src mcr.microsoft.com/playwright/python:v1.52.0-noble \
  python3 docs/screenshots/capture.py
```

### Running locally

```sh
go run ./cmd/main.go --config config/config.yaml --debug   # Linux/Pi only
```

Without a controller, run `dlbussim` on a second GPIO pin wired to tadl's input (see README, "Testing without a controller").

## Architecture

Layering is strict: `cmd` → `app` → `app/service/*` → `pkg/*`. Lower layers never import upward.

Data path: GPIO edges (`womat/gpio/rpi`) → `app.decoderEvents` → `womat/manchester/decoder` (bit clock recovered from the signal when `bitClock: 0`, IEEE convention) → `pkg/dlbus` (SYNC detection, polarity, bytes → frames) → `pkg/datalogger` (frame → `datalogger.Frame`) → `app/service/collector` (latest frame, MQTT publish, trend) → `/data`, `/health`, MQTT.

- **`cmd/main.go`** — flags, config load/validate, logger init, and the **restart loop**. `run()` subscribes to SIGHUP/SIGTERM/SIGINT **once** and hands that channel to every `App`, so a signal arriving between two lifecycles waits for the next `App` instead of killing the process — never `signal.Stop`/`Reset` it inside `app`. It passes a `checkReload` closure that loads and validates the config file, which the SIGHUP handler calls **before** tearing anything down, so a broken file is refused and the running `App` keeps going. A restart that passes the check but fails in `Run` (TLS, port, GPIO) falls back to `lastGood`, the config of the previous `App`; only a failing first start exits — with **exit code 78** (`exitConfig`) when the config does not load or validate, which `contrib/tadl.service` keeps from restarting (`RestartPreventExitStatus=78`). `tadl token …` is dispatched at the very top of `main`, before the flags: it is golib's `web.TokenCommand` and only reads the config (`app.TokenFile`) and writes the token file — no GPIO, no MQTT, no server, so it can run while the service runs. Every error path of `Run`, `Init` included, goes through `abort` (cancel, `wg.Wait`, `Cleanup`), so `Cleanup` must cope with parts that were never created. `README.md` in `cmd/` is `//go:embed`-ed as `--help` output, so keep it accurate.
- **`app/app.go`** — wiring and lifecycle. Owns the `context.Context` that every goroutine is cancelled by. The signal goroutine is the **only** caller of `shutdownProcedure`: SIGHUP → `ModeRestart`, SIGTERM/SIGINT → `ModeStop`, and a web server that stops on its own reports on `app.serverErr` → `ModeRestart` (it must not call `shutdownProcedure` itself, because that waits on `app.wg`, which tracks the server goroutine). The GPIO callback never blocks: when `decoderEvents` is full it drops the edge, counts it in `droppedEdges` and reports it as `Missed` with the next edge. It also counts every edge in `edges` and stamps `lastEdgeNs` with `time.Now()` — the same clock as `dlbus.Stats().LastFrame`, which the watchdog compares it with. A reload builds a brand-new `App`, so in-memory state (trend samples, the 24 h error window) starts over.
- **`app/config.go`** — YAML decoded with `KnownFields(true)` (unknown keys are an error), `expandEnvBraces` on the raw file (only `${VAR}`, a bare `$` stays), yaml.v3 itself refusing a duration without a unit (pinned in `config_durations_test.go`), defaults from `NewConfig()`, and `Validate()`, which `cmd` calls before `app.New`. `LoadConfig` = `readConfig` (the YAML alone; a relative `webserver.tokenFile` is resolved against the config file's directory) + `web.ReadTokenFile`, so a missing or broken token file fails the start and refuses a reload, and a reload picks up new tokens. `Validate` checks `allowedIPs`/`blockedIPs` with `web.ValidateIPList`, because `WithIPFilter` would only log and skip a bad entry, and parses `mqtt.connection` the way Paho does, because mqtt ≥ v1.1.0 refuses a URL it cannot parse. `Init` warns ("Token file is not protected", `web.CheckTokenFile`) on every start and reload when the service could change its token file or a directory above it — a warning, because a development checkout always looks like that. `webserver.apiKey` is still a field only so `Validate` can reject it with a message naming `tokenFile`. `datalogger.sensors` is checked against `datalogger.Keys` of the configured device; ranges must lie within `datalogger.MinTemperature`…`MaxTemperature`.
- **`app/routes.go` / `api_*.go`** — `http.ServeMux` with Go 1.22 method patterns. Middleware chain, outermost first: golib `web.WithLogging` (successful requests at debug via `WithSuccessLevel`, the first per caller and client at info, rejected ones as warnings; every entry has `request.client` and, once authorized, `user`) → `WithIPFilter` → `WithCORS` → mux. Auth is per-route via `web.WithAuth` with `app.webConfig()`: the bearer tokens from the token file and nothing else — no `ApiKey`, no JWT. At start and after each reload `Run` logs the token names (`Bearer tokens loaded`). `/` (`GET /{$}`) and `/version` are public; `/health` and `/data` are protected. `WithCORS` answers preflights itself, so there is no `OPTIONS /` route — it would turn the mux's 404 for an unknown path into 405. `/data` answers 503 while there is no current frame.
- **`app/api_ui.go` / `app/ui/index.html`** — the web page at `GET /{$}`, one `//go:embed`-ed HTML file with inline JS/CSS and a strict CSP; no external resources, since the Pi may be offline. It holds no data: it asks for a token, keeps it in `localStorage` (key `tadl.token`) and polls `/health` only, with `Authorization: Bearer`.
- **`app/api_health.go` / `app/bus.go`** — `/health` combines `health.GetCurrentHealth` with the MQTT state (plus broker as host:port, never credentials, and topic for the pill tooltip), `collector.Status` (values with labels/ranges from `datalogger.sensors`, 15-minute trend) and `busStatus` (signal state, bit rate parsed from `decoder.Info()` with the nominal one from `datalogger.BitRate`, edge rate, polarity, error counters, decoder resets). `errwindow` turns the cumulative counters into a 24 h sum; it is sampled once a minute.
- **Signal states and the decoder watchdog** (decided 21 October 2026, after a UVR42 receiver whose decoder had locked on 100 instead of 50 Hz showed *No signal* for hours while the line carried frames). `signalState` goes from the record, to the bus frame, to the **edge** at the input: `notDecoded` (edges, no frames) is its own state, so the page points at the decoder instead of the wiring, and `noSignal` means a silent input. `watchBus` samples the edge rate every 10 s and is the watchdog: edges in the last 10 s but no bus frame for 30 s (since the start or the previous reset) → `decoder.Reset()` (github.com/womat/manchester ≥ v1.1.0), a warning with `decoder.Info()`, `decoderResets` + 1. Only the first reset of a fault warns; further ones log at debug until a bus frame comes again (decided 25 October 2026: an input that never decodes — open, mains hum, wrong `bitClock` — otherwise warned every ~40 s, about 2000 times a day), and `decoderResets` still counts them all. A silent line is left alone — a reset cannot help there and would hide the fault — and the bus frame, not the valid record, is the measure, because a wrong `datalogger.type` is not the decoder's fault. The watchdog resets, it does not rebuild the decoder: `Reset` keeps a configured `bitClock`, and the chain GPIO → decoder → dlbus stays as it is. The decoder itself (v1.0.1) leaves a clock at twice the bit rate; the watchdog is for whatever else makes it stall.
- **`app/webservices.go`** — HTTPS only, TLS 1.3 only (`newTLSConfig`, pinned by `TestTLSConfigRequires13`). Falls back to the embedded dev cert when `certFile` does not exist, but only with `env: dev`; with `env: prod` that is a start-up error.
- **`app/service/collector`** — the domain layer. Stores the latest record, decides when to publish (`hasChanged`: first frame, changed output, sensor appearing/disappearing, or a temperature change ≥ `minDeltaTemp` against the **last published** frame) and runs the periodic publish. A frame older than `StaleAfter` (max(3 × `publishInterval`, 30 s)) is neither published nor served. A broker outage is logged once via the `offline` flag.
- **`pkg/dlbus`** — DL-Bus framing on the decoded bit stream: a SYNC is a run of at least 12 identical bits followed by the opposite start bit; its polarity decides whether the bits are inverted. Bytes are start bit, 8 data bits LSB first, stop bit. Knows nothing of controllers.
- **`pkg/datalogger`** — per-controller frame decoding (`uvr42.go`, `uvr31.go`), the `Key*` constants, `Keys(type)`, and `frame.go` with `FrameUVR42`/`FrameUVR31`, which build frames for `dlbussim` and the tests. Temperatures outside −50 … 300 °C are left out of the record.

**External dependencies:** `github.com/womat/golib/v2` supplies `web` (token auth and `TokenCommand`, access log, CORS, IP filter, `Encode`) and `xlog`; `github.com/womat/gpio` supplies `gpio`/`gpio/rpi`; `github.com/womat/manchester` supplies `decoder` and `encoder`; `github.com/womat/mqtt` the MQTT client. tadl is the **template for the sibling services** (relayhat, s0meter, smartmeter, modbusgateway): token file + `token` subcommand, golib access log, `xlog`, exit code 78, TLS 1.3, `contrib/<svc>.service` with a sandbox, everything under `/opt/<svc>` owned by root. Keep it that way, and carry a change of that pattern over to them. They are not vendored — read them in `$(go env GOMODCACHE)/github.com/womat/<module>@<version>` when behavior is unclear.

### Telegram contract

The MQTT payload and the `/data` response are the same `datalogger.Frame`, marshalled as JSON: `device` (`datalogger.name`, default the type; set by `collector`, not the decoders), `temperature1`…`temperature4` (UVR31: …3), `out1`, `out2` (UVR31: `out1` only), `timestamp` (`time.Time`, local time, truncated to whole seconds, RFC 3339). It is published to `mqtt.topicPrefix` itself, QoS 0. Changing a key breaks consumers — document it in the README when you do.

The bytes are pinned by `app/golden_test.go` (files in `app/testdata/golden/`): MQTT payload, `/data` and the controller part of `/health`, for UVR42 and UVR31, with a sensor out of range and with a device name, through decoder, collector and handler. `Frame` keeps the JSON shape fixed: fields in alphabetical order, `omitempty` on the pointers so an absent sensor or `out2` stays absent rather than `null`, and a sensor at 0 °C still present because its pointer is set. A deliberate change goes through `go test ./app -run TestGolden -update` and into the README.

## Conventions

- Logging is `log/slog` with key/value pairs throughout; `slog.SetDefault` is set in `cmd`. Do not use `fmt.Print` outside pre-logger startup and `--about`/`--version`/`--help`.
- Doc comments: every package and exported symbol is documented, in English, and Swagger annotations live directly on the handlers.
- Config field docs live in `README.md` (example and reference table) and `config/config.yaml` — update both when adding a config key. `cmd/README.md` is the short `--help` text and only points to them. `README.de.md` is a short German summary.
- Commit subjects follow Conventional Commits, `type(scope): description` with an optional scope (`fix: default port 8443`, `feat(ui): …`), types `feat`, `fix`, `docu`, `chore`, `refactor`. The release changelog groups on them (`.goreleaser.yaml`), and anything unprefixed lands under "Other". Note it is `docu`, not `docs`.
- Example host names, broker addresses and users stay neutral (`mqtt.example.com`, `raspberrypi`); this repository is public.
