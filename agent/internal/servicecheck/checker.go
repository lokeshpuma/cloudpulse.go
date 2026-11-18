package servicecheck

import (
	"context"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/url"
	"sync"
	"time"

	"github.com/prometheus/client_golang/prometheus"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
)

// Result represents the latest outcome of a single service check.
type Result struct {
	Name       string    `json:"name"`
	Type       string    `json:"type"`
	Target     string    `json:"target"`
	Up         bool      `json:"up"`
	StatusCode int       `json:"status_code,omitempty"`
	DurationMs float64   `json:"duration_ms"`
	Error      string    `json:"error,omitempty"`
	CheckedAt  time.Time `json:"checked_at"`
}

// Runner executes configured HTTP and TCP service availability checks.
type Runner struct {
	cfg      config.Config
	logger   *slog.Logger
	services []config.ServiceConfig

	serviceUp        *prometheus.GaugeVec
	responseDuration *prometheus.GaugeVec
	checkErrors      *prometheus.CounterVec
	httpStatusCode   *prometheus.GaugeVec
	tlsExpirySeconds *prometheus.GaugeVec

	mu      sync.RWMutex
	results map[string]Result
}

// NewRunner registers service check Prometheus metrics on reg.
func NewRunner(cfg config.Config, reg prometheus.Registerer, logger *slog.Logger) *Runner {
	if logger == nil {
		logger = slog.Default()
	}
	constLabels := prometheus.Labels{
		"server_id":   cfg.Agent.ID,
		"server_name": cfg.Agent.Name,
		"environment": cfg.Agent.Environment,
	}

	r := &Runner{
		cfg:      cfg,
		logger:   logger,
		services: cfg.Services,
		results:  make(map[string]Result, len(cfg.Services)),

		serviceUp: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_service_up",
			Help:        "1 if the monitored service check succeeded, 0 otherwise.",
			ConstLabels: constLabels,
		}, []string{"service", "type", "target"}),
		responseDuration: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_service_response_duration_seconds",
			Help:        "Duration of the most recent service check in seconds.",
			ConstLabels: constLabels,
		}, []string{"service", "type", "target"}),
		checkErrors: prometheus.NewCounterVec(prometheus.CounterOpts{
			Name:        "cloudpulse_service_check_errors_total",
			Help:        "Total failed service availability checks.",
			ConstLabels: constLabels,
		}, []string{"service", "type", "target"}),
		httpStatusCode: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_service_http_status_code",
			Help:        "Last HTTP status code received from an HTTP service check.",
			ConstLabels: constLabels,
		}, []string{"service", "target"}),
		tlsExpirySeconds: prometheus.NewGaugeVec(prometheus.GaugeOpts{
			Name:        "cloudpulse_service_tls_expiry_seconds",
			Help:        "Seconds remaining until the peer TLS certificate expires.",
			ConstLabels: constLabels,
		}, []string{"service", "target"}),
	}

	if reg != nil {
		reg.MustRegister(r.serviceUp, r.responseDuration, r.checkErrors, r.httpStatusCode, r.tlsExpirySeconds)
	}
	return r
}

// SanitizeTarget strips credentials and query parameters so secrets never leak into Prometheus labels.
func SanitizeTarget(svcType, raw string) string {
	if svcType != "http" {
		return raw
	}
	u, err := url.Parse(raw)
	if err != nil {
		return "invalid-target"
	}
	u.User = nil
	u.RawQuery = ""
	u.Fragment = ""
	return u.String()
}

// CheckOne executes a single service check immediately and records metrics.
func (r *Runner) CheckOne(ctx context.Context, svc config.ServiceConfig) Result {
	safeTarget := SanitizeTarget(svc.Type, svc.Target)
	start := time.Now()
	cctx, cancel := context.WithTimeout(ctx, svc.Timeout.Duration)
	defer cancel()

	res := Result{
		Name:      svc.Name,
		Type:      svc.Type,
		Target:    safeTarget,
		CheckedAt: time.Now().UTC(),
	}

	var err error
	var statusCode int
	var tlsExpiry *float64

	switch svc.Type {
	case "http":
		statusCode, tlsExpiry, err = checkHTTP(cctx, svc)
	case "tcp":
		err = checkTCP(cctx, svc)
	default:
		err = fmt.Errorf("unsupported check type %q", svc.Type)
	}

	elapsed := time.Since(start)
	res.DurationMs = float64(elapsed.Milliseconds())
	res.StatusCode = statusCode

	r.responseDuration.WithLabelValues(svc.Name, svc.Type, safeTarget).Set(elapsed.Seconds())

	if statusCode > 0 {
		r.httpStatusCode.WithLabelValues(svc.Name, safeTarget).Set(float64(statusCode))
	}
	if tlsExpiry != nil {
		r.tlsExpirySeconds.WithLabelValues(svc.Name, safeTarget).Set(*tlsExpiry)
	}

	if err != nil {
		res.Up = false
		res.Error = err.Error()
		r.serviceUp.WithLabelValues(svc.Name, svc.Type, safeTarget).Set(0)
		r.checkErrors.WithLabelValues(svc.Name, svc.Type, safeTarget).Inc()
		r.logger.Warn("service check failed", "service", svc.Name, "type", svc.Type, "target", safeTarget, "error", err)
	} else {
		res.Up = true
		r.serviceUp.WithLabelValues(svc.Name, svc.Type, safeTarget).Set(1)
	}

	r.mu.Lock()
	r.results[svc.Name] = res
	r.mu.Unlock()

	return res
}

// Run spawns one bounded goroutine per configured service check and stops cleanly when ctx is done.
func (r *Runner) Run(ctx context.Context) {
	if len(r.services) == 0 {
		return
	}

	var wg sync.WaitGroup
	for _, svc := range r.services {
		svc := svc
		wg.Add(1)
		go func() {
			defer wg.Done()
			r.CheckOne(ctx, svc)
			ticker := time.NewTicker(svc.Interval.Duration)
			defer ticker.Stop()

			for {
				select {
				case <-ctx.Done():
					return
				case <-ticker.C:
					r.CheckOne(ctx, svc)
				}
			}
		}()
	}
	wg.Wait()
}

// Results returns a snapshot of all latest service check results.
func (r *Runner) Results() []Result {
	r.mu.RLock()
	defer r.mu.RUnlock()
	out := make([]Result, 0, len(r.results))
	for _, res := range r.results {
		out = append(out, res)
	}
	return out
}

func checkHTTP(ctx context.Context, svc config.ServiceConfig) (int, *float64, error) {
	client := &http.Client{
		Timeout: svc.Timeout.Duration,
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, svc.Target, nil)
	if err != nil {
		return 0, nil, err
	}
	req.Header.Set("User-Agent", "CloudPulse-Agent/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return 0, nil, err
	}
	defer func() {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 4096))
		_ = resp.Body.Close()
	}()

	var tlsExpiry *float64
	if svc.CheckTLSExpiry && resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		remaining := time.Until(resp.TLS.PeerCertificates[0].NotAfter).Seconds()
		tlsExpiry = &remaining
	}

	if svc.ExpectedStatusCode > 0 {
		if resp.StatusCode != svc.ExpectedStatusCode {
			return resp.StatusCode, tlsExpiry, fmt.Errorf("unexpected HTTP status %d (expected %d)", resp.StatusCode, svc.ExpectedStatusCode)
		}
	} else if resp.StatusCode < 200 || resp.StatusCode >= 400 {
		return resp.StatusCode, tlsExpiry, fmt.Errorf("unhealthy HTTP status %d", resp.StatusCode)
	}

	return resp.StatusCode, tlsExpiry, nil
}

func checkTCP(ctx context.Context, svc config.ServiceConfig) error {
	dialer := net.Dialer{Timeout: svc.Timeout.Duration}
	conn, err := dialer.DialContext(ctx, "tcp", svc.Target)
	if err != nil {
		return err
	}
	_ = conn.Close()
	return nil
}
