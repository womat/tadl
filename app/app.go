// Package app provides the main application.
//
// It initializes tadl, handles MQTT publishing,
// web server startup, and OS signal handling...
//
// Usage:
//
//	signals := make(chan os.Signal, 1)
//	signal.Notify(signals, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
//	a, err := app.New(cfg, "/opt/tadl", signals, checkReload).Run()
//	select {
//	case <-a.Restart():  // build the next App
//	case <-a.Shutdown(): // exit
//	}
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"os"
	"strconv"
	"sync"
	"sync/atomic"
	"syscall"
	"time"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/gpio/rpi"
	"github.com/womat/golib/manchester/decoder"
	"github.com/womat/golib/mqtt"
	"github.com/womat/tadl/app/service/collector"
	"github.com/womat/tadl/app/service/errwindow"
	"github.com/womat/tadl/pkg/datalogger"
	"github.com/womat/tadl/pkg/dlbus"
)

// VERSION is the application version, following semantic versioning
// as described in https://semver.org/.
//
// It is not maintained in source: the Git tag is the single source of truth and
// the value is injected at build time via -ldflags (see Makefile and
// .goreleaser.yaml). The "dev" default applies to builds made without them.
var VERSION = "dev"

const (
	MODULE = "tadl"

	ModeStop    = 0
	ModeRestart = 1

	// minStaleAfter is the shortest age after which the last frame counts as stale.
	minStaleAfter = 30 * time.Second
)

// App is the main application struct.
// App is where the application is wired up.
type App struct {
	wg          sync.WaitGroup   // tracks the web server and the collector goroutines
	baseDir     string           // working directory
	config      *Config          // app configuration
	web         *http.Server     // HTTP server
	signals     <-chan os.Signal // OS signals, subscribed once by the caller for all lifecycles
	checkReload func() error     // loads and validates the config file before a SIGHUP restart
	serverErr   chan error       // reports a web server that stopped on its own
	restart     chan struct{}    // signals application restart
	shutdown    chan struct{}    // signals application shutdown
	ctx         context.Context
	cancelFunc  context.CancelFunc

	// pin is the GPIO input for the DL-Bus signal.
	pin gpio.Pin

	// decoder processes GPIO edge events into a Manchester-decoded bit stream.
	decoder       *decoder.Decoder
	decoderEvents chan decoder.Event
	// droppedEdges counts the GPIO edges the decoder had no room for.
	droppedEdges atomic.Uint64
	// busErrors sums the DL-Bus errors of the last 24 hours for the web UI.
	busErrors *errwindow.Window

	// dlbus ist the handler of the dlbus
	dlbus *dlbus.Handler

	datalogger        datalogger.DL
	dataloggerService *collector.Handler

	mqtt *mqtt.Handler
}

// New initializes the App struct but does not start services.
//
// signals must already be subscribed (signal.Notify) to SIGHUP, SIGTERM and SIGINT, and stay
// subscribed across restarts: a signal arriving while one App is torn down and the next is
// built then waits in the channel for the next App, instead of hitting the default action,
// which would end the process.
//
// checkReload is called on SIGHUP before anything is torn down. If it reports an error, the
// restart is refused and the App keeps running with its current configuration, so a broken
// config file cannot stop the data logger. Passing nil skips the check.
func New(config *Config, baseDir string, signals <-chan os.Signal, checkReload func() error) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		baseDir:     baseDir,
		config:      config,
		signals:     signals,
		checkReload: checkReload,
		serverErr:   make(chan error, 1),
		web: &http.Server{
			Addr: net.JoinHostPort(config.Webserver.ListenHost, strconv.Itoa(config.Webserver.ListenPort)),
		},

		restart:    make(chan struct{}),
		shutdown:   make(chan struct{}),
		ctx:        ctx,
		cancelFunc: cancel,

		decoderEvents: make(chan decoder.Event, 1024),
		busErrors:     errwindow.New(),
	}
}

// Run initializes the application, starts services,
// and the web server, and sets up OS signal handling.
func (app *App) Run() (*App, error) {
	slog.Info("Initializing application")

	if err := app.Init(); err != nil {
		return app, err
	}

	dlbusWatcher, err := app.dlbus.Watch(app.decoder.Bits())
	if err != nil {
		slog.Error("Failed to start DL-Bus watcher", "error", err)
		return app, app.abort(err)
	}

	// The logger reports sensors out of range; frames that cannot be decoded only at debug level.
	dataloggerWatcher, err := app.datalogger.Watch(dlbusWatcher, datalogger.WithLogger(slog.Default()))
	if err != nil {
		slog.Error("Failed to start data logger watcher", "error", err)
		return app, app.abort(err)
	}

	// A nil *mqtt.Handler must not be passed on as a non-nil Publisher.
	var publisher collector.Publisher
	if app.mqtt != nil {
		publisher = app.mqtt
	}
	app.wg.Go(func() {
		app.dataloggerService.Run(app.ctx, dataloggerWatcher, publisher)
	})
	if publisher != nil {
		slog.Info("Starting periodic MQTT publishing", "interval", app.config.MQTT.PublishInterval)
		app.wg.Go(func() {
			app.dataloggerService.RunPeriodicPublish(app.ctx, app.config.MQTT.PublishInterval, publisher)
		})
	}

	// Edges the decoder had no room for. They are reported with the next edge
	// that gets through, so the decoder does not take the interval across them
	// for a bit period. Only the GPIO callback touches it.
	var missed uint64
	debugEdges := slog.Default().Enabled(app.ctx, slog.LevelDebug)
	err = app.pin.WatchFunc(gpio.RisingEdge|gpio.FallingEdge,
		func(evt gpio.Event) {
			edge := decoder.FallingEdge
			if evt.Edge == gpio.RisingEdge {
				edge = decoder.RisingEdge
			}

			// Never block the GPIO callback: a full decoder drops the edge instead.
			select {
			case app.decoderEvents <- decoder.Event{Edge: edge, Time: evt.Time, Missed: evt.Missed + missed}:
				missed = 0
			default:
				missed += evt.Missed + 1
				app.droppedEdges.Add(1)
			}
			// About 1000 edges a second: skip building the arguments unless they are logged.
			if debugEdges {
				slog.Debug("GPIO Event", "pin", app.pin.Number(), "edge", evt.Edge, "time", evt.Time.Format("15:04:05.000000"), "missed", evt.Missed)
			}
		})

	if err != nil {
		slog.Error("can't watch gpio pin", "gpio", app.config.DlBus.GPIO, "error", err)
		return app, app.abort(err)
	}

	// Book the bus errors in the 24 h window once a minute.
	app.wg.Go(func() {
		ticker := time.NewTicker(time.Minute)
		defer ticker.Stop()
		for {
			select {
			case <-app.ctx.Done():
				return
			case <-ticker.C:
				app.busErrors.Observe(app.busErrorCounts())
			}
		}
	})

	// handle the OS signals
	app.HandleOSSignals()

	slog.Info("Starting web server", "url", app.web.Addr)
	err = app.StartWebServer()
	if err != nil {
		slog.Error("Web server failed to start", "url", app.web.Addr, "error", err)
		return app, app.abort(err)
	}

	slog.Info("Module started successfully",
		"module", MODULE,
		"version", VERSION,
		"pid", os.Getpid(),
	)
	return app, nil
}

// Init prepares the application:
// - initialize serives
// - initializes API routes
func (app *App) Init() error {
	var err error

	if broker := app.config.MQTT.Connection; broker == "" {
		slog.Info("MQTT disabled, no broker configured")
	} else {
		// The broker URL may carry credentials (tcp://user:password@host); never log them.
		logBroker := redactURL(broker)
		hostname, _ := os.Hostname()
		clientID := MODULE + "-" + hostname

		slog.Info("Connecting to MQTT broker", "broker", logBroker, "clientID", clientID)
		// New never fails: a broker that cannot be reached is retried in the background.
		app.mqtt, _ = mqtt.New(broker, clientID,
			mqtt.WithLogger(slog.Default()),
			mqtt.WithOnConnected(func() {
				slog.Info("MQTT connected", "broker", logBroker)
			}),
			mqtt.WithOnConnectionLost(func(err error) {
				slog.Warn("MQTT connection lost", "broker", logBroker, "error", err)
			}))
	}

	options := []rpi.Option{
		rpi.WithMode(gpio.Input),
		rpi.WithDebounce(app.config.DlBus.DebounceTime)}

	switch app.config.DlBus.GPIOTermination {
	case "pullup":
		options = append(options, rpi.WithPullup(gpio.PullUp))
	case "pulldown":
		options = append(options, rpi.WithPullup(gpio.PullDown))
	}

	if app.pin, err = rpi.NewPin(app.config.DlBus.GPIO, options...); err != nil {
		slog.Error("can't open gpio pin", "gpio", app.config.DlBus.GPIO, "error", err)
		return err
	}

	// Start the Manchester decoder. A bit clock of 0 recovers the clock from
	// the signal, so any controller is read whatever its bit rate. The
	// convention stays IEEE: an inverted line (optocoupler) is detected by the
	// DL-Bus handler at the SYNC.
	app.decoder, err = decoder.New(app.decoderEvents,
		app.config.DlBus.BitClock,
		decoder.WithManchesterEncoding(decoder.IEEE))
	if err != nil {
		slog.Error("Failed to start Manchester decoder", "error", err)
		return err
	}
	if app.config.DlBus.BitClock == 0 {
		slog.Info("Manchester decoder recovers the bit clock from the signal")
	} else {
		slog.Info("Manchester decoder uses a fixed bit clock", "bitClock", app.config.DlBus.BitClock)
	}

	app.dlbus = dlbus.New(dlbus.WithLogger(slog.Default()))

	var typ int
	switch t := app.config.DataLogger.Type; t {
	case "uvr42":
		app.datalogger = datalogger.NewUVR42()
		typ = datalogger.UVR42
	case "uvr31":
		app.datalogger = datalogger.NewUVR31()
		typ = datalogger.UVR31
	default:
		slog.Error("Unsupported data logger", "type", t)
		return fmt.Errorf("unsupported data logger type: %q", t)
	}
	app.dataloggerService = collector.New(collector.Config{
		PublishInterval: app.config.MQTT.PublishInterval,
		MinDeltaTemp:    app.config.MQTT.MinDeltaTemp,
		Topic:           app.config.MQTT.TopicPrefix,
		Retained:        app.config.MQTT.Retained,
		Device:          app.config.DataLogger.DeviceName(),
		StaleAfter:      max(3*app.config.MQTT.PublishInterval, minStaleAfter),
	},
		typ)

	// initRoutes should always be called at the end
	slog.Debug("Initializing API routes")
	app.SetupRoutes()

	return nil
}

// Restart returns a read-only channel for restart signals.
func (app *App) Restart() <-chan struct{} {
	return app.restart
}

// Shutdown returns a read-only channel for shutdown signals.
func (app *App) Shutdown() <-chan struct{} {
	return app.shutdown
}

// HandleOSSignals handles SIGHUP (restart), SIGTERM and SIGINT (stop) from app.signals, and
// restarts the App when the web server stopped on its own.
//
// The subscription itself belongs to the caller and outlives this App, so nothing here
// stops or resets it; one goroutine per App consumes at most one signal. Being the only
// caller of shutdownProcedure, it also rules out two shutdowns running at once.
func (app *App) HandleOSSignals() {

	go func() {
		slog.Debug("Starting signal handler")

		// Use select instead of a plain channel receive so the goroutine has
		// two exit paths and always terminates cleanly:
		//   - a signal or a server error is received and handled, or
		//   - the context is cancelled externally (e.g. from a concurrent shutdown).
		// Without the second path the goroutine would outlive its App and take
		// the next signal away from the App that replaced it. The loop only
		// continues after a SIGHUP whose config was rejected.
		for {
			select {
			case receivedSignal := <-app.signals:
				slog.Info("Received OS signal", "signal", receivedSignal)
				switch receivedSignal {
				case syscall.SIGHUP:
					if app.checkReload != nil {
						if err := app.checkReload(); err != nil {
							slog.Error("Config reload rejected, keeping the running configuration", "error", err)
							continue
						}
					}
					slog.Info("SIGHUP received, initiating restart")
					app.shutdownProcedure(ModeRestart)
				case syscall.SIGTERM, syscall.SIGINT:
					slog.Info("SIGTERM/SIGINT received, stopping")
					app.shutdownProcedure(ModeStop)
				}
				return
			case err := <-app.serverErr:
				slog.Error("Web server stopped unexpectedly, initiating restart", "error", err)
				app.shutdownProcedure(ModeRestart)
				return
			case <-app.ctx.Done():
				// Context was cancelled externally – exit without triggering
				// a second shutdown procedure.
				slog.Debug("Signal handler: context cancelled, exiting goroutine")
				return
			}
		}
	}()
}

// shutdownProcedure gracefully stops or restarts the app based on mode.
//   - ModeStop: graceful shutdown the web server, Cleanup app resources and exit the application.
//   - ModeRestart: graceful shutdown the web server and Cleanup app resources and restart the application.
func (app *App) shutdownProcedure(mode int) {
	slog.Info("Initiating shutdown", "mode", mode)

	// cancel the application context to stop all running goroutines
	app.cancelFunc()
	// Wait for the web server and the collector goroutines, so MQTT is not
	// disconnected in Cleanup while a publish is still in flight.
	app.wg.Wait()

	if err := app.Cleanup(); err != nil {
		slog.Error("Cleanup failed", "error", err)
	}

	switch mode {
	case ModeRestart:
		slog.Info("Shutdown complete, restarting")
		app.restart <- struct{}{}
		// Channels are intentionally left open: cmd/main.go receives the restart
		// signal and calls New(), which creates fresh channels for the next lifecycle.
	case ModeStop:
		slog.Info("Module stopped", "module", MODULE, "version", VERSION, "pid", os.Getpid())
		app.shutdown <- struct{}{}
		close(app.shutdown)
	}

}

// Cleanup releases all application resources in the correct order.
// It is called on shutdown and restart, after the context has been cancelled
// and the web server has stopped.
func (app *App) Cleanup() error {
	var errs error

	slog.Info("Stopping GPIO watch", "gpio", app.config.DlBus.GPIO)
	if err := app.pin.StopWatching(); err != nil {
		errs = errors.Join(errs, err)
	}

	slog.Info("Closing DL-Bus decoder")
	if err := app.dlbus.Close(); err != nil {
		errs = errors.Join(errs, err)
	}

	slog.Info("Closing data logger")
	if err := app.datalogger.Close(); err != nil {
		errs = errors.Join(errs, err)
	}

	slog.Info("Closing Manchester decoder")
	if err := app.decoder.Close(); err != nil {
		errs = errors.Join(errs, err)
	}

	slog.Info("Closing GPIO pin")
	if err := app.pin.Close(); err != nil {
		errs = errors.Join(errs, err)
	}

	if app.mqtt != nil {
		slog.Info("Disconnecting from MQTT broker")
		app.mqtt.Disconnect()
	}

	return errs
}

// abort stops what Run has started so far after a start failure and returns err.
// The caller exits, so the GPIO pin and the MQTT connection are released here.
func (app *App) abort(err error) error {
	app.cancelFunc()
	app.wg.Wait()
	if cerr := app.Cleanup(); cerr != nil {
		slog.Error("Cleanup failed", "error", cerr)
	}
	return err
}

// redactURL returns broker with a password replaced by "xxxxx", for logging.
func redactURL(broker string) string {
	u, err := url.Parse(broker)
	if err != nil {
		return "<unparsable broker URL>"
	}
	return u.Redacted()
}
