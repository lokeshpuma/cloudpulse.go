package server

import (
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudpulse/cloudpulse-go/agent/internal/collector"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/config"
	"github.com/cloudpulse/cloudpulse-go/agent/internal/servicecheck"
)

func TestAgentEndpoints(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Agent.ID = "agent-unit-01"
	cfg.Agent.Name = "unit-host"
	cfg.Collection.IncludeGoMetric = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	coll := collector.NewManager(cfg, "1.2.3", logger, nil)
	coll.CollectOnce(context.Background())
	svc := servicecheck.NewRunner(cfg, coll.Registry(), logger)
	srv := New(cfg, coll, svc, "1.2.3", logger)

	t.Run("GET /metrics returns prometheus exposition format", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		body := rec.Body.String()
		assert.Contains(t, body, "cloudpulse_agent_up")
		assert.Contains(t, body, "cloudpulse_cpu_usage_percent")
		assert.Contains(t, body, "cloudpulse_memory_total_bytes")
	})

	t.Run("GET /health returns collector health", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/health", nil)
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var payload HealthPayload
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &payload))
		assert.True(t, payload.Healthy)
		assert.Equal(t, "healthy", payload.Status)
		assert.NotNil(t, payload.LastCollection)
	})

	t.Run("GET /agent/info returns host metadata", func(t *testing.T) {
		rec := httptest.NewRecorder()
		req := httptest.NewRequest(http.MethodGet, "/agent/info", nil)
		srv.Handler().ServeHTTP(rec, req)

		assert.Equal(t, http.StatusOK, rec.Code)
		var info InfoPayload
		require.NoError(t, json.Unmarshal(rec.Body.Bytes(), &info))
		assert.Equal(t, "agent-unit-01", info.AgentID)
		assert.Equal(t, "1.2.3", info.Version)
		assert.NotEmpty(t, info.OperatingSystem)
		assert.NotEmpty(t, info.Architecture)
	})

	t.Run("Shutdown drains cleanly", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		require.NoError(t, srv.Shutdown(ctx))
	})
}

func TestAgentBearerAuthProtection(t *testing.T) {
	t.Setenv("TEST_AGENT_TOKEN", "s3cr3t-test-token")
	cfg := config.DefaultConfig()
	cfg.Auth.BearerTokenEnv = "TEST_AGENT_TOKEN"
	cfg.Collection.IncludeGoMetric = false
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	coll := collector.NewManager(cfg, "1.0.0", logger, nil)
	svc := servicecheck.NewRunner(cfg, coll.Registry(), logger)
	srv := New(cfg, coll, svc, "1.0.0", logger)

	rec := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/metrics", nil))
	assert.Equal(t, http.StatusUnauthorized, rec.Code)

	req := httptest.NewRequest(http.MethodGet, "/metrics", nil)
	req.Header.Set("Authorization", "Bearer s3cr3t-test-token")
	rec2 := httptest.NewRecorder()
	srv.Handler().ServeHTTP(rec2, req)
	assert.Equal(t, http.StatusOK, rec2.Code)
}
