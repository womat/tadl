package dlbus

import (
	"github.com/womat/debug"
	"io"
	"strconv"
	"sync"
	"tadl/pkg/raspberry"
	"time"
)

// Handler contains the raspi of the mqtt broker
type Handler struct {
	// raspi is the handler of the raspberry pin, where the dl bus is connected
	raspi raspberry.Pin
	// ticker is the display clock of the dl bus
	ticker *time.Ticker
	// done is the channel to stop the m.service()
	done chan bool

	// time is the timestamp of the last falling edge (low bit)
	time time.Time
	// syncCounter is the count of consecutive high bits
	syncCounter int

	// rxBit is number of the currently received bit
	rxBit int
	// rxRegister is the buffer of the currently received byte
	rxRegister byte
	// rxBuffer is the received data record between two syncs
	rxBuffer []byte
	// rl lock the rxBuffer until data are received
	rl sync.Mutex

	// period is the duration of time of one display clock (1/frequency)
	period time.Duration
	// highBit is the period of a high bit >> 110% of display clock
	highBit time.Duration
	// startBit is the period of a start bit >> 150% of display clock + 10%
	startBit time.Duration
}

// Open listen the dl bus on the configured raspberry pin
func Open(h raspberry.Pin, f int, b time.Duration) (io.ReadCloser, error) {
	m := Handler{
		raspi:       h,
		ticker:      time.NewTicker(time.Minute),
		done:        make(chan bool, 1),
		time:        time.Now(),
		syncCounter: 0,
		rxBit:       0,
		rxRegister:  0,
		rxBuffer:    []byte{},
		rl:          sync.Mutex{},
		period:      time.Duration(1000/f) * time.Millisecond,
		highBit:     0,
		startBit:    0,
	}

	// init dl bus display clock
	m.ticker.Stop()

	// calc highBit duration (110% of period)
	m.highBit = m.period
	m.highBit += m.highBit / 10

	// calc startBit duration (150% of period + 10%)
	m.startBit = m.period + m.period/2
	m.startBit += m.startBit / 10

	m.raspi.Input()
	m.raspi.PullUp()
	m.raspi.SetBounceTime(b)

	// call the handler when pin changes from low to high.
	if err := m.raspi.Watch(raspberry.EdgeFalling, m.handler); err != nil {
		debug.ErrorLog.Printf("can't open watcher: %v", err)
		return &m, err
	}

	// listen signals on dl bus
	go m.service()

	return &m, nil
}

// Read current dl bus frame (data before last sync)
func (m *Handler) Read(b []byte) (int, error) {
	m.rl.Lock()
	n := copy(b, m.rxBuffer)
	m.rl.Unlock()

	return n, nil
}

// Close stops listening dl bus >> stop watching raspberry pin and stops m.service()
func (m *Handler) Close() error {
	m.raspi.Unwatch()
	m.done <- true
	m.rxBuffer = []byte{}
	m.rl.Unlock()
	return nil
}

// handler listen raspberry pin
func (m *Handler) handler(p raspberry.Pin) {
	m.sync()
}

// service is synchronized with the display clock and listen the signals of the dl bus
// the done channel stops the service
func (m *Handler) service() {
	defer m.ticker.Stop()

	for {
		select {
		case <-m.ticker.C:
			m.readBit()
		case <-m.done:
			debug.DebugLog.Print("stop service()")
			return
		}
	}
}

// Sync wait for a sync sequence, clears the rxRegister and rxBuffer and restarts the ticker (synchronizing the dl bus)
// to recognize the sync sequence, wait for 17.5 periods (16 high bits + start bit)
// a period is 1/frequency
func (m *Handler) sync() {
	t := time.Now()
	interval := t.Sub(m.time)
	m.time = t
	debug.TraceLog.Printf("falling edge detected, DL signal: low,  interval: %v", interval)

	if m.syncCounter >= 16 && interval >= m.highBit && interval < m.startBit {
		// sync detected (16 high bits and a start bit was received)
		go func() {
			// time.Sleep(m.period / 4)
			// time.Sleep(100*time.Microsecond)
			m.ticker.Reset(m.period)
		}()

		debug.TraceLog.Printf("SYNC detected, count of high bits: %v, interval: %v", m.syncCounter, interval)
		debug.DebugLog.Print("SYNC detected")

		m.rl.Lock()
		m.rxBit = 1
		m.rxRegister = 0
		m.rxBuffer = m.rxBuffer[0:0]
		m.syncCounter = 0

		go m.readBit()
		return
	}

	if interval < m.highBit {
		if m.syncCounter > 0 {
			debug.TraceLog.Printf("consecutive high bit received (%v), interval: %v", m.syncCounter, interval)
		}

		m.syncCounter++
		return
	}

	m.syncCounter = 0
	return
}

// stop to receiving data from dl bus, writes data to buffer and wait for sync
func (m *Handler) stop() {
	m.ticker.Stop()
	m.rl.Unlock()

	debug.TraceLog.Printf("buffer: %v", m.rxBuffer)
	debug.DebugLog.Print("wait for SYNC")
	return
}

// readBit reads a bit from dl bus and checks, if bit is a start, a stop or a data bit
// check start/stop sequences, received data bytes and fill the rxBuffer.
// if a sync sequence starts, the rxBuffer is competed
func (m *Handler) readBit() {
	if m.raspi.Read() {
		// DL signal High
		debug.TraceLog.Printf("DL signal: high, bit: %v", m.rxBit)

		switch {
		case m.rxBit == 0:
			// if th first bit is high (no start bit), the sync sequence starts
			m.stop()
			debug.TraceLog.Println("no start bit, sync is starting")
		case m.rxBit == 9:
			// stop bit received
			debug.TraceLog.Println("stop bit received")
			debug.TraceLog.Printf("received byte %v (%v)", m.rxRegister, strconv.FormatInt(int64(m.rxRegister), 2))
			m.rxBuffer = append(m.rxBuffer, m.rxRegister)
			m.rxBit = 0
		default:
			// data bit received and set bit in register
			m.rxRegister |= 1 << (m.rxBit - 1)
			m.rxBit++
		}

		return
	}

	// DL signal Low
	debug.TraceLog.Printf("DL signal: low, bit: %v", m.rxBit)
	switch {
	case m.rxBit == 0:
		// start bit received
		debug.TraceLog.Println("start bit received")
		m.rxRegister = 0
		m.rxBit = 1
	case m.rxBit > 8:
		// no stop bit received, wait for sync
		m.stop()
		debug.TraceLog.Println("stop bit missing, wait for sync")
	default:
		// data bit received
		m.rxBit++
	}
}
