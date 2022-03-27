// Package manchester is a software Decoder for manchester code
// https://en.wikipedia.org/wiki/Manchester_code
package manchester

import (
	"sort"
	"time"

	"github.com/womat/debug"
	"tadl/pkg/port"
)

const (
	// tolerance are the time tolerance values to detect bit periods (in percent).
	tolerance = 20
	// eventSamples are the count of event samples to calculate the clock.
	eventSamples = 500

	//synchronizing is the process state to synchronize to clock.
	synchronizing stateType = iota
	// synchronized is the process state to decode edge events.
	synchronized
)

// stateType represents the state of the decoding process.
type stateType int

// Decoder represents the handler of the Decoder.
type Decoder struct {
	// state contains the current decoding state (synchronizing/synchronized).
	state stateType
	// eventSamples holds event samples to calculate bit periods.
	eventSamples []time.Duration

	// firstHalfBit is true, if the first half of a half bit period has been received.
	// __-_--__-_-_
	//   ^ first half bit
	firstHalfBit bool
	// lastTimestamp is the time of the last detected event.
	lastTimestamp time.Duration

	// defines the calculated bit periods
	fullPeriodMin time.Duration
	fullPeriodMax time.Duration
	halfPeriodMin time.Duration
	halfPeriodMax time.Duration

	// C is the channel to send the decoded bit stream
	C chan port.StateType

	// rx is the channel to receive the line events
	rx chan port.Event

	// quit is the channel to stop the Decoder
	quit chan struct{}
}

// New initials a new Decoder
func New(c chan port.Event) *Decoder {
	h := Decoder{
		C:    make(chan port.StateType),
		rx:   c,
		quit: make(chan struct{}),
	}

	// start synchronizing manchester bit periods
	h.reset()
	debug.InfoLog.Print("synchronizing clock for manchester decoding started")

	go h.run()
	return &h
}

// Close stops decoding
func (d *Decoder) Close() error {
	debug.TraceLog.Println("close decoding handler")

	d.quit <- struct{}{}

	// wait until run() is terminated
	<-d.quit

	debug.TraceLog.Println("decoding handler closed")

	close(d.C)
	close(d.quit)
	return nil
}

// run revives events and send it to eventHandler to decode
func (d *Decoder) run() {
	for {
		select {
		case <-d.quit:
			debug.TraceLog.Println("terminate decoding handler")
			return
		case evt, open := <-d.rx:
			if !open {
				debug.TraceLog.Println("rx channel is closed, quit decoding handler")
				d.quit <- struct{}{}
				continue
			}

			d.eventHandler(evt)
		}
	}
}

// EventHandler decodes line events (edges) to a bit stream and includes:
//  * synchronizing clock:
//           the clock is synchronized by analyzing the bit periods (measuring full bit periods)
//  * decode line events:
//           High: falling edge while a full bit period
//                 or a falling edge while the second half of a half bit period
//           Low:  rising edge while a full bit period
//                 or a rising edge while the second half of a half bit period
//  decoding manchester code:  https://www.elektroniktutor.de/internet/codes.html
func (d *Decoder) eventHandler(event port.Event) {
	period := event.Timestamp - d.lastTimestamp
	d.lastTimestamp = event.Timestamp

	switch d.state {
	case synchronizing:
		if len(d.eventSamples) < cap(d.eventSamples) {
			d.eventSamples = append(d.eventSamples, period)

			if len(d.eventSamples) == cap(d.eventSamples) {
				go func() {
					halfPeriod, fullPeriod := calcBitPeriods(d.eventSamples)

					d.fullPeriodMin = fullPeriod - fullPeriod*tolerance/100
					d.fullPeriodMax = fullPeriod + fullPeriod*tolerance/100
					d.halfPeriodMin = halfPeriod - halfPeriod*tolerance/100
					d.halfPeriodMax = halfPeriod + halfPeriod*tolerance/100

					d.state = synchronized
					d.eventSamples = nil
					debug.InfoLog.Println("synchronizing clock for manchester decoding finished")
					debug.InfoLog.Printf("clock: %.1f Hz\n", 1/float64(fullPeriod)*float64(time.Second))
					debug.DebugLog.Printf("full bit period: %v\n", fullPeriod)
					debug.DebugLog.Printf("half bit period: %v\n", halfPeriod)
				}()
			}
		}

	case synchronized:
		if period >= d.halfPeriodMin && period <= d.halfPeriodMax {
			if !d.firstHalfBit {
				d.firstHalfBit = true
				return
			}

			switch event.Type {
			case port.RisingEdge:
				d.C <- port.Low
			case port.FallingEdge:
				d.C <- port.High
			}

			d.firstHalfBit = false
			return
		}

		if period >= d.fullPeriodMin && period <= d.fullPeriodMax {
			if d.firstHalfBit {
				debug.ErrorLog.Println("illegal previous half period")

				d.firstHalfBit = false
				d.C <- port.Invalid
				return
			}

			switch event.Type {
			case port.RisingEdge:
				d.C <- port.Low
			case port.FallingEdge:
				d.C <- port.High
			}
			return
		}

		debug.ErrorLog.Printf("invalid bit period: %v:", period)
		d.firstHalfBit = false
		// start synchronizing clock
		// debug.DebugLog.Print(" clock synchronizing for manchester decoding started")
		// m.reset()		d.C <- port.Invalid
		d.C <- port.Invalid
	}
}

// calcBitPeriods calculates the manchester bit periods from the event samples
func calcBitPeriods(samples []time.Duration) (halfBitPeriod, fullBitPeriod time.Duration) {
	// the first entry in the slice must be a half bit period
	// so, sorting the samples helps to identify a half bit period
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })

	// drop the lowest and highest event sample
	samples = samples[1 : len(samples)-1]

	halfBitPeriodSum := time.Duration(0)
	fullBitPeriodSum := time.Duration(0)

	// since the slice is sorted, the first entry is a half bit period!
	halfBitPeriod = samples[0] //
	fullBitPeriod = halfBitPeriod * 2

	ixFull := 1
	ixHalf := 1

	// the calculation of half bit period and full bit period is based on average calculation
	// of the received half bit Periods and full bit periods
	for _, t := range samples {
		// if time duration is greater than 150% of a half bit period, it is full bit period
		if t > halfBitPeriod+halfBitPeriod/2 {
			fullBitPeriodSum += t
			fullBitPeriod = fullBitPeriodSum / time.Duration(ixFull)
			ixFull++
			continue
		}

		// otherwise, it is a half period
		halfBitPeriodSum += t
		halfBitPeriod = halfBitPeriodSum / time.Duration(ixHalf)
		ixHalf++
	}

	return halfBitPeriod, fullBitPeriod
}

// reset restarts to synchronize manchester bit periods
func (d *Decoder) reset() {
	d.eventSamples = make([]time.Duration, 0, eventSamples)
	d.state = synchronizing
	d.firstHalfBit = false
}
