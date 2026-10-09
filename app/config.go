package app

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"
	"regexp"
	"slices"
	"strings"
	"time"

	"github.com/womat/tadl/pkg/datalogger"
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
	MQTT           *MQTTConfig      `yaml:"mqtt"`           // MQTT client configuration; nil = no MQTT
	DlBus          DlBusConfig      `yaml:"dlbus"`          // DL-Bus configuration
	DataLogger     DataLoggerConfig `yaml:"datalogger"`     // Data logger configuration

}

// WebserverConfig holds HTTPS server settings.
type WebserverConfig struct {
	ListenHost string   `yaml:"listenHost"` // Host address for web server
	ListenPort int      `yaml:"listenPort"` // Port for web server
	ApiKey     string   `yaml:"apiKey"`     // API key for requests
	KeyFile    string   `yaml:"keyFile"`    // SSL private key file
	CertFile   string   `yaml:"certFile"`   // SSL certificate file
	BlockedIPs []string `yaml:"blockedIPs"` // Forbidden IP addresses or networks
	AllowedIPs []string `yaml:"allowedIPs"` // Allowed IP addresses or networks
}

// MQTTConfig holds MQTT client settings. MQTT is on when the mqtt block is present.
type MQTTConfig struct {
	Connection      string        `yaml:"connection"`      // Broker connection string, required
	Retained        bool          `yaml:"retained"`        // Whether messages are retained
	PublishInterval time.Duration `yaml:"publishInterval"` // publish interval in Go duration string (default 10s)
	TopicPrefix     string        `yaml:"topicPrefix"`     // MQTT topic prefix for meter data, required
	MinDeltaTemp    float64       `yaml:"minDeltaTemp"`    // Minimum temperature change in Kelvin to trigger an update
}

// defaultPublishInterval applies to a present mqtt block without publishInterval, and to the
// staleness of the data when there is no mqtt block.
const defaultPublishInterval = 10 * time.Second

// mqttSettings returns the MQTT settings the collector works with: the mqtt block, or the
// defaults without one.
func (c *Config) mqttSettings() MQTTConfig {
	if c.MQTT == nil {
		return MQTTConfig{PublishInterval: defaultPublishInterval}
	}
	return *c.MQTT
}

type DlBusConfig struct {
	GPIO            int           `yaml:"gpio"`            // GPIO pin for DL-Bus input
	DebounceTime    time.Duration `yaml:"debounceTime"`    // Debounce duration (e.g. "100ms"), 0s = disabled
	GPIOTermination string        `yaml:"gpioTermination"` // Termination type for GPIO (e.g. "pullup", "pulldown", "none")
	BitClock        int           `yaml:"bitClock"`        // DL-Bus bit clock frequency in Hz (used for timing), 0 = auto-detect
}

type DataLoggerConfig struct {
	Type string `yaml:"type"` // Type of data logger Technische Alternative: uvr42 | uvr31
	// Name is sent as "device" in every telegram; empty means the type, e.g. "uvr42".
	Name string `yaml:"name"`
	// Sensors names the temperatures and outputs in the web UI and sets the range
	// of each temperature bar, keyed like the data (temperature1, out1, ...). Optional.
	Sensors map[string]SensorConfig `yaml:"sensors"`
}

// SensorConfig is how the web UI shows one temperature or output.
type SensorConfig struct {
	Label string   `yaml:"label"` // name shown in the web UI
	Min   *float64 `yaml:"min"`   // lower end of the temperature bar in °C, default DefaultSensorMin
	Max   *float64 `yaml:"max"`   // upper end of the temperature bar in °C, default DefaultSensorMax
}

// Default range of a temperature bar in the web UI, in °C.
const (
	DefaultSensorMin = -20.0
	DefaultSensorMax = 150.0
)

// deviceType returns the datalogger device ID for a configured type name, or 0.
func deviceType(name string) int {
	switch name {
	case "uvr42":
		return datalogger.UVR42
	case "uvr31":
		return datalogger.UVR31
	}
	return 0
}

// DeviceName returns the name sent as "device" in every telegram: Name, or the
// type when Name is empty.
func (c DataLoggerConfig) DeviceName() string {
	if n := strings.TrimSpace(c.Name); n != "" {
		return n
	}
	return c.Type
}

// SensorRange returns the bar range of a temperature, with the defaults filled in.
func (s SensorConfig) SensorRange() (lo, hi float64) {
	lo, hi = DefaultSensorMin, DefaultSensorMax
	if s.Min != nil {
		lo = *s.Min
	}
	if s.Max != nil {
		hi = *s.Max
	}
	return lo, hi
}

// validate applies the defaults of a present mqtt block and checks it; a nil block (no MQTT)
// is valid.
func (m *MQTTConfig) validate() error {
	if m == nil {
		return nil
	}

	if m.Connection == "" {
		return errors.New("mqtt connection is required; delete the mqtt block to run without MQTT")
	}

	if m.PublishInterval == 0 {
		m.PublishInterval = defaultPublishInterval
	}
	if m.PublishInterval < time.Second {
		return fmt.Errorf("mqtt publishInterval must be at least 1s, got %v", m.PublishInterval)
	}

	if m.TopicPrefix == "" {
		return fmt.Errorf("mqtt topicPrefix must be configured")
	}

	if m.MinDeltaTemp < 0 {
		return fmt.Errorf("mqtt minDeltaTemp must be non-negative, got %v", m.MinDeltaTemp)
	}

	return nil
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
		DlBus: DlBusConfig{
			DebounceTime:    0,
			GPIOTermination: "pullup",
		},
		DataLogger: DataLoggerConfig{
			Type: "uvr42",
		},
	}
}

// envBraces matches ${VAR} references; see expandEnvBraces.
var envBraces = regexp.MustCompile(`\$\{([A-Za-z_][A-Za-z0-9_]*)\}`)

// expandEnvBraces replaces ${VAR} with the value of the environment variable VAR, or with an
// empty string when it is unset. Unlike os.ExpandEnv it leaves every other "$" alone, so an API
// key or password containing "$" is not silently cut short.
func expandEnvBraces(s string) string {
	return envBraces.ReplaceAllStringFunc(s, func(ref string) string {
		return os.Getenv(envBraces.FindStringSubmatch(ref)[1])
	})
}

// LoadConfig loads configuration from a YAML file and expands ${VAR} environment references.
//
// Unknown keys are an error rather than ignored, so a misspelled or renamed key cannot silently
// leave its setting at the default.
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

	dec := yaml.NewDecoder(bytes.NewReader([]byte(expandEnvBraces(string(content)))))
	dec.KnownFields(true)
	if err = dec.Decode(cfg); err != nil && !errors.Is(err, io.EOF) {
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

	if err := c.MQTT.validate(); err != nil {
		return err
	}

	validGPIOs := []int{2, 3, 4, 5, 6, 7, 8, 9, 10, 11, 12, 13, 14, 15, 16, 17, 18, 19, 20, 21, 22, 23, 24, 25, 26, 27}
	if !slices.Contains(validGPIOs, c.DlBus.GPIO) {
		return fmt.Errorf("invalid gpio pin: %d, must be a valid Raspberry Pi BCM GPIO", c.DlBus.GPIO)
	}

	if c.DlBus.DebounceTime < 0 {
		return fmt.Errorf("DebounceTime must be non-negative, got %v", c.DlBus.DebounceTime)
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

	return c.validateSensors()
}

// validateSensors checks datalogger.sensors against the keys of the configured device.
func (c *Config) validateSensors() error {
	temperatures, outputs := datalogger.Keys(deviceType(c.DataLogger.Type))
	for key, s := range c.DataLogger.Sensors {
		switch {
		case slices.Contains(temperatures, key):
			lo, hi := s.SensorRange()
			if lo < datalogger.MinTemperature || hi > datalogger.MaxTemperature {
				return fmt.Errorf("datalogger.sensors.%s: range %v…%v must be within %d…%d °C",
					key, lo, hi, datalogger.MinTemperature, datalogger.MaxTemperature)
			}
			if lo >= hi {
				return fmt.Errorf("datalogger.sensors.%s: min %v must be less than max %v", key, lo, hi)
			}
		case slices.Contains(outputs, key):
			if s.Min != nil || s.Max != nil {
				return fmt.Errorf("datalogger.sensors.%s: an output has no min or max", key)
			}
		default:
			return fmt.Errorf("datalogger.sensors.%s: unknown key for %s, must be one of %v",
				key, c.DataLogger.Type, slices.Concat(temperatures, outputs))
		}
	}
	return nil
}
