package app

import (
	"net/url"
	"tadl/pkg/app/config"
	"tadl/pkg/dlbus"
	"tadl/pkg/mqtt"
	"tadl/pkg/raspberry"

	"github.com/gofiber/fiber/v2"
	"github.com/womat/debug"
)

// App is the main application struct.
// App is where the application is wired up.
type App struct {
	// web is the fiber web framework instance
	web *fiber.App

	// config is the application configuration
	config *config.Config

	// urlParsed contains the parsed Config.Url parameter
	// and makes it easier to get params out of e.g.
	// url: https://0.0.0.0:7844/?minTls=1.2&bodyLimit=50MB
	urlParsed *url.URL

	// mqtt is the handler to the mqtt broker
	mqtt *mqtt.Handler

	// gpio is the handler to the rpi gpio memory
	gpio        raspberry.GPIO
	lineHandler raspberry.Pin

	dl *dlbus.Handler

	// restart signals application restart
	restart chan struct{}
	// shutdown signals application shutdown
	shutdown chan struct{}
}

// New checks the Web server URL and initialize the main app structure
func New(config *config.Config) (*App, error) {
	u, err := url.Parse(config.Webserver.URL)
	if err != nil {
		debug.ErrorLog.Printf("Error parsing url %q: %s", config.Webserver.URL, err.Error())
		return &App{}, err
	}

	gpio, err := raspberry.Open()
	if err != nil {
		debug.ErrorLog.Printf("can't open gpio: %v", err)
		return &App{}, err
	}

	return &App{
		config:    config,
		urlParsed: u,
		gpio:      gpio,

		web:  fiber.New(),
		mqtt: mqtt.New(),
		dl:   dlbus.New(),

		restart:  make(chan struct{}),
		shutdown: make(chan struct{}),
	}, err
}

// Run starts the application.
func (app *App) Run() error {
	if err := app.init(); err != nil {
		return err
	}

	go testPinEmu(app.lineHandler)
	go app.mqtt.Service()
	go app.runWebServer()
	go app.dl.Service()

	return nil
}

// init initializes the application.
func (app *App) init() (err error) {

	if app.lineHandler, err = app.gpio.NewPin(app.config.Gpio); err != nil {
		debug.ErrorLog.Printf("can't open pin: %v", err)
		return
	}

	app.lineHandler.Input()
	app.lineHandler.PullUp()
	app.lineHandler.SetBounceTime(app.config.BounceTime)

	if err = app.dl.Connect(app.lineHandler, app.config.Frequency); err != nil {
		debug.ErrorLog.Printf("can't open dl: %v", err)
		return err
	}

	// call handler when pin changes from low to high.
	if err = app.lineHandler.Watch(raspberry.EdgeFalling, app.handler); err != nil {
		debug.ErrorLog.Printf("can't open watcher: %v", err)
		return err
	}

	if err = app.mqtt.Connect(app.config.MQTT.Connection); err != nil {
		debug.ErrorLog.Printf("can't open mqtt broker %v", err)
		return err
	}

	// initRoutes and initDefaultRoutes should be always called last because it may access things like app.api
	// which must be initialized before in initAPI()
	app.initDefaultRoutes()

	return nil
}

// Restart returns the read only restart channel.
// Restart is used to be able to react on application restart. (see cmd/main.go)
func (app *App) Restart() <-chan struct{} {
	return app.restart
}

// Shutdown returns the read only shutdown channel.
// Shutdown is used to be able to react on application shutdown. (see cmd/main.go)
func (app *App) Shutdown() <-chan struct{} {
	return app.shutdown
}

func (app *App) Close() error {
	// app.chip.Close() unwatch all pins and release the gpio memory!
	_ = app.gpio.Close()

	if app.mqtt != nil {
		_ = app.mqtt.Disconnect()
	}

	return nil
}
