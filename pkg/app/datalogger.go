package app

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"time"

	"tadl/pkg/datalogger"

	"github.com/womat/debug"
	"github.com/womat/mqtt"
)

// service wait in an endless loop for valid data logger frames.
// It save the data frame to app main structure and send the dataframe to the mqtt broker
func (app *App) run() {
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

// validateMeasurements checks the dataframe by deltaT and delta
// and send dataframe to mqtt if data changed or by send intervall
func (app *App) validateMeasurements(d interface{}) error {
	var diff bool

	app.mqttData.Lock()
	defer app.mqttData.Unlock()

	switch f := d.(type) {
	case datalogger.UVR42Frame:
		switch m := app.mqttData.data.(type) {
		case datalogger.UVR42Frame:
			diff = f.TimeStamp.Sub(m.TimeStamp) > app.config.MQTT.Interval ||
				f.Out1 != m.Out1 || f.Out2 != m.Out2 ||
				math.Abs(f.Temperature1-m.Temperature1) > app.config.MQTT.DeltaKelvin ||
				math.Abs(f.Temperature2-m.Temperature2) > app.config.MQTT.DeltaKelvin ||
				math.Abs(f.Temperature3-m.Temperature3) > app.config.MQTT.DeltaKelvin ||
				math.Abs(f.Temperature4-m.Temperature4) > app.config.MQTT.DeltaKelvin
		default:
			return fmt.Errorf("unsupported frame type")
		}
	default:
		return fmt.Errorf("unsupported frame type")
	}

	if diff {
		app.mqttData.data = d
		app.sendMQTT(app.config.MQTT.Topic, app.mqttData.data)
	}

	return nil
}

// sendMQTT send message struct to the mqtt broker.
func (app *App) sendMQTT(topic string, msg interface{}) {
	debug.TraceLog.Printf("prepare mqtt message %v %v", topic, msg)

	b, err := json.MarshalIndent(msg, "", "  ")
	if err != nil {
		debug.ErrorLog.Printf("sendMQTT marshal: %v", err)
		return
	}

	go app.mqtt.Publish(mqtt.Message{
		Qos:      0,
		Retained: true,
		Topic:    topic,
		Payload:  b,
	})

}
