package app

import (
	"net/url"
	"sync"
	"tadl/pkg/app/config"
	"tadl/pkg/datalogger"
	"tadl/pkg/dlbus"
	"tadl/pkg/manchester"
	"tadl/pkg/raspberry"

	"github.com/gofiber/fiber/v2"
	"github.com/womat/debug"
	"github.com/womat/mqtt"
)

// App is the main application struct and where the application is wired up.
type App struct {
	// web is the fiber web framework instance
	web *fiber.App

	// config contain the application configuration.
	config *config.Config

	// urlParsed contains the parsed Config.Url parameter
	// and makes it easier to get params out of e.g.
	//  url: https://0.0.0.0:7844/?minTls=1.2&bodyLimit=50MB
	urlParsed *url.URL

	// PublisherSubscriber is the handler to the mqtt broker.
	mqtt mqtt.PublisherSubscriber

	// chip is the handler to the rpi gpio memory.
	chip *raspberry.Chip

	// gpio is the handler to the rpi gpio.
	gpio *raspberry.Line

	// decoder ist the handler of the manchester decoder
	decoder *manchester.Decoder

	// dlbus ist the handler of the dlbus
	dlbus *dlbus.ReadCloser
	//ReadCloser

	// dl is the handler to the data logger.
	dl datalogger.DL

	// DataFrame contains the last read data frame of uvr42.
	DataFrame struct {
		sync.Mutex
		data interface{}
	}

	// mqttData contains the last sent data frame to mqtt.
	mqttData struct {
		sync.Mutex
		data interface{}
	}

	// restart signals application restart.
	restart chan struct{}
	// shutdown signals application shutdown.
	shutdown chan struct{}
}

// New checks the Web server URL and initialize the main app structure
func New(config *config.Config) (*App, error) {
	u, err := url.Parse(config.Webserver.URL)
	if err != nil {
		debug.ErrorLog.Printf("Error parsing url %q: %s", config.Webserver.URL, err.Error())
		return &App{}, err
	}

	app := App{
		config:    config,
		urlParsed: u,
		web:       fiber.New(),
		restart:   make(chan struct{}),
		shutdown:  make(chan struct{}),
	}

	return &app, err
}

// Run starts the application.
func (app *App) Run() error {
	if err := app.init(); err != nil {
		return err
	}

	go app.runWebServer()

	// receive data frames from datalogger and sent it to mqtt broker
	go app.run()

	return nil
}

// init initializes the used modules of the application:
//  * check if data logger type is supported
//	* mqtt
//	* gpio pin
//	* data logger
func (app *App) init() (err error) {
	// initialize gpio
	if app.chip, err = raspberry.Open(); err != nil {
		debug.ErrorLog.Printf("can't open chip: %v", err)
		return err
	}

	// requests control of gpio pin
	if app.gpio, err = app.chip.NewLine(app.config.DLbus.Gpio, app.config.DLbus.Terminator, app.config.DLbus.DebouncePeriod); err != nil {
		debug.ErrorLog.Printf("can't open to gpio: %v", err)
		return err
	}

	// start manchaster decoder
	decoder := manchester.New(app.gpio.C)

	// start dlbus decoder
	app.dlbus = dlbus.NewReader(decoder.C)

	// initialize datalogger reader
	switch t := app.config.DataLogger.Type; t {
	case "uvr42":
		app.dl = datalogger.NewUVR42()
		app.DataFrame.data = datalogger.UVR42Frame{}
		app.mqttData.data = datalogger.UVR42Frame{}
	default:
		debug.ErrorLog.Printf("unsupported data logger: %q", t)
	}

	// start datenlogger reader
	if err = app.dl.Connect(app.dlbus); err != nil {
		debug.ErrorLog.Printf("can't open uvr42 %v", err)
		return err
	}

	// initialize mqtt handler and connect to mqtt broker
	if app.mqtt, err = mqtt.New(app.config.MQTT.Connection); err != nil {
		debug.ErrorLog.Printf("can't open mqtt broker %v", err)
		return err
	}

	// initRoutes and initDefaultRoutes should be always called last because it may access things like app.api
	// which must be initialized before in initAPI()
	app.initDefaultRoutes()

	return nil
}

// Restart returns the read only restart channel.
//  It is used to be able to react on application restart (see cmd/main.go).
func (app *App) Restart() <-chan struct{} {
	return app.restart
}

// Shutdown returns the read only shutdown channel.
//  It is used to be able to react on application shutdown (see cmd/main.go).
func (app *App) Shutdown() <-chan struct{} {
	return app.shutdown
}

// Close all handler used by app:
//  * mqtt
//  * data logger
//  * gpio
func (app *App) Close() error {
	_ = app.mqtt.Close()
	_ = app.dl.Close()
	_ = app.dlbus.Close()
	//_ = app.decoder.Close()
	_ = app.gpio.Close()
	_ = app.chip.Close()

	return nil
}
