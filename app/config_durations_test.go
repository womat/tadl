package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func loadYAML(t *testing.T, content string) (*Config, error) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "config.yaml")
	if err := os.WriteFile(file, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	return LoadConfig(file)
}

// yaml.v3 refuses a duration without a unit, 0 included; this pins that, so a bare number can
// never be read as nanoseconds.
func TestLoadConfigRejectsDurationWithoutUnit(t *testing.T) {
	for name, tc := range map[string]struct{ yaml, key string }{
		"publishInterval": {"mqtt:\n  publishInterval: 300\n", "mqtt.publishInterval"},
		"debounceTime":    {"dlbus:\n  debounceTime: 10\n", "dlbus.debounceTime"},
		"zero":            {"dlbus:\n  debounceTime: 0\n", "dlbus.debounceTime"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := loadYAML(t, tc.yaml)
			if err == nil || !strings.Contains(err.Error(), "time.Duration") {
				t.Errorf("%s: err = %v, want a refused duration", tc.key, err)
			}
		})
	}
}

func TestLoadConfigAcceptsDurationsWithUnit(t *testing.T) {
	if _, err := loadYAML(t, "mqtt:\n  publishInterval: 5m\ndlbus:\n  debounceTime: 0s\n"); err != nil {
		t.Errorf("valid durations refused: %v", err)
	}
}
