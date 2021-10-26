package dlbus

import (
	"github.com/womat/debug"
	"io"
	"sync"
	"tadl/pkg/raspberry"
	"time"
)

// Handler contains the handler to read data from the dl bus.
type Handler struct {
	// raspi is the handler of the raspberry pin, where the dl bus is connected.
	raspi raspberry.Pin

	// clock is the display clock in Hz.
	clock int
	// bitClock is a period of the display clock (-15%).
	bitClock time.Duration
	// lastLevelChange is the timestamp of the last level change of the manchester code (falling or rising edge).
	lastLevelChange time.Time

	// syncCounter is the count of consecutive high bits.
	syncCounter int
	// isSync marks that a data stream can be received (sync sequence is finished).
	isSync bool

	// rx channel receives data stream from manchester code.
	rx chan bool
	// rxBit is the number of the currently received bit of the rxRegister.
	rxBit int
	// rxRegister is the buffer of the currently received byte.
	rxRegister byte
	// rxBuffer is the received data record between two syncs.
	rxBuffer []byte
	// rl lock the rxBuffer until data are received.
	rl sync.Mutex
}

// Open starts to listen the dl bus on the configured raspberry pin
func Open(h raspberry.Pin, c int, b time.Duration) (io.ReadCloser, error) {
	m := Handler{
		raspi:           h,
		lastLevelChange: time.Now(),

		clock:       c,
		syncCounter: 0,
		isSync:      false,
		rxBit:       0,
		rxRegister:  0,
		rxBuffer:    []byte{},
		rl:          sync.Mutex{},
		rx:          make(chan bool, 1024),
		bitClock:    time.Duration(850/c) * time.Millisecond,
	}

	go m.service()

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

// Read the current dl bus frame (data before last sync).
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

// Close stops listening dl bus >> stop watching raspberry pin and stops m.service() because of close(m.rx) channel.
func (m *Handler) Close() error {
	m.raspi.Unwatch()
	m.rxBuffer = []byte{}

	if m.isSync {
		m.rl.Unlock()
	}

	close(m.rx)
	return nil
}

// handler is the raspi watch function and listen to raspberry pin and encode manchester code and send the received bits to buffered channel rx.
//  decoding manchester code: siehe https://www.elektroniktutor.de/internet/codes.html
//  Nach dem Ethernet-Standard codiert die ansteigende Flanke im Manchestercode die logische 1 als High-Zustand im Datenstrom.
//  Die fallende Flanke im Manchestercode, bezogen auf die zeitliche Abfolge des Datenstroms steht für die logische 0, dem Low-Zustand des Datenstroms.
//  Die Information ist an die Signalflanken gebunden, die Codierung entspricht damit einem digitalen Phase-Shift-Keying-Verfahren.
func (m *Handler) handler(p raspberry.Pin) {
	t := time.Now()

	if t.Sub(m.lastLevelChange) < m.bitClock {
		// only full periods are valid
		return
	}

	m.lastLevelChange = t
	m.rx <- !m.raspi.Read()
}

// service receives incoming bits on channel rx. Handle the sync sequence and receive byte for byte to rxBuffer.
func (m *Handler) service() {
	bitTime := time.Now()
	tMin := time.Duration(900/m.clock) * time.Millisecond  // 18ms
	tMax := time.Duration(1150/m.clock) * time.Millisecond // 23ms

	for b := range m.rx {
		t := time.Now()

		d := t.Sub(bitTime)
		bitTime = t

		if m.isSync && (d > tMax || d < tMin) {
			debug.DebugLog.Println("bit clock out of range, start sync: ", d)
			m.rxBuffer = m.rxBuffer[0:0]
			m.stop()
			continue
		}

		if b {
			m.syncCounter++

			if m.syncCounter >= 15 {
				if !m.isSync {
					m.rl.Lock()
					m.isSync = true
					m.rxBit = 0
					m.rxBuffer = m.rxBuffer[0:0]
				}
				continue
			}

			m.readBit(true)
			continue
		}

		m.syncCounter = 0
		m.readBit(false)
	}
}

// stop receiving data from dl bus and release (unlock) rxBuffer for reader.
func (m *Handler) stop() {
	debug.DebugLog.Printf("rxBuffer: %v", m.rxBuffer)
	m.isSync = false
	m.syncCounter = 0
	m.rl.Unlock()
	return
}

// readBit gets a bit from dl bus and recognizes start, stop and  a data bits.
// It recognizes start/stop sequences, received the data bytes and fill the rxBuffer.
// If a sync sequence starts, the rxBuffer is competed.
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
		debug.DebugLog.Print("missing stop bit, start sync")
		m.rxBuffer = m.rxBuffer[0:0]
		m.stop()
	default:
		// data bit received
		m.rxBit++
	}
}
