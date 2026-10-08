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
	"sync"
	"sync/atomic"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR31Handler decodes UVR31 data frames read from the DL-Bus.
type UVR31Handler struct {
	watching atomic.Bool
	wg       sync.WaitGroup
	mu       sync.Mutex // guards cancel, and Watch against a concurrent Close
	cancel   context.CancelFunc
	counters
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
// Call Close() to stop the goroutine and wait for it to finish.
func (h *UVR31Handler) Watch(rx <-chan []byte, opts ...Option) (<-chan keyvalue.Record, error) {
	h.mu.Lock()
	defer h.mu.Unlock()

	if !h.watching.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}

	ctx, cancel := context.WithCancel(context.Background())
	h.cancel = cancel

	o := &options{}
	for _, opt := range opts {
		opt(o)
	}
	logger := o.logger
	warner := &rangeWarner{logger: logger}

	// Buffered like the DL-Bus frames: the reader may be busy publishing, and
	// an unbuffered channel would drop every record that arrives meanwhile.
	c := make(chan keyvalue.Record, 10)
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

				kv, invalid, err := h.decode(b)
				warner.warn(invalid, time.Now())
				if err != nil {
					h.rejected.Add(1)
					// Frames of other devices and broken frames are common on the
					// bus, so they are only reported at debug level.
					if logger != nil {
						logger.Debug("failed to decode frame", "error", err)
					}
					continue
				}
				h.decoded.Add(1)
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
// Temperatures out of range are left out of the record and returned.
// Returns ErrInvalidSize, ErrUnsupportedDevice, or ErrInvalidTemperature
// (no temperature in range) if the frame cannot be decoded.
func (h *UVR31Handler) decode(b []byte) (keyvalue.Record, []outOfRange, error) {
	const (
		out1      byte = 1 << 5 // bitmask for Out1 (bit 5)
		frameSize      = 8
	)

	r := keyvalue.NewRecord()

	if len(b) != frameSize {
		return r, nil, ErrInvalidSize
	}

	if b[0] != UVR31 {
		return r, nil, ErrUnsupportedDevice
	}

	invalid, err := setTemperatures(r, b[1:7], []string{KeyTemperature1, KeyTemperature2, KeyTemperature3})
	if err != nil {
		return r, invalid, err
	}

	r.Set(KeyTimestamp, time.Now())
	r.Set(KeyOut1, b[7]&out1 > 0)
	return r, invalid, nil
}

// Close stops the decoding goroutine and waits for it to terminate.
// It is a no-op if Watch has never been called.
func (h *UVR31Handler) Close() error {
	h.mu.Lock()
	cancel := h.cancel
	h.cancel = nil
	h.mu.Unlock()

	if cancel == nil {
		return nil
	}

	cancel()
	h.wg.Wait()
	return nil
}
