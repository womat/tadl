package app

import (
	"os"
	"testing"
)

// A Run that fails in Init must release what it opened without panicking on the parts it
// never created, so cmd can start the previous configuration.
func TestFailedRunCleansUp(t *testing.T) {
	cfg := NewConfig()
	cfg.DlBus.GPIO = 999 // no GPIO chip offers this line
	cfg.MQTT.Connection = ""

	if _, err := New(cfg, t.TempDir(), make(chan os.Signal), nil).Run(); err == nil {
		t.Fatal("Run with an unavailable GPIO line succeeded")
	}
}
