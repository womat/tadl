package dlbus

import (
	"encoding/binary"
	"errors"
	"github.com/womat/debug"
	"tadl/pkg/raspberry"
	"time"
)

const (
	Invalid = ""
	_UVR42  = "UVR42"
)

// Handler contains the handler of the mqtt broker
type Handler struct {
	handler raspberry.Pin
	// C is the channel to service the mqtt message
	// sending a message to channel C will send the message
	// C chan Message
	ticker *time.Ticker
	period time.Duration
	//	sync         int
	//	wait4sync    bool
	bitCount int
	b        byte
	t        time.Time
	buffer   []byte
	//	tick         bool
	syncCounter1 int
	//	ignoreIntr   bool

	uvr42 UVR42
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
		ticker: time.NewTicker(time.Second),
	}
}

func (m *Handler) Connect(h raspberry.Pin, f int) error {
	m.handler = h
	m.period = time.Duration(1000/f) * time.Millisecond
	m.ticker = time.NewTicker(time.Minute)
	m.ticker.Stop()
	//	m.wait4sync = true
	m.buffer = []byte{}
	//	m.tick = true
	//	m.ignoreIntr = false
	m.t = time.Now()

	debug.TraceLog.Println("call function Connect")

	return nil
}

// Start restart the ticker without reading the singal on dl
func (m *Handler) Start() error {
	m.ticker.Reset(m.period)
	return nil
}

func (m *Handler) Stop() error {
	m.ticker.Stop()
	debug.TraceLog.Println("buffer:", m.buffer)

	if len(m.buffer) == 0 {
		return nil
	}

	switch m.buffer[0] {
	case 16:
		if x, err := getUVR42(m.buffer); err == nil {
			m.uvr42 = x
		}
	}

	debug.DebugLog.Println("UVR232:", m.uvr42)

	return nil
}

// Restart wait a half half-period, reads the signal on dl and restart the ticker
func (m *Handler) Restart() error {
	t := time.Now()
	dt := t.Sub(m.t)
	m.t = t

	if m.syncCounter1 >= 16 && dt >= 22*time.Millisecond && dt < 35*time.Millisecond {
		//	debug.TraceLog.Printf("delta: %v", dt)
		//	debug.TraceLog.Printf("sync! %v", m.syncCounter1)

		m.syncCounter1 = 0
		m.b = 0
		m.bitCount = 1
		m.buffer = m.buffer[0:0]
		go m.ReadBit()

		go func() {
			//time.Sleep(m.period / 4)
			//	time.Sleep(100*time.Microsecond)
			m.ticker.Reset(m.period)
		}()

		return nil
	}

	if dt < 22*time.Millisecond {
		m.syncCounter1++
		//debug.TraceLog.Printf("%v", m.syncCounter1)
		return nil
	}

	m.syncCounter1 = 0
	return nil
}

// Service listens to a message on the channel C and sends the message
// if no handler or topic is defined, the message will be ignored
func (m *Handler) Service() {
	for range m.ticker.C {
		m.ReadBit()
	}
}

func (m *Handler) ReadBit() {
	if m.handler.Read() {
		// DL signal High
		//		debug.TraceLog.Printf("DL signal: high, data bit: %v", m.bitCount)

		switch {
		case m.bitCount == 0:
			m.Stop()
			debug.TraceLog.Println("start bit missing, wait for sync")
		case m.bitCount == 9:
			//debug.TraceLog.Println("stop bit received")
			//debug.TraceLog.Printf("received byte %v, %v", m.b, strconv.FormatInt(int64(m.b), 2))
			m.buffer = append(m.buffer, m.b)
			m.bitCount = 0
		default:
			//	debug.TraceLog.Printf("set bit %v",strconv.FormatInt(1 <<  (m.bitCount-1), 2) )
			m.b |= 1 << (m.bitCount - 1)
			m.bitCount++
		}

		return
	}

	// DL signal Low
	//	debug.TraceLog.Printf("DL signal: low, data bit: %v", m.bitCount)
	switch {
	case m.bitCount == 0:
		//debug.TraceLog.Println("start bit received")
		m.b = 0
		m.bitCount++
	case m.bitCount > 8:
		m.Stop()
		debug.TraceLog.Println("stop bit missing, wait for sync")
	default:
		m.bitCount++
	}
}

func getUVR42(b []byte) (f UVR42, err error) {
	if len(b) == 0 || len(b) != 10 || b[0] != 16 {
		f.Type = Invalid
		err = errors.New("invalid data")
		return
	}

	f.Type = _UVR42
	f.Time = time.Now()
	f.Temp1 = float64(int16(binary.LittleEndian.Uint16(b[1:3]))) / 10
	f.Temp2 = float64(int16(binary.LittleEndian.Uint16(b[3:5]))) / 10
	f.Temp3 = float64(int16(binary.LittleEndian.Uint16(b[5:7]))) / 10
	f.Temp4 = float64(int16(binary.LittleEndian.Uint16(b[7:9]))) / 10
	f.Out1 = b[9]|1<<5 == 1
	f.Out1 = b[9]|1<<6 == 1
	return
}

func (m *Handler) GetMeasurements() interface{} {
	return m.uvr42

}

/*
   func (m *Handler) ReadBit1() {
   	//t := time.Now()
   	//dt := t.Sub(m.t)
   	//m.t = t
   	p := m.handler.Read()

   	if p {
   		//debug.TraceLog.Printf("delta %v, DL signal: high, data bit: %v, sync bit: %v", dt, m.bitCount, m.sync)
   		m.sync++

   		// ignore high signal, if no start bit has revived
   		if m.wait4sync || m.bitCount == 0 {
   			return
   		}

   		if m.bitCount == 9 {
   			//		debug.TraceLog.Println("stop bit received")
   			// debug.TraceLog.Printf("received byte %v, %v", m.b, strconv.FormatInt(int64(m.b), 2))
   			m.buffer = append(m.buffer, m.b)
   			m.bitCount = 0
   			return
   		}

   		//debug.TraceLog.Printf("set bit %v",strconv.FormatInt(1 <<  (m.bitCount-1), 2) )

   		m.b |= 1 << (m.bitCount - 1)
   		m.bitCount++
   	} else {
   		//debug.TraceLog.Printf("delta %v, DL signal: low, data bit: %v, sync bit: %v", dt, m.bitCount, m.sync)

   		if m.sync >= 16 {

   			debug.TraceLog.Printf("sync finished, tick: %v", m.tick)
   			debug.TraceLog.Println("buffer:", m.buffer)
   			m.bitCount = 0
   			m.wait4sync = false
   			m.buffer = m.buffer[0:0]
   		}

   		m.sync = 0

   		if m.wait4sync {
   			return
   		}

   		if m.bitCount == 0 {
   			//		debug.TraceLog.Println("start bit received")
   			m.b = 0
   			m.bitCount++
   			return
   		}

   		if m.bitCount > 8 {
   			// debug.TraceLog.Println("stop bit missing, wait for sync")
   			m.sync = 0
   			m.wait4sync = false
   			return
   		}

   		m.bitCount++
   	}
   }


*/
