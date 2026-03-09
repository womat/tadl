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
//	frames, err := dl.Watch(ctx, rawFrameCh, datalogger.WithLogger(slog.Default()))
//	if err != nil {
//	    log.Fatal(err)
//	}
//	for record := range frames {
//	    fmt.Println(record)
//	}
//	dl.Close()
package datalogger

import (
	"errors"
	"log/slog"

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

// Valid temperature range in °C. Values outside this range are rejected.
const (
	tMax = 300
	tMin = -50
)

// DL is the interface implemented by all data logger handlers.
// Call Watch to start decoding and Close to release resources.
type DL interface {
	// Watch starts the decoding goroutine and returns a channel on which
	// decoded keyvalue.Records are delivered. Cancel ctx to stop decoding.
	Watch(rx <-chan []byte, opts ...Option) (<-chan keyvalue.Record, error)

	// Close blocks until the decoding goroutine has terminated.
	// It is a no-op if Watch has never been called.
	Close() error
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
