# Metrics Prometheus

[![CI](https://git.zem.systems/muxcore/metrics-prometheus/actions/workflows/ci.yml/badge.svg)](https://git.zem.systems/muxcore/metrics-prometheus/actions/workflows/ci.yml)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Prometheus metrics exporter and provider.**

A MuxCore sidecar module that registers counters, gauges, and histograms over gRPC and scrapes them via a Prometheus HTTP endpoint. Provides the `metrics` capability.

---

## How It Works

```
Modules ──→ metrics-prometheus (gRPC :9900) ──→ in-process registry
                                                    │
Prometheus ──→ GET /metrics (HTTP :9901) ───────────┘
```

Fresh starts expose Go/process collectors immediately; application series appear after modules register via gRPC.

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_GRPC_ADDR` | `127.0.0.1:9900` | Metrics gRPC listen address |
| `METRICS_HTTP_ADDR` | `127.0.0.1:9901` | Prometheus scrape HTTP address |
| `METRICS_PATH` | `/metrics` | Scrape path (also live via `metrics_path` setting) |
| `METRICS_SCRAPE_TOKEN` | *(unset)* | When set, scrape and gRPC require `Authorization: Bearer <token>` |

Loopback defaults keep the sidecar off the LAN. Bind `0.0.0.0` explicitly when Prometheus runs on another host, and set `METRICS_SCRAPE_TOKEN`.

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./metrics-prometheus --muxcore-mesh-addr localhost:9090
# scrape: curl http://127.0.0.1:9901/metrics
# health: curl http://127.0.0.1:9901/health
```

Docker Compose with Prometheus scrape config: `deploy/docker-compose.yml` + `deploy/prometheus.yml`.

---

## Capability

`metrics` — Prometheus metrics provider (`metrics.prometheus`, `settings`)

### Caller example

```go
ctx := context.Background()
cl, conn, err := metricsclient.Dial(ctx, os.Getenv("METRICS_GRPC_ADDR"))
if err != nil { return err }
defer conn.Close()

if err := cl.RegisterCounter(ctx, "my_module_ops_total", "operations", []string{"op"}); err != nil {
    return err
}
return cl.IncrementCounter(ctx, "my_module_ops_total", map[string]string{"op": "sync"}, 1)
```

Import: `github.com/Muxcore-Media/metrics-prometheus/pkg/client`

---

## Client

`pkg/client` dials `METRICS_GRPC_ADDR` and wraps `Register*` / `IncrementCounter` / `SetGauge` / `ObserveHistogram`.

## License

GPL-3.0
