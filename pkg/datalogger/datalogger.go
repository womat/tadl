package datalogger

import (
	"errors"

	"github.com/womat/golib/keyvalue"
)

var (
	ErrInvalidSize        = errors.New("invalid frame size")
	ErrInvalidTemperature = errors.New("invalid temperature")
	ErrUnsupportedDevice  = errors.New("unsupported device id")
	ErrNotConnected       = errors.New("not connected")
)

const (
	// device identifiers
	UVR31 = 0x30
	UVR42 = 0x10

	// common keys for data frames
	KeyTimestamp    = "timestamp"
	KeyTemperature1 = "temperature1"
	KeyTemperature2 = "temperature2"
	KeyTemperature3 = "temperature3"
	KeyTemperature4 = "temperature4"
	KeyOut1         = "out1"
	KeyOut2         = "out2"
)

// DL is the interface implemented by a data logger type
type DL interface {
	// Get reads a frame from the DL-Bus, parses it and validates the temperature values.
	// Temperature values are valid if they are within the range [tMin, tMax].
	Get() (keyvalue.Record, error)
	// Close closes the underlying ReadCloser.
	Close() error
}

const (
	// valid temperature range in °C
	tMax = 300
	tMin = -50
)
