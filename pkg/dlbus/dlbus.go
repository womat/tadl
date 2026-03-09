// Package dlbus implements the DL-Bus protocol decoder from Technische Alternative.
// The DL-Bus is a single-wire serial protocol used to read data from heating controllers.
//
// Protocol description:
//   - Sync sequence: detected as 16 consecutive high bits on the decoded bit stream
//   - Each byte: 1 start bit (low) + 8 data bits (LSB first) + 1 stop bit (high)
//   - Frame ends when a new sync sequence is detected
//
// Usage:
//
//	h := dlbus.New()
//	ch, err := h.Watch(ctx, bitChannel)
//	for frame := range ch {
//	    // process frame
//	}
//	h.Close()
package dlbus

import (
	"context"
	"errors"
	"sync"
	"sync/atomic"

	"github.com/womat/golib/manchester/decoder"
)

// ErrWatcherAlreadyStarted is returned by Watch if the decoding goroutine is already running.
var ErrWatcherAlreadyStarted = errors.New("watcher already started")

// Stats holds counters for monitoring the DL-Bus decoder.
type Stats struct {
	DroppedFrames  uint64 // number of frames discarded because the receiver was not keeping up
	ProtocolErrors uint64 // number of frames discarded due to a missing stop bit
}

// Handler decodes a DL-Bus bit stream into complete data frames.
type Handler struct {
	watching atomic.Bool
	wg       sync.WaitGroup
	cancel   context.CancelFunc

	droppedFrames  atomic.Uint64
	protocolErrors atomic.Uint64
}

// New returns a new Handler.
// Call Watch to start decoding.
func New() *Handler {
	return &Handler{}
}

// Watch starts the decoding goroutine and returns a channel on which complete
// frames are delivered. Each frame is a freshly allocated byte slice.
// It returns ErrWatcherAlreadyStarted if the decoder is already running.
// Call Close() to stop the goroutine and wait for it to terminate.
func (r *Handler) Watch(rx <-chan decoder.Bit) (<-chan []byte, error) {
	if !r.watching.CompareAndSwap(false, true) {
		return nil, ErrWatcherAlreadyStarted
	}

	ctx, cancel := context.WithCancel(context.Background())
	r.cancel = cancel

	tx := make(chan []byte, 10)
	r.wg.Add(1)
	go r.readFrames(ctx, rx, tx)
	return tx, nil
}

// Close stops the decoding goroutine and waits for it to terminate.
// It is a no-op if Watch has never been called.
func (r *Handler) Close() error {
	if !r.watching.Load() {
		return nil
	}

	r.cancel()
	r.wg.Wait()
	return nil
}

// Stats returns a snapshot of the current decoder counters.
func (r *Handler) Stats() Stats {
	return Stats{
		DroppedFrames:  r.droppedFrames.Load(),
		ProtocolErrors: r.protocolErrors.Load(),
	}
}

// readFrames receives bits on rx, assembles them into bytes and collects bytes into
// frames. A complete frame is sent to tx when a sync sequence is detected.
// It stops when ctx is cancelled or rx is closed.
func (r *Handler) readFrames(ctx context.Context, rx <-chan decoder.Bit, tx chan []byte) {
	defer func() {
		close(tx)
		r.watching.Store(false)
		r.wg.Done()
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
