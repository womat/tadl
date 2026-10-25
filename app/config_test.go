package app

import (
	"strings"
	"testing"
	"time"
)

func validConfig() *Config {
	c := NewConfig()
	c.Webserver.TokenFile = "/opt/tadl/etc/token-hashes"
	c.DlBus.GPIO = 20
	c.MQTT = &MQTTConfig{Connection: "tcp://broker:1883", TopicPrefix: "test/uvr42"}
	return c
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateWithoutMQTTBlock(t *testing.T) {
	c := validConfig()
	c.MQTT = nil
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate without mqtt block: %v", err)
	}
	if got := c.mqttSettings().PublishInterval; got != defaultPublishInterval {
		t.Errorf("publishInterval without mqtt block = %v, want the default %v for the staleness", got, defaultPublishInterval)
	}
}

// A present block must be complete: an empty connection used to mean "MQTT off".
func TestValidateMQTTBlockNeedsConnection(t *testing.T) {
	c := validConfig()
	c.MQTT.Connection = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "delete the mqtt block") {
		t.Fatalf("Validate with empty connection: err=%v, want the hint to delete the mqtt block", err)
	}
}

// mqtt v1.1.0 refuses a broker URL Paho cannot parse; Validate must refuse it first, without
// printing the URL, which may carry a password.
func TestValidateMQTTRejectsUnparsableConnection(t *testing.T) {
	for _, conn := range []string{"tcp://user:secret@host:18 83", "%zz"} {
		c := validConfig()
		c.MQTT.Connection = conn
		err := c.Validate()
		if err == nil || !strings.Contains(err.Error(), "mqtt connection cannot be parsed") {
			t.Errorf("Validate(%q): err=%v, want a parse error", conn, err)
		} else if strings.Contains(err.Error(), "secret") {
			t.Errorf("Validate(%q): error %q shows the password", conn, err)
		}
	}
	for _, conn := range []string{"tcp://host:1883", "host:1883", "ssl://user:pw@host:8883"} {
		c := validConfig()
		c.MQTT.Connection = conn
		if err := c.Validate(); err != nil {
			t.Errorf("Validate(%q): %v", conn, err)
		}
	}
}

func TestValidateRejectsInvalidIPList(t *testing.T) {
	c := validConfig()
	c.Webserver.BlockedIPs = []string{"10.0.0.1", "192.168.0.300"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "webserver.blockedIPs") {
		t.Errorf("Validate with a bad blockedIPs entry: err=%v", err)
	}

	c = validConfig()
	c.Webserver.AllowedIPs = []string{"192.168.0.300"}
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "webserver.allowedIPs") {
		t.Errorf("Validate with a bad allowedIPs entry: err=%v", err)
	}

	c = validConfig()
	c.Webserver.AllowedIPs = []string{"192.168.1.0/24", "127.0.0.1"}
	c.Webserver.BlockedIPs = []string{"192.168.1.13"}
	if err := c.Validate(); err != nil {
		t.Errorf("Validate with valid IP lists: %v", err)
	}
}

func TestValidateMQTTDefaults(t *testing.T) {
	c := validConfig()
	if err := c.Validate(); err != nil {
		t.Fatal(err)
	}
	if c.MQTT.PublishInterval != defaultPublishInterval {
		t.Errorf("publishInterval = %v, want the default %v", c.MQTT.PublishInterval, defaultPublishInterval)
	}
}

func TestValidateMQTTEnabledNeedsTopic(t *testing.T) {
	c := validConfig()
	c.MQTT.TopicPrefix = ""
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "topicPrefix") {
		t.Fatalf("Validate without topicPrefix: err=%v", err)
	}
}

func TestValidateErrorsNameTheConfigKeys(t *testing.T) {
	c := validConfig()
	c.MQTT.PublishInterval = 500 * time.Millisecond
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "publishInterval") {
		t.Errorf("publishInterval error = %v", err)
	}

	c = validConfig()
	c.MQTT.MinDeltaTemp = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "minDeltaTemp") {
		t.Errorf("minDeltaTemp error = %v", err)
	}
}

func TestValidateSensors(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		sensors map[string]SensorConfig
		wantErr string
	}{
		{"valid", "uvr42", map[string]SensorConfig{
			"temperature1": {Label: "Collector", Min: new(float64(-20)), Max: new(float64(150))},
			"temperature4": {Label: "Boiler room", Min: new(float64(-5)), Max: new(float64(25))},
			"out1":         {Label: "Solar pump"},
		}, ""},
		{"label only uses the default range", "uvr31", map[string]SensorConfig{"temperature2": {Label: "Tank"}}, ""},
		{"typo", "uvr42", map[string]SensorConfig{"temprature1": {Label: "x"}}, "unknown key"},
		{"key of the other device", "uvr31", map[string]SensorConfig{"temperature4": {Label: "x"}}, "unknown key"},
		{"min not below max", "uvr42", map[string]SensorConfig{"temperature1": {Min: new(float64(50)), Max: new(float64(50))}}, "less than max"},
		{"min above the default max", "uvr42", map[string]SensorConfig{"temperature1": {Min: new(float64(200))}}, "less than max"},
		{"range on an output", "uvr42", map[string]SensorConfig{"out2": {Min: new(float64(0))}}, "no min or max"},
		{"range outside the sensor range", "uvr42", map[string]SensorConfig{"temperature1": {Max: new(float64(400))}}, "within"},
	}
	for _, tt := range tests {
		c := validConfig()
		c.DataLogger.Type = tt.typ
		c.DataLogger.Sensors = tt.sensors
		err := c.Validate()
		switch {
		case tt.wantErr == "" && err != nil:
			t.Errorf("%s: unexpected error %v", tt.name, err)
		case tt.wantErr != "" && (err == nil || !strings.Contains(err.Error(), tt.wantErr)):
			t.Errorf("%s: err=%v, want it to contain %q", tt.name, err, tt.wantErr)
		}
	}
}

func TestDeviceNameDefaultsToType(t *testing.T) {
	c := DataLoggerConfig{Type: "uvr42"}
	if got := c.DeviceName(); got != "uvr42" {
		t.Errorf("DeviceName = %q, want uvr42", got)
	}
	c.Name = "  solar "
	if got := c.DeviceName(); got != "solar" {
		t.Errorf("DeviceName = %q, want solar", got)
	}
}

func TestLoadConfigRejectsUnknownKeys(t *testing.T) {
	for name, content := range map[string]string{
		"renamed dlbus key": "dlbus:\n  bounceTime: 1\n",
		"removed jwtSecret": "webserver:\n  jwtSecret: x\n",
	} {
		t.Run(name, func(t *testing.T) {
			if _, err := loadYAML(t, content); err == nil {
				t.Error("expected an error for the unknown key")
			}
		})
	}
}

// Only ${VAR} is expanded, so a value containing "$" otherwise stays as it is.
func TestLoadConfigExpandsBracedVariablesOnly(t *testing.T) {
	t.Setenv("TADL_TEST_BROKER", "tcp://from-env:1883")
	c, err := loadYAML(t, "mqtt:\n  connection: ${TADL_TEST_BROKER}\n  topicPrefix: a$b\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.MQTT.Connection != "tcp://from-env:1883" || c.MQTT.TopicPrefix != "a$b" {
		t.Errorf("connection = %q, topicPrefix = %q", c.MQTT.Connection, c.MQTT.TopicPrefix)
	}
}

// The example names /opt/tadl/etc/token-hashes, which does not exist here, so only the config file
// itself is read.
func TestLoadConfigExample(t *testing.T) {
	c, err := readConfig("../config/config.yaml")
	if err != nil {
		t.Fatalf("example config: %v", err)
	}
	if err := c.Validate(); err != nil {
		t.Errorf("example config: %v", err)
	}
}
