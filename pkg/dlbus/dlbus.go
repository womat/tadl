// Package dlbus is the decoder of th dlbus protocol from Technische Alternative
package dlbus

import (
	"github.com/womat/debug"
	"io"
	"sync"
	"tadl/pkg/port"
)

const (
	//synchronizing is the process state to synchronize the dlbus.
	synchronizing stateType = iota
	// synchronized is the process state to receive bitstream.
	synchronized
)

// stateType represents the state of the decoding process.
type stateType int

// Handler contains the handler to read data from the dl bus.
type Handler struct {
	// syncCounter is the count of consecutive high bits.
	syncCounter int
	// state contains the current decoding state (synchronizing/synchronized).
	state stateType
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
		state:    synchronizing,
		rxBuffer: []byte{},
		rl:       sync.Mutex{},
		rx:       c,
		quit:     make(chan struct{}),
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

	if h.state == synchronized {
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
				debug.ErrorLog.Println("invalid data stream, wait for dlbus sync")
				h.reset()
			case port.High, port.Low:
				h.decoder(b)
			}
		}
	}
}

// reset restart synchronizing dl bus
func (h *Handler) reset() {
	h.rxBuffer = h.rxBuffer[0:0]
	h.syncCounter = 0

	if h.state == synchronized {
		h.rl.Unlock()
		h.state = synchronizing
	}
}

// decoder decodes the dlbus dataframe
//  the dataframe starts and ends with 16 high bits (sync).
//  each data byte consists of one start bit (low), eight dat bits (LSB first) and one stop bit (high)
func (h *Handler) decoder(bit port.StateType) {
	switch h.state {
	case synchronizing:
		switch bit {
		case port.High:
			h.syncCounter++
		case port.Low:
			if h.syncCounter < 16 {
				h.syncCounter = 0
				return
			}

			// it looks like a start bit after sync
			h.rl.Lock()
			h.state = synchronized
			h.rxBit = 0
			h.rxBuffer = h.rxBuffer[0:0]
			h.low()
		}

	case synchronized:
		switch bit {
		case port.High:
			h.high()
		case port.Low:
			h.low()
		}
	}
}

// high handles high data bits, stop bits and recognizes a starting sync sequence.
// data bits fills the rxRegister.
// The stop bit competes the rxRegister and add it to the rxBuffer.
// If a sync sequence starts, the rxBuffer is competed.
func (h *Handler) high() {
	switch h.rxBit {
	case 0:
		// if the first bit is high (no start bit), the dataframe is complete and a new sync sequence starts
		// release (unlock) the rxBuffer for reader.
		debug.DebugLog.Printf("rxBuffer: %v", h.rxBuffer)
		h.state = synchronizing
		h.syncCounter = 1
		h.rl.Unlock()
	case 9:
		// stop bit received
		h.rxBuffer = append(h.rxBuffer, h.rxRegister)
		h.rxBit = 0
	default:
		// data bit received and set bit in register
		h.rxRegister |= 1 << (h.rxBit - 1)
		h.rxBit++
	}
}

// low handles start bits and low data bits.
// data bits fills the rxRegister.
// the start bit clears the rxRegister
func (h *Handler) low() {
	switch h.rxBit {
	case 0:
		// start bit received
		h.rxRegister = 0
		h.rxBit = 1
	case 9:
		// no stop bit received, wait for sync
		debug.DebugLog.Print("missing stop bit, wait for dlbus sync")
		h.reset()
	default:
		h.rxBit++
	}
}
