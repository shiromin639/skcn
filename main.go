package main

import (
	"context"
	"encoding/json"
	"errors"
	"flag"
	"log"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

type simulation struct {
	mu          sync.Mutex
	mode        string
	tick        uint64
	received    prometheus.Counter
	transmitted prometheus.Counter
	dropped     prometheus.Counter
	sessions    prometheus.Gauge
	lcore       *prometheus.GaugeVec
}

func newSimulation(mode string, registry *prometheus.Registry) (*simulation, error) {
	if !validMode(mode) {
		return nil, errors.New("SIMULATION_MODE must be normal or degraded")
	}
	s := &simulation{
		mode:        mode,
		received:    prometheus.NewCounter(prometheus.CounterOpts{Name: "upf_packets_received_total", Help: "Simulated packets received by the UPF."}),
		transmitted: prometheus.NewCounter(prometheus.CounterOpts{Name: "upf_packets_transmitted_total", Help: "Simulated packets transmitted by the UPF."}),
		dropped:     prometheus.NewCounter(prometheus.CounterOpts{Name: "upf_packets_dropped_total", Help: "Simulated packets dropped by the UPF."}),
		sessions:    prometheus.NewGauge(prometheus.GaugeOpts{Name: "upf_active_sessions", Help: "Simulated active UPF sessions."}),
		lcore:       prometheus.NewGaugeVec(prometheus.GaugeOpts{Name: "upf_lcore_utilization_ratio", Help: "Simulated lcore utilization as a ratio from 0 to 1."}, []string{"lcore"}),
	}
	registry.MustRegister(s.received, s.transmitted, s.dropped, s.sessions, s.lcore)
	for i := 0; i < 4; i++ {
		s.lcore.WithLabelValues(string(rune('0' + i))).Set(0)
	}
	s.advance()
	return s, nil
}

func validMode(mode string) bool { return mode == "normal" || mode == "degraded" }

// advance applies one deterministic second of simulated traffic.
func (s *simulation) advance() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.tick++
	wave := int(s.tick % 10)
	var rx, drops, sessions int
	var base float64
	if s.mode == "degraded" {
		rx, drops, sessions, base = 520+wave*8, 75+wave*2, 950+wave*3, 0.78
	} else {
		rx, drops, sessions, base = 1200+wave*15, 2+wave%3, 1000+wave*5, 0.32
	}
	s.received.Add(float64(rx))
	s.dropped.Add(float64(drops))
	s.transmitted.Add(float64(rx - drops))
	s.sessions.Set(float64(sessions))
	for i := 0; i < 4; i++ {
		s.lcore.WithLabelValues(string(rune('0' + i))).Set(base + float64(i)*0.035 + float64(wave)*0.005)
	}
}

func (s *simulation) handler(registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok\n"))
	})
	mux.HandleFunc("/mode", func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodGet && r.Method != http.MethodPost {
			http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
			return
		}
		s.mu.Lock()
		if r.Method == http.MethodPost {
			mode := r.URL.Query().Get("mode")
			if !validMode(mode) {
				s.mu.Unlock()
				http.Error(w, "mode must be normal or degraded", http.StatusBadRequest)
				return
			}
			s.mode = mode
		}
		mode := s.mode
		s.mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"mode": mode})
	})
	return mux
}

func main() {
	addr := flag.String("listen", ":8080", "HTTP listen address")
	flag.Parse()
	mode := strings.ToLower(strings.TrimSpace(os.Getenv("SIMULATION_MODE")))
	if mode == "" {
		mode = "normal"
	}
	registry := prometheus.NewRegistry()
	sim, err := newSimulation(mode, registry)
	if err != nil {
		log.Fatal(err)
	}

	server := &http.Server{Addr: *addr, Handler: sim.handler(registry), ReadHeaderTimeout: 5 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		for {
			select {
			case <-ticker.C:
				sim.advance()
			case <-ctx.Done():
				return
			}
		}
	}()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			log.Printf("shutdown: %v", err)
		}
	}()
	log.Printf("mock UPF listening on %s in %s mode", *addr, mode)
	if err := server.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
