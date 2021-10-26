package datalogger

import (
	"encoding/binary"
	"io"
	"math"
	"time"
)

// UVR42Handler is the handler to read an uvr42 dataframe.
type UVR42Handler struct {
	io.ReadCloser
	lastValue UVR42Frame
}

// UVR42Frame is the dataframe of an uvr42 controller.
type UVR42Frame struct {
	TimeStamp     time.Time
	Temperature1  float64
	Temperature2  float64
	Temperature3  float64
	Temperature4  float64
	Out1          bool
	Out2          bool
	RotationSpeed int
}

// NewUVR42 generate a new handler struct for UVR42.
func NewUVR42() *UVR42Handler {
	return &UVR42Handler{}
}

// Connect defines the io.ReadWriterCloser
func (h *UVR42Handler) Connect(handler io.ReadCloser) error {
	h.ReadCloser = handler
	return nil
}

// Get reads the DL buffer, convert the buffer to an uvr42 structure and check the values.
// The temperature values are valid, if the current values are within a temperature range (tMax, tMin) and
// the difference to the last measured values are less than maxDelta.
func (h *UVR42Handler) Get() (error, interface{}) {
	var f UVR42Frame
	// bitmask of Out1 and Out2
	const out1 = 1 << 5
	const out2 = 1 << 6

	b := make([]byte, 64)

	n, err := h.Read(b)

	if err != nil {
		return err, f
	}

	if n != 10 {
		return ErrInvalidSize, f
	}

	if b[0] != uvr42 {
		return ErrUnsupportedDevice, f
	}

	f.TimeStamp = time.Now()
	f.Temperature1 = float64(int16(binary.LittleEndian.Uint16(b[1:3]))) / 10
	f.Temperature2 = float64(int16(binary.LittleEndian.Uint16(b[3:5]))) / 10
	f.Temperature3 = float64(int16(binary.LittleEndian.Uint16(b[5:7]))) / 10
	f.Temperature4 = float64(int16(binary.LittleEndian.Uint16(b[7:9]))) / 10

	f.Out1 = b[9]&out1 > 0
	f.Out2 = b[9]&out2 > 0

	if !h.lastValue.TimeStamp.IsZero() && (math.Abs(f.Temperature1-h.lastValue.Temperature1) > maxDelta ||
		math.Abs(f.Temperature2-h.lastValue.Temperature2) > maxDelta ||
		math.Abs(f.Temperature3-h.lastValue.Temperature3) > maxDelta ||
		math.Abs(f.Temperature4-h.lastValue.Temperature4) > maxDelta) {
		return ErrInvalidTemperature, f
	}

	if f.Temperature1 > tMax || f.Temperature2 > tMax || f.Temperature3 > tMax || f.Temperature4 > tMax ||
		f.Temperature1 < tMin || f.Temperature2 < tMin || f.Temperature3 < tMin || f.Temperature4 < tMin {
		return ErrInvalidTemperature, f
	}

	return nil, f
}

// Close the ReadCloser handler.
func (h *UVR42Handler) Close() error {
	return h.ReadCloser.Close()
}
