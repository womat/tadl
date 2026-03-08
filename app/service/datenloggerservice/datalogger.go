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

// service wait in an endless loop for valid data logger frames.
// It save the data frame to app main structure and send the dataframe to the mqtt broker
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

				h.mux.Lock()
				hasChanged, err := h.hasChanged(f)
				h.mux.Unlock()

				if err != nil {
					slog.Error("Failed to validate measurements", "err", err)
					h.mux.Unlock()
					continue
				}

				h.mux.Lock()
				h.DataFrame = f
				h.mux.Unlock()

				if hasChanged {
					err = h.PublishFrame(mqtt)
					if err != nil {
						slog.Error("Failed to publish data frame", "err", err)
					}
				}
			}
		}
	}()
}

func (h *Handler) GetData() keyvalue.Record {
	h.mux.Lock()
	defer h.mux.Unlock()
	return h.DataFrame.Copy()
}

// validateMeasurements checks the dataframe by deltaT and delta
// and send dataframe to mqtt if data changed or by send Interval
func (h *Handler) hasChanged(current keyvalue.Record) (bool, error) {

	var hasChanged bool
	t1, err := toDate(current, datalogger.KeyTimestamp)
	if err != nil {
		return false, err
	}

	t2, err := toDate(h.DataFrame, datalogger.KeyTimestamp)
	if err != nil {
		return false, err
	}

	switch h.typ {
	case datalogger.UVR42:
		hasChanged = t1.Sub(t2) > h.config.PublishInterval ||
			current.Int(datalogger.KeyOut1) != h.DataFrame.Int(datalogger.KeyOut1) ||
			current.Int(datalogger.KeyOut2) != h.DataFrame.Int(datalogger.KeyOut2) ||
			math.Abs(current.Float64(datalogger.KeyTemperature1)-h.DataFrame.Float64(datalogger.KeyTemperature1)) > h.config.MinDeltaTemp ||
			math.Abs(current.Float64(datalogger.KeyTemperature2)-h.DataFrame.Float64(datalogger.KeyTemperature2)) > h.config.MinDeltaTemp ||
			math.Abs(current.Float64(datalogger.KeyTemperature3)-h.DataFrame.Float64(datalogger.KeyTemperature3)) > h.config.MinDeltaTemp ||
			math.Abs(current.Float64(datalogger.KeyTemperature4)-h.DataFrame.Float64(datalogger.KeyTemperature4)) > h.config.MinDeltaTemp
	case datalogger.UVR31:
		hasChanged = t1.Sub(t2) > h.config.PublishInterval ||
			current.Int(datalogger.KeyOut1) != h.DataFrame.Int(datalogger.KeyOut1) ||
			math.Abs(current.Float64(datalogger.KeyTemperature1)-h.DataFrame.Float64(datalogger.KeyTemperature1)) > h.config.MinDeltaTemp ||
			math.Abs(current.Float64(datalogger.KeyTemperature2)-h.DataFrame.Float64(datalogger.KeyTemperature2)) > h.config.MinDeltaTemp ||
			math.Abs(current.Float64(datalogger.KeyTemperature3)-h.DataFrame.Float64(datalogger.KeyTemperature3)) > h.config.MinDeltaTemp
	default:
		return false, errors.New("unsupported frame type")
	}

	return hasChanged, nil
}

func toDate(record keyvalue.Record, key string) (time.Time, error) {
	tmp, ok := record.Value(key)
	if !ok {
		return time.Time{}, errors.New("missing timestamp")
	}

	t, ok := tmp.(time.Time)
	if !ok {
		return time.Time{}, errors.New("invalid timestamp type")
	}
	return t, nil
}
