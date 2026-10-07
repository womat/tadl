package datalogger

import (
	"errors"
	"testing"
)

// The frames are written out by hand from the DL-Bus protocol description v1.7,
// not built with FrameUVR42/FrameUVR31, so the decoders are checked against the
// protocol rather than against the encoder.

func TestDecodeUVR42(t *testing.T) {
	b := []byte{
		0x10,       // UVR42
		0xD7, 0x00, // 215 → 21.5 °C
		0xC5, 0x01, // 453 → 45.3 °C
		0x9C, 0xFF, // -100 → -10.0 °C
		0x00, 0x00, // 0.0 °C
		0x60, // out1 (bit 5) and out2 (bit 6)
	}
	r, invalid, err := NewUVR42().decode(b)
	if err != nil || len(invalid) != 0 {
		t.Fatalf("decode: err=%v invalid=%v", err, invalid)
	}
	want := map[string]float64{KeyTemperature1: 21.5, KeyTemperature2: 45.3, KeyTemperature3: -10, KeyTemperature4: 0}
	for k, v := range want {
		if got := r.Float64(k); got != v {
			t.Errorf("%s = %v, want %v", k, got, v)
		}
	}
	if !r.Bool(KeyOut1) || !r.Bool(KeyOut2) {
		t.Errorf("outputs = %v/%v, want true/true", r.Bool(KeyOut1), r.Bool(KeyOut2))
	}
	if !r.Exists(KeyTimestamp) {
		t.Error("timestamp missing")
	}
}

func TestDecodeUVR31(t *testing.T) {
	b := []byte{
		0x30,       // UVR31
		0xC8, 0x00, // 20.0 °C
		0x2C, 0x01, // 30.0 °C
		0x38, 0xFF, // -20.0 °C
		0x20, // out1
	}
	r, _, err := NewUVR31().decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if r.Float64(KeyTemperature1) != 20 || r.Float64(KeyTemperature2) != 30 || r.Float64(KeyTemperature3) != -20 {
		t.Errorf("temperatures = %v", r)
	}
	if !r.Bool(KeyOut1) {
		t.Error("out1 = false, want true")
	}
}

// A single sensor out of range must not cost the other values.
func TestDecodeSensorOutOfRange(t *testing.T) {
	b := FrameUVR42([4]float64{21.5, 400, -7.2, 0}, true, false)
	r, invalid, err := NewUVR42().decode(b)
	if err != nil {
		t.Fatalf("decode: %v", err)
	}
	if len(invalid) != 1 || invalid[0].key != KeyTemperature2 || invalid[0].value != 400 {
		t.Errorf("invalid = %v, want temperature2 = 400", invalid)
	}
	if r.Exists(KeyTemperature2) {
		t.Error("out-of-range temperature2 is in the record")
	}
	if r.Float64(KeyTemperature1) != 21.5 || r.Float64(KeyTemperature3) != -7.2 || !r.Bool(KeyOut1) {
		t.Errorf("remaining values lost: %v", r)
	}
}

func TestDecodeAllSensorsOutOfRange(t *testing.T) {
	b := FrameUVR31([3]float64{-60, 400, 999}, false)
	if _, invalid, err := NewUVR31().decode(b); !errors.Is(err, ErrInvalidTemperature) || len(invalid) != 3 {
		t.Errorf("err=%v invalid=%v, want ErrInvalidTemperature and 3 invalid", err, invalid)
	}
}

func TestDecodeRejects(t *testing.T) {
	tests := []struct {
		name string
		b    []byte
		want error
	}{
		{"short", []byte{0x10, 0x00}, ErrInvalidSize},
		{"other device", append([]byte{0x30}, make([]byte, 9)...), ErrUnsupportedDevice},
	}
	for _, tt := range tests {
		if _, _, err := NewUVR42().decode(tt.b); !errors.Is(err, tt.want) {
			t.Errorf("%s: err=%v, want %v", tt.name, err, tt.want)
		}
	}
}

func TestCloseWithoutWatch(t *testing.T) {
	if err := NewUVR42().Close(); err != nil {
		t.Errorf("Close without Watch: %v", err)
	}
}

func TestCloseAfterInputClosed(t *testing.T) {
	h := NewUVR42()
	rx := make(chan []byte)
	out, err := h.Watch(rx)
	if err != nil {
		t.Fatal(err)
	}
	close(rx)
	for range out {
	}
	if err := h.Close(); err != nil {
		t.Errorf("Close after input closed: %v", err)
	}
}
