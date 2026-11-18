package config

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"time"
)

// Config holds validated runtime configuration loaded from environment variables.
type Config struct {
	AppEnv                  string
	AppPort                 string
	AppBaseURL              string
	DatabaseURL             string
	JWTSecret               string
	JWTAccessTTL            time.Duration
	JWTRefreshTTL           time.Duration
	CookieSecure            bool
	PrometheusURL           string
	PrometheusFileSDPath    string
	PrometheusRulesDir      string
	PrometheusReloadEnabled bool
	GrafanaURL              string
	GrafanaPublicURL        string
	AlertmanagerURL         string
	CORSAllowedOrigins      []string
	LogLevel                string
	Version                 string
}

// Load parses environment variables and enforces strict validation rules.
func Load() (Config, error) {
	cfg := Config{
		AppEnv:                  envOr("APP_ENV", "development"),
		AppPort:                 envOr("APP_PORT", "8080"),
		AppBaseURL:              envOr("APP_BASE_URL", "http://localhost:8080"),
		DatabaseURL:             strings.TrimSpace(os.Getenv("DATABASE_URL")),
		JWTSecret:               strings.TrimSpace(os.Getenv("JWT_SECRET")),
		PrometheusURL:           strings.TrimRight(envOr("PROMETHEUS_URL", "http://prometheus:9090"), "/"),
		PrometheusFileSDPath:    envOr("PROMETHEUS_FILE_SD_PATH", "/etc/prometheus/file_sd/targets.json"),
		PrometheusRulesDir:      envOr("PROMETHEUS_RULES_DIR", "/etc/prometheus/rules"),
		PrometheusReloadEnabled: strings.EqualFold(envOr("PROMETHEUS_RELOAD_ENABLED", "true"), "true"),
		GrafanaURL:              strings.TrimRight(envOr("GRAFANA_URL", "http://grafana:3000"), "/"),
		GrafanaPublicURL:        strings.TrimRight(envOr("GRAFANA_PUBLIC_URL", "http://localhost:3001"), "/"),
		AlertmanagerURL:         strings.TrimRight(envOr("ALERTMANAGER_URL", "http://alertmanager:9093"), "/"),
		LogLevel:                strings.ToLower(envOr("LOG_LEVEL", "info")),
		Version:                 envOr("APP_VERSION", "1.0.0"),
	}

	accessTTL, err := time.ParseDuration(envOr("JWT_ACCESS_TTL", "15m"))
	if err != nil {
		return cfg, fmt.Errorf("invalid JWT_ACCESS_TTL: %w", err)
	}
	cfg.JWTAccessTTL = accessTTL

	refreshTTL, err := time.ParseDuration(envOr("JWT_REFRESH_TTL", "168h"))
	if err != nil {
		return cfg, fmt.Errorf("invalid JWT_REFRESH_TTL: %w", err)
	}
	cfg.JWTRefreshTTL = refreshTTL

	if rawSecure := strings.TrimSpace(os.Getenv("COOKIE_SECURE")); rawSecure != "" {
		cfg.CookieSecure = strings.EqualFold(rawSecure, "true")
	} else {
		cfg.CookieSecure = strings.EqualFold(cfg.AppEnv, "production")
	}

	rawOrigins := envOr("CORS_ALLOWED_ORIGINS", "http://localhost:5173,http://localhost:3000")
	for _, o := range strings.Split(rawOrigins, ",") {
		o = strings.TrimSpace(o)
		if o != "" {
			cfg.CORSAllowedOrigins = append(cfg.CORSAllowedOrigins, o)
		}
	}

	if err := cfg.Validate(); err != nil {
		return cfg, err
	}
	return cfg, nil
}

// Validate checks mandatory settings and rejects unsafe production defaults.
func (c Config) Validate() error {
	var errs []string

	if c.DatabaseURL == "" {
		errs = append(errs, "DATABASE_URL is required")
	}
	if c.JWTSecret == "" {
		errs = append(errs, "JWT_SECRET is required")
	} else if len(c.JWTSecret) < 32 {
		errs = append(errs, "JWT_SECRET must be at least 32 characters")
	}

	if c.JWTAccessTTL < time.Minute || c.JWTAccessTTL > 2*time.Hour {
		errs = append(errs, "JWT_ACCESS_TTL must be between 1m and 2h")
	}
	if c.JWTRefreshTTL < time.Hour {
		errs = append(errs, "JWT_REFRESH_TTL must be at least 1h")
	}
	if len(c.CORSAllowedOrigins) == 0 {
		errs = append(errs, "CORS_ALLOWED_ORIGINS must specify at least one explicit origin")
	}

	if strings.EqualFold(c.AppEnv, "production") {
		if strings.Contains(strings.ToLower(c.JWTSecret), "change-me") {
			errs = append(errs, "JWT_SECRET uses an unsafe placeholder in production")
		}
		if strings.Contains(strings.ToLower(c.DatabaseURL), "change-me") {
			errs = append(errs, "DATABASE_URL uses an unsafe placeholder password in production")
		}
		if !c.CookieSecure {
			errs = append(errs, "COOKIE_SECURE must be true in production")
		}
		for _, origin := range c.CORSAllowedOrigins {
			if origin == "*" {
				errs = append(errs, "wildcard '*' CORS origin is forbidden in production")
			}
		}
	}

	if len(errs) > 0 {
		return errors.New(strings.Join(errs, "; "))
	}
	return nil
}

func envOr(key, fallback string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return fallback
}
