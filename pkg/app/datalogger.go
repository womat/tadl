package app

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"tadl/pkg/datalogger"
	"tadl/pkg/mqtt"

	"github.com/womat/debug"
)

// service wait in an endless loop for valid data logger frames.
// It save the data frame to app main structure and send the dataframe to the mqtt broker
func (app *App) service() {
	for {
		if err, f := app.dl.Get(); err != nil {
			if err == io.EOF {
				time.Sleep(100 * time.Millisecond)
				continue
			}

			debug.ErrorLog.Println(err)
		} else {
			debug.DebugLog.Printf("Frame: %v", f)
			app.DataFrame.Lock()
			app.DataFrame.data = f
			app.DataFrame.Unlock()
			_ = app.validateMeasurements(f)
		}
	}
}

// validateMeasurements checks the dataframe by deltaT and deltaK
// and send dataframe to mqtt if the delta values are exceeded or the state of an output port has change.
func (app *App) validateMeasurements(d interface{}) error {
	var deltaT time.Duration
	var deltaK float64
	var deltaOut bool

	app.mqttData.Lock()
	defer app.mqttData.Unlock()

	switch f := d.(type) {
	case datalogger.UVR42Frame:
		switch m := app.mqttData.data.(type) {
		case datalogger.UVR42Frame:

			deltaT = f.TimeStamp.Sub(m.TimeStamp)
			deltaK = math.Abs(f.Temperature1 - m.Temperature1)
			if t := math.Abs(f.Temperature2 - m.Temperature2); t > deltaK {
				deltaK = t
			}
			if t := math.Abs(f.Temperature3 - m.Temperature3); t > deltaK {
				deltaK = t
			}
			if t := math.Abs(f.Temperature4 - m.Temperature4); t > deltaK {
				deltaK = t
			}

			deltaOut = f.Out1 != m.Out1 || f.Out2 != m.Out2
		default:
			return fmt.Errorf("unsupported frame type")
		}
	default:
		return fmt.Errorf("unsupported frame type")
	}

	if deltaT >= app.config.MQTT.Interval || deltaK >= app.config.MQTT.DeltaKelvin || deltaOut {
		app.sendMQTT(app.config.MQTT.Topic, d)
		app.mqttData.data = d
	}

	return nil
}

// sendMQTT send message struct to the mqtt broker.
func (app *App) sendMQTT(topic string, message interface{}) {
	go func(t string, r interface{}) {
		debug.TraceLog.Printf("prepare mqtt message %v %v", t, r)

		b, err := json.MarshalIndent(r, "", "  ")
		if err != nil {
			debug.ErrorLog.Printf("sendMQTT marshal: %v", err)
			return
		}

		app.mqtt.C <- mqtt.Message{
			Qos:      0,
			Retained: true,
			Topic:    t,
			Payload:  b,
		}
	}(topic, message)
}
