# Mock UPF Observability

A small, self-contained 5G UPF observability demo. A Go process generates repeatable traffic metrics; Prometheus scrapes them every five seconds; Grafana opens with a provisioned dashboard.

The technical report, presentation slides, and cover PDFs are in [`docs/`](docs/README.md).

## Start

Prerequisites: Docker with the Compose plugin. From this directory:

```sh
docker compose up --build
```

Open the services:

| Service | URL | Notes |
| --- | --- | --- |
| Grafana dashboard | http://localhost:3000/d/mock-upf-demo/mock-upf-overview | Available without signing in; admin login is `admin` / `admin` for editing. |
| Prometheus targets | http://localhost:9090/targets | The `mock-upf` target should be UP. |
| Exporter metrics | http://localhost:8080/metrics | Prometheus text format. |
| Exporter health | http://localhost:8080/healthz | Returns `ok`. |

Wait about 20 seconds after startup for rate panels to have enough samples. Grafana reloads the dashboard automatically from the checked-in JSON file. To stop, press Ctrl+C. Use `docker compose down` to remove the containers; Grafana's named volume retains its data.

## Demo scenario

Start in normal mode. Watch the throughput, drops, and lcore panels, then run:

```sh
curl -X POST 'http://localhost:8080/mode?mode=degraded'
```

Within roughly 20 seconds, receive/transmit throughput falls, dropped packets rise, and all four lcores become busier. Switch back with:

```sh
curl -X POST 'http://localhost:8080/mode?mode=normal'
```

`GET /mode` returns the current mode as JSON. An invalid mode returns HTTP 400. To start directly in degraded mode, change `SIMULATION_MODE` in `compose.yaml` to `degraded` (or run the exporter with that environment variable). The demo is deterministic: each second advances a ten-step repeating pattern; there is no random input or real UPF traffic.

## Metrics

| Metric | Type | Meaning |
| --- | --- | --- |
| `upf_packets_received_total` | Counter | Packets received since exporter start. |
| `upf_packets_transmitted_total` | Counter | Packets transmitted since exporter start. |
| `upf_packets_dropped_total` | Counter | Packets dropped since exporter start. |
| `upf_active_sessions` | Gauge | Current simulated session count. |
| `upf_lcore_utilization_ratio{lcore="0".."3"}` | Gauge | Per-lcore utilization, from 0 to 1. |

The dashboard uses Prometheus `rate()` for packet counters, shown in packets per second. Lcore ratios are displayed as percentages. The simulator updates once per second; Prometheus scrapes every five seconds.

## Architecture and development

`mock-upf:8080` exposes metrics and mode controls. `prometheus:9090` scrapes it over the Compose network. Grafana queries `prometheus:9090` through its provisioned datasource and loads the dashboard from `grafana/dashboards/mock-upf.json`.

To run the basic unit tests locally with Go 1.23 or newer:

```sh
go test ./...
```
