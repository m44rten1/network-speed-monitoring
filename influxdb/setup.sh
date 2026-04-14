#!/bin/bash
# This script runs as part of InfluxDB's init process.
# It creates the long-term bucket and a downsampling task.

set -e

echo "Creating long-term storage bucket..."
influx bucket create \
  --host http://localhost:8086 \
  --token network-monitor-token \
  --org network-monitor \
  --name network_monitor_longterm \
  --retention 365d \
  2>/dev/null || echo "Bucket network_monitor_longterm already exists"

echo "Creating downsampling task..."
influx task create \
  --host http://localhost:8086 \
  --token network-monitor-token \
  --org network-monitor \
  --flux '
option task = {name: "downsample_ping", every: 5m}

from(bucket: "network_monitor")
  |> range(start: -10m)
  |> filter(fn: (r) => r._measurement == "ping")
  |> filter(fn: (r) => r._field == "latency_ms" or r._field == "success" or r._field == "timeout")
  |> aggregateWindow(every: 5m, fn: mean, createEmpty: false)
  |> to(bucket: "network_monitor_longterm", org: "network-monitor")
' 2>/dev/null || echo "Downsampling task may already exist"

echo "InfluxDB setup complete!"
