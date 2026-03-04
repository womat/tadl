package datalogger

import (
	"encoding/binary"
	"io"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR42Handler is the handler to read a UVR42 dataframe.
type UVR42Handler struct {
	io.ReadCloser
}

// NewUVR42 creates a new UVR42Handler with the given io.ReadCloser.
func NewUVR42(readCloser io.ReadCloser) *UVR42Handler {
	return &UVR42Handler{ReadCloser: readCloser}
}

// Get reads a UVR42 frame from the DL-Bus, parses it and validates the temperature values.
func (h *UVR42Handler) Get() (keyvalue.Record, error) {
	const (
		out1      byte = 1 << 5 // bitmask for Out1 (bit 5)
		out2      byte = 1 << 6 // bitmask for Out2 (bit 6)
		frameSize      = 10
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

	if b[0] != uvr42 {
		return r, ErrUnsupportedDevice
	}

	temperature1 := float64(int16(binary.LittleEndian.Uint16(b[1:3]))) / 10
	temperature2 := float64(int16(binary.LittleEndian.Uint16(b[3:5]))) / 10
	temperature3 := float64(int16(binary.LittleEndian.Uint16(b[5:7]))) / 10
	temperature4 := float64(int16(binary.LittleEndian.Uint16(b[7:9]))) / 10

	if temperature1 > tMax || temperature1 < tMin ||
		temperature2 > tMax || temperature2 < tMin ||
		temperature3 > tMax || temperature3 < tMin ||
		temperature4 > tMax || temperature4 < tMin {
		return r, ErrInvalidTemperature
	}

	r.Set(KeyTimestamp, time.Now())
	r.Set(KeyTemperature1, temperature1)
	r.Set(KeyTemperature2, temperature2)
	r.Set(KeyTemperature3, temperature3)
	r.Set(KeyTemperature4, temperature4)
	r.Set(KeyOut1, b[9]&out1 > 0)
	r.Set(KeyOut2, b[9]&out2 > 0)
	return r, nil
}

// Close the ReadCloser handler.
func (h *UVR42Handler) Close() error {
	return nil
}
