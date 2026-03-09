// Package app provides the main application.
//
// It initializes tadl, handles MQTT publishing,
// web server startup, and OS signal handling...
//
// Usage:
//
//	config := LoadConfig()
//	app := app.New(config, "/opt/tadl")
//	app.Run()
package app

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strconv"
	"sync"
	"syscall"
	dataloggerservice "tadl/app/service/collector"
	"tadl/pkg/datalogger"
	"tadl/pkg/dlbus"
	"time"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/gpio/rpi"
	"github.com/womat/golib/manchester/decoder"
	"github.com/womat/golib/mqtt"
)

// VERSION holds the version information with the following logic in mind
//
//	4 ... fixed
//	0 ... year 2020, 1->year 2021, etc.
//	7 ... month of year (7=July)
//	the date format after the + is always the first of the month
//
// VERSION differs from semantic versioning as described in https://semver.org/
// but we keep the correct syntax.
// TODO: increase version number
const (
	VERSION = "1.6.2+20260228"
	MODULE  = "tadl"

	ModeStop    = 0
	ModeRestart = 1
)

// App is the main application struct.
// App is where the application is wired up.
type App struct {
	wg         sync.WaitGroup // wait group to track running webserver
	baseDir    string         // working directory
	config     *Config        // app configuration
	web        *http.Server   // HTTP server
	restart    chan struct{}  // signals application restart
	shutdown   chan struct{}  // signals application shutdown
	ctx        context.Context
	cancelFunc context.CancelFunc

	// pin is the GPIO input for the DL-Bus signal.
	pin gpio.Pin

	// decoder processes GPIO edge events into a Manchester-decoded bit stream.
	decoder       *decoder.Decoder
	decoderEvents chan decoder.Event

	// dlbus ist the handler of the dlbus
	dlbus *dlbus.Handler

	datalogger        datalogger.DL
	dataloggerService *dataloggerservice.Handler

	mqtt *mqtt.Handler
}

// New initializes the App struct but does not start services.
func New(config *Config, baseDir string) *App {
	ctx, cancel := context.WithCancel(context.Background())

	return &App{
		baseDir: baseDir,
		config:  config,
		web: &http.Server{
			Addr: net.JoinHostPort(config.Webserver.ListenHost, strconv.Itoa(config.Webserver.ListenPort)),
		},

		restart:    make(chan struct{}),
		shutdown:   make(chan struct{}),
		ctx:        ctx,
		cancelFunc: cancel,

		decoderEvents: make(chan decoder.Event, 1024),
	}
}

// Run initializes the application, starts services,
// and the web server, and sets up OS signal handling.
func (app *App) Run() (*App, error) {
	slog.Info("Initializing application")

	if err := app.Init(); err != nil {
		return app, err
	}

	dlbusWatcher, err := app.dlbus.Watch(app.decoder.C)
	if err != nil {
		slog.Error("Failed to start DL-Bus watcher", "error", err)
		return app, err
	}

	// here start your services
	var options []datalogger.Option
	if app.config.LogLevel == "debug" {
		options = append(options, datalogger.WithLogger(slog.Default()))
	}

	dataloggerWatcher, err := app.datalogger.Watch(dlbusWatcher, options...)
	if err != nil {
		slog.Error("Failed to start data logger watcher", "error", err)
		return app, err
	}
	app.dataloggerService.Run(app.ctx, dataloggerWatcher, app.mqtt)
	app.dataloggerService.StartPeriodicPublish(app.ctx, time.Duration(app.config.MQTT.PublishInterval)*time.Second, app.mqtt)
	err = app.pin.WatchFunc(app.ctx,
		gpio.RisingEdge|gpio.FallingEdge,
		func(evt gpio.Event) {
			switch evt.Edge {
			case gpio.RisingEdge:
				app.decoderEvents <- decoder.Event{Edge: decoder.RisingEdge, Time: evt.Time}
			case gpio.FallingEdge:
				app.decoderEvents <- decoder.Event{Edge: decoder.FallingEdge, Time: evt.Time}
			}
			slog.Debug("GPIO Event", "pin", app.pin.Number(), "edge", evt.Edge, "time", evt.Time.Format("15:04:05.000000"))
		})

	if err != nil {
		slog.Error("can't watch gpio pin", "gpio", app.config.DlBus.GPIO, "error", err)
	}

	// handle the OS signals
	app.HandleOSSignals()

	slog.Info("Starting web server", "url", app.web.Addr)
	err = app.StartWebServer()
	if err != nil {
		slog.Error("Web server failed to start", "url", app.web.Addr, "error", err)
		return app, err
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

	if app.mqtt, err = mqtt.New(app.config.MQTT.Connection, MODULE,
		mqtt.WithOnConnected(func() {}),
		mqtt.WithOnConnectionLost(func(err error) {})); err != nil {
		slog.Error("Failed to connect to MQTT broker", "broker", app.config.MQTT.Connection, "error", err)
		return err
	}

	options := []rpi.Option{
		rpi.WithMode(gpio.Input),
		rpi.WithDebounce(time.Duration(app.config.DlBus.BounceTime) * time.Millisecond)}

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

	// start manchaster decoder
	app.decoder = decoder.New(app.decoderEvents,
		app.config.DlBus.BitClock,
		decoder.WithManchesterEncoding(decoder.IEEE))

	app.dlbus = dlbus.New()

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
	app.dataloggerService = dataloggerservice.New(dataloggerservice.Config{
		PublishInterval: time.Duration(app.config.MQTT.PublishInterval) * time.Second,
		MinDeltaTemp:    app.config.MQTT.MinDeltaTemp,
		Topic:           app.config.MQTT.TopicPrefix,
		Retained:        app.config.MQTT.Retained,
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

// HandleOSSignals listens for SIGHUP, SIGTERM, and SIGINT signals.
func (app *App) HandleOSSignals() {

	go func() {
		sig := make(chan os.Signal, 1)
		signal.Notify(sig, syscall.SIGHUP, syscall.SIGTERM, syscall.SIGINT)
		defer signal.Stop(sig) // Cleanup: rollback signal.Notify

		slog.Debug("Starting signal handler")

		// Use select instead of a plain channel receive so the goroutine has
		// two exit paths and always terminates cleanly:
		//   - a signal is received and handled, or
		//   - the context is cancelled externally (e.g. from a concurrent shutdown).
		// Without this, the goroutine would block forever after signal.Reset()
		// on a SIGHUP restart, leaking one goroutine per reload cycle.
		select {
		case receivedSignal := <-sig:
			slog.Info("Received OS signal", "signal", receivedSignal)
			switch receivedSignal {
			case syscall.SIGHUP:
				slog.Info("SIGHUP received, initiating restart")
				app.shutdownProcedure(ModeRestart)
			case syscall.SIGTERM, syscall.SIGINT:
				slog.Info("SIGTERM/SIGINT received, stopping")
				app.shutdownProcedure(ModeStop)
			}
		case <-app.ctx.Done():
			// Context was cancelled externally – exit without triggering
			// a second shutdown procedure.
			slog.Debug("Signal handler: context cancelled, exiting goroutine")
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
	app.wg.Wait() //wait for the web server to shutdown before cleaning up resources

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
