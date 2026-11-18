package config

import (
	"errors"
	"fmt"
	"net/url"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v3"
)

// Config holds the full validated configuration for cloudpulse-agent.
type Config struct {
	Agent      AgentConfig      `yaml:"agent"`
	Server     ServerConfig     `yaml:"server"`
	Auth       AuthConfig       `yaml:"auth"`
	Collection CollectionConfig `yaml:"collection"`
	CPU        CPUConfig        `yaml:"cpu"`
	Memory     MemoryConfig     `yaml:"memory"`
	Disk       DiskConfig       `yaml:"disk"`
	Network    NetworkConfig    `yaml:"network"`
	Services   []ServiceConfig  `yaml:"services"`
	Logging    LoggingConfig    `yaml:"logging"`
}

type AgentConfig struct {
	ID          string `yaml:"id"`
	Name        string `yaml:"name"`
	Environment string `yaml:"environment"`
}

type ServerConfig struct {
	ListenAddress string `yaml:"listen_address"`
	MetricsPath   string `yaml:"metrics_path"`
	HealthPath    string `yaml:"health_path"`
	InfoPath      string `yaml:"info_path"`
}

type AuthConfig struct {
	// BearerTokenEnv names an environment variable containing the shared bearer token.
	// Never hard-code secrets in config files.
	BearerTokenEnv string `yaml:"bearer_token_env"`
	TLSCertFile    string `yaml:"tls_cert_file"`
	TLSKeyFile     string `yaml:"tls_key_file"`
	TLSClientCA    string `yaml:"tls_client_ca"`
}

type CollectionConfig struct {
	Interval        Duration `yaml:"interval"`
	Timeout         Duration `yaml:"timeout"`
	IncludeGoMetric bool     `yaml:"include_go_metrics"`
}

type CPUConfig struct {
	Enabled        bool `yaml:"enabled"`
	PerCoreEnabled bool `yaml:"per_core_enabled"`
}

type MemoryConfig struct {
	Enabled bool `yaml:"enabled"`
}

type DiskConfig struct {
	Enabled             bool     `yaml:"enabled"`
	ExcludedMountpoints []string `yaml:"excluded_mountpoints"`
	ExcludedFilesystems []string `yaml:"excluded_filesystems"`
}

type NetworkConfig struct {
	Enabled            bool     `yaml:"enabled"`
	ExcludedInterfaces []string `yaml:"excluded_interfaces"`
}

type ServiceConfig struct {
	Name               string   `yaml:"name"`
	Type               string   `yaml:"type"` // "http" or "tcp"
	Target             string   `yaml:"target"`
	Interval           Duration `yaml:"interval"`
	Timeout            Duration `yaml:"timeout"`
	ExpectedStatusCode int      `yaml:"expected_status_code"`
	CheckTLSExpiry     bool     `yaml:"check_tls_expiry"`
}

type LoggingConfig struct {
	Level  string `yaml:"level"`
	Format string `yaml:"format"`
}

// Duration wraps time.Duration for YAML string parsing (e.g. "15s").
type Duration struct {
	time.Duration
}

func (d *Duration) UnmarshalYAML(value *yaml.Node) error {
	var raw string
	if err := value.Decode(&raw); err != nil {
		return err
	}
	parsed, err := time.ParseDuration(strings.TrimSpace(raw))
	if err != nil {
		return fmt.Errorf("invalid duration %q: %w", raw, err)
	}
	d.Duration = parsed
	return nil
}

func (d Duration) MarshalYAML() (any, error) {
	return d.Duration.String(), nil
}

// DefaultConfig returns safe defaults aligned with the PRD.
func DefaultConfig() Config {
	hostname, _ := os.Hostname()
	if hostname == "" {
		hostname = "unknown-host"
	}
	return Config{
		Agent: AgentConfig{
			ID:          hostname,
			Name:        hostname,
			Environment: "development",
		},
		Server: ServerConfig{
			ListenAddress: "127.0.0.1:9180",
			MetricsPath:   "/metrics",
			HealthPath:    "/health",
			InfoPath:      "/agent/info",
		},
		Collection: CollectionConfig{
			Interval:        Duration{15 * time.Second},
			Timeout:         Duration{5 * time.Second},
			IncludeGoMetric: true,
		},
		CPU: CPUConfig{
			Enabled:        true,
			PerCoreEnabled: true,
		},
		Memory: MemoryConfig{
			Enabled: true,
		},
		Disk: DiskConfig{
			Enabled: true,
			ExcludedMountpoints: []string{
				"/proc", "/sys", "/dev", "/run", "/snap", "/System/Volumes/VM", "/System/Volumes/Preboot",
			},
			ExcludedFilesystems: []string{
				"proc", "sysfs", "tmpfs", "devtmpfs", "devpts", "cgroup", "cgroup2",
				"overlay", "squashfs", "nsfs", "tracefs", "debugfs", "securityfs",
				"fusectl", "mqueue", "hugetlbfs", "devfs", "autofs", "nullfs",
			},
		},
		Network: NetworkConfig{
			Enabled:            true,
			ExcludedInterfaces: []string{"lo", "lo0"},
		},
		Logging: LoggingConfig{
			Level:  "info",
			Format: "json",
		},
	}
}

// Load reads and validates YAML configuration from path.
func Load(path string) (Config, error) {
	cfg := DefaultConfig()
	if path != "" {
		data, err := os.ReadFile(path)
		if err != nil {
			return cfg, fmt.Errorf("read config %q: %w", path, err)
		}
		if err := yaml.Unmarshal(data, &cfg); err != nil {
			return cfg, fmt.Errorf("parse yaml config %q: %w", path, err)
		}
	}

	// Environment overrides for container deployments
	if v := strings.TrimSpace(os.Getenv("AGENT_ID")); v != "" {
		cfg.Agent.ID = v
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_NAME")); v != "" {
		cfg.Agent.Name = v
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_ENVIRONMENT")); v != "" {
		cfg.Agent.Environment = v
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_LISTEN_ADDRESS")); v != "" {
		cfg.Server.ListenAddress = v
	}
	if v := strings.TrimSpace(os.Getenv("AGENT_COLLECTION_INTERVAL")); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			cfg.Collection.Interval = Duration{d}
		}
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks every configuration field for safety and correctness.
func (c *Config) Validate() error {
	var errs []string

	if strings.TrimSpace(c.Agent.ID) == "" {
		errs = append(errs, "agent.id is required")
	}
	if strings.TrimSpace(c.Agent.Name) == "" {
		errs = append(errs, "agent.name is required")
	}
	if strings.TrimSpace(c.Agent.Environment) == "" {
		errs = append(errs, "agent.environment is required")
	}
	if strings.TrimSpace(c.Server.ListenAddress) == "" {
		errs = append(errs, "server.listen_address is required")
	}
	if !strings.HasPrefix(c.Server.MetricsPath, "/") {
		errs = append(errs, "server.metrics_path must start with '/'")
	}
	if !strings.HasPrefix(c.Server.HealthPath, "/") {
		errs = append(errs, "server.health_path must start with '/'")
	}
	if c.Server.InfoPath == "" {
		c.Server.InfoPath = "/agent/info"
	}
	if !strings.HasPrefix(c.Server.InfoPath, "/") {
		errs = append(errs, "server.info_path must start with '/'")
	}

	if c.Collection.Interval.Duration < time.Second {
		errs = append(errs, "collection.interval must be at least 1s")
	}
	if c.Collection.Timeout.Duration < 500*time.Millisecond {
		errs = append(errs, "collection.timeout must be at least 500ms")
	}
	if c.Collection.Timeout.Duration > c.Collection.Interval.Duration {
		errs = append(errs, "collection.timeout must not exceed collection.interval")
	}

	if (c.Auth.TLSCertFile != "") != (c.Auth.TLSKeyFile != "") {
		errs = append(errs, "auth.tls_cert_file and auth.tls_key_file must both be set when TLS is enabled")
	}

	seenServices := make(map[string]struct{}, len(c.Services))
	for i := range c.Services {
		svc := &c.Services[i]
		if strings.TrimSpace(svc.Name) == "" {
			errs = append(errs, fmt.Sprintf("services[%d].name is required", i))
		}
		if _, dup := seenServices[svc.Name]; dup {
			errs = append(errs, fmt.Sprintf("services[%d].name %q is duplicated", i, svc.Name))
		}
		seenServices[svc.Name] = struct{}{}

		svc.Type = strings.ToLower(strings.TrimSpace(svc.Type))
		if svc.Type != "http" && svc.Type != "tcp" {
			errs = append(errs, fmt.Sprintf("services[%d].type must be 'http' or 'tcp'", i))
		}
		if strings.TrimSpace(svc.Target) == "" {
			errs = append(errs, fmt.Sprintf("services[%d].target is required", i))
		} else if svc.Type == "http" {
			u, err := url.Parse(svc.Target)
			if err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
				errs = append(errs, fmt.Sprintf("services[%d].target must be a valid http(s) URL", i))
			}
		}
		if svc.Interval.Duration == 0 {
			svc.Interval = Duration{30 * time.Second}
		}
		if svc.Interval.Duration < time.Second {
			errs = append(errs, fmt.Sprintf("services[%d].interval must be at least 1s", i))
		}
		if svc.Timeout.Duration == 0 {
			svc.Timeout = Duration{5 * time.Second}
		}
		if svc.Timeout.Duration < 200*time.Millisecond {
			errs = append(errs, fmt.Sprintf("services[%d].timeout must be at least 200ms", i))
		}
		if svc.ExpectedStatusCode != 0 && (svc.ExpectedStatusCode < 100 || svc.ExpectedStatusCode > 599) {
			errs = append(errs, fmt.Sprintf("services[%d].expected_status_code must be between 100 and 599", i))
		}
	}

	switch strings.ToLower(c.Logging.Level) {
	case "", "debug", "info", "warn", "error":
	default:
		errs = append(errs, "logging.level must be one of debug, info, warn, error")
	}
	switch strings.ToLower(c.Logging.Format) {
	case "", "json", "text":
	default:
		errs = append(errs, "logging.format must be json or text")
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

// BearerToken returns the configured bearer token read from environment, if any.
func (c *Config) BearerToken() string {
	if strings.TrimSpace(c.Auth.BearerTokenEnv) == "" {
		return ""
	}
	return strings.TrimSpace(os.Getenv(c.Auth.BearerTokenEnv))
}
