# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [0.1.1] — 2026-08-10

### Added

- SettingsProvider for `metrics_path` (`METRICS_PATH`) with live scrape-path updates
- Advertises `settings` capability for admin-ui discovery

## [0.1.0] — 2026-08-09

### Added

- Prometheus metrics sidecar (`metrics` / `metrics.prometheus`): gRPC register counters/gauges/histograms; HTTP `/metrics` scrape.
- Env: `METRICS_GRPC_ADDR` (`:9900`), `METRICS_HTTP_ADDR` (`:9901`).
