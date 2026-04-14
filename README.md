# Network Latency Monitor

A lightweight, self-hosted tool that continuously monitors your network latency by pinging a target (default: Cloudflare's `1.1.1.1`) and visualizes the results in Grafana. Designed to run 24/7 in the background on your local machine.

Built to answer questions like:

- Why do my video calls freeze at certain times of day?
- Is my WiFi signal degraded from a specific room?
- How reliable is my ISP over weeks and months?
- Did moving my router actually improve things?

## Architecture

```
┌────────────────────────────────────────────────────┐
│                  Docker Compose                    │
│                                                    │
│  ┌──────────┐     ┌───────────┐     ┌───────────┐  │
│  │  Pinger  │────>│  InfluxDB │<────│  Grafana  │  │
│  │  (Go)    │     │  (v2.7)   │     │ (v11.1)   │  │
│  └──────────┘     └───────────┘     └───────────┘  │
│       │                                   │        │
│       │ ICMP ping every 5s                │ :3000  │
│       v                                   v        │
│    1.1.1.1                         localhost:3000  │
└────────────────────────────────────────────────────┘
```

| Component       | Role                                                                                                               |
| --------------- | ------------------------------------------------------------------------------------------------------------------ |
| **Pinger**      | Go binary that sends an ICMP ping every 5 seconds and writes the result (latency, success/timeout) to InfluxDB     |
| **InfluxDB v2** | Time-series database. Stores raw ping data (30-day retention) and downsampled 5-minute averages (1-year retention) |
| **Grafana**     | Pre-configured dashboard with latency graphs, availability stats, packet loss charts, and latency distribution     |

## Quick Start

### Prerequisites

- [Docker](https://docs.docker.com/get-docker/) and Docker Compose (included with Docker Desktop)
- That's it. No Go toolchain needed -- the pinger is built inside Docker.

### Run

```bash
git clone https://github.com/m44rten1/network-speed-monitoring.git
cd network-speed-monitoring
docker compose up -d
```

Open Grafana at **http://localhost:3000** and navigate to the **Network Monitor** dashboard. Data appears within seconds.

> **Port conflict?** If port 3000 is already in use, change the Grafana port mapping in `docker-compose.yml`:
>
> ```yaml
> ports:
>   - "3005:3000" # change 3005 to any free port
> ```

### Default Credentials

| Service  | URL                   | Username | Password      |
| -------- | --------------------- | -------- | ------------- |
| Grafana  | http://localhost:3000 | admin    | admin         |
| InfluxDB | http://localhost:8086 | admin    | adminpassword |

Anonymous read access is enabled for Grafana, so you can view dashboards without logging in.

## Dashboard

The pre-provisioned Grafana dashboard includes:

| Panel                           | Description                                                                                         |
| ------------------------------- | --------------------------------------------------------------------------------------------------- |
| **Availability**                | Percentage of successful pings in the selected time range. Green (>99%), orange (>95%), red (below) |
| **Average Latency**             | Mean round-trip time in milliseconds                                                                |
| **P95 Latency**                 | 95th percentile latency -- shows worst-case performance excluding outliers                          |
| **Total Timeouts**              | Count of pings that received no response                                                            |
| **Ping Latency**                | Time-series line chart of latency over time                                                         |
| **P95 Latency (5-min windows)** | P95 latency aggregated in 5-minute buckets, with color thresholds                                   |
| **Packet Loss Over Time**       | Bar chart showing the percentage of failed pings over time                                          |
| **Latency Distribution**        | Histogram showing how latency values are distributed                                                |
| **Hourly Availability**         | Per-hour availability percentage as a bar chart                                                     |

Use Grafana's time picker (top right) to switch between views: last hour, last 24 hours, last 7 days, last 30 days, or custom ranges up to 1 year.

## Configuration

All configuration is done through environment variables in `docker-compose.yml`.

### Pinger Settings

| Variable                | Default   | Description                              |
| ----------------------- | --------- | ---------------------------------------- |
| `PING_TARGET`           | `1.1.1.1` | IP or hostname to ping                   |
| `PING_INTERVAL_SECONDS` | `5`       | Seconds between pings                    |
| `PING_TIMEOUT_SECONDS`  | `4`       | Timeout before a ping is considered lost |

### Examples

**Ping your router instead** (useful for isolating WiFi vs ISP issues):

```yaml
environment:
  - PING_TARGET=192.168.1.1
```

**Faster pings** (1 second interval for higher resolution during debugging):

```yaml
environment:
  - PING_INTERVAL_SECONDS=1
  - PING_TIMEOUT_SECONDS=1
```

## Data Retention

| Bucket                     | Retention | Resolution          | Purpose                  |
| -------------------------- | --------- | ------------------- | ------------------------ |
| `network_monitor`          | 30 days   | Per-ping (every 5s) | Detailed troubleshooting |
| `network_monitor_longterm` | 365 days  | 5-minute averages   | Monthly/yearly trends    |

The downsampling is handled by a built-in InfluxDB task that runs every 5 minutes. At 5-second intervals, raw data uses roughly 5-10 MB/day. The long-term bucket is much smaller since it only stores aggregated values.

## Project Structure

```
network-speed-monitoring/
├── docker-compose.yml                              # All 3 services
├── pinger/
│   ├── Dockerfile                                  # Multi-stage Go build
│   ├── go.mod
│   ├── go.sum
│   └── main.go                                     # Pinger source code
├── grafana/
│   └── provisioning/
│       ├── datasources/
│       │   └── influxdb.yml                        # InfluxDB connection config
│       └── dashboards/
│           ├── dashboard.yml                       # Dashboard provider config
│           └── network-monitor.json                # Dashboard definition
└── influxdb/
    └── setup.sh                                    # Creates long-term bucket + downsampling task
```

## Operations

### Start / Stop

```bash
# Start everything (runs in background)
docker compose up -d

# Stop everything (data is preserved in Docker volumes)
docker compose down

# Stop and delete all data (fresh start)
docker compose down -v
```

### View Logs

```bash
# Follow pinger output
docker compose logs -f pinger

# Check all services
docker compose logs -f
```

### Rebuild After Code Changes

```bash
docker compose build pinger
docker compose up -d pinger
```

### Auto-Start on Boot

All containers use `restart: unless-stopped`, so they will automatically restart when Docker Desktop starts. To make this fully automatic:

1. Set Docker Desktop to **Start Docker Desktop when you sign in** (Docker Desktop > Settings > General)
2. The containers will resume on their own

### Backup Data

InfluxDB data is stored in Docker volumes. To back up:

```bash
# Export raw data
docker exec network-monitor-influxdb influx query \
  --host http://localhost:8086 \
  --token network-monitor-token \
  --org network-monitor \
  'from(bucket:"network_monitor") |> range(start: -30d)' --raw > backup.csv
```

Or back up the Docker volume directly:

```bash
docker run --rm -v network-speed-monitoring_influxdb-data:/data -v $(pwd):/backup alpine \
  tar czf /backup/influxdb-backup.tar.gz /data
```

## How It Works

The pinger is a small Go program (~190 lines) that:

1. Waits for InfluxDB to be ready (retries for up to 2 minutes)
2. Sends a single ICMP ping to the target every 5 seconds
3. Records the result as an InfluxDB point:
   - `latency_ms` -- round-trip time (0 if failed)
   - `success` -- whether the ping got a response
   - `timeout` -- whether the ping timed out
4. Handles network-down scenarios gracefully (logs errors, keeps retrying)
5. Flushes buffered data on shutdown (SIGINT/SIGTERM)

The container runs with the `NET_RAW` capability to send raw ICMP packets. This is the only special permission required.

## Tech Stack

| Tool         | Version | Why                                                                                |
| ------------ | ------- | ---------------------------------------------------------------------------------- |
| **Go**       | 1.22    | Fast, compiles to a single static binary, tiny Docker image (~15 MB)               |
| **InfluxDB** | 2.7     | Purpose-built for time-series data, built-in downsampling tasks, efficient storage |
| **Grafana**  | 11.1    | Industry standard for dashboards, native InfluxDB/Flux support, free               |
| **pro-bing** | 0.4.1   | Well-maintained ICMP library for Go, supports privileged and unprivileged pings    |

## Troubleshooting

**Pinger shows "InfluxDB not ready yet"**
Normal during first startup. InfluxDB takes 10-30 seconds to initialize. The pinger retries for up to 2 minutes.

**No data in Grafana**

1. Check the pinger is running: `docker compose logs pinger`
2. Verify the datasource: Grafana > Connections > Data sources > InfluxDB > "Test"
3. Make sure the time range in the dashboard covers a period with data (default is "Last 1 hour")

**Port conflict on 3000**
Change the port mapping in `docker-compose.yml` (see [Quick Start](#quick-start)).

**Permission denied / ping errors on Linux**
The pinger uses raw ICMP sockets which require `NET_RAW`. This is granted via `cap_add` in `docker-compose.yml`. If you still have issues, check that your Docker daemon supports capabilities.

## License

MIT
