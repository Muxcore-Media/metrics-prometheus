# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.0         | v0.5.0+     | Current |

## Capabilities

| Capability | Status |
|------------|--------|
| `metrics` | Current |
| `metrics.prometheus` | Current |

Scrape endpoint: HTTP `GET /metrics` on `METRICS_HTTP_ADDR` (default `:9901`).

Inbound gRPC (`METRICS_GRPC_ADDR`, default `127.0.0.1:9900`) requires TLS unless `MUXCORE_INSECURE_DISABLE_TLS=true` (dev only).

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.
