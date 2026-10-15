package app

import (
	"crypto/tls"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"
)

func TestLoadTLSCertProdRefusesEmbeddedCert(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.pem")
	if _, err := loadTLSCert(missing, missing, ProdEnv); err == nil {
		t.Fatal("prod fell back to the embedded development certificate")
	}
}

func TestLoadTLSCertDevUsesEmbeddedCert(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.pem")
	if _, err := loadTLSCert(missing, missing, DevEnv); err != nil {
		t.Fatalf("dev fallback: %v", err)
	}
}

func TestRedactURL(t *testing.T) {
	got := redactURL("tcp://user:secret@broker:1883")
	if got != "tcp://user:xxxxx@broker:1883" {
		t.Errorf("redactURL = %q", got)
	}
}

// A client that offers at most TLS 1.2 is refused.
func TestTLSConfigRequires13(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing.pem")
	cert, err := loadTLSCert(missing, missing, DevEnv)
	if err != nil {
		t.Fatal(err)
	}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.TLS = newTLSConfig(cert)
	srv.StartTLS()
	defer srv.Close()

	for _, tt := range []struct {
		max  uint16
		fail bool
	}{
		{tls.VersionTLS12, true},
		{tls.VersionTLS13, false},
	} {
		client := &http.Client{Transport: &http.Transport{TLSClientConfig: &tls.Config{
			InsecureSkipVerify: true, // self-signed test certificate
			MaxVersion:         tt.max,
		}}}
		resp, err := client.Get(srv.URL)
		if err == nil {
			resp.Body.Close()
		}
		if (err != nil) != tt.fail {
			t.Errorf("max version %x: err = %v, want failure %v", tt.max, err, tt.fail)
		}
	}
}
