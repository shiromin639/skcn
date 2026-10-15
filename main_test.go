package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/prometheus/client_golang/prometheus"
)

func metricValue(t *testing.T, registry *prometheus.Registry, name, lcore string) float64 {
	t.Helper()
	families, err := registry.Gather()
	if err != nil {
		t.Fatal(err)
	}
	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if lcore != "" {
				matched := false
				for _, label := range metric.GetLabel() {
					if label.GetName() == "lcore" && label.GetValue() == lcore {
						matched = true
					}
				}
				if !matched {
					continue
				}
			}
			if metric.Counter != nil {
				return metric.Counter.GetValue()
			}
			if metric.Gauge != nil {
				return metric.Gauge.GetValue()
			}
		}
	}
	t.Fatalf("metric %s with lcore %q missing", name, lcore)
	return 0
}

func TestNormalMetricsAdvance(t *testing.T) {
	registry := prometheus.NewRegistry()
	sim, err := newSimulation("normal", registry)
	if err != nil {
		t.Fatal(err)
	}
	rx := metricValue(t, registry, "upf_packets_received_total", "")
	tx := metricValue(t, registry, "upf_packets_transmitted_total", "")
	drops := metricValue(t, registry, "upf_packets_dropped_total", "")
	if rx != tx+drops {
		t.Fatalf("received %v != transmitted %v + dropped %v", rx, tx, drops)
	}
	sim.advance()
	if got := metricValue(t, registry, "upf_packets_received_total", ""); got <= rx {
		t.Fatalf("received counter did not increase: %v", got)
	}
	if got := metricValue(t, registry, "upf_active_sessions", ""); got <= 0 {
		t.Fatalf("sessions = %v", got)
	}
	for _, id := range []string{"0", "1", "2", "3"} {
		got := metricValue(t, registry, "upf_lcore_utilization_ratio", id)
		if got <= 0 || got > 1 {
			t.Fatalf("lcore %s utilization = %v", id, got)
		}
	}
}

func TestModeSwitchChangesMetrics(t *testing.T) {
	registry := prometheus.NewRegistry()
	sim, err := newSimulation("normal", registry)
	if err != nil {
		t.Fatal(err)
	}
	handler := sim.handler(registry)
	beforeRx := metricValue(t, registry, "upf_packets_received_total", "")
	beforeDrops := metricValue(t, registry, "upf_packets_dropped_total", "")
	beforeLcore := metricValue(t, registry, "upf_lcore_utilization_ratio", "0")

	bad := httptest.NewRecorder()
	handler.ServeHTTP(bad, httptest.NewRequest(http.MethodPost, "/mode?mode=broken", nil))
	if bad.Code != http.StatusBadRequest {
		t.Fatalf("invalid mode status = %d", bad.Code)
	}

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodPost, "/mode?mode=degraded", nil))
	if response.Code != http.StatusOK {
		t.Fatalf("mode switch status = %d", response.Code)
	}
	sim.advance()
	rxDelta := metricValue(t, registry, "upf_packets_received_total", "") - beforeRx
	dropDelta := metricValue(t, registry, "upf_packets_dropped_total", "") - beforeDrops
	if rxDelta >= 1200 {
		t.Fatalf("degraded throughput too high: %v", rxDelta)
	}
	if dropDelta <= 50 {
		t.Fatalf("degraded drop count too low: %v", dropDelta)
	}
	if got := metricValue(t, registry, "upf_lcore_utilization_ratio", "0"); got <= beforeLcore {
		t.Fatalf("lcore utilization did not rise: %v", got)
	}
}

func TestInvalidInitialMode(t *testing.T) {
	if _, err := newSimulation("broken", prometheus.NewRegistry()); err == nil {
		t.Fatal("expected invalid mode error")
	}
}
