// Package app provides HTTP handlers for the application's REST API.
package app

import (
	"net/http"
	"net/url"
	"time"

	"github.com/womat/golib/web"
	"github.com/womat/tadl/app/service/health"
)

// HandleHealth returns the current health data of the application.
//
//	@Summary		Get health data
//	@Description	Retrieves memory usage, goroutine count, version, hostname, Go runtime version and OS, the MQTT connection state
//	@Description	with broker (host:port, never credentials) and topic,
//	@Description	the latest controller values with their names, bar ranges and 15 min trend, and the state of the DL-Bus input
//	@Description	(signal, bit rate, line polarity, error counters since start and for the last 24 h). The web UI at / reads it.
//	@Tags			info
//	@Produce		json
//	@Security		ApiKeyAuth
//	@Success		200	{object}	health.Model	"Health data successfully retrieved"
//	@Failure		401	{string}	string			"Unauthorized"
//	@Router			/health [get]
func (app *App) HandleHealth() http.Handler {
	return http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			now := time.Now()
			resp := health.GetCurrentHealth(MODULE, VERSION)
			resp.Mqtt = app.mqttState()
			resp.MqttBroker, resp.MqttTopic = app.mqttTarget()
			dl := app.dataloggerService.Status(app.sensorViews())
			bus := app.busStatus(dl, now)
			resp.Datalogger, resp.Bus = &dl, &bus
			web.Encode(w, http.StatusOK, resp)
		},
	)
}

// mqttState reports the broker connection for /health. IsConnectionOpen is
// false during a reconnect as well.
func (app *App) mqttState() string {
	switch {
	case app.mqtt == nil:
		return health.MqttDisabled
	case app.mqtt.IsConnectionOpen():
		return health.MqttConnected
	default:
		return health.MqttDisconnected
	}
}

// mqttTarget returns the broker as host:port and the topic for /health, both
// empty without an mqtt block. The page shows them as the pill's tooltip.
func (app *App) mqttTarget() (broker, topic string) {
	if app.config.MQTT == nil {
		return "", ""
	}
	return brokerHost(app.config.MQTT.Connection), app.config.MQTT.TopicPrefix
}

// brokerHost is the broker as the page shows it: host and port, never a user
// or password someone wrote into the URL.
func brokerHost(s string) string {
	u, err := url.Parse(s)
	if err != nil || u.Host == "" {
		return ""
	}
	return u.Host
}
