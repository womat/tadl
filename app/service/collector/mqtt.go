// Package collector processes decoded datalogger frames,
// detects significant changes, and publishes measurements via MQTT.
package collector

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"time"

	"github.com/womat/golib/mqtt"
)

// StartPeriodicPublish runs a periodic publishing loop in a separate goroutine.
// The loop stops when ctx is cancelled. Publish errors are logged but do not stop the loop.
func (h *Handler) StartPeriodicPublish(ctx context.Context, interval time.Duration, mqttHandler *mqtt.Handler) {
	ticker := time.NewTicker(interval)

	go func() {
		defer ticker.Stop()

		for {
			select {
			case <-ctx.Done():
				slog.Info("Stopping periodic MQTT publishing")
				return
			case <-ticker.C:
				err := h.PublishFrame(mqttHandler)
				if err != nil {
					slog.Error("Failed to publish data frame", "err", err)
				}
			}
		}
	}()
}

// PublishFrame publishes the current data frame to MQTT as JSON.
// Returns an error if mqttHandler is nil, JSON marshaling fails, or publishing fails.
func (h *Handler) PublishFrame(mqttHandler *mqtt.Handler) error {
	if mqttHandler == nil {
		return fmt.Errorf("mqtt handler is nil")
	}

	h.mux.Lock()
	b, err := json.Marshal(h.DataFrame)
	h.mux.Unlock()

	if err != nil {
		return err
	}

	msg := mqtt.Message{
		Topic:    h.config.Topic,
		Payload:  b,
		Qos:      0,
		Retained: h.config.Retained,
	}
	err = mqttHandler.Publish(msg)
	if err != nil {
		return err
	}

	return nil
}
