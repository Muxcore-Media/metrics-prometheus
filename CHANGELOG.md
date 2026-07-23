# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0.html).

## [Unreleased]

## [0.1.0] - 2026-07-21

### Added

- Prometheus metrics sidecar: gRPC `MetricsService` (register/increment counters, gauges, histograms) and HTTP `/metrics` scrape endpoint
- Env config: `METRICS_GRPC_ADDR` (default `:9900`), `METRICS_HTTP_ADDR` (default `:9901`)
- Capability: `metrics`
