package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestDefaultConfigValidates(t *testing.T) {
	cfg := DefaultConfig()
	require.NoError(t, cfg.Validate())
	assert.Equal(t, "/metrics", cfg.Server.MetricsPath)
	assert.Equal(t, "/health", cfg.Server.HealthPath)
	assert.Equal(t, "/agent/info", cfg.Server.InfoPath)
}

func TestLoadValidYAML(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.yaml")
	content := `
agent:
  id: "srv-test-1"
  name: "test-server"
  environment: "staging"
server:
  listen_address: "127.0.0.1:9181"
  metrics_path: "/metrics"
  health_path: "/health"
collection:
  interval: "10s"
  timeout: "4s"
services:
  - name: "web-health"
    type: "http"
    target: "http://127.0.0.1:8080/healthz"
    interval: "20s"
    timeout: "3s"
    expected_status_code: 200
  - name: "db-tcp"
    type: "tcp"
    target: "127.0.0.1:5432"
`
	require.NoError(t, os.WriteFile(path, []byte(content), 0o600))

	cfg, err := Load(path)
	require.NoError(t, err)
	assert.Equal(t, "srv-test-1", cfg.Agent.ID)
	assert.Equal(t, 10*time.Second, cfg.Collection.Interval.Duration)
	assert.Len(t, cfg.Services, 2)
	assert.Equal(t, 30*time.Second, cfg.Services[1].Interval.Duration)
}

func TestValidateRejectsInvalidConfigs(t *testing.T) {
	t.Run("missing agent id", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Agent.ID = ""
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "agent.id")
	})

	t.Run("timeout exceeds interval", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Collection.Interval = Duration{2 * time.Second}
		cfg.Collection.Timeout = Duration{5 * time.Second}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "collection.timeout")
	})

	t.Run("invalid service url and duplicate name", func(t *testing.T) {
		cfg := DefaultConfig()
		cfg.Services = []ServiceConfig{
			{Name: "api", Type: "http", Target: "not-a-url"},
			{Name: "api", Type: "tcp", Target: "127.0.0.1:5432"},
		}
		err := cfg.Validate()
		require.Error(t, err)
		assert.Contains(t, err.Error(), "duplicated")
		assert.Contains(t, err.Error(), "valid http(s) URL")
	})
}
