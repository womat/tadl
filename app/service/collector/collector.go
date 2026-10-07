// Package collector processes decoded datalogger frames,
// detects significant changes, and publishes measurements via MQTT.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"time"

	"github.com/womat/golib/keyvalue"
	"github.com/womat/golib/mqtt"
	"github.com/womat/tadl/pkg/datalogger"
)

var (
	ErrUnsupportedFrameType = errors.New("unsupported frame type")
	// ErrNoData is returned by PublishFrame when no frame has been received yet
	// or the last one is older than Config.StaleAfter.
	ErrNoData = errors.New("no current data frame")
)

// deltaEpsilon absorbs float rounding when a temperature difference is compared
// with MinDeltaTemp; the values have a resolution of 0.1 K.
const deltaEpsilon = 1e-9

// Publisher sends a message to the MQTT broker. *mqtt.Handler implements it.
type Publisher interface {
	Publish(msg mqtt.Message) error
}

type Handler struct {
	mux    sync.Mutex
	typ    int // Datalogger Type UVR31 or UVR42
	config Config
	now    func() time.Time

	// DataFrame is the latest frame received, served by Current.
	DataFrame keyvalue.Record
	// lastPublished is the last frame sent to the broker. A new frame is compared
	// with it, not with its predecessor, so that a slow drift adds up to MinDeltaTemp.
	lastPublished keyvalue.Record
}

type Config struct {
	PublishInterval time.Duration
	// MinDeltaTemp is the temperature change in Kelvin since the last publish that
	// triggers a publish. 0 disables the trigger: temperatures are then only sent
	// by the periodic publish; a changed output is always sent at once.
	MinDeltaTemp float64
	Topic        string
	Retained     bool
	// StaleAfter is the age after which the last frame no longer counts as current.
	// 0 disables the check.
	StaleAfter time.Duration
}

func New(conf Config, t int) *Handler {
	return &Handler{
		typ:    t,
		config: conf,
		now:    time.Now,
	}
}

// Run consumes decoded data logger frames from rx until ctx is cancelled or rx is
// closed. It stores every frame and publishes it via pub when it differs
// significantly from the last published frame. A nil pub only stores the frames.
func (h *Handler) Run(ctx context.Context, rx <-chan keyvalue.Record, pub Publisher) {
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

			// checkAndUpdate stores the frame and reports whether it should be published.
			changed, err := h.checkAndUpdate(f)
			if err != nil {
				slog.Error("Failed to validate measurements", "err", err)
				continue
			}

			if changed && pub != nil {
				logPublishError(h.PublishFrame(pub))
			}
		}
	}
}

// checkAndUpdate stores the frame and reports whether it differs significantly
// from the last published frame. Both happen under the same lock.
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

// Current returns a copy of the latest data frame and whether it is current:
// a frame has been received and it is not older than Config.StaleAfter.
func (h *Handler) Current() (keyvalue.Record, bool) {
	h.mux.Lock()
	defer h.mux.Unlock()
	return h.DataFrame.Copy(), h.isCurrent(h.DataFrame)
}

// isCurrent reports whether frame holds data that is not older than StaleAfter.
// Must be called with h.mux held.
func (h *Handler) isCurrent(frame keyvalue.Record) bool {
	if len(frame) == 0 {
		return false
	}
	if h.config.StaleAfter <= 0 {
		return true
	}
	t, err := toTime(frame, datalogger.KeyTimestamp)
	if err != nil {
		return false
	}
	return h.now().Sub(t) <= h.config.StaleAfter
}

// hasChanged reports whether current differs from the last published frame
// enough to warrant an MQTT publish.
// Must be called with h.mux held.
func (h *Handler) hasChanged(current keyvalue.Record) (bool, error) {
	if _, err := toTime(current, datalogger.KeyTimestamp); err != nil {
		return false, err
	}

	var outputs, temperatures []string
	switch h.typ {
	case datalogger.UVR42:
		outputs = []string{datalogger.KeyOut1, datalogger.KeyOut2}
		temperatures = []string{datalogger.KeyTemperature1, datalogger.KeyTemperature2,
			datalogger.KeyTemperature3, datalogger.KeyTemperature4}
	case datalogger.UVR31:
		outputs = []string{datalogger.KeyOut1}
		temperatures = []string{datalogger.KeyTemperature1, datalogger.KeyTemperature2,
			datalogger.KeyTemperature3}
	default:
		return false, ErrUnsupportedFrameType
	}

	if len(h.lastPublished) == 0 {
		// Nothing published yet — always publish the first frame.
		return true, nil
	}

	for _, key := range outputs {
		if current.Bool(key) != h.lastPublished.Bool(key) {
			return true, nil
		}
	}
	for _, key := range temperatures {
		if tempChanged(current, h.lastPublished, key, h.config.MinDeltaTemp) {
			return true, nil
		}
	}
	return false, nil
}

// tempChanged reports whether the temperature key changed by at least minDelta
// between previous and current. A sensor that appears or disappears (out of
// range) counts as changed. With minDelta 0 a change in value never counts.
func tempChanged(current, previous keyvalue.Record, key string, minDelta float64) bool {
	inCurrent, inPrevious := current.Exists(key), previous.Exists(key)
	if inCurrent != inPrevious {
		return true
	}
	if !inCurrent || minDelta <= 0 {
		return false
	}
	return math.Abs(current.Float64(key)-previous.Float64(key)) >= minDelta-deltaEpsilon
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

// logPublishError logs a failed publish at a level that matches its cause.
func logPublishError(err error) {
	switch {
	case err == nil:
	case errors.Is(err, ErrNoData):
		slog.Debug("No current data frame to publish")
	case errors.Is(err, mqtt.ErrNotConnected):
		slog.Warn("MQTT not connected, data frame not published")
	default:
		slog.Error("Failed to publish data frame", "err", err)
	}
}
