//+build !windows

package raspberry

import (
	"fmt"
	"time"

	"github.com/warthog618/gpio"
)

type RpiPin struct {
	gpioPin *gpio.Pin
	edge    Edge
	// the bounceTime defines the key bounce time (ms)
	// the value 0 ignores key bouncing
	bounceTime time.Duration
	// while bounceTimer is running, new signal are ignored (suppress key bouncing)
	bounceTimer *time.Timer
	shadow      bool
	handler     func(Pin)
}

// must be global in package, because handler function handler(pin *gpio.Pin) need this line Infos
var pins map[int]*RpiPin

type RpiGPIO struct{}

// Open GPIO memory range from /dev/gpiomem.
func Open() (*RpiGPIO, error) {
	pins = map[int]*RpiPin{}

	if err := gpio.Open(); err != nil {
		return nil, err
	}
	return &RpiGPIO{}, nil
}

// Close removes the interrupt handlers and unmaps GPIO memory
func (c *RpiGPIO) Close() (err error) {
	return gpio.Close()
}

// NewPin creates a new pin object.
// The pin number provided is the BCM GPIO number.
func (c *RpiGPIO) NewPin(p int) (Pin, error) {
	if _, ok := pins[p]; ok {
		return nil, fmt.Errorf("pin %v already used", p)
	}

	l := RpiPin{gpioPin: gpio.NewPin(p), bounceTimer: time.NewTimer(0)}
	pins[p] = &l
	return pins[p], nil
}

// Watch the pin for changes to level.
// The handler is called after bounce timeout and the state is still changed from shadow
// The edge determines which edge to watch.
// There can only be one watcher on the pin at a time.
func (p *RpiPin) Watch(edge Edge, handler func(Pin)) error {
	p.handler = handler
	p.edge = edge
	p.gpioPin.Mode()
	return p.gpioPin.Watch(gpio.Edge(edge), debounce)
}

// Unwatch removes any watch from the pin.
func (p *RpiPin) Unwatch() {
	p.gpioPin.Unwatch()
}

// SetBounceTime defines Timer which has to expired to check if the pin has still the correct level
func (p *RpiPin) SetBounceTime(t time.Duration) {
	p.bounceTime = t
	return
}

// Input sets pin as Input.
func (p *RpiPin) Input() {
	p.shadow = p.Read()
	p.gpioPin.Input()
}

// PullUp sets the pull state of the pin to PullUp
func (p *RpiPin) PullUp() {
	p.gpioPin.PullUp()
}

// PullDown sets the pull state of the pin to PullDown
func (p *RpiPin) PullDown() {
	p.gpioPin.PullDown()
}

// Pin returns the pin number that this Pin represents.
func (p *RpiPin) Pin() int {
	return p.gpioPin.Pin()
}

// Read pin state (high/low)
func (p *RpiPin) Read() bool {
	return bool(p.gpioPin.Read())
}

// EmuEdge emulate a statechange of given pin on Windows systems
// not supported for linux
func (p *RpiPin) EmuEdge(edge Edge) {
	return
}

// debounce ensures that state change lasts for at least the BounceTime without interruption and only then the handler is called
func debounce(g *gpio.Pin) {
	// check if map with pin struct exists
	pin, ok := pins[g.Pin()]
	if !ok {
		return
	}

	// if debounce is inactive, call handler function and returns
	if pin.bounceTime == 0 {
		pin.shadow = pin.Read()
		pin.handler(pin)
		return
	}

	select {
	case <-pin.bounceTimer.C:
		// if bounce Timer is expired, accept new signals
		pin.bounceTimer.Reset(pin.bounceTime)
	default:
		// if bounce Timer is still running, ignore the signal
		return
	}

	go func(p *RpiPin) {
		// wait until bounce Timer is expired and check if the pin has still the correct level
		// the correct level depends on the edge configuration
		<-p.bounceTimer.C
		p.bounceTimer.Reset(0)

		switch p.edge {
		case EdgeBoth:
			if pin.Read() != p.shadow {
				p.shadow = p.Read()
				pin.handler(p)
			}
		case EdgeFalling:
			if !p.Read() {
				p.shadow = p.Read()
				pin.handler(p)
			}
		case EdgeRising:
			if p.Read() {
				p.shadow = p.Read()
				pin.handler(p)
			}
		}
	}(pin)
	return
}
