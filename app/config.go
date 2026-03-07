package app

import (
	"errors"
	"fmt"
	"os"
	"slices"

	"gopkg.in/yaml.v3"
)

const (
	ProdEnv = "prod"
	DevEnv  = "dev"
)

// Config holds the main application configuration.
type Config struct {
	Env            string           `yaml:"env"`            // Application environment: dev | prod
	LogLevel       string           `yaml:"logLevel"`       // Log level: debug | info | warning | error
	LogDestination string           `yaml:"logDestination"` // Log output: stdout | stderr | /path/to/logfile
	Webserver      WebserverConfig  `yaml:"webserver"`      // Webserver configuration
	MQTT           MQTTConfig       `yaml:"mqtt"`           // MQTT client configuration
	DlBus          DlBusConfig      `yaml:"dlbus"`          // DL-Bus configuration
	DataLogger     DataLoggerConfig `yaml:"datalogger"`     // Data logger configuration

}

// WebserverConfig holds HTTPS server settings.
type WebserverConfig struct {
	ListenHost string   `yaml:"listenHost"` // Host address for web server
	ListenPort int      `yaml:"listenPort"` // Port for web server
	ApiKey     string   `yaml:"apiKey"`     // API key for requests
	JwtSecret  string   `yaml:"jwtSecret"`  // Secret for JWT tokens
	JwtID      string   `yaml:"jwtID"`      // Unique JWT ID
	KeyFile    string   `yaml:"keyFile"`    // SSL private key file
	CertFile   string   `yaml:"certFile"`   // SSL certificate file
	BlockedIPs []string `yaml:"blockedIPs"` // Forbidden IP addresses or networks
	AllowedIPs []string `yaml:"allowedIPs"` // Allowed IP addresses or networks
}

type MQTTConfig struct {
	Connection      string  `yaml:"connection"`      // Broker connection string
	Retained        bool    `yaml:"retained"`        // Whether messages are retained
	PublishInterval int     `yaml:"publishInterval"` // publish interval in seconds
	TopicPrefix     string  `yaml:"topicPrefix"`     // MQTT topic prefix for meter data
	MinDeltaTemp    float64 `yaml:"minDeltaTemp"`    // Minimum temperature change in Kelvin to trigger an update
}

type DlBusConfig struct {
	GPIO            int    `yaml:"gpio"`            // GPIO pin for DL-Bus input
	BounceTime      int    `yaml:"bounceTime"`      // Debounce in ms
	GPIOTermination string `yaml:"gpioTermination"` // Termination type for GPIO (e.g. "pullup", "pulldown", "none")
	BitClock        int    `yaml:"bitClock"`        // DL-Bus bit clock frequency in Hz (used for timing), 0 = auto-detect
}

type DataLoggerConfig struct {
	Type string `yaml:"type"` // Type of data logger Technische Alternative uvr42
}

// NewConfig returns a Config with sane defaults
func NewConfig() *Config {
	return &Config{
		Env:            DevEnv,
		LogLevel:       "info",
		LogDestination: "stdout",
		Webserver: WebserverConfig{
			ListenHost: "0.0.0.0",
			ListenPort: 8443,
			BlockedIPs: []string{},
			AllowedIPs: []string{},
		},
		MQTT: MQTTConfig{
			Connection:      "", // e.g. "tcp://mqtt.example.com:1883", empty means MQTT is disabled
			PublishInterval: 10,
		},
		DlBus: DlBusConfig{
			BounceTime:      0,
			GPIOTermination: "pullup",
		},
		DataLogger: DataLoggerConfig{
			Type: "uvr42",
		},
	}
}

// LoadConfig loads configuration from a YAML file and expands environment variables.
func LoadConfig(fileName string) (*Config, error) {
	cfg := NewConfig()

	fileInfo, err := os.Stat(fileName)
	if err != nil {
		return cfg, err
	}
	if fileInfo.IsDir() {
		return cfg, errors.New("config path is a directory, not a file")
	}

	content, err := os.ReadFile(fileName)
	if err != nil {
		return cfg, err
	}

	// Replace environment variables in the YAML
	replaced := os.ExpandEnv(string(content))

	// Unmarshal YAML into the config struct
	if err = yaml.Unmarshal([]byte(replaced), cfg); err != nil {
		return cfg, fmt.Errorf("failed to unmarshal config: %w", err)
	}

	return cfg, nil
}

// IsDevEnv returns true if the environment is development.
func (c *Config) IsDevEnv() bool {
	return c.Env == DevEnv
}

// Validate checks the Config for invalid or missing values.
func (c *Config) Validate() error {

	if c.Env != ProdEnv && c.Env != DevEnv {
		return fmt.Errorf("invalid environment: %s, must be %s or %s", c.Env, ProdEnv, DevEnv)
	}

	if c.Webserver.ApiKey == "" {
		return errors.New("ApiKey is not configured")
	}

	validLogLevels := []string{"debug", "info", "warning", "warn", "error"}
	if !slices.Contains(validLogLevels, c.LogLevel) {
		return fmt.Errorf("invalid log level: %s, must be one of %v", c.LogLevel, validLogLevels)
	}

	if c.Webserver.ListenPort < 1 || c.Webserver.ListenPort > 65535 {
		return fmt.Errorf("invalid port: %d", c.Webserver.ListenPort)
	}

	if c.MQTT.PublishInterval <= 0 {
		return fmt.Errorf("dataCollectionInterval must be greater than 0, got %v", c.MQTT.PublishInterval)
	}

	if c.MQTT.TopicPrefix == "" {
		return fmt.Errorf("mqtt topicPrefix must be configured")
	}

	if c.MQTT.MinDeltaTemp < 0 {
		return fmt.Errorf("mqtt minDelta must be non-negative, got %v", c.MQTT.MinDeltaTemp)
	}

	validGPIOs := []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}
	if !slices.Contains(validGPIOs, c.DlBus.GPIO) {
		return fmt.Errorf("invalid gpio pin: %d, must be a valid Raspberry Pi BCM GPIO", c.DlBus.GPIO)
	}

	if c.DlBus.BounceTime < 0 {
		return fmt.Errorf("BounceTime must be greater than 0, got %v", c.DlBus.BounceTime)
	}

	if c.DlBus.BitClock < 0 {
		return fmt.Errorf("bitClock must be non-negative, got %v", c.DlBus.BitClock)
	}

	validGPIOTerminations := []string{"pullup", "pulldown", "none"}
	if !slices.Contains(validGPIOTerminations, c.DlBus.GPIOTermination) {
		return fmt.Errorf("invalid gpioTermination: %s, must be one of %v", c.DlBus.GPIOTermination, validGPIOTerminations)
	}

	validDataLoggerTypes := []string{"uvr42", "uvr31"}
	if !slices.Contains(validDataLoggerTypes, c.DataLogger.Type) {
		return fmt.Errorf("invalid data logger type: %s, must be one of %v", c.DataLogger.Type, validDataLoggerTypes)
	}

	return nil
}
