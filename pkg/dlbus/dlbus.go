// Package dlbus is the decoder of th dlbus protocol from Technische Alternative
package dlbus

import (
	"io"
	"sync"

	"github.com/womat/debug"
	"tadl/pkg/port"
)

// Handler contains the handler to read data from the dl bus.
type Handler struct {
	// syncCounter is the count of consecutive high bits.
	syncCounter int
	// isSync marks that a data stream can be received (sync sequence is finished).
	isSynchronized bool
	// rx channel receives data stream from manchester code.
	rx chan port.StateType
	// rxBit is the number of the currently received bit of the rxRegister.
	rxBit int
	// rxRegister is the buffer of the currently received byte.
	rxRegister byte
	// rxBuffer is the received data record between two syncs.
	rxBuffer []byte
	// rl lock the rxBuffer until data are received.
	rl sync.Mutex
	// quit stops the handler
	quit chan struct{}
}

// New initials a new dlbus handler
func New(c chan port.StateType) io.ReadCloser {
	h := Handler{
		isSynchronized: false,
		rxBuffer:       []byte{},
		rl:             sync.Mutex{},
		rx:             c,
		quit:           make(chan struct{}),
	}

	go h.run()
	return &h
}

// Read the current dlbus frame (data before last sync).
func (h *Handler) Read(b []byte) (int, error) {
	h.rl.Lock()
	defer h.rl.Unlock()

	if len(h.rxBuffer) == 0 {
		return 0, io.EOF
	}
	n := copy(b, h.rxBuffer)
	h.rxBuffer = h.rxBuffer[0:0]

	return n, nil
}

// Close stops listening dl bus >> stop watching raspberry pin and stops m.service() because of close(m.rx) channel.
func (h *Handler) Close() error {
	h.rxBuffer = []byte{}

	if h.isSynchronized {
		h.rl.Unlock()
	}

	h.quit <- struct{}{}

	// wait until run() is terminated
	<-h.quit
	return nil
}

// run receives incoming bits on channel rx. Handle the sync sequence and receive byte for byte to rxBuffer.
func (h *Handler) run() {
	for {
		select {
		case <-h.quit:
			return
		case b, open := <-h.rx:
			if !open {
				h.quit <- struct{}{}
				continue
			}

			switch b {
			case port.Invalid:
				debug.DebugLog.Println("invalid data stream, wait for dlbus sync")
				h.reset()

			case port.High:
				h.syncCounter++

				if h.syncCounter >= 16 {
					if !h.isSynchronized {
						h.rl.Lock()
						h.isSynchronized = true
						h.rxBit = 0
						h.rxBuffer = h.rxBuffer[0:0]
					}
					continue
				}

				h.readBit(true)
				continue
			case port.Low:
				h.syncCounter = 0
				h.readBit(false)
			}
		}
	}
}

// reset restart synchronizing dl bus
func (h *Handler) reset() {
	h.rxBuffer = h.rxBuffer[0:0]
	h.syncCounter = 0

	if h.isSynchronized {
		h.rl.Unlock()
		h.isSynchronized = false
	}
}

// readBit gets a bit from dl bus and recognizes start, stop and  a data bits.
// It recognizes start/stop sequences, received the data bytes and fill the rxBuffer.
// If a sync sequence starts, the rxBuffer is competed.
func (h *Handler) readBit(bit bool) {
	if !h.isSynchronized {
		return
	}
	if bit {
		switch {
		case h.rxBit == 0:
			// if the first bit is high (no start bit), the dataframe is complete and a new sync sequence starts
			// release (unlock) the rxBuffer for reader.
			debug.DebugLog.Printf("rxBuffer: %v", h.rxBuffer)
			h.isSynchronized = false
			h.syncCounter = 1
			h.rl.Unlock()
		case h.rxBit == 9:
			// stop bit received
			h.rxBuffer = append(h.rxBuffer, h.rxRegister)
			h.rxBit = 0
		default:
			// data bit received and set bit in register
			h.rxRegister |= 1 << (h.rxBit - 1)
			h.rxBit++
		}
		return
	}

	switch {
	case h.rxBit == 0:
		// start bit received
		h.rxRegister = 0
		h.rxBit = 1
	case h.rxBit > 8:
		// no stop bit received, wait for sync
		debug.DebugLog.Print("missing stop bit, wait for dlbus sync")
		h.reset()
	default:
		// data bit received
		h.rxBit++
	}
}
