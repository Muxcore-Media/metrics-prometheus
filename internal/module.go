package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"strings"
	"sync"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/core/pkg/contracts"
	metricsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/metrics/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type Module struct {
	metricsv1.UnimplementedMetricsServiceServer
	mu          sync.Mutex
	cfgMu       sync.RWMutex
	registry    *prometheus.Registry
	counters    map[string]prometheus.Counter
	gauges      map[string]prometheus.Gauge
	histograms  map[string]prometheus.Histogram
	grpcSrv     *grpc.Server
	httpSrv     *http.Server
	grpcLis     net.Listener
	httpLis     net.Listener
	scrape      *scrapeHandler
	id          string
	grpcAddr    string
	httpAddr    string
	metricsPath string
}

type Config struct {
	ID          string
	GRPCAddr    string
	HTTPAddr    string
	MetricsPath string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "metrics-prometheus"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = ":9900"
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = ":9901"
	}
	if cfg.MetricsPath == "" {
		cfg.MetricsPath = "/metrics"
	}
	if v := os.Getenv("METRICS_GRPC_ADDR"); v != "" {
		cfg.GRPCAddr = v
	}
	if v := os.Getenv("METRICS_HTTP_ADDR"); v != "" {
		cfg.HTTPAddr = v
	}
	if v := strings.TrimSpace(os.Getenv("METRICS_PATH")); v != "" {
		cfg.MetricsPath = v
	}
	return &Module{
		id:          cfg.ID,
		grpcAddr:    cfg.GRPCAddr,
		httpAddr:    cfg.HTTPAddr,
		metricsPath: normalizePath(cfg.MetricsPath),
		registry:    prometheus.NewRegistry(),
		counters:    make(map[string]prometheus.Counter),
		gauges:      make(map[string]prometheus.Gauge),
		histograms:  make(map[string]prometheus.Histogram),
	}
}

func normalizePath(p string) string {
	p = strings.TrimSpace(p)
	if p == "" {
		return "/metrics"
	}
	if !strings.HasPrefix(p, "/") {
		p = "/" + p
	}
	return p
}

func (m *Module) Info() contracts.ModuleInfo {
	return contracts.ModuleInfo{
		ID:           m.id,
		Name:         "Metrics Prometheus",
		Version:      "0.1.1",
		Roles:        []string{"infrastructure"},
		Description:  "Prometheus metrics exporter and provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityMetrics, "metrics.prometheus", "settings"},
		HTTPAddr:     m.grpcAddr,
	}
}

func (m *Module) Init(ctx context.Context) error {
	var err error
	m.grpcLis, err = net.Listen("tcp", m.grpcAddr)
	if err != nil {
		return fmt.Errorf("listen gRPC %s: %w", m.grpcAddr, err)
	}
	m.httpLis, err = net.Listen("tcp", m.httpAddr)
	if err != nil {
		return fmt.Errorf("listen HTTP %s: %w", m.httpAddr, err)
	}
	m.cfgMu.RLock()
	path := m.metricsPath
	m.cfgMu.RUnlock()
	slog.Info("metrics-prometheus initialized", "grpc", m.grpcAddr, "http", m.httpAddr, "path", path)
	return nil
}

func (m *Module) Start(ctx context.Context) error {
	m.grpcSrv = grpc.NewServer()
	metricsv1.RegisterMetricsServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		slog.Info("metrics gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("metrics gRPC serve error", "error", err)
		}
	}()

	m.cfgMu.RLock()
	path := m.metricsPath
	m.cfgMu.RUnlock()
	m.scrape = &scrapeHandler{
		path: path,
		h:    promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}),
	}
	m.httpSrv = &http.Server{Handler: m.scrape}
	go func() {
		slog.Info("metrics HTTP endpoint started", "addr", m.httpAddr, "path", path)
		if err := m.httpSrv.Serve(m.httpLis); err != nil && err != http.ErrServerClosed {
			slog.Error("metrics HTTP serve error", "error", err)
		}
	}()
	return nil
}

func (m *Module) Stop(ctx context.Context) error {
	if m.grpcSrv != nil {
		m.grpcSrv.GracefulStop()
	}
	if m.httpSrv != nil {
		_ = m.httpSrv.Shutdown(ctx)
	}
	slog.Info("metrics-prometheus stopped")
	return nil
}

func (m *Module) Health(ctx context.Context) error {
	return nil
}

type scrapeHandler struct {
	mu   sync.RWMutex
	path string
	h    http.Handler
}

func (s *scrapeHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	s.mu.RLock()
	path := s.path
	h := s.h
	s.mu.RUnlock()
	if r.URL.Path != path {
		http.NotFound(w, r)
		return
	}
	h.ServeHTTP(w, r)
}

func (s *scrapeHandler) setPath(path string) {
	s.mu.Lock()
	s.path = path
	s.mu.Unlock()
}

func (m *Module) labelsToMap(labels []*metricsv1.LabelPair) prometheus.Labels {
	lm := make(prometheus.Labels, len(labels))
	for _, l := range labels {
		lm[l.GetName()] = l.GetValue()
	}
	return lm
}

func (m *Module) metricKey(name string, labels prometheus.Labels) string {
	parts := []string{name}
	for k, v := range labels {
		parts = append(parts, k+"="+v)
	}
	return strings.Join(parts, "{")
}

func (m *Module) RegisterCounter(ctx context.Context, req *metricsv1.RegisterCounterRequest) (*metricsv1.RegisterCounterResponse, error) {
	labels := m.labelsToMap(req.GetLabels())
	key := m.metricKey(req.GetName(), labels)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.counters[key]; ok {
		return &metricsv1.RegisterCounterResponse{Status: "already exists"}, nil
	}
	c := prometheus.NewCounter(prometheus.CounterOpts{
		Name: req.GetName(), Help: req.GetHelp(), ConstLabels: labels,
	})
	m.registry.MustRegister(c)
	m.counters[key] = c
	return &metricsv1.RegisterCounterResponse{Status: "ok"}, nil
}

func (m *Module) RegisterGauge(ctx context.Context, req *metricsv1.RegisterGaugeRequest) (*metricsv1.RegisterGaugeResponse, error) {
	labels := m.labelsToMap(req.GetLabels())
	key := m.metricKey(req.GetName(), labels)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.gauges[key]; ok {
		return &metricsv1.RegisterGaugeResponse{Status: "already exists"}, nil
	}
	g := prometheus.NewGauge(prometheus.GaugeOpts{
		Name: req.GetName(), Help: req.GetHelp(), ConstLabels: labels,
	})
	m.registry.MustRegister(g)
	m.gauges[key] = g
	return &metricsv1.RegisterGaugeResponse{Status: "ok"}, nil
}

func (m *Module) RegisterHistogram(ctx context.Context, req *metricsv1.RegisterHistogramRequest) (*metricsv1.RegisterHistogramResponse, error) {
	labels := m.labelsToMap(req.GetLabels())
	key := m.metricKey(req.GetName(), labels)
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.histograms[key]; ok {
		return &metricsv1.RegisterHistogramResponse{Status: "already exists"}, nil
	}
	buckets := req.GetBuckets()
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	h := prometheus.NewHistogram(prometheus.HistogramOpts{
		Name: req.GetName(), Help: req.GetHelp(), ConstLabels: labels, Buckets: buckets,
	})
	m.registry.MustRegister(h)
	m.histograms[key] = h
	return &metricsv1.RegisterHistogramResponse{Status: "ok"}, nil
}

func (m *Module) IncrementCounter(ctx context.Context, req *metricsv1.IncrementCounterRequest) (*metricsv1.IncrementCounterResponse, error) {
	labels := m.labelsToMap(req.GetLabels())
	key := m.metricKey(req.GetName(), labels)
	m.mu.Lock()
	c, ok := m.counters[key]
	m.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "counter %q not registered", key)
	}
	c.Add(req.GetDelta())
	return &metricsv1.IncrementCounterResponse{Status: "ok"}, nil
}

func (m *Module) SetGauge(ctx context.Context, req *metricsv1.SetGaugeRequest) (*metricsv1.SetGaugeResponse, error) {
	labels := m.labelsToMap(req.GetLabels())
	key := m.metricKey(req.GetName(), labels)
	m.mu.Lock()
	g, ok := m.gauges[key]
	m.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "gauge %q not registered", key)
	}
	g.Set(req.GetValue())
	return &metricsv1.SetGaugeResponse{Status: "ok"}, nil
}

func (m *Module) ObserveHistogram(ctx context.Context, req *metricsv1.ObserveHistogramRequest) (*metricsv1.ObserveHistogramResponse, error) {
	labels := m.labelsToMap(req.GetLabels())
	key := m.metricKey(req.GetName(), labels)
	m.mu.Lock()
	h, ok := m.histograms[key]
	m.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "histogram %q not registered", key)
	}
	h.Observe(req.GetValue())
	return &metricsv1.ObserveHistogramResponse{Status: "ok"}, nil
}
