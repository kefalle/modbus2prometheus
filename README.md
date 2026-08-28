## modbus2prometheus

Simple prometheus exporter and controller for modbus RTU TCP protocol

### Architecture

```mermaid
flowchart LR
    Config["YAML config<br/>device, tags, Telegram"]
    Flags["CLI flags"]
    Main["main<br/>composition and lifecycle"]

    Config --> Main
    Flags --> Main

    subgraph Runtime["modbus2prometheus process"]
        Main --> Controller["Controller<br/>polling, cache, writes"]
        Main --> HTTP["HTTP server"]
        Main --> Telegram["Telegram bot"]
        Controller --> Metrics["VictoriaMetrics registry"]
        HTTP --> Controller
        HTTP --> Metrics
        Telegram --> Controller
    end

    Modbus["Modbus RTU over TCP device"]
    NodeRED["Node-RED<br/>/current_th"]
    Prometheus["Prometheus / vmagent"]
    Owner["Authorized Telegram owner"]
    APIClient["HTTP API client"]

    Controller <-->|"read/write holding registers"| Modbus
    Telegram -->|"GET sensor data"| NodeRED
    Prometheus -->|"GET /metrics"| HTTP
    APIClient -->|"GET /tags"| HTTP
    APIClient -->|"POST /api/v1/write"| HTTP
    Owner <-->|"state, sensors, setpoints"| Telegram

    classDef control fill:#ffe0e0,stroke:#b42318,color:#101828
    classDef observe fill:#e0f2fe,stroke:#026aa2,color:#101828
    class Controller,Telegram,APIClient,Owner,Modbus control
    class Metrics,Prometheus,NodeRED observe
```

Mermaid source: [docs/architecture.mmd](docs/architecture.mmd).

Detailed project map and refactoring documents:

- [Project guide](docs/PROJECT_GUIDE.md)
- [Safe controller refactoring plan](docs/REFACTORING_PLAN.md)

### Configuring

Modbus device url configuring with [doc for modbus library](https://github.com/simonvetter/modbus/blob/master/README.md)

Simple configuration for RTU via TCP modbus:
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
```

### Build

```bash
$ go build
```

### Install as service

You can copy files from ./etc/systemd/ folder to /etc/systemd/system and enable service

```bash
$ sudo cp ./etc/systemd/modbus2prometheus.config.yaml
$ sudo systemctl enable modbus2prometheus --now
```

### Scraping metrics

Metrics exporting to /metrics endpoint. You can scrape metrics with prometheus or vmagent service. Configuration for vmagent in /etc folder
