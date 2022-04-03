// Package dlbus is the decoder of th dlbus protocol from Technische Alternative
package dlbus

import (
	"io"
	"sync"
	"tadl/pkg/port"

	"github.com/womat/debug"
)

const (
	//synchronizing is the process state to synchronize the dlbus.
	synchronizing stateType = iota
	// synchronized is the process state to receive bitstream.
	synchronized
)

// stateType represents the state of the decoding process.
type stateType int

// ReadCloser contains the handler to read data from the dl bus.
type ReadCloser struct {
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
	quit chan bool
	// done signals that handler is stopped
	done chan bool
}

// NewReader initials a new dlbus handler
func NewReader(c chan port.StateType) *ReadCloser {
	h := ReadCloser{
		state:    synchronizing,
		rxBuffer: []byte{},
		rl:       sync.Mutex{},
		rx:       c,
		done:     make(chan bool),
		quit:     make(chan bool),
	}

	go h.run()

	return &h
}

// Read the current dlbus frame (data before last sync).
func (r *ReadCloser) Read(b []byte) (int, error) {
	r.rl.Lock()
	defer r.rl.Unlock()

	if len(r.rxBuffer) == 0 {
		return 0, io.EOF
	}
	n := copy(b, r.rxBuffer)
	r.rxBuffer = r.rxBuffer[0:0]

	return n, nil
}

// Close stops listening dl bus >> stop watching raspberry pin and stops m.service() because of close(m.rx) channel.
func (r *ReadCloser) Close() error {
	r.rxBuffer = []byte{}

	if r.state == synchronized {
		r.rl.Unlock()
	}

	r.quit <- true

	// wait until run() is terminated
	<-r.done
	close(r.quit)
	close(r.done)

	return nil
}

// run receives incoming bits on channel rx. Handle the sync sequence and receive byte for byte to rxBuffer.
func (r *ReadCloser) run() {
	for {
		select {
		case <-r.quit:
			r.done <- true
			return
		case b, open := <-r.rx:
			if !open {
				r.quit <- true
				continue
			}

			switch b {
			case port.Invalid:
				debug.ErrorLog.Println("invalid data stream, wait for dlbus sync")
				r.reset()
			case port.High, port.Low:
				r.decoder(b)
			}
		}
	}
}

// reset restart synchronizing dl bus
func (r *ReadCloser) reset() {
	r.rxBuffer = r.rxBuffer[0:0]
	r.syncCounter = 0

	if r.state == synchronized {
		r.rl.Unlock()
		r.state = synchronizing
	}
}

// decoder decodes the dlbus dataframe
//  the dataframe starts and ends with 16 high bits (sync).
//  each data byte consists of one start bit (low), eight dat bits (LSB first) and one stop bit (high)
func (r *ReadCloser) decoder(bit port.StateType) {
	switch r.state {
	case synchronizing:
		switch bit {
		case port.High:
			r.syncCounter++
		case port.Low:
			if r.syncCounter < 16 {
				r.syncCounter = 0
				return
			}

			// it looks like a start bit after sync
			r.rl.Lock()
			r.state = synchronized
			r.rxBit = 0
			r.rxBuffer = r.rxBuffer[0:0]
			r.low()
		}

	case synchronized:
		switch bit {
		case port.High:
			r.high()
		case port.Low:
			r.low()
		}
	}
}

// high handles high data bits, stop bits and recognizes a starting sync sequence.
// data bits fills the rxRegister.
// The stop bit competes the rxRegister and add it to the rxBuffer.
// If a sync sequence starts, the rxBuffer is competed.
func (r *ReadCloser) high() {
	switch r.rxBit {
	case 0:
		// if the first bit is high (no start bit), the dataframe is complete and a new sync sequence starts
		// release (unlock) the rxBuffer for reader.
		debug.TraceLog.Printf("rxBuffer: %v", r.rxBuffer)
		r.state = synchronizing
		r.syncCounter = 1
		r.rl.Unlock()
	case 9:
		// stop bit received
		r.rxBuffer = append(r.rxBuffer, r.rxRegister)
		r.rxBit = 0
	default:
		// data bit received and set bit in register
		r.rxRegister |= 1 << (r.rxBit - 1)
		r.rxBit++
	}
}

// low handles start bits and low data bits.
// data bits fills the rxRegister.
// the start bit clears the rxRegister
func (r *ReadCloser) low() {
	switch r.rxBit {
	case 0:
		// start bit received
		r.rxRegister = 0
		r.rxBit = 1
	case 9:
		// no stop bit received, wait for sync
		debug.ErrorLog.Print("missing stop bit, wait for dlbus sync")
		r.reset()
	default:
		r.rxBit++
	}
}
