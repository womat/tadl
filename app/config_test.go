package app

import (
	"strings"
	"testing"
)

func validConfig() *Config {
	c := NewConfig()
	c.Webserver.ApiKey = "a-long-enough-test-key"
	c.DlBus.GPIO = 20
	c.MQTT.Connection = "tcp://broker:1883"
	c.MQTT.TopicPrefix = "test/uvr42"
	return c
}

func TestValidateAcceptsValidConfig(t *testing.T) {
	if err := validConfig().Validate(); err != nil {
		t.Fatalf("Validate: %v", err)
	}
}

func TestValidateMQTTDisabledNeedsNoTopic(t *testing.T) {
	c := validConfig()
	c.MQTT.Connection = ""
	c.MQTT.TopicPrefix = ""
	if err := c.Validate(); err != nil {
		t.Fatalf("Validate with MQTT disabled: %v", err)
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
	c.MQTT.PublishInterval = 0
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "publishInterval") {
		t.Errorf("publishInterval error = %v", err)
	}

	c = validConfig()
	c.MQTT.MinDeltaTemp = -1
	if err := c.Validate(); err == nil || !strings.Contains(err.Error(), "minDeltaTemp") {
		t.Errorf("minDeltaTemp error = %v", err)
	}
}

func ptr(v float64) *float64 { return &v }

func TestValidateSensors(t *testing.T) {
	tests := []struct {
		name    string
		typ     string
		sensors map[string]SensorConfig
		wantErr string
	}{
		{"valid", "uvr42", map[string]SensorConfig{
			"temperature1": {Label: "Collector", Min: ptr(-20), Max: ptr(150)},
			"temperature4": {Label: "Boiler room", Min: ptr(-5), Max: ptr(25)},
			"out1":         {Label: "Solar pump"},
		}, ""},
		{"label only uses the default range", "uvr31", map[string]SensorConfig{"temperature2": {Label: "Tank"}}, ""},
		{"typo", "uvr42", map[string]SensorConfig{"temprature1": {Label: "x"}}, "unknown key"},
		{"key of the other device", "uvr31", map[string]SensorConfig{"temperature4": {Label: "x"}}, "unknown key"},
		{"min not below max", "uvr42", map[string]SensorConfig{"temperature1": {Min: ptr(50), Max: ptr(50)}}, "less than max"},
		{"min above the default max", "uvr42", map[string]SensorConfig{"temperature1": {Min: ptr(200)}}, "less than max"},
		{"range on an output", "uvr42", map[string]SensorConfig{"out2": {Min: ptr(0)}}, "no min or max"},
		{"range outside the sensor range", "uvr42", map[string]SensorConfig{"temperature1": {Max: ptr(400)}}, "within"},
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

// Only ${VAR} is expanded, so an API key containing "$" stays as it is.
func TestLoadConfigExpandsBracedVariablesOnly(t *testing.T) {
	t.Setenv("TADL_TEST_KEY", "from-env")
	c, err := loadYAML(t, "webserver:\n  apiKey: ${TADL_TEST_KEY}\nmqtt:\n  topicPrefix: a$b\n")
	if err != nil {
		t.Fatal(err)
	}
	if c.Webserver.ApiKey != "from-env" || c.MQTT.TopicPrefix != "a$b" {
		t.Errorf("apiKey = %q, topicPrefix = %q", c.Webserver.ApiKey, c.MQTT.TopicPrefix)
	}
}

func TestLoadConfigExample(t *testing.T) {
	if _, err := LoadConfig("../config/config.yaml"); err != nil {
		t.Errorf("example config: %v", err)
	}
}
