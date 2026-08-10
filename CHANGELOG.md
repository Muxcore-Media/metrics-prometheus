# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [Unreleased]

## [0.1.0] — 2026-08-09

### Added

- Prometheus metrics sidecar (`metrics` / `metrics.prometheus`): gRPC register counters/gauges/histograms; HTTP `/metrics` scrape.
- Env: `METRICS_GRPC_ADDR` (`:9900`), `METRICS_HTTP_ADDR` (`:9901`).
