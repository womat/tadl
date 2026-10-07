// Command dlbussim emulates a Technische Alternative controller on the DL-Bus,
// so tadl can be tested without one.
//
// It drives a GPIO pin with data frames back to back, the way a controller
// does: a SYNC of 16 high bits, then the frame bytes, each with a low start
// bit, eight data bits LSB first and a high stop bit, Manchester-encoded
// (IEEE). The values are fixed by flags.
//
// Wire the pin to tadl's input, directly or through the same optocoupler a
// real bus would use. On a Raspberry Pi with GPIO21 driving an optocoupler
// whose output is GPIO20:
//
//	dlbussim -gpio 21 -temps 21.5,45.3,-7.2,0 -out1     # tadl reads gpio 20
//	dlbussim -gpio 21 -bitClock 50 -offset 5            # a controller 5 % fast
//	dlbussim -gpio 21 -bitClock 488 -frames 10          # 488 Hz, ten frames
//
// It runs until the given number of frames is sent, or until interrupted, and
// leaves the pin low.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"math"
	"os"
	"os/signal"
	"strconv"
	"strings"
	"syscall"

	"github.com/womat/golib/gpio"
	"github.com/womat/golib/gpio/rpi"
	"github.com/womat/golib/manchester/encoder"
	"github.com/womat/tadl/pkg/datalogger"
)

func main() {
	gpioLine := flag.Int("gpio", 21, "GPIO pin (BCM) to drive")
	typ := flag.String("type", "uvr42", "controller to emulate: uvr42 | uvr31")
	bitClock := flag.Int("bitClock", 50, "nominal bit clock in Hz (UVR42/UVR31: 50, UVR1611: 488)")
	offset := flag.Float64("offset", 0, "deviation of the sender's clock in percent, e.g. 5 or -5")
	temps := flag.String("temps", "21.5,45.3,-7.2,0", "temperatures in °C, comma separated (uvr42: 4, uvr31: 3)")
	out1 := flag.Bool("out1", false, "output 1 on")
	out2 := flag.Bool("out2", false, "output 2 on (uvr42 only)")
	frames := flag.Int("frames", 0, "number of frames to send, 0 = until interrupted")
	flag.Parse()

	frame, err := buildFrame(*typ, *temps, *out1, *out2)
	if err != nil {
		log.Fatal(err)
	}

	clock := int(math.Round(float64(*bitClock) * (1 + *offset/100)))
	if clock <= 0 {
		log.Fatalf("bit clock %d Hz with offset %v %% is not positive", *bitClock, *offset)
	}

	pin, err := rpi.NewPin(*gpioLine, rpi.WithMode(gpio.Output))
	if err != nil {
		log.Fatal(err)
	}
	defer pin.Close()
	if err := pin.SetValue(gpio.Low); err != nil {
		log.Fatal(err)
	}

	enc, err := encoder.New(clock,
		func(level encoder.Level) error { return pin.SetValue(gpio.Level(level)) },
		encoder.WithSyncBytes(2), // 16 high bits without start and stop bit
		encoder.WithBitOrder(encoder.LSBFirst),
		encoder.WithManchesterEncoding(encoder.IEEE),
		encoder.WithErrorHandler(func(err error) { log.Printf("GPIO error: %v", err) }),
	)
	if err != nil {
		log.Fatal(err)
	}

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		_ = enc.Close() // aborts a Send waiting for buffer space
	}()

	log.Printf("emulating %s on GPIO%d at %d Hz (nominal %d Hz, offset %+.1f %%), frame % x",
		*typ, *gpioLine, clock, *bitClock, *offset, frame)

	sent := 0
	for *frames == 0 || sent < *frames {
		if _, err := enc.Send(frame); err != nil {
			if !errors.Is(err, encoder.ErrEncoderStopped) {
				log.Print(err)
			}
			break
		}
		sent++
	}
	if ctx.Err() == nil {
		enc.Wait() // the given number of frames: let the last one go out
	}
	_ = enc.Close()

	if err := pin.SetValue(gpio.Low); err != nil {
		log.Printf("could not leave GPIO%d low: %v", *gpioLine, err)
	}
	log.Printf("stopped after queuing %d frames", sent)
}

// buildFrame returns the data bytes of one frame of the given controller.
func buildFrame(typ, temps string, out1, out2 bool) ([]byte, error) {
	var values []float64
	for _, s := range strings.Split(temps, ",") {
		v, err := strconv.ParseFloat(strings.TrimSpace(s), 64)
		if err != nil {
			return nil, fmt.Errorf("invalid temperature %q: %w", s, err)
		}
		values = append(values, v)
	}

	switch typ {
	case "uvr42":
		if len(values) != 4 {
			return nil, fmt.Errorf("uvr42 needs 4 temperatures, got %d", len(values))
		}
		return datalogger.FrameUVR42([4]float64(values), out1, out2), nil
	case "uvr31":
		if len(values) != 3 {
			return nil, fmt.Errorf("uvr31 needs 3 temperatures, got %d", len(values))
		}
		return datalogger.FrameUVR31([3]float64(values), out1), nil
	default:
		return nil, fmt.Errorf("unsupported controller type %q, want uvr42 or uvr31", typ)
	}
}
