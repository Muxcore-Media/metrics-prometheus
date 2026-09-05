package grpctls

import (
	"os"
	"path/filepath"
	"testing"
)

func TestInsecureAllowed(t *testing.T) {
	t.Setenv(envInsecureDisable, "")
	t.Setenv(envGRPCInsecure, "")

	if InsecureAllowed() {
		t.Fatal("expected secure by default")
	}

	t.Setenv(envInsecureDisable, "true")
	if !InsecureAllowed() {
		t.Fatal("expected insecure when MUXCORE_INSECURE_DISABLE_TLS=true")
	}

	t.Setenv(envInsecureDisable, "")
	t.Setenv(envGRPCInsecure, "1")
	if !InsecureAllowed() {
		t.Fatal("expected insecure when MUXCORE_GRPC_INSECURE=1")
	}
}

func TestServerConfig_InsecureReturnsNil(t *testing.T) {
	t.Setenv(envInsecureDisable, "true")
	cfg, err := ServerConfig()
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg != nil {
		t.Fatal("expected nil TLS config in insecure mode")
	}
}

func TestServerConfig_AutoGenerate(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(envInsecureDisable, "")
	t.Setenv(envGRPCInsecure, "")
	t.Setenv(envMetricsTLSDir, dir)
	t.Setenv(envMetricsTLSCert, "")
	t.Setenv(envMetricsTLSKey, "")

	cfg, err := ServerConfig()
	if err != nil {
		t.Fatalf("ServerConfig: %v", err)
	}
	if cfg == nil {
		t.Fatal("expected TLS config")
	}
	if len(cfg.Certificates) != 1 {
		t.Fatalf("certificates=%d", len(cfg.Certificates))
	}

	for _, name := range []string{"ca.crt", "ca.key", "server.crt", "server.key"} {
		path := filepath.Join(dir, name)
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("missing %s: %v", name, err)
		}
	}

	cfg2, err := ServerConfig()
	if err != nil {
		t.Fatalf("second ServerConfig: %v", err)
	}
	if cfg2 == nil || len(cfg2.Certificates) != 1 {
		t.Fatal("expected reused TLS config")
	}
}
