package dataloggerservice

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"tadl/pkg/datalogger"
	"time"

	"github.com/womat/golib/keyvalue"
	"github.com/womat/golib/mqtt"
)

var ErrUnsupportedFrameType = errors.New("unsupported frame type")

type Handler struct {
	mux       sync.Mutex
	typ       int // Datalogger Type UVR31 or UVR42
	config    Config
	DataFrame keyvalue.Record
}

type Config struct {
	PublishInterval time.Duration
	MinDeltaTemp    float64
	Topic           string
	Retained        bool
}

func New(conf Config, t int) *Handler {
	return &Handler{
		typ:    t,
		config: conf,
	}
}

// Run starts a goroutine that consumes decoded data logger frames from rx.
// On each frame it checks whether the values changed significantly and, if so,
// publishes them to the MQTT broker.
func (h *Handler) Run(ctx context.Context, rx <-chan keyvalue.Record, mqtt *mqtt.Handler) {
	go func() {
		for {
			select {
			case <-ctx.Done():
				slog.Info("Stopping datalogger service")
				return

			case f, open := <-rx:
				if !open {
					slog.Info("Datalogger watcher channel closed")
					return
				}

				// Evaluate under lock, then release before publishing.
				changed, err := h.checkAndUpdate(f)

				if err != nil {
					slog.Error("Failed to validate measurements", "err", err)
					h.mux.Unlock()
					continue
				}

				if changed {
					err = h.PublishFrame(mqtt)
					if err != nil {
						slog.Error("Failed to publish data frame", "err", err)
					}
				}
			}
		}
	}()
}

// checkAndUpdate checks whether the new frame differs significantly from the
// last stored frame and, if so, stores it. Both operations are performed under
// the same lock to avoid a TOCTOU race.
// Returns (true, nil) when the frame was stored and should be published.
func (h *Handler) checkAndUpdate(current keyvalue.Record) (bool, error) {
	h.mux.Lock()
	defer h.mux.Unlock()

	changed, err := h.hasChanged(current)
	if err != nil {
		return false, err
	}

	// Always store the latest frame regardless of whether it triggered a publish.
	h.DataFrame = current
	return changed, nil
}

// GetData returns a copy of the most recently stored data frame.
func (h *Handler) GetData() keyvalue.Record {
	h.mux.Lock()
	defer h.mux.Unlock()
	return h.DataFrame.Copy()
}

// hasChanged reports whether current differs from the previously stored frame
// enough to warrant an MQTT publish.
// Must be called with h.mux held.
func (h *Handler) hasChanged(current keyvalue.Record) (bool, error) {

	t1, err := toTime(current, datalogger.KeyTimestamp)
	if err != nil {
		return false, err
	}

	t2, err := toTime(h.DataFrame, datalogger.KeyTimestamp)
	if err != nil {
		// No previous frame stored yet — always publish the first frame.
		return true, nil
	}

	elapsed := t1.Sub(t2)

	switch h.typ {
	case datalogger.UVR42:
		return elapsed > h.config.PublishInterval ||
			current.Bool(datalogger.KeyOut1) != h.DataFrame.Bool(datalogger.KeyOut1) ||
			current.Bool(datalogger.KeyOut2) != h.DataFrame.Bool(datalogger.KeyOut2) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature1, h.config.MinDeltaTemp) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature2, h.config.MinDeltaTemp) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature3, h.config.MinDeltaTemp) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature4, h.config.MinDeltaTemp), nil
	case datalogger.UVR31:
		return elapsed > h.config.PublishInterval ||
			current.Bool(datalogger.KeyOut1) != h.DataFrame.Bool(datalogger.KeyOut1) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature1, h.config.MinDeltaTemp) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature2, h.config.MinDeltaTemp) ||
			tempChanged(current, h.DataFrame, datalogger.KeyTemperature3, h.config.MinDeltaTemp), nil
	default:
		return false, ErrUnsupportedFrameType
	}
}

// tempChanged returns true when the absolute temperature difference between
// the current and previous frame exceeds minDelta.
func tempChanged(current, previous keyvalue.Record, key string, minDelta float64) bool {
	return math.Abs(current.Float64(key)-previous.Float64(key)) > minDelta
}

// toTime extracts a time.Time value for key from record.
func toTime(record keyvalue.Record, key string) (time.Time, error) {
	v, ok := record.Value(key)
	if !ok {
		return time.Time{}, errors.New("missing timestamp")
	}

	t, ok := v.(time.Time)
	if !ok {
		return time.Time{}, errors.New("invalid timestamp type")
	}
	return t, nil
}
