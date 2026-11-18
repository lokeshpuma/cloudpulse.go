package models

import (
	"time"

	"github.com/google/uuid"
)

const (
	RoleAdmin    = "admin"
	RoleOperator = "operator"
	RoleViewer   = "viewer"
)

const (
	StatusUp       = "UP"
	StatusDown     = "DOWN"
	StatusWarning  = "WARNING"
	StatusCritical = "CRITICAL"
	StatusUnknown  = "UNKNOWN"
	StatusPending  = "PENDING"
)

type Organization struct {
	ID                       uuid.UUID `json:"id"`
	Name                     string    `json:"name"`
	Description              string    `json:"description"`
	Timezone                 string    `json:"timezone"`
	DisplayUnits             string    `json:"display_units"`
	DateFormat               string    `json:"date_format"`
	TimeFormat               string    `json:"time_format"`
	DefaultScrapeInterval    int       `json:"default_scrape_interval"`
	DefaultMonitoringTimeout int       `json:"default_monitoring_timeout"`
	AgentConnectionTimeout   int       `json:"agent_connection_timeout"`
	DiskExclusionPatterns    string    `json:"disk_exclusion_patterns"`
	DefaultEnvironment       string    `json:"default_environment"`
	OnboardingCompleted      bool      `json:"onboarding_completed"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type User struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	Name           string    `json:"name"`
	Email          string    `json:"email"`
	PasswordHash   string    `json:"-"`
	Role           string    `json:"role"`
	IsActive       bool      `json:"is_active"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

type RefreshSession struct {
	ID        uuid.UUID  `json:"id"`
	UserID    uuid.UUID  `json:"user_id"`
	TokenHash string     `json:"-"`
	ExpiresAt time.Time  `json:"expires_at"`
	RevokedAt *time.Time `json:"revoked_at,omitempty"`
	CreatedAt time.Time  `json:"created_at"`
}

type ServerTag struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

type Server struct {
	ID                 uuid.UUID   `json:"id"`
	OrganizationID     uuid.UUID   `json:"organization_id"`
	Name               string      `json:"name"`
	Hostname           string      `json:"hostname"`
	Environment        string      `json:"environment"`
	OperatingSystem    string      `json:"operating_system"`
	Architecture       string      `json:"architecture"`
	Description        string      `json:"description"`
	ServerGroup        string      `json:"server_group"`
	ScrapeAddress      string      `json:"scrape_address"`
	MonitoringInterval int         `json:"monitoring_interval"`
	AgentVersion       string      `json:"agent_version"`
	MonitoringEnabled  bool        `json:"monitoring_enabled"`
	LastSeenAt         *time.Time  `json:"last_seen_at,omitempty"`
	Tags               []ServerTag `json:"tags"`
	CreatedAt          time.Time   `json:"created_at"`
	UpdatedAt          time.Time   `json:"updated_at"`

	// Live derived fields populated from Prometheus (never stored in Postgres as time series)
	Status        string     `json:"status"`
	Availability  string     `json:"availability"`
	CPUPercent    *float64   `json:"cpu_percent"`
	MemoryPercent *float64   `json:"memory_percent"`
	DiskPercent   *float64   `json:"disk_percent"`
	IsStale       bool       `json:"is_stale"`
	LastSampleAt  *time.Time `json:"last_sample_at,omitempty"`
	DataSource    string     `json:"data_source"`
}

type ServiceCheck struct {
	ID                 uuid.UUID `json:"id"`
	ServerID           uuid.UUID `json:"server_id"`
	ServerName         string    `json:"server_name,omitempty"`
	Environment        string    `json:"environment,omitempty"`
	Name               string    `json:"name"`
	CheckType          string    `json:"check_type"`
	Target             string    `json:"target"`
	IntervalSeconds    int       `json:"interval_seconds"`
	TimeoutSeconds     int       `json:"timeout_seconds"`
	ExpectedStatusCode int       `json:"expected_status_code"`
	Enabled            bool      `json:"enabled"`
	CreatedAt          time.Time `json:"created_at"`
	UpdatedAt          time.Time `json:"updated_at"`

	// Live Prometheus-derived fields
	Status               string     `json:"status"`
	LatencyMs            *float64   `json:"latency_ms"`
	HTTPStatus           *int       `json:"http_status,omitempty"`
	Availability24h      *float64   `json:"availability_24h"`
	ValidIntervals24h    int        `json:"valid_intervals_24h"`
	LastCheckedAt        *time.Time `json:"last_checked_at,omitempty"`
	TLSExpirySecondsLeft *float64   `json:"tls_expiry_seconds_left,omitempty"`
}

type AlertRule struct {
	ID             uuid.UUID  `json:"id"`
	OrganizationID uuid.UUID  `json:"organization_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	Expression     string     `json:"expression"`
	MetricTemplate string     `json:"metric_template"`
	Threshold      float64    `json:"threshold"`
	Severity       string     `json:"severity"`
	Duration       string     `json:"duration"`
	Environments   []string   `json:"environments"`
	IsDefault      bool       `json:"is_default"`
	Enabled        bool       `json:"enabled"`
	LastEvalStatus string     `json:"last_eval_status"`
	CreatedBy      *uuid.UUID `json:"created_by,omitempty"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type AlertAcknowledgement struct {
	ID               uuid.UUID  `json:"id"`
	OrganizationID   uuid.UUID  `json:"organization_id"`
	AlertFingerprint string     `json:"alert_fingerprint"`
	AcknowledgedBy   *uuid.UUID `json:"acknowledged_by,omitempty"`
	AcknowledgedName string     `json:"acknowledged_by_name,omitempty"`
	AcknowledgedAt   time.Time  `json:"acknowledged_at"`
	Note             string     `json:"note"`
}

type NotificationChannel struct {
	ID                     uuid.UUID  `json:"id"`
	OrganizationID         uuid.UUID  `json:"organization_id"`
	Name                   string     `json:"name"`
	ChannelType            string     `json:"channel_type"`
	ConfigurationReference string     `json:"configuration_reference"`
	TargetSummary          string     `json:"target_summary"`
	SeverityFilters        []string   `json:"severity_filters"`
	LastStatus             string     `json:"last_status"`
	LastTestedAt           *time.Time `json:"last_tested_at,omitempty"`
	Enabled                bool       `json:"enabled"`
	CreatedAt              time.Time  `json:"created_at"`
	UpdatedAt              time.Time  `json:"updated_at"`
}

type NotificationRecord struct {
	ID               uuid.UUID  `json:"id"`
	OrganizationID   uuid.UUID  `json:"organization_id"`
	ChannelID        *uuid.UUID `json:"channel_id,omitempty"`
	AlertFingerprint string     `json:"alert_fingerprint"`
	Title            string     `json:"title"`
	Message          string     `json:"message"`
	Category         string     `json:"category"`
	Severity         string     `json:"severity"`
	Status           string     `json:"status"`
	IsRead           bool       `json:"is_read"`
	ReadAt           *time.Time `json:"read_at,omitempty"`
	AttemptedAt      time.Time  `json:"attempted_at"`
	DeliveredAt      *time.Time `json:"delivered_at,omitempty"`
	ErrorSummary     string     `json:"error_summary,omitempty"`
}

type AuditLog struct {
	ID             uuid.UUID      `json:"id"`
	OrganizationID uuid.UUID      `json:"organization_id"`
	ActorUserID    *uuid.UUID     `json:"actor_user_id,omitempty"`
	ActorName      string         `json:"actor_name,omitempty"`
	ActorEmail     string         `json:"actor_email,omitempty"`
	Action         string         `json:"action"`
	ResourceType   string         `json:"resource_type"`
	ResourceID     string         `json:"resource_id"`
	SourceIP       string         `json:"source_ip"`
	Metadata       map[string]any `json:"metadata"`
	CreatedAt      time.Time      `json:"created_at"`
}

type QueryHistoryItem struct {
	ID             uuid.UUID `json:"id"`
	OrganizationID uuid.UUID `json:"organization_id"`
	UserID         uuid.UUID `json:"user_id"`
	MetricName     string    `json:"metric_name"`
	Query          string    `json:"query"`
	TimeRange      string    `json:"time_range"`
	CreatedAt      time.Time `json:"created_at"`
}
