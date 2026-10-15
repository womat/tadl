package app

import (
	"bytes"
	"context"
	"encoding/json"
	"flag"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"testing"

	"github.com/womat/golib/mqtt"
	"github.com/womat/tadl/app/service/collector"
	"github.com/womat/tadl/pkg/datalogger"
)

// update rewrites the golden files: go test ./app -run TestGolden -update
var update = flag.Bool("update", false, "rewrite the golden files in testdata/golden")

// capturePublisher keeps the last MQTT payload.
type capturePublisher struct{ payload []byte }

func (c *capturePublisher) Publish(msg mqtt.Message) error {
	c.payload = msg.Payload
	return nil
}

// timeField matches the two time fields of the output. Their format is checked by the pattern,
// whole seconds with a zone offset; their value depends on the clock and is replaced.
var (
	timeField = regexp.MustCompile(`"(timestamp|lastFrame)":"\d{4}-\d{2}-\d{2}T\d{2}:\d{2}:\d{2}(Z|[+-]\d{2}:\d{2})"`)
	ageField  = regexp.MustCompile(`"lastFrameAgeSeconds":[0-9.e+-]+`)
)

func normalize(t *testing.T, name string, b []byte) []byte {
	t.Helper()
	if bytes.Contains(b, []byte(`"timestamp"`)) && !timeField.Match(b) {
		t.Fatalf("%s: timestamp not in RFC 3339 with whole seconds: %s", name, b)
	}
	b = timeField.ReplaceAll(b, []byte(`"$1":"<time>"`))
	return ageField.ReplaceAll(b, []byte(`"lastFrameAgeSeconds":"<age>"`))
}

// TestGoldenOutput pins the bytes a consumer sees - the MQTT payload, /data and the controller
// part of /health - for both controllers, with and without a sensor out of range. The frames go
// through the real chain: decoder, collector, handler. A change here breaks Node-RED and every
// other consumer; it must be deliberate and go into the README.
func TestGoldenOutput(t *testing.T) {
	for _, tc := range []struct {
		name, typ, device string
		frame             []byte
	}{
		{"uvr42", "uvr42", "", datalogger.FrameUVR42([4]float64{21.5, 45.3, -7.2, 0}, true, false)},
		{"uvr42-fault-named", "uvr42", "solar", datalogger.FrameUVR42([4]float64{21.5, 999, -7.2, 0}, false, true)},
		{"uvr31", "uvr31", "", datalogger.FrameUVR31([3]float64{20, 30, -20}, true)},
		{"uvr31-fault", "uvr31", "", datalogger.FrameUVR31([3]float64{-60, 30.1, 0.1}, false)},
	} {
		t.Run(tc.name, func(t *testing.T) {
			typ := deviceType(tc.typ)
			var dl datalogger.DL = datalogger.NewUVR42()
			if typ == datalogger.UVR31 {
				dl = datalogger.NewUVR31()
			}

			rx := make(chan []byte, 1)
			rx <- tc.frame
			close(rx)
			frames, err := dl.Watch(rx)
			if err != nil {
				t.Fatal(err)
			}

			svc := collector.New(collector.Config{Topic: "test", Device: tc.device}, typ)
			pub := &capturePublisher{}
			svc.Run(context.Background(), frames, pub) // returns once the decoder closes frames
			_ = dl.Close()

			c := validConfig()
			c.DataLogger.Type = tc.typ
			a := New(c, "", nil, nil)
			a.dataloggerService = svc

			rec := httptest.NewRecorder()
			a.HandleData().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/data", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("/data = %d: %s", rec.Code, rec.Body)
			}

			status, err := json.Marshal(svc.Status(a.sensorViews()))
			if err != nil {
				t.Fatal(err)
			}

			for suffix, got := range map[string][]byte{
				"mqtt":   pub.payload,
				"data":   rec.Body.Bytes(),
				"status": status,
			} {
				file := filepath.Join("testdata", "golden", tc.name+"."+suffix+".json")
				got = normalize(t, file, got)
				if *update {
					if err := os.MkdirAll(filepath.Dir(file), 0o755); err != nil {
						t.Fatal(err)
					}
					if err := os.WriteFile(file, append(got, '\n'), 0o644); err != nil {
						t.Fatal(err)
					}
					continue
				}
				want, err := os.ReadFile(file)
				if err != nil {
					t.Fatal(err)
				}
				if !bytes.Equal(append(got, '\n'), want) {
					t.Errorf("%s changed:\n got  %s\n want %s", file, got, want)
				}
			}
		})
	}
}
