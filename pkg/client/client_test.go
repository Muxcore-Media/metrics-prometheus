package client_test

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"

	"google.golang.org/grpc"
	"google.golang.org/grpc/credentials/insecure"
	"google.golang.org/grpc/test/bufconn"

	metricsv1 "github.com/Muxcore-Media/core/proto/gen/muxcore/metrics/v1"
	"github.com/Muxcore-Media/metrics-prometheus/internal"
	"github.com/Muxcore-Media/metrics-prometheus/pkg/client"
)

const bufSize = 1 << 20

func startBufconnModule(t *testing.T) (*internal.Module, *bufconn.Listener, string) {
	t.Helper()
	m := internal.NewModule(internal.Config{GRPCAddr: "127.0.0.1:0", HTTPAddr: "127.0.0.1:0"})
	ctx := context.Background()
	if err := m.Init(ctx); err != nil {
		t.Fatal(err)
	}
	if err := m.Start(ctx); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = m.Stop(ctx) })

	lis := bufconn.Listen(bufSize)
	gs := grpc.NewServer()
	metricsv1.RegisterMetricsServiceServer(gs, m)
	go func() { _ = gs.Serve(lis) }()
	t.Cleanup(func() {
		gs.Stop()
		_ = lis.Close()
	})
	return m, lis, "http://" + m.HTTPListenAddr()
}

func dialClient(t *testing.T, lis *bufconn.Listener) *client.Client {
	t.Helper()
	conn, err := grpc.NewClient("passthrough:///bufnet",
		grpc.WithContextDialer(func(context.Context, string) (net.Conn, error) { return lis.Dial() }),
		grpc.WithTransportCredentials(insecure.NewCredentials()),
	)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = conn.Close() })
	return client.New(conn)
}

func TestClientRegisterIncrementScrape(t *testing.T) {
	_, lis, httpBase := startBufconnModule(t)
	cl := dialClient(t, lis)
	ctx := context.Background()

	if err := cl.RegisterCounter(ctx, "client_requests_total", "via client", []string{"op"}); err != nil {
		t.Fatal(err)
	}
	if err := cl.IncrementCounter(ctx, "client_requests_total", map[string]string{"op": "sync"}, 5); err != nil {
		t.Fatal(err)
	}

	resp, err := http.Get(httpBase + "/metrics")
	if err != nil {
		t.Fatal(err)
	}
	body, _ := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if !strings.Contains(string(body), `client_requests_total{op="sync"} 5`) {
		t.Fatalf("missing series: %s", body)
	}
}
