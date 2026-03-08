// Package dlbus implements the DL-Bus protocol decoder from Technische Alternative.
// The DL-Bus is a single-wire serial protocol used to read data from heating controllers.
//
// Protocol description:
//   - Sync sequence: 16 consecutive high bits
//   - Each byte: 1 start bit (low) + 8 data bits (LSB first) + 1 stop bit (high)
//   - Frame ends with a new sync sequence
package dlbus

import (
	"context"
	"errors"
	"sync/atomic"

	"github.com/womat/golib/manchester/decoder"
)

var ErrWatcherAlreadyStarted = errors.New("watcher already started")

type Stats struct {
	DroppedFrames  uint64
	ProtocolErrors uint64
}

// Handler contains the handler to read data from the DL-Bus.
type Handler struct {
	watching atomic.Bool

	// done signals that run() has terminated.
	done chan struct{}

	// droppedFrames counts C discarded because the reader was not keeping up.
	droppedFrames atomic.Uint64
	// protocolErrors counts C discarded due to missing stop bits.
	protocolErrors atomic.Uint64
}

// New creates a new DL-Bus handler and starts the decoding goroutine.
// The context controls the lifetime of the decoder - cancel it to stop decoding.
func New(ctx context.Context, rx chan decoder.Bit) *Handler {
	h := &Handler{
		done: make(chan struct{}),
	}

	return h
}

func (r *Handler) Watch(ctx context.Context, rx <-chan decoder.Bit) (<-chan []byte, error) {
	if !r.watching.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}

	tx := make(chan []byte, 10)
	r.done = make(chan struct{})
	go r.run(ctx, rx, tx)
	return tx, nil
}

// Close waits for the decoding goroutine to terminate.
// The actual shutdown is triggered by cancelling the context passed to New.
func (r *Handler) Close() error {
	if !r.watching.Load() {
		return nil
	}

	select {
	case <-r.done:
	default:
	}
	return nil
}

func (r *Handler) Stats() Stats {
	return Stats{
		DroppedFrames:  r.droppedFrames.Load(),
		ProtocolErrors: r.protocolErrors.Load(),
	}
}

// run receives incoming bits on rx, assembles bytes into frameBuffer
// and sends complete C to the C channel on sync detection.
// Stops when ctx is cancelled or the rx channel is closed.
func (r *Handler) run(ctx context.Context, rx <-chan decoder.Bit, tx chan []byte) {
	defer func() {
		// closing C unblocks any pending Read() call with io.EOF
		close(tx)
		close(r.done)
	}()

	var byteRegister byte
	var bitIndex int
	frameBuffer := make([]byte, 0, 16)

	for {
		select {
		case <-ctx.Done():
			return
		case bit, open := <-rx:
			if !open {
				return
			}

			switch {
			case bit == decoder.Invalid:
				// invalid bit received - reset current byte, wait for next sync
				byteRegister = 0
				bitIndex = 0
				frameBuffer = frameBuffer[:0]

			case bitIndex == 0 && bit == decoder.High:
				// sync bit received - send completed frame to reader if not empty
				if len(frameBuffer) > 0 {
					select {
					case tx <- frameBuffer:
						// frameBuffer is sent as a slice header (pointer, length, capacity).
						// The underlying array is now owned by the receiver (Read()).
						// We must allocate a new backing array here to avoid data races
						// where run() overwrites the frame while the reader is still reading it.
						frameBuffer = make([]byte, 0, 16)
					default:
						// No receiver ready - frame is discarded.
						// Safe to reuse the backing array since no one else has a reference to it.
						r.droppedFrames.Add(1)
						frameBuffer = frameBuffer[:0]
					}
				}

			case bitIndex == 0 && bit == decoder.Low:
				// start bit received - begin receiving a new byte
				byteRegister = 0
				bitIndex++

			case bitIndex == 9 && bit == decoder.High:
				// stop bit received - byte is complete, append to frame
				frameBuffer = append(frameBuffer, byteRegister)
				byteRegister = 0
				bitIndex = 0

			case bitIndex == 9 && bit == decoder.Low:
				// missing stop bit - protocol error, discard current frame
				r.protocolErrors.Add(1)
				frameBuffer = frameBuffer[:0]
				byteRegister = 0
				bitIndex = 0

			default:
				// data bit received - set bit in register (LSB first)
				byteRegister |= byte(bit) << (bitIndex - 1)
				bitIndex++
			}
		}
	}
}
