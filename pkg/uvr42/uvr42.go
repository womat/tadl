package uvr42

import (
	"encoding/binary"
	"errors"
	"io"
	"math"
	"time"
)

var (
	ErrInvalidSize        = errors.New("invalid frame size")
	ErrInvalidTemperature = errors.New("invalid temperature")
	ErrUnsupportedDevice  = errors.New("unsupported device id")
)

type Handler struct {
	io.ReadCloser
	lastValue UVR42
}

type UVR42 struct {
	Time  time.Time
	Temp1 float64
	Temp2 float64
	Temp3 float64
	Temp4 float64
	Out1  bool
	Out2  bool
}

const (
	// device Id
	uvr42 = 0x10

	// max temperature range
	tMax = 300
	tMin = -50

	// max temperature difference to the last measurement
	maxDelta = 50

	// bitmask of Out1 and Out2
	out1 = 1 << 5
	out2 = 1 << 6
)

// New generate a new handler struct
func New() *Handler {
	return &Handler{}
}

// Connect defines the io.ReadWriterCloser
func (h *Handler) Connect(handler io.ReadCloser) error {
	h.ReadCloser = handler
	return nil
}

// Get reads the DL buffer, convert the buffer to an uvr42 structure and check the values
// the temperature values ar valid, if the current values are within a temperature range (Tmax, Tmin) and
// the difference to the last measured values are less than maxDelta
func (h *Handler) Get() (UVR42, error) {
	var f UVR42
	b := make([]byte, 64)

	n, err := h.Read(b)

	if err != nil {
		return f, err
	}

	if n != 10 {
		return f, ErrInvalidSize
	}

	if b[0] != uvr42 {
		return f, ErrUnsupportedDevice
	}

	f.Time = time.Now()
	f.Temp1 = float64(int16(binary.LittleEndian.Uint16(b[1:3]))) / 10
	f.Temp2 = float64(int16(binary.LittleEndian.Uint16(b[3:5]))) / 10
	f.Temp3 = float64(int16(binary.LittleEndian.Uint16(b[5:7]))) / 10
	f.Temp4 = float64(int16(binary.LittleEndian.Uint16(b[7:9]))) / 10

	f.Out1 = b[9]&out1 > 0
	f.Out2 = b[9]&out2 > 0

	if !h.lastValue.Time.IsZero() && (math.Abs(f.Temp1-h.lastValue.Temp1) > maxDelta ||
		math.Abs(f.Temp2-h.lastValue.Temp2) > maxDelta ||
		math.Abs(f.Temp3-h.lastValue.Temp3) > maxDelta ||
		math.Abs(f.Temp4-h.lastValue.Temp4) > maxDelta) {
		return f, ErrInvalidTemperature
	}

	if f.Temp1 > tMax || f.Temp2 > tMax || f.Temp3 > tMax || f.Temp4 > tMax ||
		f.Temp1 < tMin || f.Temp2 < tMin || f.Temp3 < tMin || f.Temp4 < tMin {
		return f, ErrInvalidTemperature
	}

	h.lastValue = f
	return f, nil
}

// Close the handler
func (m *Handler) Close() error {
	return m.ReadCloser.Close()
}
