package config

import (
	"fmt"
	"io"
	"os"
	"strings"
	"time"

	"github.com/womat/debug"
	"gopkg.in/yaml.v2"
)

// Config holds the application configuration. Attention!
// To make it possible to overwrite fields with the -overwrite command
// line option each of the struct fields must be in the format
// first letter uppercase -> followed by CamelCase as in the config file.
// Config defines the struct of global config and the struct of the configuration file
type Config struct {
	Flag       FlagConfig       `yaml:"-"`
	DataLogger DataLoggerConfig `yaml:"datalogger"`
	DLbus      DLbusConfig      `yaml:"dlbus"`
	MQTT       MQTTConfig       `yaml:"mqtt"`
	Webserver  WebserverConfig  `yaml:"webserver"`
	Log        LogConfig        `yaml:"log"`
}

// FlagConfig defines the configured command line flags (parameters).
type FlagConfig struct {
	LogLevel   string `json:"LogLevel,omitempty" yaml:"LogLevel,omitempty"`
	ConfigFile string `json:"Config,omitempty" yaml:"Config,omitempty"`
}

// WebserverConfig defines the struct of the webserver and webservice configuration.
type WebserverConfig struct {
	URL         string          `yaml:"url"`
	Webservices map[string]bool `yaml:"webservices"`
}

// MQTTConfig defines the struct of the mqtt client configuration.
type MQTTConfig struct {
	Connection  string        `yaml:"connection"`
	Interval    time.Duration `yaml:"-"`
	IntervalInt int           `yaml:"interval"`
	DeltaKelvin float64       `yaml:"deltakelvin"`
	Topic       string        `yaml:"topic"`
}

// LogConfig defines the struct of the debug configuration and configuration file.
type LogConfig struct {
	File       io.WriteCloser `yaml:"-"`
	Flag       int            `yaml:"-"`
	FlagString string         `yaml:"flag"`
	FileString string         `yaml:"file"`
}

// DataLoggerConfig defines the struct of the Data Logger.
type DataLoggerConfig struct {
	Type string `yaml:"type"`
}

// DLbusConfig defines the struct of the dl-bus configuration.
type DLbusConfig struct {
	Gpio              int  `yaml:"gpio"`
	DebouncePeriodInt int  `yaml:"debounceperiod"`
	PullUp            bool `yaml:"pullup"`
	PullDown          bool `yaml:"pulldown"`
}

// NewConfig create the structure of the application configuration.
func NewConfig() *Config {
	return &Config{
		DataLogger: DataLoggerConfig{
			Type: "UVR4",
		},
		DLbus: DLbusConfig{},
		Flag:  FlagConfig{},
		Log: LogConfig{
			FileString: "stderr",
			FlagString: "standard",
		},
		Webserver: WebserverConfig{
			URL: "http://0.0.0.0:4000",
			Webservices: map[string]bool{
				"version": true,
				"health":  true,
				"data":    true,
			},
		},
		MQTT: MQTTConfig{
			Connection:  "tcp:127.0.0.1883",
			IntervalInt: 5,
			DeltaKelvin: 0.5,
			Topic:       "/test/uvr42"},
	}
}

// LoadConfig reads the config file and set the application configuration.
func (c *Config) LoadConfig() error {
	if err := c.readConfigFile(); err != nil {
		return fmt.Errorf("error reading config file %q: %w", c.Flag.ConfigFile, err)
	}

	if c.Flag.LogLevel != "" {
		c.Log.FlagString = c.Flag.LogLevel
	}
	if err := c.setDebugConfig(); err != nil {
		return fmt.Errorf("unable to open debug file %q: %w", c.Log, err)
	}

	c.MQTT.Interval = time.Duration(c.MQTT.IntervalInt) * time.Second

	switch l := c.DataLogger.Type; l {
	case "uvr42":
	default:
		return fmt.Errorf("unsupported Datalogger: %q: ", l)
	}

	return nil
}

// readConfigFile read the configuration File and store the content to the config structure.
func (c *Config) readConfigFile() error {
	file, err := os.Open(c.Flag.ConfigFile)
	if err != nil {
		return err
	}
	defer func() { _ = file.Close() }()

	decoder := yaml.NewDecoder(file)
	if err = decoder.Decode(c); err != nil {
		return err
	}

	return nil
}

// setDebugConfig translate the log parameter to values of the debug module and open the log file.
func (c *Config) setDebugConfig() (err error) {
	switch s := strings.ToLower(c.Log.FlagString); s {
	case "trace", "full":
		c.Log.Flag = debug.Full
	case "debug":
		c.Log.Flag = debug.Fatal | debug.Info | debug.Error | debug.Warning | debug.Debug
	case "warning", "standard":
		c.Log.Flag = debug.Fatal | debug.Info | debug.Error | debug.Warning
	case "error":
		c.Log.Flag = debug.Fatal | debug.Info | debug.Error
	case "info":
		c.Log.Flag = debug.Fatal | debug.Info
	case "fatal":
		c.Log.Flag = debug.Fatal
	}

	switch c.Log.FileString {
	case "stderr":
		c.Log.File = os.Stderr
	case "stdout":
		c.Log.File = os.Stdout
	default:
		if c.Log.File, err = os.OpenFile(c.Log.FileString, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666); err != nil {
			return
		}
	}

	return
}
