package internal

import (
	"context"
	"io"
	"net/http"
	"strings"
	"testing"
	"time"

	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	metricsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/metrics/v1"
)

func TestModuleInfo(t *testing.T) {
	m := NewModule(Config{})
	info := m.Info()
	if info.ID == "" {
		t.Error("module ID must not be empty")
	}
	if info.Version != Version {
		t.Errorf("version = %q", info.Version)
	}
	if info.HTTPAddr != defaultHTTPAddr {
		t.Errorf("HTTPAddr = %q", info.HTTPAddr)
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

func startTestModule(t *testing.T) (*Module, string) {
	t.Helper()
	m := NewModule(Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatalf("Init: %v", err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}
	t.Cleanup(func() {
		_ = m.Stop(ctx)
	})
	// Wait until serving goroutines mark themselves live.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if m.grpcServing.Load() && m.httpServing.Load() {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if err := m.Health(ctx); err != nil {
		t.Fatalf("Health: %v", err)
	}
	return m, "http://" + m.httpLis.Addr().String()
}

func TestModuleLifecycle(t *testing.T) {
	m, base := startTestModule(t)
	client := &http.Client{Timeout: 2 * time.Second}

	resp, err := client.Get(base + "/metrics")
	if err != nil {
		t.Fatalf("GET /metrics: %v", err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status=%d", resp.StatusCode)
	}
	if !strings.Contains(string(body), "go_goroutines") {
		t.Fatalf("expected go collector metrics, got: %s", body)
	}

	respHealth, err := client.Get(base + healthPath)
	if err != nil {
		t.Fatalf("GET /health: %v", err)
	}
	_ = respHealth.Body.Close()
	if respHealth.StatusCode != http.StatusOK {
		t.Fatalf("health status=%d", respHealth.StatusCode)
	}

	if err := m.UpdateSetting("metrics_path", "/prom"); err != nil {
		t.Fatal(err)
	}
	resp2, err := client.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusNotFound {
		t.Fatalf("old path status=%d", resp2.StatusCode)
	}
	resp3, err := client.Get(base + "/prom")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp3.Body.Close()
	if resp3.StatusCode != http.StatusOK {
		t.Fatalf("new path status=%d", resp3.StatusCode)
	}
}

func TestRegisterIncrementScrape(t *testing.T) {
	m, base := startTestModule(t)
	ctx := context.Background()

	_, err := m.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{
		Name: "requests_total",
		Help: "test counter",
		Labels: []*metricsv1.LabelPair{
			{Name: "method"},
		},
	})
	if err != nil {
		t.Fatalf("RegisterCounter: %v", err)
	}
	_, err = m.IncrementCounter(ctx, &metricsv1.IncrementCounterRequest{
		Name:  "requests_total",
		Delta: 3,
		Labels: []*metricsv1.LabelPair{
			{Name: "method", Value: "GET"},
		},
	})
	if err != nil {
		t.Fatalf("IncrementCounter: %v", err)
	}

	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	text := string(body)
	if !strings.Contains(text, `requests_total{method="GET"} 3`) {
		t.Fatalf("missing exposition line, got:\n%s", text)
	}
}

func TestTwoLabelValuesOneFamily(t *testing.T) {
	m, base := startTestModule(t)
	ctx := context.Background()

	_, err := m.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{
		Name: "http_requests_total",
		Help: "by method and status",
		Labels: []*metricsv1.LabelPair{
			{Name: "method"},
			{Name: "status"},
		},
	})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		method string
		status string
		delta  float64
	}{
		{"GET", "200", 1},
		{"POST", "500", 2},
	} {
		_, err := m.IncrementCounter(ctx, &metricsv1.IncrementCounterRequest{
			Name:  "http_requests_total",
			Delta: tc.delta,
			Labels: []*metricsv1.LabelPair{
				{Name: "method", Value: tc.method},
				{Name: "status", Value: tc.status},
			},
		})
		if err != nil {
			t.Fatalf("increment %s/%s: %v", tc.method, tc.status, err)
		}
	}

	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	text := string(body)
	if !strings.Contains(text, `method="GET",status="200"`) {
		t.Fatalf("missing GET/200 series:\n%s", text)
	}
	if !strings.Contains(text, `method="POST",status="500"`) {
		t.Fatalf("missing POST/500 series:\n%s", text)
	}
}

func TestSameNameCounterGaugeError(t *testing.T) {
	m, _ := startTestModule(t)
	ctx := context.Background()

	_, err := m.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{
		Name: "dup_metric",
		Help: "counter",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.RegisterGauge(ctx, &metricsv1.RegisterGaugeRequest{
		Name: "dup_metric",
		Help: "gauge",
	})
	if err == nil {
		t.Fatal("expected error registering gauge with same name as counter")
	}
	if status.Code(err) != codes.AlreadyExists {
		t.Fatalf("code=%v err=%v", status.Code(err), err)
	}
}

func TestIncrementUnknownCounter(t *testing.T) {
	m, _ := startTestModule(t)
	_, err := m.IncrementCounter(context.Background(), &metricsv1.IncrementCounterRequest{
		Name:  "missing_total",
		Delta: 1,
	})
	if err == nil {
		t.Fatal("expected not found")
	}
	if status.Code(err) != codes.NotFound {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func TestValidateMetricName(t *testing.T) {
	m, _ := startTestModule(t)
	ctx := context.Background()

	_, err := m.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{Name: ""})
	if err == nil {
		t.Fatal("expected empty name error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v", status.Code(err))
	}

	_, err = m.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{Name: "1bad"})
	if err == nil {
		t.Fatal("expected invalid name error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func TestIncrementNegativeDelta(t *testing.T) {
	m, _ := startTestModule(t)
	ctx := context.Background()

	_, err := m.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{
		Name: "neg_total",
		Help: "test",
	})
	if err != nil {
		t.Fatal(err)
	}
	_, err = m.IncrementCounter(ctx, &metricsv1.IncrementCounterRequest{
		Name:  "neg_total",
		Delta: -1,
	})
	if err == nil {
		t.Fatal("expected negative delta error")
	}
	if status.Code(err) != codes.InvalidArgument {
		t.Fatalf("code=%v", status.Code(err))
	}
}

func TestScrapeAuth(t *testing.T) {
	m := NewModule(Config{
		GRPCAddr:    "127.0.0.1:0",
		HTTPAddr:    "127.0.0.1:0",
		ScrapeToken: "secret",
	})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })
	base := "http://" + m.httpLis.Addr().String()

	resp, err := http.Get(base + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("unauth status=%d", resp.StatusCode)
	}

	req, _ := http.NewRequest(http.MethodGet, base+"/metrics", nil)
	req.Header.Set("Authorization", "Bearer secret")
	resp2, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp2.Body.Close()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("auth status=%d", resp2.StatusCode)
	}
}

func TestHealthFailsBeforeStart(t *testing.T) {
	m := NewModule(Config{})
	if err := m.Health(context.Background()); err == nil {
		t.Fatal("expected health error before start")
	}
}
