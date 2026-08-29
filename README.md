# modbus2prometheus

Prometheus exporter and controller for Modbus RTU over TCP. The service polls
holding registers, publishes their latest values through HTTP and Prometheus,
and writes configured setpoints through HTTP or a Telegram bot whose message
handlers use an owner allowlist.

## Requirements

- Go 1.25 for local builds.
- Access to a Modbus device supported by `github.com/simonvetter/modbus`.
- A Telegram bot token for the current startup path; the bot is not optional
  yet.
- Docker with Compose for the container deployment, or systemd for the native
  deployment.

## Architecture

```mermaid
flowchart LR
    Config["YAML config<br/>device, tags, Telegram"]
    Flags["CLI flags"]
    Main["main<br/>composition and lifecycle"]

    Config --> Main
    Flags --> Main

    subgraph Compose["Docker Compose deployment"]
        Proxy["nginx reverse proxy<br/>port 80"]

        subgraph Runtime["modbus2prometheus process"]
            Main --> Controller["Controller<br/>polling, cache, writes"]
            Main --> HTTP["HTTP server<br/>port 9101"]
            Main --> Telegram["Telegram bot"]
            Controller --> Metrics["VictoriaMetrics registry"]
            HTTP --> Controller
            HTTP --> Metrics
            Telegram --> Controller
        end

        Proxy -->|"/modbus2prometheus/"| HTTP
    end

    Modbus["Modbus RTU over TCP device"]
    NodeRED["Node-RED<br/>/current_th"]
    HostApps["Host services<br/>Grafana, VictoriaMetrics,<br/>Node-RED, Zigbee2MQTT"]
    Prometheus["Prometheus / vmagent"]
    Owner["Authorized Telegram owner"]
    APIClient["HTTP API client"]

    Controller <-->|"read/write holding registers"| Modbus
    Telegram -->|"GET sensor data"| NodeRED
    Prometheus -->|"GET /metrics"| HTTP
    APIClient -->|"HTTP :80"| Proxy
    Proxy -->|"reverse-proxy routes"| HostApps
    Owner <-->|"state, sensors, setpoints"| Telegram

    classDef control fill:#ffe0e0,stroke:#b42318,color:#101828
    classDef observe fill:#e0f2fe,stroke:#026aa2,color:#101828
    class Controller,Telegram,Proxy,APIClient,Owner,Modbus control
    class Metrics,Prometheus,NodeRED,HostApps observe
```

Mermaid source: [docs/architecture.mmd](docs/architecture.mmd).

Detailed project documentation:

- [Project guide](docs/PROJECT_GUIDE.md)
- [Safe controller refactoring plan](docs/REFACTORING_PLAN.md)

## Configuration

The default configuration path is `./config.yaml`; pass another path with
`-config`. Modbus URL formats are documented by the
[Modbus library](https://github.com/simonvetter/modbus/blob/master/README.md).

Minimal RTU-over-TCP configuration:

```yaml
device-url: "rtuovertcp://192.168.1.200:8899"
device-id: 16
speed: 19200
timeout: 1s
polling-time: 1s
read-period: 10ms
tags:
  - name: "temp_floor"
    address: 513
    operation: "read_float"
  - name: "servo_otopl"
    address: 522
    operation: "read_uint"
telegram:
  apiToken: "<telegram-bot-token>"
  owners:
    123456789: "owner"
```

See [etc/modbus2prometheus.config.yaml](etc/modbus2prometheus.config.yaml) for
all tags used by the supplied deployment. Set `telegram.nodeRedUrl` to a base
URL reachable from the service when the `/sens_th` command is needed.

## HTTP endpoints

| Endpoint | Purpose |
| --- | --- |
| `GET /tags` | Latest tag values as JSON. |
| `GET /metrics` | Prometheus metrics. |
| `POST /api/v1/write` | Write a tag configured with `write_uint` or `write_float`. |

Under Docker Compose, prefix these paths with `/modbus2prometheus` on nginx
port 80. The application listens on port 9101 inside the Compose network.

The write endpoint currently has no authentication. Do not expose it outside a
trusted network; the supplied nginx configuration forwards it under
`/modbus2prometheus/api/v1/write`.

## Development

Build, test, and lint with the checked-in Makefile:

```bash
make build
make test
make lint
```

The binary is written to `build/modbus2prometheus`.

## Docker Compose

Compose expects the runtime configuration at
`/etc/modbus2prometheus.config.yaml` on the host:

```bash
docker compose -f docker/docker-compose.yml up --build -d
```

The nginx proxy listens on port 80. Its configuration also forwards routes to
Grafana, VictoriaMetrics, Node-RED, and Zigbee2MQTT running on the Docker host.

## Install as a systemd service

Build the binary first, then install the files at the paths used by the unit:

```bash
sudo install -Dm755 build/modbus2prometheus /opt/modbus2prometheus/modbus2prometheus
sudo install -Dm644 etc/modbus2prometheus.config.yaml /etc/modbus2prometheus.config.yaml
sudo install -Dm644 etc/systemd/system/modbus2prometheus.service /etc/systemd/system/modbus2prometheus.service
sudo systemctl daemon-reload
sudo systemctl enable --now modbus2prometheus.service
```

## Scraping metrics

Metrics are exposed at `/metrics`. The supplied vmagent example is
[etc/vmagent.scrape.config.yaml](etc/vmagent.scrape.config.yaml).
