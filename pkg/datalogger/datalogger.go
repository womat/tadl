// Package datalogger decodes data frames from Technische Alternative heating
// controllers transmitted over the DL-Bus protocol.
//
// Supported devices:
//   - UVR31 (device ID 0x30): 3 temperature sensors, 1 output
//   - UVR42 (device ID 0x10): 4 temperature sensors, 2 outputs
//
// Usage:
//
//	dl := datalogger.NewUVR42()
//	frames, err := dl.Watch(rawFrameCh, datalogger.WithLogger(slog.Default()))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for record := range frames {
//	    fmt.Println(record)
//	}
//	dl.Close()
package datalogger

import (
	"encoding/binary"
	"errors"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/womat/golib/keyvalue"
)

// Sentinel errors returned by the decoder.
var (
	ErrInvalidSize           = errors.New("invalid frame size")
	ErrInvalidTemperature    = errors.New("invalid temperature")
	ErrUnsupportedDevice     = errors.New("unsupported device id")
	ErrWatcherAlreadyStarted = errors.New("watcher already started")
)

// Device identifiers as transmitted on the DL-Bus.
const (
	UVR31 = 0x30
	UVR42 = 0x10
)

// Key constants for fields in a decoded keyvalue.Record.
const (
	KeyTimestamp    = "timestamp"
	KeyTemperature1 = "temperature1"
	KeyTemperature2 = "temperature2"
	KeyTemperature3 = "temperature3"
	KeyTemperature4 = "temperature4"
	KeyOut1         = "out1"
	KeyOut2         = "out2"
)

// Valid temperature range in °C. A value outside this range, such as from a
// broken or disconnected sensor, is left out of the record.
const (
	MinTemperature = -50
	MaxTemperature = 300

	tMax = MaxTemperature
	tMin = MinTemperature
)

// rangeWarnInterval is how often a sensor that stays out of range is reported.
const rangeWarnInterval = time.Minute

// DL is the interface implemented by all data logger handlers.
// Call Watch to start decoding and Close to release resources.
type DL interface {
	// Watch starts the decoding goroutine and returns a channel on which
	// decoded keyvalue.Records are delivered. Call Close to stop decoding.
	Watch(rx <-chan []byte, opts ...Option) (<-chan keyvalue.Record, error)

	// Close blocks until the decoding goroutine has terminated.
	// It is a no-op if Watch has never been called.
	Close() error

	// Stats returns how many frames were decoded and rejected so far.
	Stats() Stats
}

// Stats counts the frames a handler has seen since it was created.
type Stats struct {
	Decoded  uint64 // frames decoded into a record
	Rejected uint64 // frames of the wrong size or device, or without any valid temperature
}

// counters is embedded in each handler and counted by its decoding goroutine.
type counters struct {
	decoded  atomic.Uint64
	rejected atomic.Uint64
}

// Stats returns a snapshot of the frame counters.
func (c *counters) Stats() Stats {
	return Stats{Decoded: c.decoded.Load(), Rejected: c.rejected.Load()}
}

// Keys returns the record keys of the temperatures and outputs a device sends,
// in the order of its frame. It returns nil for an unknown device.
func Keys(device int) (temperatures, outputs []string) {
	switch device {
	case UVR42:
		return []string{KeyTemperature1, KeyTemperature2, KeyTemperature3, KeyTemperature4},
			[]string{KeyOut1, KeyOut2}
	case UVR31:
		return []string{KeyTemperature1, KeyTemperature2, KeyTemperature3},
			[]string{KeyOut1}
	}
	return nil, nil
}

// options holds optional configuration applied via Option functions.
type options struct {
	logger *slog.Logger
}

// Option configures a data logger handler.
type Option func(*options)

// WithLogger enables debug and warning output during frame decoding.
// If not set, no logging is performed.
func WithLogger(l *slog.Logger) Option {
	return func(o *options) {
		o.logger = l
	}
}

// outOfRange is a temperature that was left out of a record.
type outOfRange struct {
	key   string
	value float64
}

// setTemperatures decodes the temperatures in b, int16 little-endian in 1/10 °C,
// into r under keys, one per two bytes. A value outside tMin…tMax is left out
// and returned instead. When no value is in range, the frame carries no
// measurement at all and ErrInvalidTemperature is returned.
func setTemperatures(r keyvalue.Record, b []byte, keys []string) ([]outOfRange, error) {
	var invalid []outOfRange
	for i, key := range keys {
		t := float64(int16(binary.LittleEndian.Uint16(b[2*i:]))) / 10
		if t > tMax || t < tMin {
			invalid = append(invalid, outOfRange{key: key, value: t})
			continue
		}
		r.Set(key, t)
	}
	if len(invalid) == len(keys) {
		return invalid, ErrInvalidTemperature
	}
	return invalid, nil
}

// rangeWarner reports sensors out of range, each at most once per
// rangeWarnInterval. It is used by a single decoding goroutine.
type rangeWarner struct {
	logger *slog.Logger
	last   map[string]time.Time
}

func (w *rangeWarner) warn(invalid []outOfRange, now time.Time) {
	if w.logger == nil {
		return
	}
	if w.last == nil {
		w.last = make(map[string]time.Time)
	}
	for _, v := range invalid {
		if t, ok := w.last[v.key]; ok && now.Sub(t) < rangeWarnInterval {
			continue
		}
		w.last[v.key] = now
		w.logger.Warn("temperature out of range, sensor left out",
			"sensor", v.key, "value", v.value, "min", tMin, "max", tMax)
	}
}
