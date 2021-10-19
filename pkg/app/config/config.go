package config

import (
	"fmt"
	"io"
	"os"
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
	Gpio          int             `yaml:"gpio"`
	BounceTimeInt int             `yaml:"bouncetime"`
	BounceTime    time.Duration   `yaml:"-"`
	Clock         int             `yaml:"clock"`
	Flag          FlagConfig      `yaml:"-"`
	DataFile      string          `yaml:"datafile"`
	Debug         DebugConfig     `yaml:"debug"`
	Webserver     WebserverConfig `yaml:"webserver"`
	MQTT          MQTTConfig      `yaml:"mqtt"`
}

// FlagConfig defines the configured flags (parameters)
type FlagConfig struct {
	Version    bool
	Debug      string
	ConfigFile string
}

// WebserverConfig defines the struct of the webserver and webservice configuration and configuration file
type WebserverConfig struct {
	URL         string          `yaml:"url"`
	Webservices map[string]bool `yaml:"webservices"`
}

// MQTTConfig defines the struct of the mqtt client configuration and configuration file
type MQTTConfig struct {
	Connection  string        `yaml:"connection"`
	Interval    time.Duration `yaml:"-"`
	IntervalInt int           `yaml:"interval"`
	Topic       string        `yaml:"topic"`
}

// DebugConfig defines the struct of the debug configuration and configuration file
type DebugConfig struct {
	File       io.WriteCloser `yaml:"-"`
	Flag       int            `yaml:"-"`
	FlagString string         `yaml:"flag"`
	FileString string         `yaml:"file"`
}

func NewConfig() *Config {
	return &Config{
		Clock:         50,
		BounceTimeInt: 0,
		Flag:          FlagConfig{},
		Debug: DebugConfig{
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
			Connection: "tcp:127.0.0.1883",
			Interval:   5 * time.Second,
			Topic:      "/test/uvr42"},
	}
}

func (c *Config) LoadConfig() error {
	if err := c.readConfigFile(); err != nil {
		return fmt.Errorf("error reading config file %q: %w", c.Flag.ConfigFile, err)
	}

	if c.Flag.Debug != "" {
		c.Debug.FlagString = c.Flag.Debug
	}
	if err := c.setDebugConfig(); err != nil {
		return fmt.Errorf("unable to open debug file %q: %w", c.Debug, err)
	}

	c.MQTT.Interval = time.Duration(c.MQTT.IntervalInt) * time.Second
	c.BounceTime = time.Duration(c.BounceTimeInt) * time.Millisecond

	return nil
}

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

func (c *Config) setDebugConfig() (err error) {
	// defines Debug section of global.Config
	switch c.Debug.FlagString {
	case "trace", "full":
		c.Debug.Flag = debug.Full
	case "debug":
		c.Debug.Flag = debug.Warning | debug.Info | debug.Error | debug.Fatal | debug.Debug
	case "standard":
		c.Debug.Flag = debug.Standard
	}

	switch c.Debug.FileString {
	case "stderr":
		c.Debug.File = os.Stderr
	case "stdout":
		c.Debug.File = os.Stdout
	default:
		if c.Debug.File, err = os.OpenFile(c.Debug.FileString, os.O_RDWR|os.O_CREATE|os.O_APPEND, 0o666); err != nil {
			return
		}
	}

	return
}
