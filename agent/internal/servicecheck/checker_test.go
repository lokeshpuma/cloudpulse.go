package servicecheck

import (
	"context"
	"io"
	"log/slog"
	"net"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
)

func TestSanitizeTargetStripsSecrets(t *testing.T) {
	sanitized := SanitizeTarget("http", "https://admin:supersecret@api.example.com/health?token=secret123#frag")
	assert.Equal(t, "https://api.example.com/health", sanitized)
	assert.NotContains(t, sanitized, "supersecret")
	assert.NotContains(t, sanitized, "secret123")
}

func TestHTTPServiceCheckSuccessAndStatusMismatch(t *testing.T) {
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/ok" {
			w.WriteHeader(http.StatusOK)
			return
		}
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer ts.Close()

	cfg := config.DefaultConfig()
	reg := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(cfg, reg, logger)

	okRes := runner.CheckOne(context.Background(), config.ServiceConfig{
		Name:               "api-ok",
		Type:               "http",
		Target:             ts.URL + "/ok",
		Timeout:            config.Duration{Duration: 2 * time.Second},
		ExpectedStatusCode: 200,
	})
	assert.True(t, okRes.Up)
	assert.Equal(t, 200, okRes.StatusCode)

	failRes := runner.CheckOne(context.Background(), config.ServiceConfig{
		Name:               "api-fail",
		Type:               "http",
		Target:             ts.URL + "/fail",
		Timeout:            config.Duration{Duration: 2 * time.Second},
		ExpectedStatusCode: 200,
	})
	assert.False(t, failRes.Up)
	assert.Equal(t, 500, failRes.StatusCode)
}

func TestTCPServiceCheck(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	require.NoError(t, err)
	defer ln.Close()

	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	cfg := config.DefaultConfig()
	reg := prometheus.NewRegistry()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))
	runner := NewRunner(cfg, reg, logger)

	res := runner.CheckOne(context.Background(), config.ServiceConfig{
		Name:    "local-tcp",
		Type:    "tcp",
		Target:  ln.Addr().String(),
		Timeout: config.Duration{Duration: 2 * time.Second},
	})
	assert.True(t, res.Up)
}
