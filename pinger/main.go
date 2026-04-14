package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	influxdb2 "github.com/influxdata/influxdb-client-go/v2"
	"github.com/influxdata/influxdb-client-go/v2/api"
	probing "github.com/prometheus-community/pro-bing"
)

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getEnvInt(key string, fallback int) int {
	if v := os.Getenv(key); v != "" {
		if i, err := strconv.Atoi(v); err == nil {
			return i
		}
	}
	return fallback
}

func main() {
	// Configuration via environment variables
	target := getEnv("PING_TARGET", "1.1.1.1")
	intervalSec := getEnvInt("PING_INTERVAL_SECONDS", 5)
	timeoutSec := getEnvInt("PING_TIMEOUT_SECONDS", 4)

	influxURL := getEnv("INFLUXDB_URL", "http://influxdb:8086")
	influxToken := getEnv("INFLUXDB_TOKEN", "network-monitor-token")
	influxOrg := getEnv("INFLUXDB_ORG", "network-monitor")
	influxBucket := getEnv("INFLUXDB_BUCKET", "network_monitor")

	log.Printf("Starting network pinger")
	log.Printf("  Target: %s", target)
	log.Printf("  Interval: %ds", intervalSec)
	log.Printf("  Timeout: %ds", timeoutSec)
	log.Printf("  InfluxDB: %s (org=%s, bucket=%s)", influxURL, influxOrg, influxBucket)

	// Create InfluxDB client
	client := influxdb2.NewClientWithOptions(influxURL, influxToken,
		influxdb2.DefaultOptions().
			SetBatchSize(20).
			SetFlushInterval(uint(intervalSec*1000*2)), // flush every 2 intervals
	)
	defer client.Close()

	writeAPI := client.WriteAPI(influxOrg, influxBucket)

	// Handle write errors in background
	go func() {
		for err := range writeAPI.Errors() {
			log.Printf("InfluxDB write error: %v", err)
		}
	}()

	// Wait for InfluxDB to be ready
	waitForInfluxDB(client, influxURL)

	// Set up graceful shutdown
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)

	ticker := time.NewTicker(time.Duration(intervalSec) * time.Second)
	defer ticker.Stop()

	log.Printf("Pinger running. Sending pings every %ds...", intervalSec)

	// Do an initial ping immediately
	doPing(target, time.Duration(timeoutSec)*time.Second, writeAPI)

	for {
		select {
		case <-ticker.C:
			doPing(target, time.Duration(timeoutSec)*time.Second, writeAPI)
		case <-sigCh:
			log.Println("Shutdown signal received, flushing data...")
			writeAPI.Flush()
			cancel()
			return
		case <-ctx.Done():
			return
		}
	}
}

func waitForInfluxDB(client influxdb2.Client, url string) {
	log.Printf("Waiting for InfluxDB at %s...", url)
	for i := 0; i < 60; i++ {
		ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
		ok, err := client.Ready(ctx)
		cancel()
		if err == nil && ok != nil {
			log.Println("InfluxDB is ready")
			return
		}
		if err != nil {
			log.Printf("InfluxDB not ready yet (attempt %d/60): %v", i+1, err)
		}
		time.Sleep(2 * time.Second)
	}
	log.Fatal("Could not connect to InfluxDB after 120 seconds")
}

func doPing(target string, timeout time.Duration, writeAPI api.WriteAPI) {
	pinger, err := probing.NewPinger(target)
	if err != nil {
		log.Printf("Error creating pinger: %v", err)
		writeErrorPoint(target, writeAPI, "create_error")
		return
	}

	pinger.Count = 1
	pinger.Timeout = timeout
	pinger.SetPrivileged(true) // Use raw ICMP sockets (requires NET_RAW capability)

	err = pinger.Run()
	if err != nil {
		log.Printf("Ping error: %v", err)
		writeErrorPoint(target, writeAPI, "run_error")
		return
	}

	stats := pinger.Statistics()

	now := time.Now()

	if stats.PacketsRecv > 0 {
		// Successful ping
		latencyMs := float64(stats.AvgRtt) / float64(time.Millisecond)
		log.Printf("Ping %s: %.2fms", target, latencyMs)

		p := influxdb2.NewPoint(
			"ping",
			map[string]string{"target": target},
			map[string]interface{}{
				"latency_ms": latencyMs,
				"success":    true,
				"timeout":    false,
			},
			now,
		)
		writeAPI.WritePoint(p)
	} else {
		// Timeout / packet loss
		log.Printf("Ping %s: timeout (packet loss: %.0f%%)", target, stats.PacketLoss)

		p := influxdb2.NewPoint(
			"ping",
			map[string]string{"target": target},
			map[string]interface{}{
				"latency_ms": float64(0),
				"success":    false,
				"timeout":    true,
			},
			now,
		)
		writeAPI.WritePoint(p)
	}
}

func writeErrorPoint(target string, writeAPI api.WriteAPI, errorType string) {
	p := influxdb2.NewPoint(
		"ping",
		map[string]string{"target": target},
		map[string]interface{}{
			"latency_ms": float64(0),
			"success":    false,
			"timeout":    false,
			"error":      errorType,
		},
		time.Now(),
	)
	writeAPI.WritePoint(p)

	// Log but don't crash - we want to keep running even if the network is completely down
	fmt.Printf("Recorded error point: %s for target %s\n", errorType, target)
}
