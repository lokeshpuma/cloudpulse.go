package server

import (
	"context"
	"crypto/subtle"
	"crypto/tls"
	"crypto/x509"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/prometheus/client_golang/prometheus/promhttp"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/collector"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/servicecheck"
)

// InfoPayload is returned by GET /agent/info.
type InfoPayload struct {
	AgentID            string    `json:"agent_id"`
	AgentName          string    `json:"agent_name"`
	Environment        string    `json:"environment"`
	Version            string    `json:"version"`
	Hostname           string    `json:"hostname"`
	OperatingSystem    string    `json:"operating_system"`
	Architecture       string    `json:"architecture"`
	StartTime          time.Time `json:"start_time"`
	CollectionInterval string    `json:"collection_interval"`
}

// HealthPayload is returned by GET /health.
type HealthPayload struct {
	Status         string                               `json:"status"`
	Healthy        bool                                 `json:"healthy"`
	LastCollection *time.Time                           `json:"last_collection,omitempty"`
	Collectors     map[string]collector.CollectorStatus `json:"collectors"`
	Services       []servicecheck.Result                `json:"services"`
}

// HTTPServer exposes /metrics, /health, and /agent/info.
type HTTPServer struct {
	cfg       config.Config
	collector *collector.Manager
	services  *servicecheck.Runner
	version   string
	startTime time.Time
	logger    *slog.Logger
	srv       *http.Server
}

func New(cfg config.Config, coll *collector.Manager, svc *servicecheck.Runner, version string, logger *slog.Logger) *HTTPServer {
	if logger == nil {
		logger = slog.Default()
	}
	s := &HTTPServer{
		cfg:       cfg,
		collector: coll,
		services:  svc,
		version:   version,
		startTime: time.Now().UTC(),
		logger:    logger,
	}

	mux := http.NewServeMux()
	metricsHandler := promhttp.HandlerFor(coll.Registry(), promhttp.HandlerOpts{
		EnableOpenMetrics: true,
	})
	mux.Handle(cfg.Server.MetricsPath, s.authMiddleware(metricsHandler))
	mux.HandleFunc(cfg.Server.HealthPath, s.handleHealth)
	mux.HandleFunc(cfg.Server.InfoPath, s.handleInfo)

	s.srv = &http.Server{
		Addr:              cfg.Server.ListenAddress,
		Handler:           mux,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	return s
}

// Handler returns the underlying HTTP handler (useful for httptest).
func (s *HTTPServer) Handler() http.Handler {
	return s.srv.Handler
}

// ListenAndServe starts the HTTP/HTTPS server and supports optional mTLS.
func (s *HTTPServer) ListenAndServe() error {
	if s.cfg.Auth.TLSCertFile != "" && s.cfg.Auth.TLSKeyFile != "" {
		tlsCfg := &tls.Config{MinVersion: tls.VersionTLS12}
		if s.cfg.Auth.TLSClientCA != "" {
			caPEM, err := os.ReadFile(s.cfg.Auth.TLSClientCA)
			if err != nil {
				return fmt.Errorf("read tls_client_ca: %w", err)
			}
			pool := x509.NewCertPool()
			if !pool.AppendCertsFromPEM(caPEM) {
				return fmt.Errorf("parse tls_client_ca: invalid PEM")
			}
			tlsCfg.ClientCAs = pool
			tlsCfg.ClientAuth = tls.RequireAndVerifyClientCert
		}
		s.srv.TLSConfig = tlsCfg
		s.logger.Info("agent listening with TLS", "addr", s.cfg.Server.ListenAddress)
		return s.srv.ListenAndServeTLS(s.cfg.Auth.TLSCertFile, s.cfg.Auth.TLSKeyFile)
	}

	s.logger.Info("agent listening", "addr", s.cfg.Server.ListenAddress)
	return s.srv.ListenAndServe()
}

// Shutdown gracefully drains active requests.
func (s *HTTPServer) Shutdown(ctx context.Context) error {
	return s.srv.Shutdown(ctx)
}

func (s *HTTPServer) authMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		expected := s.cfg.BearerToken()
		if expected != "" {
			authHeader := r.Header.Get("Authorization")
			token := strings.TrimPrefix(authHeader, "Bearer ")
			if authHeader == token || subtle.ConstantTimeCompare([]byte(token), []byte(expected)) != 1 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusUnauthorized)
				_ = json.NewEncoder(w).Encode(map[string]string{"error": "unauthorized"})
				return
			}
		}
		next.ServeHTTP(w, r)
	})
}

func (s *HTTPServer) handleHealth(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	snap := s.collector.Health()
	var last *time.Time
	if !snap.LastCollection.IsZero() {
		t := snap.LastCollection
		last = &t
	}

	status := "healthy"
	code := http.StatusOK
	if !snap.Healthy {
		status = "degraded"
		code = http.StatusServiceUnavailable
	}

	payload := HealthPayload{
		Status:         status,
		Healthy:        snap.Healthy,
		LastCollection: last,
		Collectors:     snap.Collectors,
		Services:       s.services.Results(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(payload)
}

func (s *HTTPServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		w.WriteHeader(http.StatusMethodNotAllowed)
		return
	}
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = s.cfg.Agent.Name
	}

	payload := InfoPayload{
		AgentID:            s.cfg.Agent.ID,
		AgentName:          s.cfg.Agent.Name,
		Environment:        s.cfg.Agent.Environment,
		Version:            s.version,
		Hostname:           hostname,
		OperatingSystem:    runtime.GOOS,
		Architecture:       runtime.GOARCH,
		StartTime:          s.startTime,
		CollectionInterval: s.cfg.Collection.Interval.Duration.String(),
	}
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	_ = json.NewEncoder(w).Encode(payload)
}
