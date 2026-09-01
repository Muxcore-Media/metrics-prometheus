package internal

import (
	"context"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"regexp"
	"sort"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/status"

	"github.com/Muxcore-Media/core/pkg/contracts"
	metricsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/metrics/v1"
	modulesdk "github.com/Muxcore-Media/core/sdk/go/module"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/collectors"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	Version         = "0.1.2"
	defaultGRPCAddr = "127.0.0.1:9900"
	defaultHTTPAddr = "127.0.0.1:9901"
	healthPath      = "/health"
)

var metricNameRE = regexp.MustCompile(`^[a-zA-Z_:][a-zA-Z0-9_:]*$`)

// DefaultHTTPAddr returns the default loopback scrape HTTP listen address.
func DefaultHTTPAddr() string {
	return defaultHTTPAddr
}

type counterFamily struct {
	vec        *prometheus.CounterVec
	labelNames []string
}

type gaugeFamily struct {
	vec        *prometheus.GaugeVec
	labelNames []string
}

type histogramFamily struct {
	vec        *prometheus.HistogramVec
	labelNames []string
}

type Module struct {
	metricsv1.UnimplementedMetricsServiceServer
	mu          sync.Mutex
	cfgMu       sync.RWMutex
	registry    *prometheus.Registry
	counters    map[string]*counterFamily
	gauges      map[string]*gaugeFamily
	histograms  map[string]*histogramFamily
	grpcSrv     *grpc.Server
	httpSrv     *http.Server
	grpcLis     net.Listener
	httpLis     net.Listener
	scrape      *httpHandler
	id          string
	grpcAddr    string
	httpAddr    string
	metricsPath string
	scrapeToken string
	grpcServing atomic.Bool
	httpServing atomic.Bool
}

type Config struct {
	ID          string
	GRPCAddr    string
	HTTPAddr    string
	MetricsPath string
	ScrapeToken string
}

func NewModule(cfg Config) *Module {
	if cfg.ID == "" {
		cfg.ID = "metrics-prometheus"
	}
	if cfg.GRPCAddr == "" {
		cfg.GRPCAddr = defaultGRPCAddr
	}
	if cfg.HTTPAddr == "" {
		cfg.HTTPAddr = defaultHTTPAddr
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
	if v := strings.TrimSpace(os.Getenv("METRICS_SCRAPE_TOKEN")); v != "" {
		cfg.ScrapeToken = v
	}
	reg := prometheus.NewRegistry()
	reg.MustRegister(collectors.NewGoCollector())
	reg.MustRegister(collectors.NewProcessCollector(collectors.ProcessCollectorOpts{}))
	return &Module{
		id:          cfg.ID,
		grpcAddr:    cfg.GRPCAddr,
		httpAddr:    cfg.HTTPAddr,
		metricsPath: normalizePath(cfg.MetricsPath),
		scrapeToken: cfg.ScrapeToken,
		registry:    reg,
		counters:    make(map[string]*counterFamily),
		gauges:      make(map[string]*gaugeFamily),
		histograms:  make(map[string]*histogramFamily),
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
		Version:      Version,
		Roles:        []string{"infrastructure"},
		Description:  "Prometheus metrics exporter and provider",
		Author:       "MuxCore",
		Capabilities: []string{contracts.CapabilityMetrics, "metrics.prometheus", "settings"},
		HTTPAddr:     m.httpAddr,
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
	opts := []grpc.ServerOption{}
	if m.scrapeToken != "" {
		opts = append(opts, grpc.UnaryInterceptor(m.authInterceptor))
	}
	m.grpcSrv = grpc.NewServer(opts...)
	metricsv1.RegisterMetricsServiceServer(m.grpcSrv, m)
	modulesdk.RegisterSettings(m.grpcSrv, m.id, m)
	go func() {
		m.grpcServing.Store(true)
		defer m.grpcServing.Store(false)
		slog.Info("metrics gRPC service started", "addr", m.grpcAddr)
		if err := m.grpcSrv.Serve(m.grpcLis); err != nil {
			slog.Error("metrics gRPC serve error", "error", err)
		}
	}()

	m.cfgMu.RLock()
	path := m.metricsPath
	token := m.scrapeToken
	m.cfgMu.RUnlock()
	m.scrape = &httpHandler{
		module:      m,
		metricsPath: path,
		scrapeToken: token,
		metrics:     promhttp.HandlerFor(m.registry, promhttp.HandlerOpts{}),
	}
	m.httpSrv = &http.Server{
		Handler:           m.scrape,
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		m.httpServing.Store(true)
		defer m.httpServing.Store(false)
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
	if m.grpcSrv == nil || m.httpSrv == nil {
		return fmt.Errorf("servers not started")
	}
	if !m.grpcServing.Load() || !m.httpServing.Load() {
		return fmt.Errorf("metrics servers not serving")
	}
	return nil
}

func (m *Module) authInterceptor(ctx context.Context, req any, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (any, error) {
	if m.scrapeToken == "" {
		return handler(ctx, req)
	}
	md, ok := metadata.FromIncomingContext(ctx)
	if !ok {
		return nil, status.Error(codes.Unauthenticated, "missing authorization metadata")
	}
	vals := md.Get("authorization")
	if len(vals) == 0 || !checkBearerToken(vals[0], m.scrapeToken) {
		return nil, status.Error(codes.Unauthenticated, "invalid bearer token")
	}
	return handler(ctx, req)
}

func checkBearerToken(header, want string) bool {
	const prefix = "Bearer "
	if !strings.HasPrefix(header, prefix) {
		return false
	}
	return strings.TrimSpace(header[len(prefix):]) == want
}

type httpHandler struct {
	mu          sync.RWMutex
	module      *Module
	metricsPath string
	scrapeToken string
	metrics     http.Handler
}

func (h *httpHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	switch r.URL.Path {
	case healthPath:
		if err := h.module.Health(r.Context()); err != nil {
			http.Error(w, err.Error(), http.StatusServiceUnavailable)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
		return
	}

	h.mu.RLock()
	path := h.metricsPath
	token := h.scrapeToken
	handler := h.metrics
	h.mu.RUnlock()

	if r.URL.Path != path {
		http.NotFound(w, r)
		return
	}
	if token != "" && !checkBearerToken(r.Header.Get("Authorization"), token) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	handler.ServeHTTP(w, r)
}

func (h *httpHandler) setPath(path string) {
	h.mu.Lock()
	h.metricsPath = path
	h.mu.Unlock()
}

func labelNamesFromPairs(labels []*metricsv1.LabelPair) []string {
	names := make([]string, 0, len(labels))
	seen := make(map[string]struct{}, len(labels))
	for _, l := range labels {
		n := strings.TrimSpace(l.GetName())
		if n == "" {
			continue
		}
		if _, ok := seen[n]; ok {
			continue
		}
		seen[n] = struct{}{}
		names = append(names, n)
	}
	sort.Strings(names)
	return names
}

func labelsToMap(labels []*metricsv1.LabelPair) map[string]string {
	lm := make(map[string]string, len(labels))
	for _, l := range labels {
		n := strings.TrimSpace(l.GetName())
		if n == "" {
			continue
		}
		lm[n] = l.GetValue()
	}
	return lm
}

func labelValues(labelNames []string, values map[string]string) []string {
	out := make([]string, len(labelNames))
	for i, name := range labelNames {
		out[i] = values[name]
	}
	return out
}

// HTTPListenAddr returns the bound HTTP listen address after Init.
func (m *Module) HTTPListenAddr() string {
	if m.httpLis == nil {
		return ""
	}
	return m.httpLis.Addr().String()
}

func validateMetricName(name string) error {
	name = strings.TrimSpace(name)
	if name == "" {
		return status.Error(codes.InvalidArgument, "metric name must not be empty")
	}
	if !metricNameRE.MatchString(name) {
		return status.Errorf(codes.InvalidArgument, "invalid metric name %q", name)
	}
	return nil
}

func (m *Module) RegisterCounter(ctx context.Context, req *metricsv1.RegisterCounterRequest) (*metricsv1.RegisterCounterResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if err := validateMetricName(name); err != nil {
		return nil, err
	}
	labelNames := labelNamesFromPairs(req.GetLabels())
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.gauges[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "metric %q already registered as gauge", name)
	}
	if _, ok := m.histograms[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "metric %q already registered as histogram", name)
	}
	if _, ok := m.counters[name]; ok {
		return &metricsv1.RegisterCounterResponse{Status: "already exists"}, nil
	}
	vec := prometheus.NewCounterVec(prometheus.CounterOpts{
		Name: name,
		Help: req.GetHelp(),
	}, labelNames)
	if err := m.registry.Register(vec); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "register counter: %v", err)
	}
	m.counters[name] = &counterFamily{vec: vec, labelNames: labelNames}
	return &metricsv1.RegisterCounterResponse{Status: "ok"}, nil
}

func (m *Module) RegisterGauge(ctx context.Context, req *metricsv1.RegisterGaugeRequest) (*metricsv1.RegisterGaugeResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if err := validateMetricName(name); err != nil {
		return nil, err
	}
	labelNames := labelNamesFromPairs(req.GetLabels())
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.counters[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "metric %q already registered as counter", name)
	}
	if _, ok := m.histograms[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "metric %q already registered as histogram", name)
	}
	if _, ok := m.gauges[name]; ok {
		return &metricsv1.RegisterGaugeResponse{Status: "already exists"}, nil
	}
	vec := prometheus.NewGaugeVec(prometheus.GaugeOpts{
		Name: name,
		Help: req.GetHelp(),
	}, labelNames)
	if err := m.registry.Register(vec); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "register gauge: %v", err)
	}
	m.gauges[name] = &gaugeFamily{vec: vec, labelNames: labelNames}
	return &metricsv1.RegisterGaugeResponse{Status: "ok"}, nil
}

func (m *Module) RegisterHistogram(ctx context.Context, req *metricsv1.RegisterHistogramRequest) (*metricsv1.RegisterHistogramResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if err := validateMetricName(name); err != nil {
		return nil, err
	}
	labelNames := labelNamesFromPairs(req.GetLabels())
	buckets := req.GetBuckets()
	if len(buckets) == 0 {
		buckets = prometheus.DefBuckets
	}
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.counters[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "metric %q already registered as counter", name)
	}
	if _, ok := m.gauges[name]; ok {
		return nil, status.Errorf(codes.AlreadyExists, "metric %q already registered as gauge", name)
	}
	if _, ok := m.histograms[name]; ok {
		return &metricsv1.RegisterHistogramResponse{Status: "already exists"}, nil
	}
	vec := prometheus.NewHistogramVec(prometheus.HistogramOpts{
		Name:    name,
		Help:    req.GetHelp(),
		Buckets: buckets,
	}, labelNames)
	if err := m.registry.Register(vec); err != nil {
		return nil, status.Errorf(codes.InvalidArgument, "register histogram: %v", err)
	}
	m.histograms[name] = &histogramFamily{vec: vec, labelNames: labelNames}
	return &metricsv1.RegisterHistogramResponse{Status: "ok"}, nil
}

func (m *Module) IncrementCounter(ctx context.Context, req *metricsv1.IncrementCounterRequest) (*metricsv1.IncrementCounterResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if err := validateMetricName(name); err != nil {
		return nil, err
	}
	if req.GetDelta() < 0 {
		return nil, status.Error(codes.InvalidArgument, "counter delta must not be negative")
	}
	values := labelsToMap(req.GetLabels())
	m.mu.Lock()
	family, ok := m.counters[name]
	m.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "counter %q not registered", name)
	}
	family.vec.WithLabelValues(labelValues(family.labelNames, values)...).Add(req.GetDelta())
	return &metricsv1.IncrementCounterResponse{Status: "ok"}, nil
}

func (m *Module) SetGauge(ctx context.Context, req *metricsv1.SetGaugeRequest) (*metricsv1.SetGaugeResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if err := validateMetricName(name); err != nil {
		return nil, err
	}
	values := labelsToMap(req.GetLabels())
	m.mu.Lock()
	family, ok := m.gauges[name]
	m.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "gauge %q not registered", name)
	}
	family.vec.WithLabelValues(labelValues(family.labelNames, values)...).Set(req.GetValue())
	return &metricsv1.SetGaugeResponse{Status: "ok"}, nil
}

func (m *Module) ObserveHistogram(ctx context.Context, req *metricsv1.ObserveHistogramRequest) (*metricsv1.ObserveHistogramResponse, error) {
	name := strings.TrimSpace(req.GetName())
	if err := validateMetricName(name); err != nil {
		return nil, err
	}
	values := labelsToMap(req.GetLabels())
	m.mu.Lock()
	family, ok := m.histograms[name]
	m.mu.Unlock()
	if !ok {
		return nil, status.Errorf(codes.NotFound, "histogram %q not registered", name)
	}
	family.vec.WithLabelValues(labelValues(family.labelNames, values)...).Observe(req.GetValue())
	return &metricsv1.ObserveHistogramResponse{Status: "ok"}, nil
}
