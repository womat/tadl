package datalogger

import (
	"context"
	"encoding/binary"
	"log/slog"
	"sync/atomic"
	"time"

	"github.com/womat/golib/keyvalue"
)

// UVR31Handler is the handler to read a UVR31 dataframe.
type UVR31Handler struct {
	// done signals that run() has terminated.
	done     chan struct{}
	logger   *slog.Logger // Optional logger for debugging and info
	watching atomic.Bool
}

func NewUVR31() *UVR31Handler {
	return &UVR31Handler{done: make(chan struct{})}
}

func (h *UVR31Handler) Watch(ctx context.Context, rx chan []byte, opts ...Option) (<-chan keyvalue.Record, error) {
	if !h.watching.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}

	c := make(chan keyvalue.Record)
	h.logger = nil

	for _, opt := range opts {
		opt(h)
	}

	go func() {
		defer func() {
			// closing C unblocks any pending Read() call with io.EOF
			close(c)
			close(h.done)
			h.watching.Store(false) // ← Reset nach Goroutine-Ende
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

// Get reads a UVR31 frame from the DL-Bus, parses it and validates the temperature values.
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

// Close the ReadCloser handler.
func (h *UVR31Handler) Close() error {
	if !h.watching.Load() {
		return nil
	}

	<-h.done
	return nil
}

func (h *UVR31Handler) SetLogger(l *slog.Logger) {
	h.logger = l
	return
}
