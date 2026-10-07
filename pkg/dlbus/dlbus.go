// Package dlbus implements the DL-Bus protocol decoder from Technische Alternative.
// The DL-Bus is a single-wire serial protocol used to read data from heating controllers.
// It is described in the DL-Bus protocol description v1.7 (German):
// https://www.mikrocontroller.net/attachment/646125/DL-Bus_Protokoll_v1.7.pdf
//
// Protocol description:
//   - Sync sequence: 16 consecutive high bits on the decoded bit stream
//   - Each byte: 1 start bit (low) + 8 data bits (LSB first) + 1 stop bit (high)
//   - Frame ends when a new sync sequence is detected
//
// Line polarity: a line driver such as an optocoupler inverts the signal, and
// the SYNC then arrives as a run of low bits. The handler detects the polarity
// at every SYNC - a run of at least syncBits identical bits that ends with the
// opposite one, the start bit - and inverts the bits when needed, so the bit
// stream may come from a Manchester decoder set to either convention. Data
// bytes hold at most 9 identical bits in a row, so they never pass for a SYNC.
// Bits before the first SYNC are discarded.
//
// Usage:
//
//	h := dlbus.New(dlbus.WithLogger(slog.Default()))
//	ch, err := h.Watch(bitChannel)
//	for frame := range ch {
//	    // process frame
//	}
//	h.Close()
package dlbus

import (
	"context"
	"errors"
	"log/slog"
	"sync"
	"sync/atomic"

	"github.com/womat/golib/manchester/decoder"
)

// ErrWatcherAlreadyStarted is returned by Watch if the decoding goroutine is already running.
var ErrWatcherAlreadyStarted = errors.New("watcher already started")

// syncBits is the shortest run of identical bits taken for a SYNC. The SYNC
// has 16; data bytes have at most 9 identical bits in a row (0x00 or 0xff
// next to its start or stop bit). The margin leaves room for a SYNC whose
// first bits were lost.
const syncBits = 12

// Stats holds counters for monitoring the DL-Bus decoder.
type Stats struct {
	DroppedFrames  uint64 // number of frames discarded because the receiver was not keeping up
	ProtocolErrors uint64 // number of frames discarded due to a missing stop bit
	InvertedLine   bool   // the last SYNC arrived inverted, as behind an optocoupler
}

// Handler decodes a DL-Bus bit stream into complete data frames.
type Handler struct {
	watching atomic.Bool
	wg       sync.WaitGroup
	cancel   context.CancelFunc
	logger   *slog.Logger

	droppedFrames  atomic.Uint64
	protocolErrors atomic.Uint64
	invertedLine   atomic.Bool
}

// Option configures a Handler.
type Option func(*Handler)

// WithLogger reports the detected line polarity and changes of it.
// If not set, nothing is logged.
func WithLogger(l *slog.Logger) Option {
	return func(h *Handler) {
		h.logger = l
	}
}

// New returns a new Handler.
// Call Watch to start decoding.
func New(opts ...Option) *Handler {
	h := &Handler{}
	for _, opt := range opts {
		opt(h)
	}
	return h
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
		InvertedLine:   r.invertedLine.Load(),
	}
}

// readFrames receives bits on rx, assembles them into bytes and collects bytes into
// frames. A complete frame is sent to tx when the next sync sequence begins.
// A frame only starts after a SYNC: after an invalid bit or a missing stop bit
// the handler waits for the next one, instead of taking any low bit for a
// start bit. It stops when ctx is cancelled or rx is closed.
func (r *Handler) readFrames(ctx context.Context, rx <-chan decoder.Bit, tx chan []byte) {
	defer func() {
		close(tx)
		r.watching.Store(false)
		r.wg.Done()
	}()

	var byteRegister byte
	var bitIndex int
	var inFrame bool // a SYNC was seen and the current frame is intact
	var line polarity
	frameBuffer := make([]byte, 0, 16)

	// send hands the current frame to the reader and starts an empty one.
	send := func() {
		if len(frameBuffer) == 0 {
			return
		}
		select {
		case tx <- frameBuffer:
			// The backing array now belongs to the reader: start a new one, so
			// the next frame does not overwrite it while it is being read.
			frameBuffer = make([]byte, 0, 16)
		default:
			// No receiver ready - frame is discarded.
			// Safe to reuse the backing array since no one else has a reference to it.
			r.droppedFrames.Add(1)
			frameBuffer = frameBuffer[:0]
		}
	}

	// drop discards the current frame and waits for the next SYNC.
	drop := func() {
		frameBuffer = frameBuffer[:0]
		byteRegister = 0
		bitIndex = 0
		inFrame = false
	}

	for {
		select {
		case <-ctx.Done():
			return
		case raw, open := <-rx:
			if !open {
				return
			}

			wasKnown, wasInverted := line.known, line.inverted
			bit, syncEnded := line.next(raw)
			if line.known && (!wasKnown || line.inverted != wasInverted) {
				r.invertedLine.Store(line.inverted)
				if r.logger != nil {
					if wasKnown {
						r.logger.Warn("DL-Bus line polarity changed", "inverted", line.inverted)
					} else {
						r.logger.Info("DL-Bus line polarity detected", "inverted", line.inverted)
					}
				}
			}

			switch {
			case bit == decoder.Invalid:
				// invalid bit received - discard the frame, wait for the next SYNC
				drop()

			case syncEnded:
				// this bit is the start bit after a SYNC: a new frame begins
				if inFrame {
					send() // the SYNC's first bit was lost, so it did not end the frame
				}
				drop()
				inFrame = true
				bitIndex = 1

			case !inFrame:
				// no SYNC yet, or the frame was broken - wait for the next SYNC

			case bitIndex == 0 && bit == decoder.High:
				// first bit of the next SYNC - the frame is complete
				send()
				drop()

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
				// missing stop bit - protocol error, discard the frame
				r.protocolErrors.Add(1)
				drop()

			default:
				// data bit received - set bit in register (LSB first)
				byteRegister |= byte(bit) << (bitIndex - 1)
				bitIndex++
			}
		}
	}
}

// polarity detects the SYNC and the line polarity, and undoes an inversion.
type polarity struct {
	last     decoder.Bit // previous valid bit
	run      int         // identical bits in a row, ending with last
	known    bool        // a SYNC has been seen
	inverted bool        // the last SYNC arrived as low bits
}

// next takes one bit from the decoder and returns it in the polarity the
// sender meant, and whether a SYNC ended just before it - which makes it the
// start bit of a frame. Invalid is passed through and ends any run.
func (p *polarity) next(bit decoder.Bit) (decoder.Bit, bool) {
	if bit != decoder.High && bit != decoder.Low {
		p.run = 0
		return bit, false
	}

	var syncEnded bool
	if p.run > 0 && bit == p.last {
		p.run++
	} else {
		// A long run that ends here was a SYNC, and this bit is its start bit.
		if p.run >= syncBits {
			p.known = true
			p.inverted = p.last == decoder.Low
			syncEnded = true
		}
		p.last, p.run = bit, 1
	}

	if p.inverted {
		bit = decoder.High - bit
	}
	return bit, syncEnded
}
