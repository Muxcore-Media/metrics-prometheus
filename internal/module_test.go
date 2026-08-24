package internal

import (
	"context"
	"io"
	"net/http"
	"testing"
	"time"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version != "0.1.1" {
		t.Errorf("version = %q", info.Version)
	}
	found := false
	for _, c := range info.Capabilities {
		if c == "settings" {
			found = true
		}
	}
	if !found {
		t.Error("expected settings capability")
	}
}

func TestSettings_MetricsPath(t *testing.T) {
	m := NewModule(Config{})
	if err := m.UpdateSetting("metrics_path", "prom"); err != nil {
		t.Fatal(err)
	}
	if m.Settings()[0].Value != "/prom" {
		t.Fatalf("value=%q", m.Settings()[0].Value)
	}
	if err := m.UpdateSetting("metrics_path", "/"); err == nil {
		t.Fatal("expected reject root path")
	}
}

func TestModuleLifecycle(t *testing.T) {
	m := NewModule(Config{GRPCAddr: ":0", HTTPAddr: ":0"})
	ctx := context.Background()

	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	httpAddr := m.httpLis.Addr().String()
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://" + httpAddr + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	if _, err := io.Copy(io.Discard, resp.Body); err != nil {
		t.Fatalf("drain body: %v", err)
	}
	if err := resp.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	if resp.StatusCode != 200 {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if err := m.UpdateSetting("metrics_path", "/prom"); err != nil {
		t.Fatal(err)
	}
	resp2, err := client.Get("http://" + httpAddr + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	if err := resp2.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	if resp2.StatusCode != 404 {
		t.Fatalf("old path status=%d", resp2.StatusCode)
	}
	resp3, err := client.Get("http://" + httpAddr + "/prom")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := io.Copy(io.Discard, resp3.Body); err != nil {
		t.Fatalf("drain body: %v", err)
	}
	if err := resp3.Body.Close(); err != nil {
		t.Fatalf("close body: %v", err)
	}
	if resp3.StatusCode != 200 {
		t.Fatalf("new path status=%d", resp3.StatusCode)
	}
	if err := m.Stop(ctx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}
