package mqtt

import (
	mqttlib "github.com/eclipse/paho.mqtt.golang"
	"github.com/womat/debug"
)

// quiesce is the specified number of milliseconds to wait for existing work to be completed.
const (
	quiesce = 250
)

// Handler contains the handler of the mqtt broker.
type Handler struct {
	handler mqttlib.Client
	// C is the channel to service the mqtt message
	// sending a message to channel C will send the message.
	C chan Message
}

// Message contains the properties of the mqtt message.
type Message struct {
	Topic    string
	Payload  []byte
	Qos      byte
	Retained bool
}

// New generate a new mqtt broker client.
func New() *Handler {
	return &Handler{
		C: make(chan Message),
	}
}

// Connect connects to the mqtt broker.
// If no broker is defined, no mqtt message are send.
func (m *Handler) Connect(broker string) error {
	if broker == "" {
		return nil
	}

	opts := mqttlib.NewClientOptions().AddBroker(broker)
	m.handler = mqttlib.NewClient(opts)
	return m.ReConnect()
}

// ReConnect reconnects to the defined mqtt broker.
func (m *Handler) ReConnect() error {
	t := m.handler.Connect()
	<-t.Done()
	return t.Error()
}

// Disconnect will end the connection to the broker.
func (m *Handler) Disconnect() error {
	if m.handler == nil {
		return nil
	}

	m.handler.Disconnect(quiesce)
	return nil
}

// Service listen to a message on the channel C and send the message to mqtt.
// If no handler or topic is defined, the message will be ignored.
func (m *Handler) Service() {
	for d := range m.C {
		if m.handler == nil || d.Topic == "" {
			continue
		}

		go func(msg Message) {
			if !m.handler.IsConnected() {
				debug.DebugLog.Printf("mqtt broker isn't connected, reconnect it")

				if err := m.ReConnect(); err != nil {
					debug.ErrorLog.Printf("can't reconnect to mqtt broker %v", err)
					return
				}
			}

			debug.DebugLog.Printf("publishing %v bytes to topic %v", len(msg.Payload), msg.Topic)
			t := m.handler.Publish(msg.Topic, msg.Qos, msg.Retained, msg.Payload)

			// the asynchronous nature of this library makes it easy to forget to check for errors.
			// Consider using a go routine to log these
			go func() {
				<-t.Done()
				if err := t.Error(); err != nil {
					debug.ErrorLog.Printf("publishing topic %v: %v", msg.Topic, err)
				}
			}()
		}(d)
	}
}
