package datalogger

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR31Handler is the handler to read a UVR31 dataframe.
type UVR31Handler struct {
	io.ReadCloser
}

// Get reads a UVR31 frame from the DL-Bus, parses it and validates the temperature values.
func (h *UVR31Handler) Get() (keyvalue.Record, error) {
	const (
		out1      byte = 1 << 5 // bitmask for Out1 (bit 5)
		frameSize      = 8
	)

	r := keyvalue.NewRecord()

	if h.ReadCloser == nil {
		return r, ErrNotConnected
	}

	b := make([]byte, frameSize)

	n, err := h.Read(b)
	if err != nil {
		return r, err
	}

	if n != frameSize {
		return r, ErrInvalidSize
	}

	if b[0] != uvr31 {
		return r, ErrUnsupportedDevice
	}

	temperature1 := float64(int16(binary.LittleEndian.Uint16(b[1:3]))) / 10
	temperature2 := float64(int16(binary.LittleEndian.Uint16(b[3:5]))) / 10
	temperature3 := float64(int16(binary.LittleEndian.Uint16(b[5:7]))) / 10

	if temperature1 > tMax || temperature1 < tMin ||
		temperature2 > tMax || temperature2 < tMin ||
		temperature3 > tMax || temperature3 < tMin {
		return r, ErrInvalidTemperature
	}

	r.Set(KeyTimestamp, time.Now())
	r.Set(KeyTemperature1, temperature1)
	r.Set(KeyTemperature2, temperature2)
	r.Set(KeyTemperature3, temperature3)
	r.Set(KeyOut1, b[7]&out1 > 0)
	return r, nil
}

// Close the ReadCloser handler.
func (h *UVR31Handler) Close() error {
	return h.ReadCloser.Close()
}
