package app

import (
	"encoding/json"
	"strings"
	"testing"

	"github.com/womat/tadl/app/service/health"
)

func TestBrokerHostHidesCredentials(t *testing.T) {
	if got := brokerHost("tcp://user:secret@192.168.1.5:1883"); got != "192.168.1.5:1883" {
		t.Errorf("got %q, want host and port only", got)
	}
	if got := brokerHost("not a url"); got != "" {
		t.Errorf("unparsable broker = %q, want empty", got)
	}
}

// The /health body must carry broker and topic for the pill's tooltip, but never
// the user or password from mqtt.connection.
func TestHealthMQTTTargetWithoutCredentials(t *testing.T) {
	c := validConfig()
	c.MQTT.Connection = "tcp://user:secret@mqtt.example.com:1883"
	a := New(c, "", nil, nil)

	var resp health.Model
	resp.MqttBroker, resp.MqttTopic = a.mqttTarget()
	if resp.MqttBroker != "mqtt.example.com:1883" || resp.MqttTopic != "test/uvr42" {
		t.Errorf("mqttTarget = %q, %q", resp.MqttBroker, resp.MqttTopic)
	}
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(body); strings.Contains(s, "user") || strings.Contains(s, "secret") {
		t.Errorf("credentials leak into /health: %s", s)
	}
}

func TestHealthMQTTTargetWithoutMQTT(t *testing.T) {
	c := validConfig()
	c.MQTT = nil
	var resp health.Model
	resp.MqttBroker, resp.MqttTopic = New(c, "", nil, nil).mqttTarget()
	body, err := json.Marshal(resp)
	if err != nil {
		t.Fatal(err)
	}
	if s := string(body); strings.Contains(s, "mqttBroker") || strings.Contains(s, "mqttTopic") {
		t.Errorf("fields present without an mqtt block: %s", s)
	}
}
