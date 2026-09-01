package client

import (
	"context"
	"fmt"
	"os"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"

	metricsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/metrics/v1"
)

// Client wraps MetricsService gRPC for callers that emit metrics via metrics-prometheus.
type Client struct {
	rpc metricsv1.MetricsServiceClient
}

// New returns a client backed by an existing gRPC connection.
func New(conn grpc.ClientConnInterface) *Client {
	return &Client{rpc: metricsv1.NewMetricsServiceClient(conn)}
}

// Dial connects to METRICS_GRPC_ADDR (default 127.0.0.1:9900) and returns a client.
func Dial(ctx context.Context, addr string, opts ...grpc.DialOption) (*Client, grpc.ClientConnInterface, error) {
	if addr == "" {
		addr = os.Getenv("METRICS_GRPC_ADDR")
	}
	if addr == "" {
		addr = "127.0.0.1:9900"
	}
	base := []grpc.DialOption{
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	}
	base = append(base, opts...)
	conn, err := grpc.NewClient(addr, base...)
	if err != nil {
		return nil, nil, fmt.Errorf("dial metrics gRPC %s: %w", addr, err)
	}
	return New(conn), conn, nil
}

func toLabelPairs(labels map[string]string) []*metricsv1.LabelPair {
	if len(labels) == 0 {
		return nil
	}
	out := make([]*metricsv1.LabelPair, 0, len(labels))
	for k, v := range labels {
		out = append(out, &metricsv1.LabelPair{Name: k, Value: v})
	}
	return out
}

func toLabelNamePairs(names []string) []*metricsv1.LabelPair {
	if len(names) == 0 {
		return nil
	}
	out := make([]*metricsv1.LabelPair, len(names))
	for i, n := range names {
		out[i] = &metricsv1.LabelPair{Name: n}
	}
	return out
}

// RegisterCounter defines a counter family with the given label dimensions.
func (c *Client) RegisterCounter(ctx context.Context, name, help string, labelNames []string) error {
	_, err := c.rpc.RegisterCounter(ctx, &metricsv1.RegisterCounterRequest{
		Name:   name,
		Help:   help,
		Labels: toLabelNamePairs(labelNames),
	})
	return err
}

// RegisterGauge defines a gauge family with the given label dimensions.
func (c *Client) RegisterGauge(ctx context.Context, name, help string, labelNames []string) error {
	_, err := c.rpc.RegisterGauge(ctx, &metricsv1.RegisterGaugeRequest{
		Name:   name,
		Help:   help,
		Labels: toLabelNamePairs(labelNames),
	})
	return err
}

// RegisterHistogram defines a histogram family with the given label dimensions.
func (c *Client) RegisterHistogram(ctx context.Context, name, help string, labelNames []string, buckets []float64) error {
	_, err := c.rpc.RegisterHistogram(ctx, &metricsv1.RegisterHistogramRequest{
		Name:    name,
		Help:    help,
		Labels:  toLabelNamePairs(labelNames),
		Buckets: buckets,
	})
	return err
}

// IncrementCounter adds delta to a counter time series.
func (c *Client) IncrementCounter(ctx context.Context, name string, labels map[string]string, delta float64) error {
	_, err := c.rpc.IncrementCounter(ctx, &metricsv1.IncrementCounterRequest{
		Name:   name,
		Labels: toLabelPairs(labels),
		Delta:  delta,
	})
	return err
}

// SetGauge sets a gauge time series.
func (c *Client) SetGauge(ctx context.Context, name string, labels map[string]string, value float64) error {
	_, err := c.rpc.SetGauge(ctx, &metricsv1.SetGaugeRequest{
		Name:   name,
		Labels: toLabelPairs(labels),
		Value:  value,
	})
	return err
}

// ObserveHistogram records an observation.
func (c *Client) ObserveHistogram(ctx context.Context, name string, labels map[string]string, value float64) error {
	_, err := c.rpc.ObserveHistogram(ctx, &metricsv1.ObserveHistogramRequest{
		Name:   name,
		Labels: toLabelPairs(labels),
		Value:  value,
	})
	return err
}
