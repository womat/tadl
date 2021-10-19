package dlbus

import (
	"github.com/womat/debug"
	"io"
	"sync"
	"tadl/pkg/raspberry"
	"time"
)

// Handler contains the raspi of the mqtt broker
type Handler struct {
	// raspi is the handler of the raspberry pin, where the dl bus is connected
	raspi raspberry.Pin
	// time is the timestamp of the last falling edge (low bit)
	time time.Time
	// syncCounter is the count of consecutive high bits
	syncCounter int
	isSync      bool

	// rxBit is number of the currently received bit
	rxBit int
	// rxRegister is the buffer of the currently received byte
	rxRegister byte
	// rxBuffer is the received data record between two syncs
	rxBuffer []byte
	// rl lock the rxBuffer until data are received
	rl sync.Mutex
}

// Open listen the dl bus on the configured raspberry pin
func Open(h raspberry.Pin, f int, b time.Duration) (io.ReadCloser, error) {
	m := Handler{
		raspi: h,
		time:  time.Now(),

		syncCounter: 0,
		isSync:      false,
		rxBit:       0,
		rxRegister:  0,
		rxBuffer:    []byte{},
		rl:          sync.Mutex{},
	}

	m.raspi.Input()
	m.raspi.PullUp()
	m.raspi.SetBounceTime(b)

	// call the handler when pin changes from low to high.
	if err := m.raspi.Watch(raspberry.EdgeBoth, m.handler); err != nil {
		debug.ErrorLog.Printf("can't open watcher: %v", err)
		return &m, err
	}

	debug.DebugLog.Print("wait for SYNC")
	return &m, nil
}

// Read current dl bus frame (data before last sync)
func (m *Handler) Read(b []byte) (int, error) {
	m.rl.Lock()
	defer m.rl.Unlock()

	if len(m.rxBuffer) == 0 {
		return 0, io.EOF
	}
	n := copy(b, m.rxBuffer)
	m.rxBuffer = m.rxBuffer[0:0]

	return n, nil
}

// Close stops listening dl bus >> stop watching raspberry pin and stops m.service()
func (m *Handler) Close() error {
	m.raspi.Unwatch()
	m.rxBuffer = []byte{}
	m.rl.Unlock()
	return nil
}

// handler listen raspberry pin
// Sync wait for a sync sequence, clears the rxRegister and rxBuffer and restarts the ticker (synchronizing the dl bus)
// to recognize the sync sequence, wait for 17.5 periods (16 high bits + start bit)
// a period is 1/frequency
func (m *Handler) handler(p raspberry.Pin) {
	t := time.Now()

	if t.Sub(m.time) < 18*time.Millisecond {
		return
	}

	m.time = t

	if !m.raspi.Read() {
		m.syncCounter++

		if m.syncCounter >= 15 {
			if !m.isSync {
				debug.TraceLog.Printf("SYNC detected")
				m.rl.Lock()
				m.isSync = true
				m.rxBit = 0
				m.rxBuffer = m.rxBuffer[0:0]
			}
			return
		}

		m.readBit(true)
		return
	}

	m.syncCounter = 0
	m.readBit(false)
	return
}

// stop to receiving data from dl bus, writes data to buffer and wait for sync
func (m *Handler) stop() {
	m.isSync = false
	m.syncCounter = 0

	debug.DebugLog.Printf("buffer: %v", m.rxBuffer)
	debug.TraceLog.Print("wait for SYNC")

	m.rl.Unlock()
	return
}

// readBit gets a bit from dl bus and checks, if bit is a start, a stop or a data bit
// check start/stop sequences, received data bytes and fill the rxBuffer.
// if a sync sequence starts, the rxBuffer is competed
func (m *Handler) readBit(bit bool) {
	if !m.isSync {
		return
	}
	if bit {
		switch {
		case m.rxBit == 0:
			// if the first bit is high (no start bit), the sync sequence starts
			m.stop()
		case m.rxBit == 9:
			// stop bit received
			m.rxBuffer = append(m.rxBuffer, m.rxRegister)
			m.rxBit = 0
		default:
			// data bit received and set bit in register
			m.rxRegister |= 1 << (m.rxBit - 1)
			m.rxBit++
		}
		return
	}

	switch {
	case m.rxBit == 0:
		// start bit received
		m.rxRegister = 0
		m.rxBit = 1
	case m.rxBit > 8:
		// no stop bit received, wait for sync
		m.stop()
	default:
		// data bit received
		m.rxBit++
	}
}
