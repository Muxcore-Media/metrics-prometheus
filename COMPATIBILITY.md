# Compatibility

## Core Version

| Module Version | Core Version | Status |
|----------------|-------------|--------|
| v0.1.2         | v0.5.8+     | Current |

## Capabilities

| Capability | Status |
|------------|--------|
| `metrics` | Current |
| `metrics.prometheus` | Current |
| `settings` | Current (`metrics_path` live scrape path) |

Scrape endpoint: HTTP `GET /metrics` on `METRICS_HTTP_ADDR` (default `127.0.0.1:9901`). Health: `GET /health` on the same listener.

## Breaking Changes

This is a pre-1.0 module. Interfaces may change without notice.

v0.1.2: metric families use Prometheus `*Vec` (label dimensions at register, values at increment/set/observe); loopback bind defaults; optional `METRICS_SCRAPE_TOKEN`.
