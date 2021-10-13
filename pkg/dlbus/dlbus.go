package dlbus

import (
	"encoding/binary"
	"errors"
	"github.com/womat/debug"
	"strconv"
	"tadl/pkg/raspberry"
	"time"
)

const (
	Invalid = ""
	_UVR42  = "UVR42"
)

// device ids
const (
	uvr31 = 48
	uvr42 = 16
	uvr64 = 32

	tMax = 300
	tMin = -50

	out1 = 1 << 5
	out2 = 1 << 6
)

// Handler contains the handler of the mqtt broker
type Handler struct {
	handler raspberry.Pin
	ticker  *time.Ticker

	// highBit is the period of a high bit >> 110% of display clock
	highBit time.Duration

	// startBit is the period of a high bit >> 150% of display clock + 10%
	startBit time.Duration

	// period is the duration of time of one display clock (1/frequency)
	period time.Duration

	// data is the byte buffer of the currently received byte
	data byte

	// bitCounter is number of the currently received bit
	bitCounter int

	// syncCounter is the count of consecutive high bits
	syncCounter int

	// time is the timestamp of the last falling edge (low bit)
	time time.Time

	// buffer is the received data record between two syncs
	buffer []byte
	uvr42  UVR42
}

type UVR42 struct {
	Type  string
	Time  time.Time
	Temp1 float64
	Temp2 float64
	Temp3 float64
	Temp4 float64
	Out1  bool
	Out2  bool
}

// New generate a new dl-bus handler
func New() *Handler {
	return &Handler{
		//		ticker: time.NewTicker(time.Second),
	}
}

func (m *Handler) Connect(h raspberry.Pin, f int) error {
	m.handler = h
	m.period = time.Duration(1000/f) * time.Millisecond
	m.highBit = m.period
	m.highBit += m.highBit / 10
	m.startBit = m.period + m.period/2
	m.startBit += m.startBit / 10
	m.ticker = time.NewTicker(time.Minute)
	m.ticker.Stop()
	m.buffer = []byte{}
	m.time = time.Now()
	return nil
}

// Sync wait for a sync sequence, clears the buffer and restarts the ticker
// to recognize the sync sequence, wait for 16 periods (16 high bits) and a 1.5 period (start bit)
// a period is 1/frequency
func (m *Handler) Sync() {
	t := time.Now()
	interval := t.Sub(m.time)
	m.time = t

	if m.syncCounter >= 16 && interval >= m.highBit && interval < m.startBit {
		// sync detected (16 high bits and a start bit was received)
		go func() {
			// time.Sleep(m.period / 4)
			// time.Sleep(100*time.Microsecond)
			m.ticker.Reset(m.period)
		}()

		debug.TraceLog.Printf("SYNC detected, count of high bits: %v, interval: %v", m.syncCounter, interval)
		debug.DebugLog.Print("SYNC detected")

		m.syncCounter = 0
		m.bitCounter = 1
		m.data = 0
		m.buffer = m.buffer[0:0]

		go m.ReadBit()
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

// Stop stops receiving data and writes buffer to data frame
func (m *Handler) Stop() {
	defer debug.DebugLog.Print("wait for SYNC")

	m.ticker.Stop()

	debug.TraceLog.Printf("buffer: %v", m.buffer)

	if len(m.buffer) == 0 {
		return
	}

	switch id := m.buffer[0]; id {
	case uvr42:
		var x UVR42
		var err error

		if x, err = getUVR42(m.buffer); err != nil {
			debug.ErrorLog.Printf("get data record: %v", err)
			return
		}

		m.uvr42 = x
		debug.InfoLog.Println("UVR232:", m.uvr42)

	default:
		debug.ErrorLog.Printf("unsupported device id: %v", id)
	}

	return
}

// Service listens to a message on the channel C and sends the message
// if no handler or topic is defined, the message will be ignored
func (m *Handler) Service() {
	for range m.ticker.C {
		m.ReadBit()
	}
}
func (m *Handler) ReadBit() {
	//t := time.Now()
	m.readBit()
	//debug.DebugLog.Printf("runtime ReadBit (call): %v", time.Now().Sub(t))
}

func (m *Handler) readBit() {
	if m.handler.Read() {
		// DL signal High
		debug.TraceLog.Printf("DL signal: high, bit: %v", m.bitCounter)

		switch {
		case m.bitCounter == 0:
			m.Stop()
			debug.TraceLog.Println("start bit missing, wait for sync")
		case m.bitCounter == 9:
			debug.TraceLog.Println("stop bit received")
			debug.TraceLog.Printf("received byte %v (%v)", m.data, strconv.FormatInt(int64(m.data), 2))
			m.buffer = append(m.buffer, m.data)
			m.bitCounter = 0
		default:
			m.data |= 1 << (m.bitCounter - 1)
			m.bitCounter++
		}

		return
	}

	// DL signal Low
	debug.TraceLog.Printf("DL signal: low, bit: %v", m.bitCounter)
	switch {
	case m.bitCounter == 0:
		debug.TraceLog.Println("start bit received")
		m.data = 0
		m.bitCounter++
	case m.bitCounter > 8:
		m.Stop()
		debug.TraceLog.Println("stop bit missing, wait for sync")
	default:
		m.bitCounter++
	}
}

func getUVR42(b []byte) (f UVR42, err error) {
	if len(b) == 0 || len(b) != 10 || b[0] != 16 {
		f.Type = Invalid
		err = errors.New("invalid data length")
		return
	}

	f.Type = _UVR42
	f.Time = time.Now()
	f.Temp1 = float64(int16(binary.LittleEndian.Uint16(b[1:3]))) / 10
	f.Temp2 = float64(int16(binary.LittleEndian.Uint16(b[3:5]))) / 10
	f.Temp3 = float64(int16(binary.LittleEndian.Uint16(b[5:7]))) / 10
	f.Temp4 = float64(int16(binary.LittleEndian.Uint16(b[7:9]))) / 10

	f.Out1 = b[9]&out1 > 0
	f.Out2 = b[9]&out2 > 0

	if f.Temp1 > tMax || f.Temp2 > tMax || f.Temp3 > tMax || f.Temp4 > tMax ||
		f.Temp1 < tMin || f.Temp2 < tMin || f.Temp3 < tMin || f.Temp4 < tMin {
		err = errors.New("invalid temperature")
		return
	}
	return
}

func (m *Handler) GetMeasurements() interface{} {
	return m.uvr42
}
