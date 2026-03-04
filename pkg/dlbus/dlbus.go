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
	"fmt"
	"io"
	"sync/atomic"

	"github.com/womat/golib/manchester/decoder"
)

// ReadCloser contains the handler to read data from the DL-Bus.
type ReadCloser struct {
	// rx receives the decoded bit stream from the manchester decoder.
	rx chan decoder.Bit
	// frames transports complete DL-Bus frames from run() to Read().
	// The channel has a capacity of 1 to allow run() to continue while Read() is processing.
	frames chan []byte
	// done signals that run() has terminated.
	done chan struct{}

	// droppedFrames counts frames discarded because the reader was not keeping up.
	droppedFrames atomic.Uint64
	// protocolErrors counts frames discarded due to missing stop bits.
	protocolErrors atomic.Uint64
}

// NewReader creates a new DL-Bus handler and starts the decoding goroutine.
// The context controls the lifetime of the decoder - cancel it to stop decoding.
func NewReader(ctx context.Context, rx chan decoder.Bit) *ReadCloser {
	h := &ReadCloser{
		rx:     rx,
		frames: make(chan []byte, 1),
		done:   make(chan struct{}),
	}

	go h.run(ctx)

	return h
}

// Read returns the next complete DL-Bus frame.
// Blocks until a frame is available or the decoder is stopped.
// Returns io.EOF when the decoder has been stopped via context cancellation.
func (r *ReadCloser) Read(b []byte) (int, error) {
	frame, ok := <-r.frames
	if !ok {
		// frames channel was closed by run() - decoder has stopped
		return 0, io.EOF
	}
	n := copy(b, frame)
	return n, nil
}

// Close waits for the decoding goroutine to terminate.
// The actual shutdown is triggered by cancelling the context passed to NewReader.
func (r *ReadCloser) Close() error {
	<-r.done
	return nil
}

func (r *ReadCloser) Info() string {
	return fmt.Sprintf(
		"DL-Bus dropped frames: %v, protocol errors: %v",
		r.droppedFrames.Load(),
		r.protocolErrors.Load(),
	)
}

// run receives incoming bits on rx, assembles bytes into frameBuffer
// and sends complete frames to the frames channel on sync detection.
// Stops when ctx is cancelled or the rx channel is closed.
func (r *ReadCloser) run(ctx context.Context) {
	defer func() {
		// closing frames unblocks any pending Read() call with io.EOF
		close(r.frames)
		close(r.done)
	}()

	var byteRegister byte
	var bitIndex int
	frameBuffer := make([]byte, 0, 16)

	for {
		select {
		case <-ctx.Done():
			return
		case bit, open := <-r.rx:
			if !open {
				return
			}

			switch {
			case bit == decoder.Invalid:
				// invalid bit received - reset current byte, wait for next sync
				byteRegister = 0
				bitIndex = 0

			case bitIndex == 0 && bit == decoder.High:
				// sync bit received - send completed frame to reader if not empty
				if len(frameBuffer) > 0 {
					select {
					case r.frames <- frameBuffer:
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
