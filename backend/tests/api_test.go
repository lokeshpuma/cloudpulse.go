package tests

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/alertmanager"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/audit"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/auth"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/config"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/discovery"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/grafana"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/handlers"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/middleware"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/notifications"
	prom "github.com/cloudpulse/cloudpulse-go/backend/internal/prometheus"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/reports"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/repository"
	"github.com/cloudpulse/cloudpulse-go/backend/internal/services"
)

// In-memory Store implementation for fast, deterministic API unit testing.
type memoryStore struct {
	mu       sync.RWMutex
	orgs     map[uuid.UUID]models.Organization
	users    map[uuid.UUID]models.User
	sessions map[string]models.RefreshSession
	resets   map[string]uuid.UUID
	servers  map[uuid.UUID]models.Server
	services map[uuid.UUID]models.ServiceCheck
	rules    map[uuid.UUID]models.AlertRule
	acks     []models.AlertAcknowledgement
	channels map[uuid.UUID]models.NotificationChannel
	notifs   map[uuid.UUID]models.NotificationRecord
	audits   []models.AuditLog
	queries  []models.QueryHistoryItem
}

func newMemoryStore() *memoryStore {
	return &memoryStore{
		orgs:     map[uuid.UUID]models.Organization{},
		users:    map[uuid.UUID]models.User{},
		sessions: map[string]models.RefreshSession{},
		resets:   map[string]uuid.UUID{},
		servers:  map[uuid.UUID]models.Server{},
		services: map[uuid.UUID]models.ServiceCheck{},
		rules:    map[uuid.UUID]models.AlertRule{},
		channels: map[uuid.UUID]models.NotificationChannel{},
		notifs:   map[uuid.UUID]models.NotificationRecord{},
	}
}

func (m *memoryStore) Ping(context.Context) error { return nil }

func (m *memoryStore) CreateOrgAndAdmin(_ context.Context, orgName, userName, email, passwordHash string) (models.Organization, models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return models.Organization{}, models.User{}, repository.ErrConflict
		}
	}
	now := time.Now().UTC()
	org := models.Organization{ID: uuid.New(), Name: orgName, Timezone: "UTC", DisplayUnits: "binary", DateFormat: "YYYY-MM-DD", TimeFormat: "24h", DefaultScrapeInterval: 15, DefaultMonitoringTimeout: 5, AgentConnectionTimeout: 10, DefaultEnvironment: "production", CreatedAt: now, UpdatedAt: now}
	user := models.User{ID: uuid.New(), OrganizationID: org.ID, Name: userName, Email: strings.ToLower(email), PasswordHash: passwordHash, Role: models.RoleAdmin, IsActive: true, CreatedAt: now, UpdatedAt: now}
	m.orgs[org.ID] = org
	m.users[user.ID] = user
	return org, user, nil
}

func (m *memoryStore) GetUserByEmail(_ context.Context, email string) (models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return u, nil
		}
	}
	return models.User{}, repository.ErrNotFound
}

func (m *memoryStore) GetUserByID(_ context.Context, id uuid.UUID) (models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	u, ok := m.users[id]
	if !ok {
		return u, repository.ErrNotFound
	}
	return u, nil
}

func (m *memoryStore) ListUsers(_ context.Context, orgID uuid.UUID) ([]models.User, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.User{}
	for _, u := range m.users {
		if u.OrganizationID == orgID {
			out = append(out, u)
		}
	}
	return out, nil
}

func (m *memoryStore) CreateUser(_ context.Context, orgID uuid.UUID, name, email, passwordHash, role string) (models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, u := range m.users {
		if strings.EqualFold(u.Email, email) {
			return models.User{}, repository.ErrConflict
		}
	}
	now := time.Now().UTC()
	u := models.User{ID: uuid.New(), OrganizationID: orgID, Name: name, Email: strings.ToLower(email), PasswordHash: passwordHash, Role: role, IsActive: true, CreatedAt: now, UpdatedAt: now}
	m.users[u.ID] = u
	return u, nil
}

func (m *memoryStore) UpdateUser(_ context.Context, orgID, userID uuid.UUID, name, role string, isActive *bool) (models.User, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok || u.OrganizationID != orgID {
		return u, repository.ErrNotFound
	}
	if name != "" {
		u.Name = name
	}
	if role != "" {
		u.Role = role
	}
	if isActive != nil {
		u.IsActive = *isActive
	}
	m.users[userID] = u
	return u, nil
}

func (m *memoryStore) UpdateUserPassword(_ context.Context, userID uuid.UUID, passwordHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	u, ok := m.users[userID]
	if !ok {
		return repository.ErrNotFound
	}
	u.PasswordHash = passwordHash
	m.users[userID] = u
	return nil
}

func (m *memoryStore) CreateRefreshSession(_ context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshSession, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rs := models.RefreshSession{ID: uuid.New(), UserID: userID, TokenHash: tokenHash, ExpiresAt: expiresAt, CreatedAt: time.Now().UTC()}
	m.sessions[tokenHash] = rs
	return rs, nil
}

func (m *memoryStore) GetRefreshSessionByHash(_ context.Context, tokenHash string) (models.RefreshSession, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	rs, ok := m.sessions[tokenHash]
	if !ok {
		return rs, repository.ErrNotFound
	}
	return rs, nil
}

func (m *memoryStore) RevokeRefreshSession(_ context.Context, tokenHash string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if rs, ok := m.sessions[tokenHash]; ok {
		now := time.Now().UTC()
		rs.RevokedAt = &now
		m.sessions[tokenHash] = rs
	}
	return nil
}

func (m *memoryStore) RevokeAllUserSessions(_ context.Context, userID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now().UTC()
	for k, rs := range m.sessions {
		if rs.UserID == userID {
			rs.RevokedAt = &now
			m.sessions[k] = rs
		}
	}
	return nil
}

func (m *memoryStore) CreatePasswordResetToken(_ context.Context, userID uuid.UUID, tokenHash string, _ time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.resets[tokenHash] = userID
	return nil
}

func (m *memoryStore) ConsumePasswordResetToken(_ context.Context, tokenHash string) (uuid.UUID, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	uid, ok := m.resets[tokenHash]
	if !ok {
		return uuid.Nil, repository.ErrNotFound
	}
	delete(m.resets, tokenHash)
	return uid, nil
}

func (m *memoryStore) GetOrganization(_ context.Context, orgID uuid.UUID) (models.Organization, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	o, ok := m.orgs[orgID]
	if !ok {
		return o, repository.ErrNotFound
	}
	return o, nil
}

func (m *memoryStore) UpdateOrganization(_ context.Context, org models.Organization) (models.Organization, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.orgs[org.ID] = org
	return org, nil
}

func (m *memoryStore) ListServers(_ context.Context, orgID uuid.UUID, f repository.ServerFilter) ([]models.Server, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var matched []models.Server
	for _, s := range m.servers {
		if s.OrganizationID != orgID {
			continue
		}
		if f.Environment != "" && !strings.EqualFold(s.Environment, f.Environment) {
			continue
		}
		if f.Search != "" && !strings.Contains(strings.ToLower(s.Name+s.Hostname), strings.ToLower(f.Search)) {
			continue
		}
		matched = append(matched, s)
	}
	total := len(matched)
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 {
		f.PageSize = 25
	}
	start := (f.Page - 1) * f.PageSize
	if start >= total {
		return []models.Server{}, total, nil
	}
	end := start + f.PageSize
	if end > total {
		end = total
	}
	return matched[start:end], total, nil
}

func (m *memoryStore) ListAllEnabledServers(_ context.Context) ([]models.Server, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	var out []models.Server
	for _, s := range m.servers {
		if s.MonitoringEnabled {
			out = append(out, s)
		}
	}
	return out, nil
}

func (m *memoryStore) GetServer(_ context.Context, orgID, serverID uuid.UUID) (models.Server, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	s, ok := m.servers[serverID]
	if !ok || s.OrganizationID != orgID {
		return s, repository.ErrNotFound
	}
	return s, nil
}

func (m *memoryStore) CreateServer(_ context.Context, srv models.Server) (models.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, existing := range m.servers {
		if existing.OrganizationID == srv.OrganizationID && existing.ScrapeAddress == srv.ScrapeAddress {
			return srv, repository.ErrConflict
		}
	}
	srv.ID = uuid.New()
	srv.CreatedAt = time.Now().UTC()
	srv.UpdatedAt = srv.CreatedAt
	if srv.Tags == nil {
		srv.Tags = []models.ServerTag{}
	}
	m.servers[srv.ID] = srv
	return srv, nil
}

func (m *memoryStore) UpdateServer(_ context.Context, srv models.Server) (models.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ex, ok := m.servers[srv.ID]
	if !ok || ex.OrganizationID != srv.OrganizationID {
		return srv, repository.ErrNotFound
	}
	m.servers[srv.ID] = srv
	return srv, nil
}

func (m *memoryStore) SetServerMonitoring(_ context.Context, orgID, serverID uuid.UUID, enabled bool) (models.Server, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok || s.OrganizationID != orgID {
		return s, repository.ErrNotFound
	}
	s.MonitoringEnabled = enabled
	m.servers[serverID] = s
	return s, nil
}

func (m *memoryStore) UpdateServerHeartbeat(_ context.Context, serverID uuid.UUID, ver, osName, arch string, seenAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if s, ok := m.servers[serverID]; ok {
		s.LastSeenAt = &seenAt
		if ver != "" {
			s.AgentVersion = ver
		}
		m.servers[serverID] = s
	}
	return nil
}

func (m *memoryStore) DeleteServer(_ context.Context, orgID, serverID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	s, ok := m.servers[serverID]
	if !ok || s.OrganizationID != orgID {
		return repository.ErrNotFound
	}
	delete(m.servers, serverID)
	return nil
}

func (m *memoryStore) ListServiceChecks(_ context.Context, orgID uuid.UUID, serverID *uuid.UUID) ([]models.ServiceCheck, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.ServiceCheck{}
	for _, sc := range m.services {
		srv, ok := m.servers[sc.ServerID]
		if !ok || srv.OrganizationID != orgID {
			continue
		}
		if serverID != nil && sc.ServerID != *serverID {
			continue
		}
		sc.ServerName = srv.Name
		out = append(out, sc)
	}
	return out, nil
}

func (m *memoryStore) GetServiceCheck(_ context.Context, orgID, id uuid.UUID) (models.ServiceCheck, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	sc, ok := m.services[id]
	if !ok {
		return sc, repository.ErrNotFound
	}
	if srv, ok := m.servers[sc.ServerID]; !ok || srv.OrganizationID != orgID {
		return sc, repository.ErrNotFound
	}
	return sc, nil
}

func (m *memoryStore) CreateServiceCheck(_ context.Context, orgID uuid.UUID, sc models.ServiceCheck) (models.ServiceCheck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	srv, ok := m.servers[sc.ServerID]
	if !ok || srv.OrganizationID != orgID {
		return sc, repository.ErrNotFound
	}
	sc.ID = uuid.New()
	sc.CreatedAt = time.Now().UTC()
	sc.UpdatedAt = sc.CreatedAt
	m.services[sc.ID] = sc
	return sc, nil
}

func (m *memoryStore) UpdateServiceCheck(_ context.Context, orgID uuid.UUID, sc models.ServiceCheck) (models.ServiceCheck, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if _, ok := m.services[sc.ID]; !ok {
		return sc, repository.ErrNotFound
	}
	m.services[sc.ID] = sc
	return sc, nil
}

func (m *memoryStore) DeleteServiceCheck(_ context.Context, orgID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.services, id)
	return nil
}

func (m *memoryStore) ListAlertRules(_ context.Context, orgID uuid.UUID) ([]models.AlertRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.AlertRule{}
	for _, r := range m.rules {
		if r.OrganizationID == orgID {
			out = append(out, r)
		}
	}
	return out, nil
}

func (m *memoryStore) GetAlertRule(_ context.Context, orgID, id uuid.UUID) (models.AlertRule, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	r, ok := m.rules[id]
	if !ok || r.OrganizationID != orgID {
		return r, repository.ErrNotFound
	}
	return r, nil
}

func (m *memoryStore) CreateAlertRule(_ context.Context, r models.AlertRule) (models.AlertRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	r.ID = uuid.New()
	r.CreatedAt = time.Now().UTC()
	m.rules[r.ID] = r
	return r, nil
}

func (m *memoryStore) UpdateAlertRule(_ context.Context, r models.AlertRule) (models.AlertRule, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.rules[r.ID] = r
	return r, nil
}

func (m *memoryStore) DeleteAlertRule(_ context.Context, orgID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.rules, id)
	return nil
}

func (m *memoryStore) SeedDefaultAlertRules(_ context.Context, _ uuid.UUID, _ *uuid.UUID) error {
	return nil
}

func (m *memoryStore) AcknowledgeAlert(_ context.Context, orgID uuid.UUID, fingerprint string, userID uuid.UUID, note string) (models.AlertAcknowledgement, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ack := models.AlertAcknowledgement{ID: uuid.New(), OrganizationID: orgID, AlertFingerprint: fingerprint, AcknowledgedBy: &userID, AcknowledgedAt: time.Now().UTC(), Note: note}
	m.acks = append(m.acks, ack)
	return ack, nil
}

func (m *memoryStore) ListAcknowledgements(_ context.Context, orgID uuid.UUID) (map[string]models.AlertAcknowledgement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := map[string]models.AlertAcknowledgement{}
	for _, a := range m.acks {
		if a.OrganizationID == orgID {
			out[a.AlertFingerprint] = a
		}
	}
	return out, nil
}

func (m *memoryStore) ListAcknowledgementHistory(_ context.Context, orgID uuid.UUID, fp string) ([]models.AlertAcknowledgement, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.AlertAcknowledgement{}
	for _, a := range m.acks {
		if a.OrganizationID == orgID && (fp == "" || a.AlertFingerprint == fp) {
			out = append(out, a)
		}
	}
	return out, nil
}

func (m *memoryStore) ListNotificationChannels(_ context.Context, orgID uuid.UUID) ([]models.NotificationChannel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.NotificationChannel{}
	for _, c := range m.channels {
		if c.OrganizationID == orgID {
			out = append(out, c)
		}
	}
	return out, nil
}

func (m *memoryStore) GetNotificationChannel(_ context.Context, orgID, id uuid.UUID) (models.NotificationChannel, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	c, ok := m.channels[id]
	if !ok || c.OrganizationID != orgID {
		return c, repository.ErrNotFound
	}
	return c, nil
}

func (m *memoryStore) CreateNotificationChannel(_ context.Context, ch models.NotificationChannel) (models.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	ch.ID = uuid.New()
	ch.CreatedAt = time.Now().UTC()
	m.channels[ch.ID] = ch
	return ch, nil
}

func (m *memoryStore) UpdateNotificationChannel(_ context.Context, ch models.NotificationChannel) (models.NotificationChannel, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.channels[ch.ID] = ch
	return ch, nil
}

func (m *memoryStore) UpdateNotificationChannelTestStatus(_ context.Context, _, id uuid.UUID, status string, testedAt time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	if ch, ok := m.channels[id]; ok {
		ch.LastStatus = status
		ch.LastTestedAt = &testedAt
		m.channels[id] = ch
	}
	return nil
}

func (m *memoryStore) DeleteNotificationChannel(_ context.Context, _, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	delete(m.channels, id)
	return nil
}

func (m *memoryStore) ListNotifications(_ context.Context, orgID uuid.UUID, _ string, _ bool, _, _ int) ([]models.NotificationRecord, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.NotificationRecord{}
	for _, n := range m.notifs {
		if n.OrganizationID == orgID {
			out = append(out, n)
		}
	}
	return out, len(out), nil
}

func (m *memoryStore) CreateNotification(_ context.Context, rec models.NotificationRecord) (models.NotificationRecord, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	rec.ID = uuid.New()
	rec.AttemptedAt = time.Now().UTC()
	m.notifs[rec.ID] = rec
	return rec, nil
}

func (m *memoryStore) MarkNotificationRead(_ context.Context, orgID, id uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	n, ok := m.notifs[id]
	if !ok || n.OrganizationID != orgID {
		return repository.ErrNotFound
	}
	n.IsRead = true
	m.notifs[id] = n
	return nil
}

func (m *memoryStore) MarkAllNotificationsRead(_ context.Context, orgID uuid.UUID) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for k, n := range m.notifs {
		if n.OrganizationID == orgID {
			n.IsRead = true
			m.notifs[k] = n
		}
	}
	return nil
}

func (m *memoryStore) InsertAuditLog(_ context.Context, orgID uuid.UUID, actorUserID *uuid.UUID, action, resourceType, resourceID, sourceIP string, metadata map[string]any) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.audits = append(m.audits, models.AuditLog{
		ID: uuid.New(), OrganizationID: orgID, ActorUserID: actorUserID,
		Action: action, ResourceType: resourceType, ResourceID: resourceID,
		SourceIP: sourceIP, Metadata: metadata, CreatedAt: time.Now().UTC(),
	})
	return nil
}

func (m *memoryStore) ListAuditLogs(_ context.Context, orgID uuid.UUID, _, _ string, _, _ int) ([]models.AuditLog, int, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.AuditLog{}
	for _, a := range m.audits {
		if a.OrganizationID == orgID {
			out = append(out, a)
		}
	}
	return out, len(out), nil
}

func (m *memoryStore) RecordQueryHistory(_ context.Context, orgID, userID uuid.UUID, metricName, query, timeRange string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.queries = append(m.queries, models.QueryHistoryItem{ID: uuid.New(), OrganizationID: orgID, UserID: userID, MetricName: metricName, Query: query, TimeRange: timeRange, CreatedAt: time.Now().UTC()})
	return nil
}

func (m *memoryStore) ListQueryHistory(_ context.Context, orgID, userID uuid.UUID, _ int) ([]models.QueryHistoryItem, error) {
	m.mu.RLock()
	defer m.mu.RUnlock()
	out := []models.QueryHistoryItem{}
	for _, q := range m.queries {
		if q.OrganizationID == orgID && q.UserID == userID {
			out = append(out, q)
		}
	}
	return out, nil
}

// Mock Prometheus & Alertmanager clients
type mockPromClient struct {
	offline bool
	samples map[string][]prom.Sample
	alerts  []prom.PromAlert
}

func (m *mockPromClient) Healthy(context.Context) error {
	if m.offline {
		return errors.New("prometheus unreachable")
	}
	return nil
}
func (m *mockPromClient) Reload(context.Context) error { return nil }
func (m *mockPromClient) Targets(context.Context) ([]prom.Target, error) {
	if m.offline {
		return nil, errors.New("prometheus unreachable")
	}
	return nil, nil
}
func (m *mockPromClient) Alerts(context.Context) ([]prom.PromAlert, error) {
	if m.offline {
		return nil, errors.New("prometheus unreachable")
	}
	return m.alerts, nil
}
func (m *mockPromClient) QueryInstant(_ context.Context, q string, _ time.Time) ([]prom.Sample, error) {
	if m.offline {
		return nil, errors.New("prometheus unreachable")
	}
	for k, v := range m.samples {
		if strings.Contains(q, k) {
			return v, nil
		}
	}
	return nil, nil
}
func (m *mockPromClient) QueryRange(_ context.Context, _ string, start, end time.Time, _ time.Duration) ([]prom.MatrixSeries, error) {
	if m.offline {
		return nil, errors.New("prometheus unreachable")
	}
	return []prom.MatrixSeries{
		{
			Metric: map[string]string{"server_name": "api-1"},
			Points: []prom.DataPoint{
				{Timestamp: start, Value: 35.5},
				{Timestamp: end, Value: 41.2},
			},
		},
	}, nil
}

type mockAMClient struct {
	alerts    []alertmanager.Alert
	testCalls int
}

func (m *mockAMClient) Healthy(context.Context) error                        { return nil }
func (m *mockAMClient) Alerts(context.Context) ([]alertmanager.Alert, error) { return m.alerts, nil }
func (m *mockAMClient) SendTestAlert(context.Context, string, string) error {
	m.testCalls++
	return nil
}

func setupTestAPI(t *testing.T) (http.Handler, *memoryStore, *mockPromClient, *mockAMClient) {
	t.Helper()
	store := newMemoryStore()
	promMock := &mockPromClient{samples: map[string][]prom.Sample{}}
	amMock := &mockAMClient{}
	dir := t.TempDir()
	logger := slog.New(slog.NewTextHandler(io.Discard, nil))

	cfg := config.Config{
		AppEnv:               "development",
		AppPort:              "8080",
		DatabaseURL:          "postgres://test",
		JWTSecret:            "unit-test-secret-key-at-least-32-bytes-long",
		JWTAccessTTL:         15 * time.Minute,
		JWTRefreshTTL:        24 * time.Hour,
		PrometheusURL:        "http://prometheus:9090",
		PrometheusFileSDPath: filepath.Join(dir, "targets.json"),
		PrometheusRulesDir:   filepath.Join(dir, "rules"),
		GrafanaURL:           "http://grafana:3000",
		GrafanaPublicURL:     "http://localhost:3001",
		AlertmanagerURL:      "http://alertmanager:9093",
		CORSAllowedOrigins:   []string{"http://localhost:5173"},
		Version:              "1.0.0-test",
	}

	tokens := auth.NewTokenManager(cfg.JWTSecret, cfg.JWTAccessTTL, cfg.JWTRefreshTTL)
	disc := discovery.NewManager(cfg.PrometheusFileSDPath, cfg.PrometheusRulesDir, false, promMock, logger)
	gf := grafana.NewClient(cfg.GrafanaURL, cfg.GrafanaPublicURL, time.Second)
	aud := audit.NewRecorder(store, logger)
	mon := services.NewMonitoringService(store, promMock, amMock, gf, disc, logger)
	api := handlers.NewAPI(cfg, store, tokens, mon, promMock, amMock, gf, aud, logger)
	return api.Router(), store, promMock, amMock
}

func doJSON(t *testing.T, h http.Handler, method, path, token string, body any) *httptest.ResponseRecorder {
	t.Helper()
	var rdr io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		require.NoError(t, err)
		rdr = bytes.NewReader(b)
	}
	req := httptest.NewRequest(method, path, rdr)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	rec := httptest.NewRecorder()
	h.ServeHTTP(rec, req)
	return rec
}

func TestEndToEndBackendWorkflows(t *testing.T) {
	router, _, promMock, amMock := setupTestAPI(t)

	// 1. Register Organization A Admin
	regRec := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"name":              "Alice Admin",
		"email":             "alice@org-a.io",
		"password":          "StrongPass123",
		"organization_name": "Org Alpha",
	})
	require.Equal(t, http.StatusCreated, regRec.Code)
	var regResp struct {
		Success bool `json:"success"`
		Data    struct {
			AccessToken string `json:"access_token"`
			CSRFToken   string `json:"csrf_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(regRec.Body.Bytes(), &regResp))
	adminToken := regResp.Data.AccessToken
	require.NotEmpty(t, adminToken)

	// Verify refresh cookie + CSRF rotation works and CSRF is enforced
	cookies := regRec.Result().Cookies()
	reqNoCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	for _, c := range cookies {
		reqNoCSRF.AddCookie(c)
	}
	recNoCSRF := httptest.NewRecorder()
	router.ServeHTTP(recNoCSRF, reqNoCSRF)
	assert.Equal(t, http.StatusForbidden, recNoCSRF.Code)

	reqWithCSRF := httptest.NewRequest(http.MethodPost, "/api/v1/auth/refresh", nil)
	for _, c := range cookies {
		reqWithCSRF.AddCookie(c)
	}
	reqWithCSRF.Header.Set(middleware.CSRFHeaderName, regResp.Data.CSRFToken)
	recRefresh := httptest.NewRecorder()
	router.ServeHTTP(recRefresh, reqWithCSRF)
	assert.Equal(t, http.StatusOK, recRefresh.Code)

	// 2. Login failure returns generic error
	badLogin := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email":    "alice@org-a.io",
		"password": "WrongPassword1",
	})
	assert.Equal(t, http.StatusUnauthorized, badLogin.Code)
	assert.Contains(t, badLogin.Body.String(), "Invalid email or password")

	// 3. Create Viewer user & verify RBAC enforcement
	createViewer := doJSON(t, router, http.MethodPost, "/api/v1/users", adminToken, map[string]any{
		"name":     "Victor Viewer",
		"email":    "viewer@org-a.io",
		"password": "ViewerPass123",
		"role":     "viewer",
	})
	require.Equal(t, http.StatusCreated, createViewer.Code)

	viewerLogin := doJSON(t, router, http.MethodPost, "/api/v1/auth/login", "", map[string]any{
		"email":    "viewer@org-a.io",
		"password": "ViewerPass123",
	})
	require.Equal(t, http.StatusOK, viewerLogin.Code)
	var vResp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(viewerLogin.Body.Bytes(), &vResp))
	viewerToken := vResp.Data.AccessToken

	// Viewer MUST be forbidden from registering servers or creating notification channels
	viewerCreateSrv := doJSON(t, router, http.MethodPost, "/api/v1/servers", viewerToken, map[string]any{
		"name": "blocked", "hostname": "blocked.local", "environment": "production", "scrape_address": "10.0.0.2:9180",
	})
	assert.Equal(t, http.StatusForbidden, viewerCreateSrv.Code)

	// 4. Admin registers a server -> MUST initially be PENDING (never UP)
	srvRec := doJSON(t, router, http.MethodPost, "/api/v1/servers", adminToken, map[string]any{
		"name":           "prod-api-01",
		"hostname":       "prod-api-01.internal",
		"environment":    "production",
		"scrape_address": "10.0.0.10:9180",
		"server_group":   "api",
	})
	require.Equal(t, http.StatusCreated, srvRec.Code)
	var srvResp struct {
		Data models.Server `json:"data"`
	}
	require.NoError(t, json.Unmarshal(srvRec.Body.Bytes(), &srvResp))
	assert.Equal(t, models.StatusPending, srvResp.Data.Status)
	serverID := srvResp.Data.ID.String()

	// 5. Organization Isolation: Register Org B Admin and verify they CANNOT access Org A's server
	regB := doJSON(t, router, http.MethodPost, "/api/v1/auth/register", "", map[string]any{
		"name": "Bob Beta", "email": "bob@org-b.io", "password": "StrongPass123", "organization_name": "Org Beta",
	})
	require.Equal(t, http.StatusCreated, regB.Code)
	var regBResp struct {
		Data struct {
			AccessToken string `json:"access_token"`
		} `json:"data"`
	}
	require.NoError(t, json.Unmarshal(regB.Body.Bytes(), &regBResp))

	crossOrg := doJSON(t, router, http.MethodGet, "/api/v1/servers/"+serverID, regBResp.Data.AccessToken, nil)
	assert.Equal(t, http.StatusNotFound, crossOrg.Code)

	// 6. Prometheus integration: live metrics transition server to UP and populate Overview KPIs
	now := time.Now().UTC()
	promMock.samples["up"] = []prom.Sample{{Metric: map[string]string{"cloudpulse_server_id": serverID}, Value: 1, Timestamp: now}}
	promMock.samples["cloudpulse_cpu_usage_percent"] = []prom.Sample{{Metric: map[string]string{"cloudpulse_server_id": serverID}, Value: 44.5, Timestamp: now}}
	promMock.samples["cloudpulse_memory_usage_percent"] = []prom.Sample{{Metric: map[string]string{"cloudpulse_server_id": serverID}, Value: 61.2, Timestamp: now}}

	overviewRec := doJSON(t, router, http.MethodGet, "/api/v1/monitoring/overview?range=1h", adminToken, nil)
	require.Equal(t, http.StatusOK, overviewRec.Code)
	assert.Contains(t, overviewRec.Body.String(), `"prometheus_available":true`)
	assert.Contains(t, overviewRec.Body.String(), `"average_percent":44.5`)

	// 7. Prometheus outage must clearly report unavailable, NOT fake metrics
	promMock.offline = true
	outageRec := doJSON(t, router, http.MethodGet, "/api/v1/monitoring/overview", adminToken, nil)
	require.Equal(t, http.StatusOK, outageRec.Code)
	assert.Contains(t, outageRec.Body.String(), `"prometheus_available":false`)
	assert.Contains(t, outageRec.Body.String(), `"data_source":"unavailable"`)
	promMock.offline = false

	// 8. Alert Acknowledgement persisted without silencing Prometheus
	amMock.alerts = []alertmanager.Alert{
		{
			Fingerprint: "fp-cpu-high-01",
			Labels:      map[string]string{"alertname": "CPUUsageWarning", "severity": "warning", "server_name": "prod-api-01", "cloudpulse_server_id": serverID},
			Annotations: map[string]string{"summary": "High CPU on prod-api-01"},
			StartsAt:    now.Add(-10 * time.Minute),
			UpdatedAt:   now,
		},
	}
	ackRec := doJSON(t, router, http.MethodPost, "/api/v1/alerts/fp-cpu-high-01/acknowledge", adminToken, map[string]any{
		"note": "Investigating traffic spike",
	})
	require.Equal(t, http.StatusCreated, ackRec.Code)

	alertsRec := doJSON(t, router, http.MethodGet, "/api/v1/alerts", adminToken, nil)
	require.Equal(t, http.StatusOK, alertsRec.Code)
	assert.Contains(t, alertsRec.Body.String(), `"acknowledged":true`)
	assert.Contains(t, alertsRec.Body.String(), `"Investigating traffic spike"`)

	// 9. Notification channel SSRF validation & test dispatch through Alertmanager
	badCh := doJSON(t, router, http.MethodPost, "/api/v1/notification-channels", adminToken, map[string]any{
		"name": "SSRF Attempt", "channel_type": "webhook", "configuration_reference": "http://127.0.0.1:9090/internal",
	})
	assert.Equal(t, http.StatusBadRequest, badCh.Code)

	goodCh := doJSON(t, router, http.MethodPost, "/api/v1/notification-channels", adminToken, map[string]any{
		"name": "Ops Slack", "channel_type": "slack", "configuration_reference": "env:SLACK_WEBHOOK_URL", "target_summary": "#infra-alerts",
	})
	require.Equal(t, http.StatusCreated, goodCh.Code)
	var chResp struct {
		Data models.NotificationChannel `json:"data"`
	}
	require.NoError(t, json.Unmarshal(goodCh.Body.Bytes(), &chResp))

	testCh := doJSON(t, router, http.MethodPost, "/api/v1/notification-channels/"+chResp.Data.ID.String()+"/test", adminToken, nil)
	require.Equal(t, http.StatusOK, testCh.Code)
	assert.Equal(t, 1, amMock.testCalls)

	// 10. Pagination check
	pageRec := doJSON(t, router, http.MethodGet, "/api/v1/servers?page=1&page_size=1", adminToken, nil)
	require.Equal(t, http.StatusOK, pageRec.Code)
	assert.Contains(t, pageRec.Body.String(), `"page_size":1`)
}

func TestCSVFormulaInjectionProtectionAndChannelValidator(t *testing.T) {
	csvBytes, err := reports.ExportCSV([]string{"Name", "Note"}, [][]string{
		{"=CMD|' /C calc'!A0", "+1234"},
		{"normal-server", "@SUM(A1:A2)"},
	})
	require.NoError(t, err)
	out := string(csvBytes)
	assert.Contains(t, out, "'=CMD")
	assert.Contains(t, out, "'+1234")
	assert.Contains(t, out, "'@SUM")

	assert.Error(t, notifications.ValidateChannelConfig("webhook", "https://169.254.169.254/latest/meta-data", ""))
	assert.Error(t, notifications.ValidateChannelConfig("webhook", "https://user:pass@hooks.example.com/abc", ""))
	assert.NoError(t, notifications.ValidateChannelConfig("slack", "env:SLACK_WEBHOOK_URL", "#alerts"))
	assert.NoError(t, notifications.ValidateChannelConfig("email", "ops@example.com", "ops@example.com"))
}
