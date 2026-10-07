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

// RunPeriodicPublish publishes the current data frame every interval until ctx
// is cancelled. Publish errors are logged but do not stop the loop.
func (h *Handler) RunPeriodicPublish(ctx context.Context, interval time.Duration, pub Publisher) {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			slog.Info("Stopping periodic MQTT publishing")
			return
		case <-ticker.C:
			h.publish(pub)
		}
	}
}

// PublishFrame publishes the current data frame to MQTT as JSON and remembers it
// as the last published frame. It returns ErrNoData when there is no current
// frame, so that neither an empty nor a stale frame reaches the broker.
func (h *Handler) PublishFrame(pub Publisher) error {
	if pub == nil {
		return fmt.Errorf("mqtt publisher is nil")
	}

	h.mux.Lock()
	frame := h.DataFrame
	if !h.isCurrent(frame) {
		h.mux.Unlock()
		return ErrNoData
	}
	b, err := json.Marshal(frame)
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
	if err = pub.Publish(msg); err != nil {
		return err
	}

	// Frames are replaced, never modified, so frame can be kept without a copy.
	h.mux.Lock()
	h.lastPublished = frame
	h.mux.Unlock()
	return nil
}
