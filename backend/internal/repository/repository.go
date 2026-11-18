package repository

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/cloudpulse/cloudpulse-go/backend/internal/models"
)

var (
	ErrNotFound = errors.New("resource not found")
	ErrConflict = errors.New("resource already exists")
)

// ServerFilter controls listing/pagination/filtering of servers.
type ServerFilter struct {
	Search      string
	Environment string
	ServerGroup string
	Page        int
	PageSize    int
	SortBy      string
	SortDir     string
}

// Store defines all PostgreSQL persistence operations (enabling unit tests with mocks).
type Store interface {
	Ping(ctx context.Context) error

	// Auth & Users & Orgs
	CreateOrgAndAdmin(ctx context.Context, orgName, userName, email, passwordHash string) (models.Organization, models.User, error)
	GetUserByEmail(ctx context.Context, email string) (models.User, error)
	GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error)
	ListUsers(ctx context.Context, orgID uuid.UUID) ([]models.User, error)
	CreateUser(ctx context.Context, orgID uuid.UUID, name, email, passwordHash, role string) (models.User, error)
	UpdateUser(ctx context.Context, orgID, userID uuid.UUID, name, role string, isActive *bool) (models.User, error)
	UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error

	CreateRefreshSession(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshSession, error)
	GetRefreshSessionByHash(ctx context.Context, tokenHash string) (models.RefreshSession, error)
	RevokeRefreshSession(ctx context.Context, tokenHash string) error
	RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error

	CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error
	ConsumePasswordResetToken(ctx context.Context, tokenHash string) (uuid.UUID, error)

	GetOrganization(ctx context.Context, orgID uuid.UUID) (models.Organization, error)
	UpdateOrganization(ctx context.Context, org models.Organization) (models.Organization, error)

	// Servers
	ListServers(ctx context.Context, orgID uuid.UUID, f ServerFilter) ([]models.Server, int, error)
	ListAllEnabledServers(ctx context.Context) ([]models.Server, error)
	GetServer(ctx context.Context, orgID, serverID uuid.UUID) (models.Server, error)
	CreateServer(ctx context.Context, srv models.Server) (models.Server, error)
	UpdateServer(ctx context.Context, srv models.Server) (models.Server, error)
	SetServerMonitoring(ctx context.Context, orgID, serverID uuid.UUID, enabled bool) (models.Server, error)
	UpdateServerHeartbeat(ctx context.Context, serverID uuid.UUID, agentVersion, osName, arch string, seenAt time.Time) error
	DeleteServer(ctx context.Context, orgID, serverID uuid.UUID) error

	// Services
	ListServiceChecks(ctx context.Context, orgID uuid.UUID, serverID *uuid.UUID) ([]models.ServiceCheck, error)
	GetServiceCheck(ctx context.Context, orgID, id uuid.UUID) (models.ServiceCheck, error)
	CreateServiceCheck(ctx context.Context, orgID uuid.UUID, sc models.ServiceCheck) (models.ServiceCheck, error)
	UpdateServiceCheck(ctx context.Context, orgID uuid.UUID, sc models.ServiceCheck) (models.ServiceCheck, error)
	DeleteServiceCheck(ctx context.Context, orgID, id uuid.UUID) error

	// Alert Rules & Acknowledgements
	ListAlertRules(ctx context.Context, orgID uuid.UUID) ([]models.AlertRule, error)
	GetAlertRule(ctx context.Context, orgID, id uuid.UUID) (models.AlertRule, error)
	CreateAlertRule(ctx context.Context, r models.AlertRule) (models.AlertRule, error)
	UpdateAlertRule(ctx context.Context, r models.AlertRule) (models.AlertRule, error)
	DeleteAlertRule(ctx context.Context, orgID, id uuid.UUID) error
	SeedDefaultAlertRules(ctx context.Context, orgID uuid.UUID, createdBy *uuid.UUID) error

	AcknowledgeAlert(ctx context.Context, orgID uuid.UUID, fingerprint string, userID uuid.UUID, note string) (models.AlertAcknowledgement, error)
	ListAcknowledgements(ctx context.Context, orgID uuid.UUID) (map[string]models.AlertAcknowledgement, error)
	ListAcknowledgementHistory(ctx context.Context, orgID uuid.UUID, fingerprint string) ([]models.AlertAcknowledgement, error)

	// Notification Channels & History
	ListNotificationChannels(ctx context.Context, orgID uuid.UUID) ([]models.NotificationChannel, error)
	GetNotificationChannel(ctx context.Context, orgID, id uuid.UUID) (models.NotificationChannel, error)
	CreateNotificationChannel(ctx context.Context, ch models.NotificationChannel) (models.NotificationChannel, error)
	UpdateNotificationChannel(ctx context.Context, ch models.NotificationChannel) (models.NotificationChannel, error)
	UpdateNotificationChannelTestStatus(ctx context.Context, orgID, id uuid.UUID, status string, testedAt time.Time) error
	DeleteNotificationChannel(ctx context.Context, orgID, id uuid.UUID) error

	ListNotifications(ctx context.Context, orgID uuid.UUID, severity string, unreadOnly bool, page, pageSize int) ([]models.NotificationRecord, int, error)
	CreateNotification(ctx context.Context, rec models.NotificationRecord) (models.NotificationRecord, error)
	MarkNotificationRead(ctx context.Context, orgID, id uuid.UUID) error
	MarkAllNotificationsRead(ctx context.Context, orgID uuid.UUID) error

	// Audit Logs & Query History
	InsertAuditLog(ctx context.Context, orgID uuid.UUID, actorUserID *uuid.UUID, action, resourceType, resourceID, sourceIP string, metadata map[string]any) error
	ListAuditLogs(ctx context.Context, orgID uuid.UUID, action, resourceType string, page, pageSize int) ([]models.AuditLog, int, error)
	RecordQueryHistory(ctx context.Context, orgID, userID uuid.UUID, metricName, query, timeRange string) error
	ListQueryHistory(ctx context.Context, orgID, userID uuid.UUID, limit int) ([]models.QueryHistoryItem, error)
}

// PGStore implements Store backed by PostgreSQL via pgxpool.
type PGStore struct {
	pool *pgxpool.Pool
}

func NewPGStore(pool *pgxpool.Pool) *PGStore {
	return &PGStore{pool: pool}
}

func (s *PGStore) Ping(ctx context.Context) error {
	return s.pool.Ping(ctx)
}

func (s *PGStore) CreateOrgAndAdmin(ctx context.Context, orgName, userName, email, passwordHash string) (models.Organization, models.User, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return models.Organization{}, models.User{}, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	var org models.Organization
	err = tx.QueryRow(ctx, `
		INSERT INTO organizations (name)
		VALUES ($1)
		RETURNING id, name, description, timezone, display_units, date_format, time_format,
			default_scrape_interval, default_monitoring_timeout, agent_connection_timeout,
			disk_exclusion_patterns, default_environment, onboarding_completed, created_at, updated_at
	`, orgName).Scan(
		&org.ID, &org.Name, &org.Description, &org.Timezone, &org.DisplayUnits, &org.DateFormat, &org.TimeFormat,
		&org.DefaultScrapeInterval, &org.DefaultMonitoringTimeout, &org.AgentConnectionTimeout,
		&org.DiskExclusionPatterns, &org.DefaultEnvironment, &org.OnboardingCompleted, &org.CreatedAt, &org.UpdatedAt,
	)
	if err != nil {
		return org, models.User{}, err
	}

	var user models.User
	err = tx.QueryRow(ctx, `
		INSERT INTO users (organization_id, name, email, password_hash, role)
		VALUES ($1, $2, LOWER($3), $4, 'admin')
		RETURNING id, organization_id, name, email, password_hash, role, is_active, created_at, updated_at
	`, org.ID, userName, email, passwordHash).Scan(
		&user.ID, &user.OrganizationID, &user.Name, &user.Email, &user.PasswordHash, &user.Role, &user.IsActive, &user.CreatedAt, &user.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return org, user, ErrConflict
		}
		return org, user, err
	}

	if err := seedDefaultRulesTx(ctx, tx, org.ID, &user.ID); err != nil {
		return org, user, err
	}

	if err := tx.Commit(ctx); err != nil {
		return org, user, err
	}
	return org, user, nil
}

func (s *PGStore) GetUserByEmail(ctx context.Context, email string) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, email, password_hash, role, is_active, created_at, updated_at
		FROM users WHERE LOWER(email) = LOWER($1)
	`, email).Scan(&u.ID, &u.OrganizationID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *PGStore) GetUserByID(ctx context.Context, id uuid.UUID) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, email, password_hash, role, is_active, created_at, updated_at
		FROM users WHERE id = $1
	`, id).Scan(&u.ID, &u.OrganizationID, &u.Name, &u.Email, &u.PasswordHash, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *PGStore) ListUsers(ctx context.Context, orgID uuid.UUID) ([]models.User, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, email, role, is_active, created_at, updated_at
		FROM users WHERE organization_id = $1 ORDER BY created_at ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var users []models.User
	for rows.Next() {
		var u models.User
		if err := rows.Scan(&u.ID, &u.OrganizationID, &u.Name, &u.Email, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt); err != nil {
			return nil, err
		}
		users = append(users, u)
	}
	return users, rows.Err()
}

func (s *PGStore) CreateUser(ctx context.Context, orgID uuid.UUID, name, email, passwordHash, role string) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, `
		INSERT INTO users (organization_id, name, email, password_hash, role)
		VALUES ($1, $2, LOWER($3), $4, $5)
		RETURNING id, organization_id, name, email, role, is_active, created_at, updated_at
	`, orgID, name, email, passwordHash, role).Scan(
		&u.ID, &u.OrganizationID, &u.Name, &u.Email, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return u, ErrConflict
	}
	return u, err
}

func (s *PGStore) UpdateUser(ctx context.Context, orgID, userID uuid.UUID, name, role string, isActive *bool) (models.User, error) {
	var u models.User
	err := s.pool.QueryRow(ctx, `
		UPDATE users
		SET name = COALESCE(NULLIF($3, ''), name),
		    role = COALESCE(NULLIF($4, ''), role),
		    is_active = COALESCE($5, is_active),
		    updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, email, role, is_active, created_at, updated_at
	`, userID, orgID, name, role, isActive).Scan(
		&u.ID, &u.OrganizationID, &u.Name, &u.Email, &u.Role, &u.IsActive, &u.CreatedAt, &u.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return u, ErrNotFound
	}
	return u, err
}

func (s *PGStore) UpdateUserPassword(ctx context.Context, userID uuid.UUID, passwordHash string) error {
	ct, err := s.pool.Exec(ctx, `UPDATE users SET password_hash = $2, updated_at = NOW() WHERE id = $1`, userID, passwordHash)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) CreateRefreshSession(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) (models.RefreshSession, error) {
	var rs models.RefreshSession
	err := s.pool.QueryRow(ctx, `
		INSERT INTO refresh_sessions (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
		RETURNING id, user_id, token_hash, expires_at, revoked_at, created_at
	`, userID, tokenHash, expiresAt).Scan(&rs.ID, &rs.UserID, &rs.TokenHash, &rs.ExpiresAt, &rs.RevokedAt, &rs.CreatedAt)
	return rs, err
}

func (s *PGStore) GetRefreshSessionByHash(ctx context.Context, tokenHash string) (models.RefreshSession, error) {
	var rs models.RefreshSession
	err := s.pool.QueryRow(ctx, `
		SELECT id, user_id, token_hash, expires_at, revoked_at, created_at
		FROM refresh_sessions WHERE token_hash = $1
	`, tokenHash).Scan(&rs.ID, &rs.UserID, &rs.TokenHash, &rs.ExpiresAt, &rs.RevokedAt, &rs.CreatedAt)
	if errors.Is(err, pgx.ErrNoRows) {
		return rs, ErrNotFound
	}
	return rs, err
}

func (s *PGStore) RevokeRefreshSession(ctx context.Context, tokenHash string) error {
	_, err := s.pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = NOW() WHERE token_hash = $1 AND revoked_at IS NULL`, tokenHash)
	return err
}

func (s *PGStore) RevokeAllUserSessions(ctx context.Context, userID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE refresh_sessions SET revoked_at = NOW() WHERE user_id = $1 AND revoked_at IS NULL`, userID)
	return err
}

func (s *PGStore) CreatePasswordResetToken(ctx context.Context, userID uuid.UUID, tokenHash string, expiresAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO password_reset_tokens (user_id, token_hash, expires_at)
		VALUES ($1, $2, $3)
	`, userID, tokenHash, expiresAt)
	return err
}

func (s *PGStore) ConsumePasswordResetToken(ctx context.Context, tokenHash string) (uuid.UUID, error) {
	var userID uuid.UUID
	err := s.pool.QueryRow(ctx, `
		UPDATE password_reset_tokens
		SET used_at = NOW()
		WHERE token_hash = $1 AND used_at IS NULL AND expires_at > NOW()
		RETURNING user_id
	`, tokenHash).Scan(&userID)
	if errors.Is(err, pgx.ErrNoRows) {
		return uuid.Nil, ErrNotFound
	}
	return userID, err
}

func (s *PGStore) GetOrganization(ctx context.Context, orgID uuid.UUID) (models.Organization, error) {
	var org models.Organization
	err := s.pool.QueryRow(ctx, `
		SELECT id, name, description, timezone, display_units, date_format, time_format,
			default_scrape_interval, default_monitoring_timeout, agent_connection_timeout,
			disk_exclusion_patterns, default_environment, onboarding_completed, created_at, updated_at
		FROM organizations WHERE id = $1
	`, orgID).Scan(
		&org.ID, &org.Name, &org.Description, &org.Timezone, &org.DisplayUnits, &org.DateFormat, &org.TimeFormat,
		&org.DefaultScrapeInterval, &org.DefaultMonitoringTimeout, &org.AgentConnectionTimeout,
		&org.DiskExclusionPatterns, &org.DefaultEnvironment, &org.OnboardingCompleted, &org.CreatedAt, &org.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return org, ErrNotFound
	}
	return org, err
}

func (s *PGStore) UpdateOrganization(ctx context.Context, org models.Organization) (models.Organization, error) {
	var out models.Organization
	err := s.pool.QueryRow(ctx, `
		UPDATE organizations
		SET name = $2, description = $3, timezone = $4, display_units = $5,
		    date_format = $6, time_format = $7, default_scrape_interval = $8,
		    default_monitoring_timeout = $9, agent_connection_timeout = $10,
		    disk_exclusion_patterns = $11, default_environment = $12,
		    onboarding_completed = $13, updated_at = NOW()
		WHERE id = $1
		RETURNING id, name, description, timezone, display_units, date_format, time_format,
			default_scrape_interval, default_monitoring_timeout, agent_connection_timeout,
			disk_exclusion_patterns, default_environment, onboarding_completed, created_at, updated_at
	`, org.ID, org.Name, org.Description, org.Timezone, org.DisplayUnits, org.DateFormat, org.TimeFormat,
		org.DefaultScrapeInterval, org.DefaultMonitoringTimeout, org.AgentConnectionTimeout,
		org.DiskExclusionPatterns, org.DefaultEnvironment, org.OnboardingCompleted,
	).Scan(
		&out.ID, &out.Name, &out.Description, &out.Timezone, &out.DisplayUnits, &out.DateFormat, &out.TimeFormat,
		&out.DefaultScrapeInterval, &out.DefaultMonitoringTimeout, &out.AgentConnectionTimeout,
		&out.DiskExclusionPatterns, &out.DefaultEnvironment, &out.OnboardingCompleted, &out.CreatedAt, &out.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return out, ErrNotFound
	}
	return out, err
}

func (s *PGStore) ListServers(ctx context.Context, orgID uuid.UUID, f ServerFilter) ([]models.Server, int, error) {
	if f.Page < 1 {
		f.Page = 1
	}
	if f.PageSize < 1 || f.PageSize > 100 {
		f.PageSize = 25
	}

	conds := []string{"organization_id = $1"}
	args := []any{orgID}
	argIdx := 2

	if strings.TrimSpace(f.Search) != "" {
		conds = append(conds, fmt.Sprintf("(name ILIKE $%d OR hostname ILIKE $%d OR scrape_address ILIKE $%d OR server_group ILIKE $%d)", argIdx, argIdx, argIdx, argIdx))
		args = append(args, "%"+strings.TrimSpace(f.Search)+"%")
		argIdx++
	}
	if strings.TrimSpace(f.Environment) != "" {
		conds = append(conds, fmt.Sprintf("environment = $%d", argIdx))
		args = append(args, strings.ToLower(strings.TrimSpace(f.Environment)))
		argIdx++
	}
	if strings.TrimSpace(f.ServerGroup) != "" {
		conds = append(conds, fmt.Sprintf("server_group = $%d", argIdx))
		args = append(args, strings.TrimSpace(f.ServerGroup))
		argIdx++
	}

	where := strings.Join(conds, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM servers WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	orderCol := "created_at"
	switch f.SortBy {
	case "name", "hostname", "environment", "server_group", "last_seen_at", "created_at":
		orderCol = f.SortBy
	}
	orderDir := "DESC"
	if strings.EqualFold(f.SortDir, "asc") {
		orderDir = "ASC"
	}

	query := fmt.Sprintf(`
		SELECT id, organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, agent_version,
			monitoring_enabled, last_seen_at, created_at, updated_at
		FROM servers WHERE %s
		ORDER BY %s %s
		LIMIT $%d OFFSET $%d
	`, where, orderCol, orderDir, argIdx, argIdx+1)
	args = append(args, f.PageSize, (f.Page-1)*f.PageSize)

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	servers := make([]models.Server, 0)
	for rows.Next() {
		var srv models.Server
		if err := rows.Scan(
			&srv.ID, &srv.OrganizationID, &srv.Name, &srv.Hostname, &srv.Environment, &srv.OperatingSystem,
			&srv.Architecture, &srv.Description, &srv.ServerGroup, &srv.ScrapeAddress, &srv.MonitoringInterval,
			&srv.AgentVersion, &srv.MonitoringEnabled, &srv.LastSeenAt, &srv.CreatedAt, &srv.UpdatedAt,
		); err != nil {
			return nil, 0, err
		}
		srv.Tags = []models.ServerTag{}
		servers = append(servers, srv)
	}
	if err := rows.Err(); err != nil {
		return nil, 0, err
	}

	if err := s.attachServerTags(ctx, servers); err != nil {
		return nil, 0, err
	}
	return servers, total, nil
}

func (s *PGStore) ListAllEnabledServers(ctx context.Context) ([]models.Server, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, agent_version,
			monitoring_enabled, last_seen_at, created_at, updated_at
		FROM servers WHERE monitoring_enabled = TRUE ORDER BY created_at ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var servers []models.Server
	for rows.Next() {
		var srv models.Server
		if err := rows.Scan(
			&srv.ID, &srv.OrganizationID, &srv.Name, &srv.Hostname, &srv.Environment, &srv.OperatingSystem,
			&srv.Architecture, &srv.Description, &srv.ServerGroup, &srv.ScrapeAddress, &srv.MonitoringInterval,
			&srv.AgentVersion, &srv.MonitoringEnabled, &srv.LastSeenAt, &srv.CreatedAt, &srv.UpdatedAt,
		); err != nil {
			return nil, err
		}
		servers = append(servers, srv)
	}
	return servers, rows.Err()
}

func (s *PGStore) GetServer(ctx context.Context, orgID, serverID uuid.UUID) (models.Server, error) {
	var srv models.Server
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, agent_version,
			monitoring_enabled, last_seen_at, created_at, updated_at
		FROM servers WHERE id = $1 AND organization_id = $2
	`, serverID, orgID).Scan(
		&srv.ID, &srv.OrganizationID, &srv.Name, &srv.Hostname, &srv.Environment, &srv.OperatingSystem,
		&srv.Architecture, &srv.Description, &srv.ServerGroup, &srv.ScrapeAddress, &srv.MonitoringInterval,
		&srv.AgentVersion, &srv.MonitoringEnabled, &srv.LastSeenAt, &srv.CreatedAt, &srv.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return srv, ErrNotFound
	}
	if err != nil {
		return srv, err
	}
	srv.Tags = []models.ServerTag{}
	slice := []models.Server{srv}
	if err := s.attachServerTags(ctx, slice); err != nil {
		return srv, err
	}
	return slice[0], nil
}

func (s *PGStore) CreateServer(ctx context.Context, srv models.Server) (models.Server, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return srv, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		INSERT INTO servers (organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, monitoring_enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
		RETURNING id, organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, agent_version,
			monitoring_enabled, last_seen_at, created_at, updated_at
	`, srv.OrganizationID, srv.Name, srv.Hostname, srv.Environment, srv.OperatingSystem, srv.Architecture,
		srv.Description, srv.ServerGroup, srv.ScrapeAddress, srv.MonitoringInterval, srv.MonitoringEnabled,
	).Scan(
		&srv.ID, &srv.OrganizationID, &srv.Name, &srv.Hostname, &srv.Environment, &srv.OperatingSystem,
		&srv.Architecture, &srv.Description, &srv.ServerGroup, &srv.ScrapeAddress, &srv.MonitoringInterval,
		&srv.AgentVersion, &srv.MonitoringEnabled, &srv.LastSeenAt, &srv.CreatedAt, &srv.UpdatedAt,
	)
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return srv, ErrConflict
		}
		return srv, err
	}

	for _, tag := range srv.Tags {
		if strings.TrimSpace(tag.Key) == "" {
			continue
		}
		if _, err := tx.Exec(ctx, `
			INSERT INTO server_tags (server_id, tag_key, tag_value)
			VALUES ($1, $2, $3)
			ON CONFLICT (server_id, tag_key) DO UPDATE SET tag_value = EXCLUDED.tag_value
		`, srv.ID, strings.TrimSpace(tag.Key), strings.TrimSpace(tag.Value)); err != nil {
			return srv, err
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return srv, err
	}
	if srv.Tags == nil {
		srv.Tags = []models.ServerTag{}
	}
	return srv, nil
}

func (s *PGStore) UpdateServer(ctx context.Context, srv models.Server) (models.Server, error) {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return srv, err
	}
	defer func() { _ = tx.Rollback(ctx) }()

	err = tx.QueryRow(ctx, `
		UPDATE servers
		SET name = $3, hostname = $4, environment = $5, operating_system = $6, architecture = $7,
		    description = $8, server_group = $9, scrape_address = $10, monitoring_interval = $11,
		    updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, agent_version,
			monitoring_enabled, last_seen_at, created_at, updated_at
	`, srv.ID, srv.OrganizationID, srv.Name, srv.Hostname, srv.Environment, srv.OperatingSystem, srv.Architecture,
		srv.Description, srv.ServerGroup, srv.ScrapeAddress, srv.MonitoringInterval,
	).Scan(
		&srv.ID, &srv.OrganizationID, &srv.Name, &srv.Hostname, &srv.Environment, &srv.OperatingSystem,
		&srv.Architecture, &srv.Description, &srv.ServerGroup, &srv.ScrapeAddress, &srv.MonitoringInterval,
		&srv.AgentVersion, &srv.MonitoringEnabled, &srv.LastSeenAt, &srv.CreatedAt, &srv.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return srv, ErrNotFound
	}
	if err != nil {
		if strings.Contains(err.Error(), "duplicate key") {
			return srv, ErrConflict
		}
		return srv, err
	}

	if srv.Tags != nil {
		if _, err := tx.Exec(ctx, `DELETE FROM server_tags WHERE server_id = $1`, srv.ID); err != nil {
			return srv, err
		}
		for _, tag := range srv.Tags {
			if strings.TrimSpace(tag.Key) == "" {
				continue
			}
			if _, err := tx.Exec(ctx, `
				INSERT INTO server_tags (server_id, tag_key, tag_value) VALUES ($1, $2, $3)
			`, srv.ID, strings.TrimSpace(tag.Key), strings.TrimSpace(tag.Value)); err != nil {
				return srv, err
			}
		}
	}

	if err := tx.Commit(ctx); err != nil {
		return srv, err
	}
	return srv, nil
}

func (s *PGStore) SetServerMonitoring(ctx context.Context, orgID, serverID uuid.UUID, enabled bool) (models.Server, error) {
	var srv models.Server
	err := s.pool.QueryRow(ctx, `
		UPDATE servers SET monitoring_enabled = $3, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, hostname, environment, operating_system, architecture,
			description, server_group, scrape_address, monitoring_interval, agent_version,
			monitoring_enabled, last_seen_at, created_at, updated_at
	`, serverID, orgID, enabled).Scan(
		&srv.ID, &srv.OrganizationID, &srv.Name, &srv.Hostname, &srv.Environment, &srv.OperatingSystem,
		&srv.Architecture, &srv.Description, &srv.ServerGroup, &srv.ScrapeAddress, &srv.MonitoringInterval,
		&srv.AgentVersion, &srv.MonitoringEnabled, &srv.LastSeenAt, &srv.CreatedAt, &srv.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return srv, ErrNotFound
	}
	return srv, err
}

func (s *PGStore) UpdateServerHeartbeat(ctx context.Context, serverID uuid.UUID, agentVersion, osName, arch string, seenAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE servers
		SET last_seen_at = $2,
		    agent_version = COALESCE(NULLIF($3, ''), agent_version),
		    operating_system = COALESCE(NULLIF($4, ''), operating_system),
		    architecture = COALESCE(NULLIF($5, ''), architecture),
		    updated_at = NOW()
		WHERE id = $1
	`, serverID, seenAt, agentVersion, osName, arch)
	return err
}

func (s *PGStore) DeleteServer(ctx context.Context, orgID, serverID uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM servers WHERE id = $1 AND organization_id = $2`, serverID, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) attachServerTags(ctx context.Context, servers []models.Server) error {
	if len(servers) == 0 {
		return nil
	}
	ids := make([]uuid.UUID, len(servers))
	idxByID := make(map[uuid.UUID]int, len(servers))
	for i, srv := range servers {
		ids[i] = srv.ID
		idxByID[srv.ID] = i
	}
	rows, err := s.pool.Query(ctx, `
		SELECT server_id, tag_key, tag_value FROM server_tags WHERE server_id = ANY($1) ORDER BY tag_key ASC
	`, ids)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var sid uuid.UUID
		var k, v string
		if err := rows.Scan(&sid, &k, &v); err != nil {
			return err
		}
		if idx, ok := idxByID[sid]; ok {
			servers[idx].Tags = append(servers[idx].Tags, models.ServerTag{Key: k, Value: v})
		}
	}
	return rows.Err()
}

func (s *PGStore) ListServiceChecks(ctx context.Context, orgID uuid.UUID, serverID *uuid.UUID) ([]models.ServiceCheck, error) {
	query := `
		SELECT sc.id, sc.server_id, srv.name, srv.environment, sc.name, sc.check_type, sc.target,
			sc.interval_seconds, sc.timeout_seconds, sc.expected_status_code, sc.enabled, sc.created_at, sc.updated_at
		FROM service_checks sc
		INNER JOIN servers srv ON srv.id = sc.server_id
		WHERE srv.organization_id = $1
	`
	args := []any{orgID}
	if serverID != nil {
		query += " AND sc.server_id = $2"
		args = append(args, *serverID)
	}
	query += " ORDER BY sc.created_at DESC"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.ServiceCheck, 0)
	for rows.Next() {
		var sc models.ServiceCheck
		if err := rows.Scan(
			&sc.ID, &sc.ServerID, &sc.ServerName, &sc.Environment, &sc.Name, &sc.CheckType, &sc.Target,
			&sc.IntervalSeconds, &sc.TimeoutSeconds, &sc.ExpectedStatusCode, &sc.Enabled, &sc.CreatedAt, &sc.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, sc)
	}
	return out, rows.Err()
}

func (s *PGStore) GetServiceCheck(ctx context.Context, orgID, id uuid.UUID) (models.ServiceCheck, error) {
	var sc models.ServiceCheck
	err := s.pool.QueryRow(ctx, `
		SELECT sc.id, sc.server_id, srv.name, srv.environment, sc.name, sc.check_type, sc.target,
			sc.interval_seconds, sc.timeout_seconds, sc.expected_status_code, sc.enabled, sc.created_at, sc.updated_at
		FROM service_checks sc
		INNER JOIN servers srv ON srv.id = sc.server_id
		WHERE sc.id = $1 AND srv.organization_id = $2
	`, id, orgID).Scan(
		&sc.ID, &sc.ServerID, &sc.ServerName, &sc.Environment, &sc.Name, &sc.CheckType, &sc.Target,
		&sc.IntervalSeconds, &sc.TimeoutSeconds, &sc.ExpectedStatusCode, &sc.Enabled, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	}
	return sc, err
}

func (s *PGStore) CreateServiceCheck(ctx context.Context, orgID uuid.UUID, sc models.ServiceCheck) (models.ServiceCheck, error) {
	// Verify server belongs to organization
	var exists bool
	if err := s.pool.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM servers WHERE id = $1 AND organization_id = $2)`, sc.ServerID, orgID).Scan(&exists); err != nil {
		return sc, err
	}
	if !exists {
		return sc, ErrNotFound
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO service_checks (server_id, name, check_type, target, interval_seconds, timeout_seconds, expected_status_code, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8)
		RETURNING id, server_id, name, check_type, target, interval_seconds, timeout_seconds, expected_status_code, enabled, created_at, updated_at
	`, sc.ServerID, sc.Name, sc.CheckType, sc.Target, sc.IntervalSeconds, sc.TimeoutSeconds, sc.ExpectedStatusCode, sc.Enabled).Scan(
		&sc.ID, &sc.ServerID, &sc.Name, &sc.CheckType, &sc.Target,
		&sc.IntervalSeconds, &sc.TimeoutSeconds, &sc.ExpectedStatusCode, &sc.Enabled, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if err != nil && strings.Contains(err.Error(), "duplicate key") {
		return sc, ErrConflict
	}
	return sc, err
}

func (s *PGStore) UpdateServiceCheck(ctx context.Context, orgID uuid.UUID, sc models.ServiceCheck) (models.ServiceCheck, error) {
	err := s.pool.QueryRow(ctx, `
		UPDATE service_checks sc
		SET name = $3, check_type = $4, target = $5, interval_seconds = $6, timeout_seconds = $7,
		    expected_status_code = $8, enabled = $9, updated_at = NOW()
		FROM servers srv
		WHERE sc.id = $1 AND sc.server_id = srv.id AND srv.organization_id = $2
		RETURNING sc.id, sc.server_id, srv.name, srv.environment, sc.name, sc.check_type, sc.target,
			sc.interval_seconds, sc.timeout_seconds, sc.expected_status_code, sc.enabled, sc.created_at, sc.updated_at
	`, sc.ID, orgID, sc.Name, sc.CheckType, sc.Target, sc.IntervalSeconds, sc.TimeoutSeconds, sc.ExpectedStatusCode, sc.Enabled).Scan(
		&sc.ID, &sc.ServerID, &sc.ServerName, &sc.Environment, &sc.Name, &sc.CheckType, &sc.Target,
		&sc.IntervalSeconds, &sc.TimeoutSeconds, &sc.ExpectedStatusCode, &sc.Enabled, &sc.CreatedAt, &sc.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return sc, ErrNotFound
	}
	return sc, err
}

func (s *PGStore) DeleteServiceCheck(ctx context.Context, orgID, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `
		DELETE FROM service_checks sc USING servers srv
		WHERE sc.id = $1 AND sc.server_id = srv.id AND srv.organization_id = $2
	`, id, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) ListAlertRules(ctx context.Context, orgID uuid.UUID) ([]models.AlertRule, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, description, expression, metric_template, threshold,
			severity, duration, environments, is_default, enabled, last_eval_status, created_by, created_at, updated_at
		FROM alert_rules WHERE organization_id = $1 ORDER BY is_default DESC, created_at ASC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]models.AlertRule, 0)
	for rows.Next() {
		var r models.AlertRule
		if err := rows.Scan(
			&r.ID, &r.OrganizationID, &r.Name, &r.Description, &r.Expression, &r.MetricTemplate, &r.Threshold,
			&r.Severity, &r.Duration, &r.Environments, &r.IsDefault, &r.Enabled, &r.LastEvalStatus,
			&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, r)
	}
	return out, rows.Err()
}

func (s *PGStore) GetAlertRule(ctx context.Context, orgID, id uuid.UUID) (models.AlertRule, error) {
	var r models.AlertRule
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, description, expression, metric_template, threshold,
			severity, duration, environments, is_default, enabled, last_eval_status, created_by, created_at, updated_at
		FROM alert_rules WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(
		&r.ID, &r.OrganizationID, &r.Name, &r.Description, &r.Expression, &r.MetricTemplate, &r.Threshold,
		&r.Severity, &r.Duration, &r.Environments, &r.IsDefault, &r.Enabled, &r.LastEvalStatus,
		&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *PGStore) CreateAlertRule(ctx context.Context, r models.AlertRule) (models.AlertRule, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO alert_rules (organization_id, name, description, expression, metric_template, threshold,
			severity, duration, environments, is_default, enabled, last_eval_status, created_by)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11, $12, $13)
		RETURNING id, organization_id, name, description, expression, metric_template, threshold,
			severity, duration, environments, is_default, enabled, last_eval_status, created_by, created_at, updated_at
	`, r.OrganizationID, r.Name, r.Description, r.Expression, r.MetricTemplate, r.Threshold,
		r.Severity, r.Duration, r.Environments, r.IsDefault, r.Enabled, r.LastEvalStatus, r.CreatedBy,
	).Scan(
		&r.ID, &r.OrganizationID, &r.Name, &r.Description, &r.Expression, &r.MetricTemplate, &r.Threshold,
		&r.Severity, &r.Duration, &r.Environments, &r.IsDefault, &r.Enabled, &r.LastEvalStatus,
		&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	return r, err
}

func (s *PGStore) UpdateAlertRule(ctx context.Context, r models.AlertRule) (models.AlertRule, error) {
	err := s.pool.QueryRow(ctx, `
		UPDATE alert_rules
		SET name = $3, description = $4, expression = $5, metric_template = $6, threshold = $7,
		    severity = $8, duration = $9, environments = $10, enabled = $11, last_eval_status = $12, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, description, expression, metric_template, threshold,
			severity, duration, environments, is_default, enabled, last_eval_status, created_by, created_at, updated_at
	`, r.ID, r.OrganizationID, r.Name, r.Description, r.Expression, r.MetricTemplate, r.Threshold,
		r.Severity, r.Duration, r.Environments, r.Enabled, r.LastEvalStatus,
	).Scan(
		&r.ID, &r.OrganizationID, &r.Name, &r.Description, &r.Expression, &r.MetricTemplate, &r.Threshold,
		&r.Severity, &r.Duration, &r.Environments, &r.IsDefault, &r.Enabled, &r.LastEvalStatus,
		&r.CreatedBy, &r.CreatedAt, &r.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return r, ErrNotFound
	}
	return r, err
}

func (s *PGStore) DeleteAlertRule(ctx context.Context, orgID, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM alert_rules WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) SeedDefaultAlertRules(ctx context.Context, orgID uuid.UUID, createdBy *uuid.UUID) error {
	tx, err := s.pool.Begin(ctx)
	if err != nil {
		return err
	}
	defer func() { _ = tx.Rollback(ctx) }()
	if err := seedDefaultRulesTx(ctx, tx, orgID, createdBy); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func seedDefaultRulesTx(ctx context.Context, tx pgx.Tx, orgID uuid.UUID, createdBy *uuid.UUID) error {
	defaults := []models.AlertRule{
		{Name: "CPU Usage Warning", Description: "Trigger when CPU usage exceeds 75% for 5 minutes", Expression: "cloudpulse_cpu_usage_percent > 75", MetricTemplate: "cpu_usage", Threshold: 75, Severity: "warning", Duration: "5m"},
		{Name: "CPU Usage Critical", Description: "Trigger when CPU usage exceeds 90% for 5 minutes", Expression: "cloudpulse_cpu_usage_percent > 90", MetricTemplate: "cpu_usage", Threshold: 90, Severity: "critical", Duration: "5m"},
		{Name: "Memory Usage Warning", Description: "Trigger when memory usage exceeds 80% for 5 minutes", Expression: "cloudpulse_memory_usage_percent > 80", MetricTemplate: "memory_usage", Threshold: 80, Severity: "warning", Duration: "5m"},
		{Name: "Memory Usage Critical", Description: "Trigger when memory usage exceeds 95% for 5 minutes", Expression: "cloudpulse_memory_usage_percent > 95", MetricTemplate: "memory_usage", Threshold: 95, Severity: "critical", Duration: "5m"},
		{Name: "Disk Usage Warning", Description: "Trigger when any monitored disk exceeds 80% utilization", Expression: "cloudpulse_disk_usage_percent > 80", MetricTemplate: "disk_usage", Threshold: 80, Severity: "warning", Duration: "5m"},
		{Name: "Disk Usage Critical", Description: "Trigger when any monitored disk exceeds 90% utilization", Expression: "cloudpulse_disk_usage_percent > 90", MetricTemplate: "disk_usage", Threshold: 90, Severity: "critical", Duration: "5m"},
		{Name: "Server Down", Description: "Trigger when a registered monitoring target is unreachable", Expression: `up{job="cloudpulse-agents"} == 0`, MetricTemplate: "server_down", Threshold: 0, Severity: "critical", Duration: "1m"},
		{Name: "Service Down", Description: "Trigger when a configured HTTP or TCP service check fails repeatedly", Expression: "cloudpulse_service_up == 0", MetricTemplate: "service_down", Threshold: 0, Severity: "critical", Duration: "2m"},
		{Name: "High Network Errors", Description: "Trigger when network receive or transmit error rate exceeds threshold", Expression: "rate(cloudpulse_network_receive_errors_total[5m]) + rate(cloudpulse_network_transmit_errors_total[5m]) > 5", MetricTemplate: "network_errors", Threshold: 5, Severity: "warning", Duration: "5m"},
	}
	envs := []string{"production", "staging", "development", "testing", "other"}
	for _, r := range defaults {
		if _, err := tx.Exec(ctx, `
			INSERT INTO alert_rules (organization_id, name, description, expression, metric_template, threshold,
				severity, duration, environments, is_default, enabled, last_eval_status, created_by)
			VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, TRUE, TRUE, 'active', $10)
		`, orgID, r.Name, r.Description, r.Expression, r.MetricTemplate, r.Threshold, r.Severity, r.Duration, envs, createdBy); err != nil {
			return err
		}
	}
	return nil
}

func (s *PGStore) AcknowledgeAlert(ctx context.Context, orgID uuid.UUID, fingerprint string, userID uuid.UUID, note string) (models.AlertAcknowledgement, error) {
	var ack models.AlertAcknowledgement
	err := s.pool.QueryRow(ctx, `
		INSERT INTO alert_acknowledgements (organization_id, alert_fingerprint, acknowledged_by, note)
		VALUES ($1, $2, $3, $4)
		RETURNING id, organization_id, alert_fingerprint, acknowledged_by, acknowledged_at, note
	`, orgID, fingerprint, userID, note).Scan(
		&ack.ID, &ack.OrganizationID, &ack.AlertFingerprint, &ack.AcknowledgedBy, &ack.AcknowledgedAt, &ack.Note,
	)
	return ack, err
}

func (s *PGStore) ListAcknowledgements(ctx context.Context, orgID uuid.UUID) (map[string]models.AlertAcknowledgement, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT DISTINCT ON (a.alert_fingerprint)
			a.id, a.organization_id, a.alert_fingerprint, a.acknowledged_by, COALESCE(u.name, ''), a.acknowledged_at, a.note
		FROM alert_acknowledgements a
		LEFT JOIN users u ON u.id = a.acknowledged_by
		WHERE a.organization_id = $1
		ORDER BY a.alert_fingerprint, a.acknowledged_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make(map[string]models.AlertAcknowledgement)
	for rows.Next() {
		var ack models.AlertAcknowledgement
		if err := rows.Scan(&ack.ID, &ack.OrganizationID, &ack.AlertFingerprint, &ack.AcknowledgedBy, &ack.AcknowledgedName, &ack.AcknowledgedAt, &ack.Note); err != nil {
			return nil, err
		}
		out[ack.AlertFingerprint] = ack
	}
	return out, rows.Err()
}

func (s *PGStore) ListAcknowledgementHistory(ctx context.Context, orgID uuid.UUID, fingerprint string) ([]models.AlertAcknowledgement, error) {
	query := `
		SELECT a.id, a.organization_id, a.alert_fingerprint, a.acknowledged_by, COALESCE(u.name, ''), a.acknowledged_at, a.note
		FROM alert_acknowledgements a
		LEFT JOIN users u ON u.id = a.acknowledged_by
		WHERE a.organization_id = $1
	`
	args := []any{orgID}
	if fingerprint != "" {
		query += " AND a.alert_fingerprint = $2"
		args = append(args, fingerprint)
	}
	query += " ORDER BY a.acknowledged_at DESC LIMIT 100"

	rows, err := s.pool.Query(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.AlertAcknowledgement, 0)
	for rows.Next() {
		var ack models.AlertAcknowledgement
		if err := rows.Scan(&ack.ID, &ack.OrganizationID, &ack.AlertFingerprint, &ack.AcknowledgedBy, &ack.AcknowledgedName, &ack.AcknowledgedAt, &ack.Note); err != nil {
			return nil, err
		}
		out = append(out, ack)
	}
	return out, rows.Err()
}

func (s *PGStore) ListNotificationChannels(ctx context.Context, orgID uuid.UUID) ([]models.NotificationChannel, error) {
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, name, channel_type, configuration_reference, target_summary,
			severity_filters, last_status, last_tested_at, enabled, created_at, updated_at
		FROM notification_channels WHERE organization_id = $1 ORDER BY created_at DESC
	`, orgID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.NotificationChannel, 0)
	for rows.Next() {
		var ch models.NotificationChannel
		if err := rows.Scan(
			&ch.ID, &ch.OrganizationID, &ch.Name, &ch.ChannelType, &ch.ConfigurationReference, &ch.TargetSummary,
			&ch.SeverityFilters, &ch.LastStatus, &ch.LastTestedAt, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
		); err != nil {
			return nil, err
		}
		out = append(out, ch)
	}
	return out, rows.Err()
}

func (s *PGStore) GetNotificationChannel(ctx context.Context, orgID, id uuid.UUID) (models.NotificationChannel, error) {
	var ch models.NotificationChannel
	err := s.pool.QueryRow(ctx, `
		SELECT id, organization_id, name, channel_type, configuration_reference, target_summary,
			severity_filters, last_status, last_tested_at, enabled, created_at, updated_at
		FROM notification_channels WHERE id = $1 AND organization_id = $2
	`, id, orgID).Scan(
		&ch.ID, &ch.OrganizationID, &ch.Name, &ch.ChannelType, &ch.ConfigurationReference, &ch.TargetSummary,
		&ch.SeverityFilters, &ch.LastStatus, &ch.LastTestedAt, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ch, ErrNotFound
	}
	return ch, err
}

func (s *PGStore) CreateNotificationChannel(ctx context.Context, ch models.NotificationChannel) (models.NotificationChannel, error) {
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notification_channels (organization_id, name, channel_type, configuration_reference, target_summary, severity_filters, enabled)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
		RETURNING id, organization_id, name, channel_type, configuration_reference, target_summary,
			severity_filters, last_status, last_tested_at, enabled, created_at, updated_at
	`, ch.OrganizationID, ch.Name, ch.ChannelType, ch.ConfigurationReference, ch.TargetSummary, ch.SeverityFilters, ch.Enabled).Scan(
		&ch.ID, &ch.OrganizationID, &ch.Name, &ch.ChannelType, &ch.ConfigurationReference, &ch.TargetSummary,
		&ch.SeverityFilters, &ch.LastStatus, &ch.LastTestedAt, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
	)
	return ch, err
}

func (s *PGStore) UpdateNotificationChannel(ctx context.Context, ch models.NotificationChannel) (models.NotificationChannel, error) {
	err := s.pool.QueryRow(ctx, `
		UPDATE notification_channels
		SET name = $3, channel_type = $4, configuration_reference = $5, target_summary = $6,
		    severity_filters = $7, enabled = $8, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
		RETURNING id, organization_id, name, channel_type, configuration_reference, target_summary,
			severity_filters, last_status, last_tested_at, enabled, created_at, updated_at
	`, ch.ID, ch.OrganizationID, ch.Name, ch.ChannelType, ch.ConfigurationReference, ch.TargetSummary, ch.SeverityFilters, ch.Enabled).Scan(
		&ch.ID, &ch.OrganizationID, &ch.Name, &ch.ChannelType, &ch.ConfigurationReference, &ch.TargetSummary,
		&ch.SeverityFilters, &ch.LastStatus, &ch.LastTestedAt, &ch.Enabled, &ch.CreatedAt, &ch.UpdatedAt,
	)
	if errors.Is(err, pgx.ErrNoRows) {
		return ch, ErrNotFound
	}
	return ch, err
}

func (s *PGStore) UpdateNotificationChannelTestStatus(ctx context.Context, orgID, id uuid.UUID, status string, testedAt time.Time) error {
	_, err := s.pool.Exec(ctx, `
		UPDATE notification_channels SET last_status = $3, last_tested_at = $4, updated_at = NOW()
		WHERE id = $1 AND organization_id = $2
	`, id, orgID, status, testedAt)
	return err
}

func (s *PGStore) DeleteNotificationChannel(ctx context.Context, orgID, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `DELETE FROM notification_channels WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) ListNotifications(ctx context.Context, orgID uuid.UUID, severity string, unreadOnly bool, page, pageSize int) ([]models.NotificationRecord, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 25
	}
	conds := []string{"organization_id = $1"}
	args := []any{orgID}
	idx := 2
	if severity != "" {
		conds = append(conds, fmt.Sprintf("severity = $%d", idx))
		args = append(args, strings.ToLower(severity))
		idx++
	}
	if unreadOnly {
		conds = append(conds, "is_read = FALSE")
	}
	where := strings.Join(conds, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM notification_history WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT id, organization_id, channel_id, alert_fingerprint, title, message, category,
			severity, status, is_read, read_at, attempted_at, delivered_at, error_summary
		FROM notification_history WHERE %s
		ORDER BY attempted_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.NotificationRecord, 0)
	for rows.Next() {
		var r models.NotificationRecord
		if err := rows.Scan(
			&r.ID, &r.OrganizationID, &r.ChannelID, &r.AlertFingerprint, &r.Title, &r.Message, &r.Category,
			&r.Severity, &r.Status, &r.IsRead, &r.ReadAt, &r.AttemptedAt, &r.DeliveredAt, &r.ErrorSummary,
		); err != nil {
			return nil, 0, err
		}
		out = append(out, r)
	}
	return out, total, rows.Err()
}

func (s *PGStore) CreateNotification(ctx context.Context, rec models.NotificationRecord) (models.NotificationRecord, error) {
	if rec.Severity == "" {
		rec.Severity = "info"
	}
	if rec.Status == "" {
		rec.Status = "delivered"
	}
	if rec.Category == "" {
		rec.Category = "system"
	}
	err := s.pool.QueryRow(ctx, `
		INSERT INTO notification_history (organization_id, channel_id, alert_fingerprint, title, message, category, severity, status, delivered_at, error_summary)
		VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10)
		RETURNING id, organization_id, channel_id, alert_fingerprint, title, message, category, severity, status, is_read, read_at, attempted_at, delivered_at, error_summary
	`, rec.OrganizationID, rec.ChannelID, rec.AlertFingerprint, rec.Title, rec.Message, rec.Category, rec.Severity, rec.Status, rec.DeliveredAt, rec.ErrorSummary).Scan(
		&rec.ID, &rec.OrganizationID, &rec.ChannelID, &rec.AlertFingerprint, &rec.Title, &rec.Message, &rec.Category,
		&rec.Severity, &rec.Status, &rec.IsRead, &rec.ReadAt, &rec.AttemptedAt, &rec.DeliveredAt, &rec.ErrorSummary,
	)
	return rec, err
}

func (s *PGStore) MarkNotificationRead(ctx context.Context, orgID, id uuid.UUID) error {
	ct, err := s.pool.Exec(ctx, `UPDATE notification_history SET is_read = TRUE, read_at = NOW() WHERE id = $1 AND organization_id = $2`, id, orgID)
	if err != nil {
		return err
	}
	if ct.RowsAffected() == 0 {
		return ErrNotFound
	}
	return nil
}

func (s *PGStore) MarkAllNotificationsRead(ctx context.Context, orgID uuid.UUID) error {
	_, err := s.pool.Exec(ctx, `UPDATE notification_history SET is_read = TRUE, read_at = NOW() WHERE organization_id = $1 AND is_read = FALSE`, orgID)
	return err
}

func (s *PGStore) InsertAuditLog(ctx context.Context, orgID uuid.UUID, actorUserID *uuid.UUID, action, resourceType, resourceID, sourceIP string, metadata map[string]any) error {
	metaJSON, err := json.Marshal(metadata)
	if err != nil {
		metaJSON = []byte("{}")
	}
	_, err = s.pool.Exec(ctx, `
		INSERT INTO audit_logs (organization_id, actor_user_id, action, resource_type, resource_id, source_ip, metadata)
		VALUES ($1, $2, $3, $4, $5, $6, $7)
	`, orgID, actorUserID, action, resourceType, resourceID, sourceIP, metaJSON)
	return err
}

func (s *PGStore) ListAuditLogs(ctx context.Context, orgID uuid.UUID, action, resourceType string, page, pageSize int) ([]models.AuditLog, int, error) {
	if page < 1 {
		page = 1
	}
	if pageSize < 1 || pageSize > 100 {
		pageSize = 30
	}
	conds := []string{"a.organization_id = $1"}
	args := []any{orgID}
	idx := 2
	if action != "" {
		conds = append(conds, fmt.Sprintf("a.action ILIKE $%d", idx))
		args = append(args, "%"+action+"%")
		idx++
	}
	if resourceType != "" {
		conds = append(conds, fmt.Sprintf("a.resource_type = $%d", idx))
		args = append(args, resourceType)
		idx++
	}
	where := strings.Join(conds, " AND ")

	var total int
	if err := s.pool.QueryRow(ctx, "SELECT COUNT(*) FROM audit_logs a WHERE "+where, args...).Scan(&total); err != nil {
		return nil, 0, err
	}

	args = append(args, pageSize, (page-1)*pageSize)
	rows, err := s.pool.Query(ctx, fmt.Sprintf(`
		SELECT a.id, a.organization_id, a.actor_user_id, COALESCE(u.name, ''), COALESCE(u.email, ''),
			a.action, a.resource_type, a.resource_id, a.source_ip, a.metadata, a.created_at
		FROM audit_logs a
		LEFT JOIN users u ON u.id = a.actor_user_id
		WHERE %s
		ORDER BY a.created_at DESC LIMIT $%d OFFSET $%d
	`, where, idx, idx+1), args...)
	if err != nil {
		return nil, 0, err
	}
	defer rows.Close()

	out := make([]models.AuditLog, 0)
	for rows.Next() {
		var l models.AuditLog
		var rawMeta []byte
		if err := rows.Scan(
			&l.ID, &l.OrganizationID, &l.ActorUserID, &l.ActorName, &l.ActorEmail,
			&l.Action, &l.ResourceType, &l.ResourceID, &l.SourceIP, &rawMeta, &l.CreatedAt,
		); err != nil {
			return nil, 0, err
		}
		_ = json.Unmarshal(rawMeta, &l.Metadata)
		if l.Metadata == nil {
			l.Metadata = map[string]any{}
		}
		out = append(out, l)
	}
	return out, total, rows.Err()
}

func (s *PGStore) RecordQueryHistory(ctx context.Context, orgID, userID uuid.UUID, metricName, query, timeRange string) error {
	_, err := s.pool.Exec(ctx, `
		INSERT INTO query_history (organization_id, user_id, metric_name, query, time_range)
		VALUES ($1, $2, $3, $4, $5)
	`, orgID, userID, metricName, query, timeRange)
	return err
}

func (s *PGStore) ListQueryHistory(ctx context.Context, orgID, userID uuid.UUID, limit int) ([]models.QueryHistoryItem, error) {
	if limit <= 0 || limit > 50 {
		limit = 20
	}
	rows, err := s.pool.Query(ctx, `
		SELECT id, organization_id, user_id, metric_name, query, time_range, created_at
		FROM query_history WHERE organization_id = $1 AND user_id = $2
		ORDER BY created_at DESC LIMIT $3
	`, orgID, userID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]models.QueryHistoryItem, 0)
	for rows.Next() {
		var h models.QueryHistoryItem
		if err := rows.Scan(&h.ID, &h.OrganizationID, &h.UserID, &h.MetricName, &h.Query, &h.TimeRange, &h.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, h)
	}
	return out, rows.Err()
}
