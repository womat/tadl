// UVR42Handler decodes DL-Bus frames from a Technische Alternative UVR42
// controller (device ID 0x10).
//
// Frame layout (10 bytes):
//
//	Byte 0:    Device ID (0x10)
//	Bytes 1–2: Temperature 1 (int16, little-endian, unit 0.1 °C)
//	Bytes 3–4: Temperature 2 (int16, little-endian, unit 0.1 °C)
//	Bytes 5–6: Temperature 3 (int16, little-endian, unit 0.1 °C)
//	Bytes 7–8: Temperature 4 (int16, little-endian, unit 0.1 °C)
//	Byte 9:    Output bits (bit 5 = Out1, bit 6 = Out2)
package datalogger

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR42Handler decodes UVR42 data frames read from the DL-Bus.
type UVR42Handler struct {
	wg       sync.WaitGroup
	watching atomic.Bool
}

// NewUVR42 returns a new UVR42Handler ready to use.
// Call Watch to start decoding frames.
func NewUVR42() *UVR42Handler {
	return &UVR42Handler{}
}

// Watch starts the decoding goroutine and returns a channel on which decoded
// keyvalue.Records are delivered. Each record contains the measured temperatures
// and output states keyed by the Key* constants.
//
// Returns ErrWatcherAlreadyStarted if the handler is already running.
// Cancel ctx to stop the goroutine, then call Close to wait for it to finish.
func (h *UVR42Handler) Watch(ctx context.Context, rx <-chan []byte, opts ...Option) (<-chan keyvalue.Record, error) {
	if !h.watching.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}

	c := make(chan keyvalue.Record)

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	logger := o.logger

	h.wg.Add(1)
	go func() {
		defer func() {
			close(c)
			h.watching.Store(false)
			h.wg.Done()
		}()

		for {
			select {
			case <-ctx.Done():
				if logger != nil {
					logger.Debug("context cancelled, terminating UVR42 handler")
				}
				return
			case b, open := <-rx:
				if !open {
					if logger != nil {
						logger.Debug("input channel closed, terminating UVR42 handler")
					}
					return
				}

				kv, err := h.decode(b)
				if err != nil {
					if logger != nil {
						logger.Warn("failed to decode frame", "error", err)
					}
					continue
				}
				select {
				case c <- kv:
				default:
					if logger != nil {
						logger.Warn("output channel full, dropping frame")
					}
					continue
				}
			}
		}

	}()

	return c, nil
}

// decode parses a raw 10-byte UVR42 frame into a keyvalue.Record.
// Returns ErrInvalidSize, ErrUnsupportedDevice, or ErrInvalidTemperature
// if the frame cannot be decoded.
func (h *UVR42Handler) decode(b []byte) (keyvalue.Record, error) {
	const (
		out1      byte = 1 << 5 // bitmask for Out1 (bit 5)
		out2      byte = 1 << 6 // bitmask for Out2 (bit 6)
		frameSize      = 10
	)

	r := keyvalue.NewRecord()

	if len(b) != frameSize {
		return r, ErrInvalidSize
	}

	if b[0] != UVR42 {
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

// Close blocks until the decoding goroutine has terminated.
// It is a no-op if Watch has never been called.
func (h *UVR42Handler) Close() error {
	if !h.watching.Load() {
		return nil
	}

	h.wg.Wait()
	return nil
}
