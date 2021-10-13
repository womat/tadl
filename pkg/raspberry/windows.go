//+build windows

package raspberry

import (
	"fmt"
	"time"
)

type WinPin struct {
	gpioPin int
	edge    Edge
	handler func(Pin)
}

type WinGPIO struct {
	pins map[int]*WinPin
}

// Open GPIO memory range from /dev/gpiomem .
func Open() (*WinGPIO, error) {
	return &WinGPIO{pins: map[int]*WinPin{}}, nil
}

// Close removes the interrupt handlers and unmaps GPIO memory
func (c *WinGPIO) Close() error {
	return nil
}

// NewPin creates a new pin object.
func (c *WinGPIO) NewPin(p int) (Pin, error) {
	if _, ok := c.pins[p]; ok {
		return nil, fmt.Errorf("pin %v already used", p)
	}

	l := WinPin{gpioPin: p}
	c.pins[p] = &l
	return c.pins[p], nil
}

// Watch the pin for changes to level.
// The handler is called after bounce timeout and the state is still changed from shadow
// The edge determines which edge to watch.
// There can only be one watcher on the pin at a time.
func (p *WinPin) Watch(edge Edge, handler func(Pin)) error {
	p.handler = handler
	p.edge = edge
	return nil
}

// Unwatch removes any watch from the pin.
func (p *WinPin) Unwatch() {
}

// SetBounceTime defines Timer which has to expired to check if the pin has still the correct level
func (p *WinPin) SetBounceTime(t time.Duration) {
	return
}

// Input sets pin as Input.
func (p *WinPin) Input() {
}

// PullUp sets the pull state of the pin to PullUp
func (p *WinPin) PullUp() {
}

// PullDown sets the pull state of the pin to PullDown
func (p *WinPin) PullDown() {
}

// Pin returns the pin number that this Pin represents.
func (p *WinPin) Pin() int {
	return p.gpioPin
}

// Read pin state (high/low)
func (p *WinPin) Read() bool {
	return false
}

// EmuEdge emulate a statechange of given pin on Windows systems
func (p *WinPin) EmuEdge(edge Edge) {
	switch {
	case p.edge == EdgeNone, edge == EdgeNone:
		return

	case edge == EdgeBoth:
		// if edge is EdgeBoth, handler is called twice
		if p.edge == EdgeBoth {
			p.handler(p)
		}

		if p.edge == EdgeBoth || p.edge == EdgeFalling || p.edge == EdgeRising {
			p.handler(p)
		}
	case edge == EdgeFalling:
		if p.edge == EdgeBoth || p.edge == EdgeFalling {
			p.handler(p)
		}
	case edge == EdgeRising:
		if p.edge == EdgeBoth || p.edge == EdgeRising {
			p.handler(p)
		}
	}
}
