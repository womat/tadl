// UVR31Handler decodes DL-Bus frames from a Technische Alternative UVR31
// controller (device ID 0x30).
//
// Frame layout (8 bytes):
//
//	Byte 0:    Device ID (0x30)
//	Bytes 1–2: Temperature 1 (int16, little-endian, unit 0.1 °C)
//	Bytes 3–4: Temperature 2 (int16, little-endian, unit 0.1 °C)
//	Bytes 5–6: Temperature 3 (int16, little-endian, unit 0.1 °C)
//	Byte 7:    Output bits (bit 5 = Out1)
package datalogger

import (
	"context"
	"encoding/binary"
	"sync"
	"sync/atomic"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR31Handler decodes UVR31 data frames read from the DL-Bus.
type UVR31Handler struct {
	wg       sync.WaitGroup
	watching atomic.Bool
}

// NewUVR31 returns a new UVR31Handler ready to use.
// Call Watch to start decoding frames.
func NewUVR31() *UVR31Handler {
	return &UVR31Handler{}
}

// Watch starts the decoding goroutine and returns a channel on which decoded
// keyvalue.Records are delivered. Each record contains the measured temperatures
// and output states keyed by the Key* constants.
//
// Returns ErrWatcherAlreadyStarted if the handler is already running.
// Cancel ctx to stop the goroutine, then call Close to wait for it to finish.
func (h *UVR31Handler) Watch(ctx context.Context, rx <-chan []byte, opts ...Option) (<-chan keyvalue.Record, error) {
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
					logger.Debug("context cancelled, terminating UVR31 handler")
				}
				return
			case b, open := <-rx:
				if !open {
					if logger != nil {
						logger.Debug("input channel closed, terminating UVR31 handler")
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

// decode parses a raw 8-byte UVR31 frame into a keyvalue.Record.
// Returns ErrInvalidSize, ErrUnsupportedDevice, or ErrInvalidTemperature
// if the frame cannot be decoded.
func (h *UVR31Handler) decode(b []byte) (keyvalue.Record, error) {
	const (
		out1      byte = 1 << 5 // bitmask for Out1 (bit 5)
		frameSize      = 8
	)

	r := keyvalue.NewRecord()

	if len(b) != frameSize {
		return r, ErrInvalidSize
	}

	if b[0] != UVR31 {
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

// Close blocks until the decoding goroutine has terminated.
// It is a no-op if Watch has never been called.
func (h *UVR31Handler) Close() error {
	if !h.watching.Load() {
		return nil
	}

	h.wg.Wait()
	return nil
}
