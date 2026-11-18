package discovery

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"gopkg.in/yaml.v3"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
)

// TargetGroup matches Prometheus file_sd_configs JSON schema.
type TargetGroup struct {
	Targets []string          `json:"targets"`
	Labels  map[string]string `json:"labels"`
}

// Reloader triggers Prometheus configuration reload when enabled.
type Reloader interface {
	Reload(ctx context.Context) error
}

// Manager writes Prometheus file_sd targets and custom alert rule files atomically.
type Manager struct {
	fileSDPath    string
	rulesDir      string
	reloadEnabled bool
	reloader      Reloader
	logger        *slog.Logger
	mu            sync.Mutex
}

func NewManager(fileSDPath, rulesDir string, reloadEnabled bool, reloader Reloader, logger *slog.Logger) *Manager {
	if logger == nil {
		logger = slog.Default()
	}
	return &Manager{
		fileSDPath:    fileSDPath,
		rulesDir:      rulesDir,
		reloadEnabled: reloadEnabled,
		reloader:      reloader,
		logger:        logger,
	}
}

// SyncTargets atomically rewrites the Prometheus file_sd JSON target file for all enabled servers.
func (m *Manager) SyncTargets(ctx context.Context, servers []models.Server) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	groups := make([]TargetGroup, 0, len(servers))
	for _, s := range servers {
		if !s.MonitoringEnabled {
			continue
		}
		target := normalizeTarget(s.ScrapeAddress)
		if target == "" {
			continue
		}
		labels := map[string]string{
			"cloudpulse_server_id": s.ID.String(),
			"cloudpulse_org_id":    s.OrganizationID.String(),
			"server_name":          s.Name,
			"hostname":             s.Hostname,
			"environment":          s.Environment,
			"server_group":         s.ServerGroup,
			"operating_system":     s.OperatingSystem,
		}
		groups = append(groups, TargetGroup{
			Targets: []string{target},
			Labels:  labels,
		})
	}

	payload, err := json.MarshalIndent(groups, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal file_sd targets: %w", err)
	}
	payload = append(payload, '\n')

	if err := atomicWriteFile(m.fileSDPath, payload, 0o644); err != nil {
		return fmt.Errorf("write file_sd targets: %w", err)
	}

	m.logger.Info("synchronized prometheus file_sd targets", "count", len(groups), "path", m.fileSDPath)
	m.tryReload(ctx)
	return nil
}

// SyncAlertRules writes enabled custom/organization alert rules into a Prometheus rule YAML file atomically.
func (m *Manager) SyncAlertRules(ctx context.Context, orgID string, rules []models.AlertRule) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	if strings.TrimSpace(m.rulesDir) == "" {
		return nil
	}

	type promRule struct {
		Alert       string            `yaml:"alert"`
		Expr        string            `yaml:"expr"`
		For         string            `yaml:"for,omitempty"`
		Labels      map[string]string `yaml:"labels"`
		Annotations map[string]string `yaml:"annotations"`
	}
	type ruleGroup struct {
		Name  string     `yaml:"name"`
		Rules []promRule `yaml:"rules"`
	}
	type ruleFile struct {
		Groups []ruleGroup `yaml:"groups"`
	}

	var compiled []promRule
	for _, r := range rules {
		if !r.Enabled {
			continue
		}
		alertName := sanitizeAlertName(r.Name)
		compiled = append(compiled, promRule{
			Alert: alertName,
			Expr:  r.Expression,
			For:   r.Duration,
			Labels: map[string]string{
				"severity":          r.Severity,
				"cloudpulse_org_id": orgID,
				"rule_id":           r.ID.String(),
			},
			Annotations: map[string]string{
				"summary":     r.Name,
				"description": r.Description,
			},
		})
	}

	doc := ruleFile{
		Groups: []ruleGroup{
			{
				Name:  "cloudpulse_org_" + sanitizeAlertName(orgID),
				Rules: compiled,
			},
		},
	}

	data, err := yaml.Marshal(doc)
	if err != nil {
		return fmt.Errorf("marshal prometheus rules: %w", err)
	}

	filePath := filepath.Join(m.rulesDir, fmt.Sprintf("custom_%s.yml", sanitizeAlertName(orgID)))
	if err := atomicWriteFile(filePath, data, 0o644); err != nil {
		return fmt.Errorf("write custom alert rules: %w", err)
	}

	m.logger.Info("synchronized prometheus alert rules", "count", len(compiled), "path", filePath)
	m.tryReload(ctx)
	return nil
}

func (m *Manager) tryReload(ctx context.Context) {
	if !m.reloadEnabled || m.reloader == nil {
		return
	}
	if err := m.reloader.Reload(ctx); err != nil {
		m.logger.Warn("prometheus reload signal failed (file_sd auto-refresh will still pick up targets)", "error", err)
	}
}

func normalizeTarget(addr string) string {
	s := strings.TrimSpace(addr)
	s = strings.TrimPrefix(s, "http://")
	s = strings.TrimPrefix(s, "https://")
	s = strings.TrimSuffix(s, "/metrics")
	s = strings.TrimSuffix(s, "/")
	return s
}

func sanitizeAlertName(name string) string {
	var b strings.Builder
	for _, r := range name {
		if (r >= 'a' && r <= 'z') || (r >= 'A' && r <= 'Z') || (r >= '0' && r <= '9') || r == '_' {
			b.WriteRune(r)
		} else {
			b.WriteRune('_')
		}
	}
	out := strings.Trim(b.String(), "_")
	if out == "" {
		return "CloudPulseAlert"
	}
	return out
}

func atomicWriteFile(targetPath string, data []byte, perm os.FileMode) error {
	dir := filepath.Dir(targetPath)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".cloudpulse-*.tmp")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	cleanup := true
	defer func() {
		if cleanup {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Chmod(perm); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Sync(); err != nil {
		_ = tmp.Close()
		return err
	}
	if err := tmp.Close(); err != nil {
		return err
	}
	if err := os.Rename(tmpName, targetPath); err != nil {
		return err
	}
	cleanup = false
	return nil
}
