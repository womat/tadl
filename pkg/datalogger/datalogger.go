package datalogger

import (
	"errors"
	"io"
)

var (
	ErrInvalidSize        = errors.New("invalid frame size")
	ErrInvalidTemperature = errors.New("invalid temperature")
	ErrUnsupportedDevice  = errors.New("unsupported device id")
)

// DL is the interface implemented by a data logger type
type DL interface {
	// Connect use the defined io.ReadWriterCloser.
	Connect(io.ReadCloser) error
	// Get reads the DL buffer from ReadCloser, convert the buffer to a data logger structure
	// and checks weather the values of temperature values are valid:
	//  * the current values are within a temperature range
	//  * and the difference to the last measured values are less than maxDelta
	Get() (interface{}, error)
	// Close the handler (ReadCloser).
	Close() error
}

const (
	// device Id
	uvr31 = 0x30
	uvr42 = 0x10

	// max temperature range
	tMax = 300
	tMin = -50
)
