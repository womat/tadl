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
