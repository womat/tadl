// Package manchester is a software decoder for manchester code
// the decoder determines the clock speed automatically
// it's only the decoder, not sender!
//
// inspired by:
// https://en.wikipedia.org/wiki/Manchester_code
// https://www.microchip.com/content/dam/mchp/documents/OTH/ApplicationNotes/ApplicationNotes/Atmel-9164-Manchester-Coding-Basics_Application-Note.pdf
// https://github.com/jdevelop/go-rf5v-transceiver

package manchester

import (
	"sort"
	"time"

	"github.com/womat/debug"
	"tadl/pkg/port"
)

const (
	// SensitivityFactor is the factor to calc the mid-bit time intervals (SignalT * SensitivityFactor).
	sensitivityFactor = 0.6

	// eventSamples are the count of event samples to calculate the clock.
	eventSamples = 500

	// discoverClock is the process state the clock frequency.
	discoverClock int = iota
	// synchronizing is the process state to synchronize to the clock
	// (distinguish a bit edge from a mid-bit transition).
	synchronizing
	// synchronized is the process state to decode edge events.
	synchronized
)

// Decoder represents the handler of the Decoder.
type Decoder struct {
	// state contains the current decoding state (discoverClock/synchronizing/synchronized).
	state int

	// eventSamples holds event samples to calculate bit periods (clock).
	eventSamples []time.Duration

	// lastTimestamp is the time of the last detected event.
	lastTimestamp time.Duration

	// lastInterval is the time state of the last detected event.
	// 1 >> 1 SignalT time (1/2 data rate from starting), signal is valid, level depends on falling/rising edge
	// 2 >> 1/2 data rate, start restart SignalT timer
	// 3 >> 3 SignalT time (1.5 data rate from starting), signal is valid, level depends on falling/rising edge
	// all others: invalid >> restart synchronizing
	lastInterval int

	// signalT defines the mid-bit time (T) >>  half of the clock period.
	// e.g. clock rate 50Hz (25bit/s) >> clock period 20ms >> signalT >> 10ms.
	signalT time.Duration

	// sensitivity is a helper variable to calc the mid-bit time intervals (SignalT * SensitivityFactor).
	sensitivity time.Duration

	// C is the channel to send the decoded bit stream.
	C chan port.StateType

	// rx is the channel to receive the line events.
	rx chan port.Event

	// quit is the channel to stop the Decoder.
	quit chan bool
	// done signals that handler is stopped.
	done chan bool
}

// New initials a new Decoder.
func New(c chan port.Event) *Decoder {
	d := Decoder{
		C:    make(chan port.StateType, 100),
		rx:   c,
		quit: make(chan bool),
		done: make(chan bool),
	}

	// start to discover clock frequency.
	d.eventSamples = make([]time.Duration, 0, eventSamples)
	d.state = discoverClock
	debug.DebugLog.Print("discovering clock frequency started")

	go d.run()
	return &d
}

// Close stops Decoder.
func (d *Decoder) Close() error {
	d.quit <- true

	// wait until run() is terminated
	<-d.done

	close(d.C)
	close(d.quit)
	close(d.done)
	return nil
}

// run receives events and send it to eventHandler to decode.
func (d *Decoder) run() {
	for {
		select {
		case <-d.quit:
			d.done <- true
			return
		case evt, open := <-d.rx:
			if !open {
				d.quit <- true
				continue
			}

			d.eventHandler(evt)
		}
	}
}

// eventHandler decodes line events (edges) to a bit stream.
//  * discoverClock:
//           the clock frequency is discovered by analyzing the bit periods (measuring full bit periods)
//  * synchronizing:
//          synchronize to the clock (distinguish a bit edge from a mid-bit transition)
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
	case discoverClock:
		if len(d.eventSamples) < eventSamples {
			d.eventSamples = append(d.eventSamples, period)

			if len(d.eventSamples) == eventSamples {
				halfPeriod, fullPeriod := calcBitPeriods(d.eventSamples)

				d.signalT = halfPeriod
				d.sensitivity = time.Duration(float64(d.signalT) * sensitivityFactor)

				debug.DebugLog.Println("discovering clock frequency finished")
				debug.InfoLog.Printf("clock: %.1f Hz\n", 1/fullPeriod.Seconds())
				debug.DebugLog.Printf("SignalT: %v\n", d.signalT)
				debug.DebugLog.Printf("Sensitivity: %v\n", d.sensitivity)

				d.state = synchronizing
				d.eventSamples = nil
			}
		}

	case synchronizing:
		// synchronize to the clock (distinguish a bit edge from a mid-bit transition)
		// capture next falling edge and check if period value equal 2 SignalT (T = 1⁄2 data rate)
		interval := int((period-d.sensitivity)/d.signalT) + 1

		if interval == 2 && event.Type == port.FallingEdge {
			debug.DebugLog.Println("synchronizing with the data clock finished")

			d.lastTimestamp = event.Timestamp - d.signalT
			d.lastInterval = 0
			d.state = synchronized
			return
		}

	case synchronized:
		// interval values:
		// 1 >> 1 SignalT time (1/2 data rate from starting), signal is valid, level depends on falling/rising edge
		// 2 >> 1/2 data rate, start restart SignalT timer
		// 3 >> 3 SignalT time (1.5 data rate from starting), signal is valid, level depends on falling/rising edge
		// all others: invalid >> restart synchronizing
		interval := int((period-d.sensitivity)/d.signalT) + 1

		if (interval == 1 && (d.lastInterval == 1 || d.lastInterval == 3)) ||
			(interval == 2 && d.lastInterval == 2) ||
			(interval == 3 && d.lastInterval == 2) {
			debug.WarningLog.Printf(
				"invalid interval combination: current state: %v, last state: %v (period: %v)",
				interval, d.lastInterval, period)

			d.C <- port.Invalid
			d.state = synchronizing
			return
		}

		switch interval {
		case 2:
			// d.lastTimestamp is already set at the beginning of the procedure
			// d.lastTimestamp = event.Timestamp
			d.lastInterval = interval

		case 1, 3:
			switch event.Type {
			case port.RisingEdge:
				d.C <- port.Low
			case port.FallingEdge:
				d.C <- port.High
			}

			d.lastInterval = interval
			d.lastTimestamp = event.Timestamp - d.signalT

		default:
			debug.WarningLog.Printf("invalid interval: %v (period: %v)", interval, period)

			d.C <- port.Invalid
			d.state = synchronizing
		}
	}
}

// calcBitPeriods calculates the manchester bit periods (clock) from the event samples
func calcBitPeriods(samples []time.Duration) (halfBitPeriod, fullBitPeriod time.Duration) {
	// the first entry in the slice must be a half bit period
	// so, sorting the samples helps to identify a half bit period
	sort.Slice(samples, func(i, j int) bool { return samples[i] < samples[j] })

	// drop the lowest and highest event sample
	samples = samples[1 : len(samples)-1]

	halfBitPeriodSum := time.Duration(0)
	fullBitPeriodSum := time.Duration(0)

	// since the slice is sorted, the first entry is a half bit period!
	halfBitPeriod = samples[0]
	fullBitPeriod = halfBitPeriod * 2

	ixFull := 1
	ixHalf := 1

	// the calculation of half bit period and full bit period is based on average calculation
	// of the received half bit periods and full bit periods
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
