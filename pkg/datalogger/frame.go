package datalogger

import (
	"encoding/binary"
	"math"
)

// FrameUVR42 builds the data bytes a UVR42 sends after its SYNC, the inverse
// of what UVR42Handler decodes: device ID, four temperatures in 1/10 °C and
// the output byte. It is meant for tests and for emulating a controller.
func FrameUVR42(temperatures [4]float64, out1, out2 bool) []byte {
	b := []byte{UVR42}
	for _, t := range temperatures {
		b = appendTemperature(b, t)
	}
	return append(b, outputs(out1, out2))
}

// FrameUVR31 builds the data bytes a UVR31 sends after its SYNC: device ID,
// three temperatures in 1/10 °C and the output byte.
func FrameUVR31(temperatures [3]float64, out1 bool) []byte {
	b := []byte{UVR31}
	for _, t := range temperatures {
		b = appendTemperature(b, t)
	}
	return append(b, outputs(out1, false))
}

// appendTemperature appends a temperature as int16 little-endian in 1/10 °C.
func appendTemperature(b []byte, t float64) []byte {
	return binary.LittleEndian.AppendUint16(b, uint16(int16(math.Round(t*10))))
}

// outputs returns the output byte: bit 5 = Out1, bit 6 = Out2.
func outputs(out1, out2 bool) byte {
	var o byte
	if out1 {
		o |= 1 << 5
	}
	if out2 {
		o |= 1 << 6
	}
	return o
}
