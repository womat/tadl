package datalogger

import (
	"context"
	"encoding/binary"
	"log/slog"
	"sync"
	"sync/atomic"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR42Handler decodes UVR42 data frames read from the DL-Bus.
type UVR42Handler struct {
	wg       sync.WaitGroup
	logger   *slog.Logger // Optional logger for debugging and info
	watching atomic.Bool
}

// NewUVR42 returns a new UVR42Handler.
// Call Watch to start decoding.
func NewUVR42() *UVR42Handler {
	return &UVR42Handler{}
}

// Watch starts the decoding goroutine and returns a channel on which decoded
// records are delivered. It returns ErrWatcherAlreadyStarted if the decoder
// is already running.
// Cancel ctx to stop the goroutine, then call Close to wait for it to terminate.
func (h *UVR42Handler) Watch(ctx context.Context, rx <-chan []byte, opts ...Option) (<-chan keyvalue.Record, error) {
	if !h.watching.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}

	c := make(chan keyvalue.Record)
	h.logger = nil

	for _, opt := range opts {
		opt(h)
	}

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
				if h.logger != nil {
					h.logger.Debug("context cancelled, terminating UVR42 handler")
				}
				return
			case b, open := <-rx:
				if !open {
					if h.logger != nil {
						h.logger.Debug("input channel closed, terminating UVR42 handler")
					}
					return
				}

				kv, err := h.decode(b)
				if err != nil {
					if h.logger != nil {
						h.logger.Warn("failed to decode frame", "error", err)
					}
					continue
				}
				select {
				case c <- kv:
				default:
					if h.logger != nil {
						h.logger.Warn("output channel full, dropping frame")
					}
					continue
				}
			}
		}

	}()

	return c, nil
}

// decode parses a raw UVR42 frame and returns a keyvalue.Record with the
// measured temperatures and output states.
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
// Shutdown is triggered by cancelling the context passed to Watch.
// Close is a no-op if Watch has never been called.
func (h *UVR42Handler) Close() error {
	if !h.watching.Load() {
		return nil
	}

	h.wg.Wait()
	return nil
}

func (h *UVR42Handler) SetLogger(l *slog.Logger) {
	h.logger = l
}
