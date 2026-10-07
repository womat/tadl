package app

import (
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
