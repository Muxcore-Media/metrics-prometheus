# Changelog

All notable changes to this project are documented in this file.

The format is based on [Keep a Changelog](https://keepachangelog.com/en/1.1.0/),
and this project adheres to [Semantic Versioning](https://semver.org/spec/v2.0.0/).

## [0.1.3] - 2026-10-05


### Changed
- Reported version comes from muxcore.json (ADR-0021); built on core v0.6.12 / sdk/go/module v0.6.3 (mesh enrollment, ADR-0017).

## [0.1.2] - 2026-10-05

### Changed
- CI runs on GitHub-hosted runners from the umbrella template; retired-origin workflows removed.
- Dependencies resolve from published GitHub tags (no filesystem `replace`); requires core v0.6.0.

### Changed

- Inbound gRPC listens with TLS by default; auto-generates certs under `METRICS_TLS_DIR` when unset.
- Default gRPC bind is loopback `127.0.0.1:9900` (was `:9900` on all interfaces).
- Dev escape hatch: `MUXCORE_INSECURE_DISABLE_TLS=true` or `MUXCORE_GRPC_INSECURE=true`.

## [0.1.1] — 2026-08-10

### Added

- SettingsProvider for `metrics_path` (`METRICS_PATH`) with live scrape-path updates
- Advertises `settings` capability for admin-ui discovery

## [0.1.0] — 2026-08-09

### Added

- Prometheus metrics sidecar (`metrics` / `metrics.prometheus`): gRPC register counters/gauges/histograms; HTTP `/metrics` scrape.
- Env: `METRICS_GRPC_ADDR` (`:9900`), `METRICS_HTTP_ADDR` (`:9901`).
