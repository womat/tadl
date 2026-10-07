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
