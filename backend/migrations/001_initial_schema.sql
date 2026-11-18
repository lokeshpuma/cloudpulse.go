-- CloudPulse Initial Database Schema
-- Stores application metadata, configuration, acknowledgements, notification delivery records, and audit logs.
-- Time-series metrics are stored exclusively in Prometheus, never in PostgreSQL.

CREATE EXTENSION IF NOT EXISTS "pgcrypto";

CREATE TABLE IF NOT EXISTS organizations (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    timezone TEXT NOT NULL DEFAULT 'UTC',
    display_units TEXT NOT NULL DEFAULT 'binary' CHECK (display_units IN ('binary', 'decimal')),
    date_format TEXT NOT NULL DEFAULT 'YYYY-MM-DD',
    time_format TEXT NOT NULL DEFAULT '24h' CHECK (time_format IN ('12h', '24h')),
    default_scrape_interval INTEGER NOT NULL DEFAULT 15 CHECK (default_scrape_interval >= 5 AND default_scrape_interval <= 3600),
    default_monitoring_timeout INTEGER NOT NULL DEFAULT 5 CHECK (default_monitoring_timeout >= 1 AND default_monitoring_timeout <= 120),
    agent_connection_timeout INTEGER NOT NULL DEFAULT 10 CHECK (agent_connection_timeout >= 1 AND agent_connection_timeout <= 120),
    disk_exclusion_patterns TEXT NOT NULL DEFAULT '/proc,/sys,/dev,/run',
    default_environment TEXT NOT NULL DEFAULT 'production',
    onboarding_completed BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

CREATE TABLE IF NOT EXISTS users (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    email TEXT NOT NULL UNIQUE,
    password_hash TEXT NOT NULL,
    role TEXT NOT NULL CHECK (role IN ('admin', 'operator', 'viewer')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_users_org_id ON users(organization_id);
CREATE INDEX IF NOT EXISTS idx_users_email ON users(LOWER(email));

CREATE TABLE IF NOT EXISTS refresh_sessions (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_user_id ON refresh_sessions(user_id);
CREATE INDEX IF NOT EXISTS idx_refresh_sessions_expires_at ON refresh_sessions(expires_at);

CREATE TABLE IF NOT EXISTS password_reset_tokens (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    token_hash TEXT NOT NULL UNIQUE,
    expires_at TIMESTAMPTZ NOT NULL,
    used_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_password_reset_tokens_user_id ON password_reset_tokens(user_id);

CREATE TABLE IF NOT EXISTS servers (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    hostname TEXT NOT NULL,
    environment TEXT NOT NULL CHECK (environment IN ('production', 'staging', 'development', 'testing', 'other')),
    operating_system TEXT NOT NULL DEFAULT 'linux',
    architecture TEXT NOT NULL DEFAULT 'amd64',
    description TEXT NOT NULL DEFAULT '',
    server_group TEXT NOT NULL DEFAULT 'default',
    scrape_address TEXT NOT NULL,
    monitoring_interval INTEGER NOT NULL DEFAULT 15 CHECK (monitoring_interval >= 5 AND monitoring_interval <= 3600),
    agent_version TEXT NOT NULL DEFAULT '',
    monitoring_enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_seen_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (organization_id, scrape_address)
);
CREATE INDEX IF NOT EXISTS idx_servers_org_id ON servers(organization_id);
CREATE INDEX IF NOT EXISTS idx_servers_org_env ON servers(organization_id, environment);
CREATE INDEX IF NOT EXISTS idx_servers_org_group ON servers(organization_id, server_group);

CREATE TABLE IF NOT EXISTS server_tags (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    tag_key TEXT NOT NULL,
    tag_value TEXT NOT NULL,
    UNIQUE (server_id, tag_key)
);
CREATE INDEX IF NOT EXISTS idx_server_tags_server_id ON server_tags(server_id);

CREATE TABLE IF NOT EXISTS service_checks (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    server_id UUID NOT NULL REFERENCES servers(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    check_type TEXT NOT NULL CHECK (check_type IN ('http', 'tcp')),
    target TEXT NOT NULL,
    interval_seconds INTEGER NOT NULL DEFAULT 30 CHECK (interval_seconds >= 5 AND interval_seconds <= 3600),
    timeout_seconds INTEGER NOT NULL DEFAULT 5 CHECK (timeout_seconds >= 1 AND timeout_seconds <= 120),
    expected_status_code INTEGER NOT NULL DEFAULT 200 CHECK (expected_status_code >= 0 AND expected_status_code <= 599),
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE (server_id, name)
);
CREATE INDEX IF NOT EXISTS idx_service_checks_server_id ON service_checks(server_id);

CREATE TABLE IF NOT EXISTS alert_rules (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    description TEXT NOT NULL DEFAULT '',
    expression TEXT NOT NULL,
    metric_template TEXT NOT NULL DEFAULT 'custom',
    threshold DOUBLE PRECISION NOT NULL DEFAULT 0,
    severity TEXT NOT NULL CHECK (severity IN ('critical', 'warning', 'info')),
    duration TEXT NOT NULL DEFAULT '5m',
    environments TEXT[] NOT NULL DEFAULT ARRAY['production', 'staging', 'development', 'testing', 'other'],
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    last_eval_status TEXT NOT NULL DEFAULT 'pending',
    created_by UUID REFERENCES users(id) ON DELETE SET NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_alert_rules_org_id ON alert_rules(organization_id);

CREATE TABLE IF NOT EXISTS alert_acknowledgements (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    alert_fingerprint TEXT NOT NULL,
    acknowledged_by UUID REFERENCES users(id) ON DELETE SET NULL,
    acknowledged_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    note TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_alert_ack_org_fp ON alert_acknowledgements(organization_id, alert_fingerprint);

CREATE TABLE IF NOT EXISTS notification_channels (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    name TEXT NOT NULL,
    channel_type TEXT NOT NULL CHECK (channel_type IN ('email', 'slack', 'webhook')),
    configuration_reference TEXT NOT NULL,
    target_summary TEXT NOT NULL DEFAULT '',
    severity_filters TEXT[] NOT NULL DEFAULT ARRAY['critical', 'warning'],
    last_status TEXT NOT NULL DEFAULT 'never_tested',
    last_tested_at TIMESTAMPTZ,
    enabled BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_notification_channels_org_id ON notification_channels(organization_id);

CREATE TABLE IF NOT EXISTS notification_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    channel_id UUID REFERENCES notification_channels(id) ON DELETE SET NULL,
    alert_fingerprint TEXT NOT NULL DEFAULT '',
    title TEXT NOT NULL DEFAULT '',
    message TEXT NOT NULL DEFAULT '',
    category TEXT NOT NULL DEFAULT 'alert',
    severity TEXT NOT NULL DEFAULT 'info' CHECK (severity IN ('critical', 'warning', 'info', 'resolved')),
    status TEXT NOT NULL DEFAULT 'delivered',
    is_read BOOLEAN NOT NULL DEFAULT FALSE,
    read_at TIMESTAMPTZ,
    attempted_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    delivered_at TIMESTAMPTZ,
    error_summary TEXT NOT NULL DEFAULT ''
);
CREATE INDEX IF NOT EXISTS idx_notification_history_org_time ON notification_history(organization_id, attempted_at DESC);
CREATE INDEX IF NOT EXISTS idx_notification_history_org_unread ON notification_history(organization_id, is_read);

CREATE TABLE IF NOT EXISTS audit_logs (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE RESTRICT,
    actor_user_id UUID REFERENCES users(id) ON DELETE SET NULL,
    action TEXT NOT NULL,
    resource_type TEXT NOT NULL,
    resource_id TEXT NOT NULL DEFAULT '',
    source_ip TEXT NOT NULL DEFAULT '',
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_audit_logs_org_time ON audit_logs(organization_id, created_at DESC);
CREATE INDEX IF NOT EXISTS idx_audit_logs_action ON audit_logs(organization_id, action);

CREATE TABLE IF NOT EXISTS query_history (
    id UUID PRIMARY KEY DEFAULT gen_random_uuid(),
    organization_id UUID NOT NULL REFERENCES organizations(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    metric_name TEXT NOT NULL DEFAULT '',
    query TEXT NOT NULL,
    time_range TEXT NOT NULL DEFAULT '1h',
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);
CREATE INDEX IF NOT EXISTS idx_query_history_org_user ON query_history(organization_id, user_id, created_at DESC);
