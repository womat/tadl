// Package collector processes decoded datalogger frames,
// detects significant changes, and publishes measurements via MQTT.
package collector

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"sync"
	"sync/atomic"
	"time"

	"github.com/womat/mqtt"
	"github.com/womat/tadl/pkg/datalogger"
)

var (
	ErrUnsupportedFrameType = errors.New("unsupported frame type")
	// ErrNoData is returned by PublishFrame when no frame has been received yet
	// or the last one is older than Config.StaleAfter.
	ErrNoData = errors.New("no current data frame")
	// ErrMissingTimestamp is returned for a frame without a timestamp; decoders always set one.
	ErrMissingTimestamp = errors.New("missing timestamp")
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

	// DataFrame is the latest frame received, served by Current; zero before the first.
	// Frames are replaced, never modified, so a copy may share its pointers.
	DataFrame datalogger.Frame
	// lastPublished is the last frame sent to the broker. A new frame is compared
	// with it, not with its predecessor, so that a slow drift adds up to MinDeltaTemp.
	lastPublished datalogger.Frame
	// offline is set while publishes fail because the broker is not connected, so
	// that an outage is logged once instead of for every frame.
	offline atomic.Bool

	// samples holds one set of temperatures per minute for the trend, oldest first.
	samples []sample
}

type Config struct {
	PublishInterval time.Duration
	// MinDeltaTemp is the temperature change in Kelvin since the last publish that
	// triggers a publish. 0 disables the trigger: temperatures are then only sent
	// by the periodic publish; a changed output is always sent at once.
	MinDeltaTemp float64
	Topic        string
	Retained     bool
	// Device names the data logger in every frame (Frame.Device, "device" in JSON),
	// so a consumer can tell the senders apart. Empty leaves the key out.
	Device string
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
func (h *Handler) Run(ctx context.Context, rx <-chan datalogger.Frame, pub Publisher) {
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
				h.publish(pub)
			}
		}
	}
}

// checkAndUpdate stores the frame and reports whether it differs significantly
// from the last published frame. Both happen under the same lock.
func (h *Handler) checkAndUpdate(current datalogger.Frame) (bool, error) {
	h.mux.Lock()
	defer h.mux.Unlock()

	changed, err := h.hasChanged(current)
	if err != nil {
		return false, err
	}

	if h.config.Device != "" {
		current.Device = h.config.Device
	}
	// Always store the latest frame regardless of whether it triggered a publish.
	h.DataFrame = current
	h.track(current)
	return changed, nil
}

// Current returns the latest data frame and whether it is current: a frame has
// been received and it is not older than Config.StaleAfter.
func (h *Handler) Current() (datalogger.Frame, bool) {
	h.mux.Lock()
	defer h.mux.Unlock()
	return h.DataFrame, h.isCurrent(h.DataFrame)
}

// isCurrent reports whether frame holds data that is not older than StaleAfter.
// Must be called with h.mux held.
func (h *Handler) isCurrent(frame datalogger.Frame) bool {
	if frame.IsZero() {
		return false
	}
	if h.config.StaleAfter <= 0 {
		return true
	}
	return h.now().Sub(frame.Timestamp) <= h.config.StaleAfter
}

// hasChanged reports whether current differs from the last published frame
// enough to warrant an MQTT publish.
// Must be called with h.mux held.
func (h *Handler) hasChanged(current datalogger.Frame) (bool, error) {
	if current.IsZero() {
		return false, ErrMissingTimestamp
	}

	temperatures, outputs := datalogger.Keys(h.typ)
	if temperatures == nil {
		return false, ErrUnsupportedFrameType
	}

	if h.lastPublished.IsZero() {
		// Nothing published yet — always publish the first frame.
		return true, nil
	}

	for _, key := range outputs {
		if current.Output(key) != h.lastPublished.Output(key) {
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
func tempChanged(current, previous datalogger.Frame, key string, minDelta float64) bool {
	now, inCurrent := current.Temperature(key)
	before, inPrevious := previous.Temperature(key)
	if inCurrent != inPrevious {
		return true
	}
	if !inCurrent || minDelta <= 0 {
		return false
	}
	return math.Abs(now-before) >= minDelta-deltaEpsilon
}

// publish publishes the current data frame and logs a failure at a level that
// matches its cause. A broker outage is logged once when it begins and once
// when publishing works again, not for every frame in between.
func (h *Handler) publish(pub Publisher) {
	err := h.PublishFrame(pub)
	switch {
	case err == nil:
		if h.offline.Swap(false) {
			slog.Info("MQTT publishing resumed")
		}
	case errors.Is(err, ErrNoData):
		slog.Debug("No current data frame to publish")
	case errors.Is(err, mqtt.ErrNotConnected):
		if !h.offline.Swap(true) {
			slog.Warn("MQTT not connected, data frames are not published until the broker is back")
		}
	default:
		slog.Error("Failed to publish data frame", "err", err)
	}
}
