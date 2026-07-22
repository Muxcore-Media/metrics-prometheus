# Metrics Prometheus

[![CI](https://github.com/Muxcore-Media/metrics-prometheus/actions/workflows/ci.yml/badge.svg)](https://github.com/Muxcore-Media/metrics-prometheus/actions)
[![Go Version](https://img.shields.io/badge/Go-1.26-blue)](https://go.dev/)
[![License: GPL-3.0](https://img.shields.io/badge/License-GPL--3.0-blue.svg)](LICENSE)

**Prometheus metrics exporter and provider.**

A MuxCore sidecar module that registers counters, gauges, and histograms over gRPC and scrapes them via a Prometheus HTTP `/metrics` endpoint. Provides the `metrics` capability.

---

## How It Works

```
Modules ──→ metrics-prometheus (gRPC) ──→ in-process registry
                                              │
Prometheus ──→ GET /metrics (HTTP) ───────────┘
```

---

## Configuration

| Variable | Default | Description |
|----------|---------|-------------|
| `METRICS_GRPC_ADDR` | `:9900` | Metrics gRPC listen address |
| `METRICS_HTTP_ADDR` | `:9901` | Prometheus scrape HTTP address |

---

## Quick Start

```bash
make build

export MUXCORE_INSECURE_DISABLE_TLS=true
./metrics-prometheus --muxcore-mesh-addr localhost:9090
# scrape: curl localhost:9901/metrics
```

---

## Capability

`metrics` — Prometheus metrics provider

## License

GPL-3.0
