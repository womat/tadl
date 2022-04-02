package app

import (
	"fmt"
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

	decoder *manchester.Decoder

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
//  * check if data logger type is supported
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

	switch t := config.DataLogger.Type; t {
	case "uvr42":
		app.dl = datalogger.NewUVR42()
		app.DataFrame.data = datalogger.UVR42Frame{}
		app.mqttData.data = datalogger.UVR42Frame{}
	default:
		return &App{}, fmt.Errorf("unsupported data logger: %q", t)
	}

	return &app, err
}

// Run starts the application.
func (app *App) Run() error {
	if err := app.init(); err != nil {
		return err
	}

	go app.runWebServer()
	go app.run()

	return nil
}

// init initializes the used modules of the application:
//	* mqtt
//	* gpio pin
//	* data logger
func (app *App) init() (err error) {
	if app.chip, err = raspberry.Open(); err != nil {
		debug.ErrorLog.Printf("can't open chip: %v", err)
		return err
	}

	var params string
	if app.config.DLbus.PullUp {
		params = fmt.Sprintf("%v %s %v", app.config.DLbus.Gpio, "pullup", app.config.DLbus.DebouncePeriodInt)
	} else {
		if app.config.DLbus.PullDown {
			params = fmt.Sprintf("%v %s %v", app.config.DLbus.Gpio, "pulldown", app.config.DLbus.DebouncePeriodInt)
		} else {
			params = fmt.Sprintf("%v %s %v", app.config.DLbus.Gpio, "none", app.config.DLbus.DebouncePeriodInt)
		}
	}

	debug.DebugLog.Printf("open gpio params: %s", params)

	if app.gpio, err = app.chip.Open(params); err != nil {
		debug.ErrorLog.Printf("can't open gpio: %v", err)
		return err
	}

	//	app.gpio.DebouncePeriod(app.config.DLbus.DebouncePeriod)

	if app.mqtt, err = mqtt.New(app.config.MQTT.Connection); err != nil {
		debug.ErrorLog.Printf("can't open mqtt broker %v", err)
		return err
	}

	decoder := manchester.New(app.gpio.C)
	io := dlbus.New(decoder.C)

	if err = app.dl.Connect(io); err != nil {
		debug.ErrorLog.Printf("can't open uvr42 %v", err)
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
	_ = app.decoder.Close()
	_ = app.gpio.Close()
	_ = app.chip.Close()
	return nil
}
