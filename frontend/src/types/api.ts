export type UserRole = "admin" | "operator" | "viewer";

export type ServerStatus = "UP" | "DOWN" | "WARNING" | "CRITICAL" | "UNKNOWN" | "PENDING";

export type TimeRangeKey = "15m" | "30m" | "1h" | "6h" | "24h" | "7d" | "30d" | "custom";

export interface ApiEnvelope<T> {
  success: boolean;
  data: T;
  message?: string;
  request_id?: string;
  error?: {
    code: string;
    message: string;
  };
}

export interface PaginatedResponse<T> {
  items: T[];
  total: number;
  page: number;
  page_size: number;
  total_pages: number;
  prometheus_available?: boolean;
}

export interface Organization {
  id: string;
  name: string;
  description: string;
  timezone: string;
  display_units: "binary" | "decimal";
  date_format: string;
  time_format: "12h" | "24h";
  default_scrape_interval: number;
  default_monitoring_timeout: number;
  agent_connection_timeout: number;
  disk_exclusion_patterns: string;
  default_environment: string;
  onboarding_completed: boolean;
  created_at: string;
  updated_at: string;
}

export interface User {
  id: string;
  organization_id: string;
  name: string;
  email: string;
  role: UserRole;
  is_active: boolean;
  created_at: string;
  updated_at: string;
}

export interface AuthPayload {
  access_token: string;
  expires_at: string;
  csrf_token: string;
  user: User;
  organization: Organization;
}

export interface ServerTag {
  key: string;
  value: string;
}

export interface Server {
  id: string;
  organization_id: string;
  name: string;
  hostname: string;
  environment: string;
  operating_system: string;
  architecture: string;
  description: string;
  server_group: string;
  scrape_address: string;
  monitoring_interval: number;
  agent_version: string;
  monitoring_enabled: boolean;
  last_seen_at?: string | null;
  tags: ServerTag[];
  created_at: string;
  updated_at: string;
  status: ServerStatus;
  availability: "UP" | "DOWN" | "UNKNOWN";
  cpu_percent: number | null;
  memory_percent: number | null;
  disk_percent: number | null;
  is_stale: boolean;
  last_sample_at?: string | null;
  data_source: string;
}

export interface SimpleDataPoint {
  timestamp: string;
  value: number;
}

export interface InfrastructureEvent {
  id: string;
  action: string;
  title: string;
  resource_type: string;
  resource_id: string;
  actor: string;
  severity: string;
  timestamp: string;
}

export interface GrafanaDashboardLink {
  uid: string;
  title: string;
  description: string;
  url: string;
}

export interface OverviewResponse {
  prometheus_available: boolean;
  data_source: string;
  aggregation_method: string;
  generated_at: string;
  servers: {
    total: number;
    up: number;
    down: number;
    unknown: number;
    pending: number;
    availability_percent: number | null;
  };
  health: {
    healthy: number;
    warning: number;
    critical: number;
    unknown: number;
  };
  cpu: {
    average_percent: number | null;
    change_percent: number | null;
    included_count: number;
    trend: SimpleDataPoint[];
  };
  memory: {
    average_percent: number | null;
    used_bytes: number | null;
    total_bytes: number | null;
    available_bytes: number | null;
    change_percent: number | null;
    included_count: number;
  };
  disk: {
    average_percent: number | null;
    highest_server: string;
    highest_server_id: string;
    highest_percent: number | null;
    critical_count: number;
    included_count: number;
  };
  alerts: {
    total: number;
    critical: number;
    warning: number;
    info: number;
    unacknowledged: number;
  };
  recent_events: InfrastructureEvent[];
  grafana_dashboards?: GrafanaDashboardLink[];
}

export interface InfrastructureHealthResponse {
  prometheus_available: boolean;
  total_machines: number;
  by_environment: Record<string, number>;
  by_operating_system: Record<string, number>;
  by_status: Record<string, number>;
  recently_added: Server[];
  recently_disconnected: Server[];
  grafana_dashboards?: GrafanaDashboardLink[];
}

export interface TimeseriesPoint {
  timestamp: string;
  cpu_percent: number | null;
  memory_percent: number | null;
  disk_percent: number | null;
}

export interface TimeseriesResponse {
  prometheus_available: boolean;
  range: string;
  start: string;
  end: string;
  step_seconds: number;
  points: TimeseriesPoint[];
}

export interface DiskMountMetric {
  mountpoint: string;
  device: string;
  filesystem: string;
  total_bytes: number;
  used_bytes: number;
  free_bytes: number;
  usage_percent: number;
  collected_at: string;
}

export interface NetworkInterfaceMetric {
  interface: string;
  status: string;
  receive_bytes_per_sec: number;
  transmit_bytes_per_sec: number;
  total_receive_bytes: number;
  total_transmit_bytes: number;
  receive_packets_total: number;
  transmit_packets_total: number;
  receive_errors_total: number;
  transmit_errors_total: number;
}

export interface ServiceCheck {
  id: string;
  server_id: string;
  server_name?: string;
  environment?: string;
  name: string;
  check_type: "http" | "tcp";
  target: string;
  interval_seconds: number;
  timeout_seconds: number;
  expected_status_code: number;
  enabled: boolean;
  created_at: string;
  updated_at: string;
  status: "UP" | "DOWN" | "UNKNOWN";
  latency_ms: number | null;
  http_status?: number | null;
  availability_24h: number | null;
  valid_intervals_24h: number;
  last_checked_at?: string | null;
  tls_expiry_seconds_left?: number | null;
}

export interface ServerDetailMetrics {
  prometheus_available: boolean;
  server: Server;
  cpu: {
    usage_percent: number | null;
    core_count: number | null;
    per_core_percent: Record<string, number>;
    load_1m: number | null;
    load_5m: number | null;
    load_15m: number | null;
    load_supported: boolean;
    saturation_warning: boolean;
    peak_percent: number | null;
    average_percent: number | null;
  };
  memory: {
    total_bytes: number | null;
    used_bytes: number | null;
    available_bytes: number | null;
    cached_bytes: number | null;
    free_bytes: number | null;
    usage_percent: number | null;
  };
  disks: DiskMountMetric[];
  networks: NetworkInterfaceMetric[];
  services: ServiceCheck[];
  recent_activity: InfrastructureEvent[];
  grafana_dashboards?: GrafanaDashboardLink[];
}

export interface ServerHistoryResponse {
  prometheus_available: boolean;
  range: string;
  start: string;
  end: string;
  cpu: SimpleDataPoint[];
  memory: SimpleDataPoint[];
  disk: SimpleDataPoint[];
  network_rx_rate: SimpleDataPoint[];
  network_tx_rate: SimpleDataPoint[];
}

export interface ServerStatusCheck {
  server_id: string;
  name: string;
  hostname: string;
  scrape_address: string;
  status: ServerStatus;
  availability: string;
  connected: boolean;
  is_stale: boolean;
  agent_version: string;
  last_successful_scrape?: string | null;
  collected_metrics: string[];
  prometheus_available: boolean;
}

export interface ServiceHistoryResponse {
  prometheus_available: boolean;
  service: ServiceCheck;
  availability_percent: number | null;
  valid_intervals: number;
  failed_intervals: number;
  average_latency_ms: number | null;
  up_timeline: SimpleDataPoint[];
  latency_timeline: SimpleDataPoint[];
}

export interface MetricCatalogItem {
  id: string;
  name: string;
  category: string;
  unit: string;
  description: string;
  base_promql: string;
  metric_type: "gauge" | "counter_rate";
}

export interface QueryHistoryItem {
  id: string;
  metric_name: string;
  query: string;
  time_range: string;
  created_at: string;
}

export interface ExplorerQueryResult {
  prometheus_available: boolean;
  query: string;
  range: string;
  start: string;
  end: string;
  series: {
    label: string;
    metric: Record<string, string>;
    points: SimpleDataPoint[];
  }[];
  summary: {
    min: number | null;
    max: number | null;
    avg: number | null;
    first: number | null;
    last: number | null;
    count: number;
  };
  missing_data_points: number;
  error?: string;
}

export interface AlertAcknowledgement {
  id: string;
  alert_fingerprint: string;
  acknowledged_by?: string;
  acknowledged_by_name?: string;
  acknowledged_at: string;
  note: string;
}

export interface AlertView {
  fingerprint: string;
  title: string;
  alert_name: string;
  severity: "critical" | "warning" | "info" | "resolved" | string;
  description: string;
  server_id: string;
  server_name: string;
  environment: string;
  state: "active" | "pending" | "resolved" | string;
  triggered_at: string;
  updated_at: string;
  labels: Record<string, string>;
  annotations: Record<string, string>;
  acknowledged: boolean;
  acknowledgement?: AlertAcknowledgement;
}

export interface AlertDetailResponse {
  alert: AlertView;
  acknowledgements: AlertAcknowledgement[];
  related_metrics: SimpleDataPoint[];
  related_metric_label: string;
  timeline: {
    event: string;
    actor: string;
    detail: string;
    timestamp: string;
  }[];
}

export interface AlertRule {
  id: string;
  organization_id: string;
  name: string;
  description: string;
  expression: string;
  metric_template: string;
  threshold: number;
  severity: "critical" | "warning" | "info";
  duration: string;
  environments: string[];
  is_default: boolean;
  enabled: boolean;
  last_eval_status: string;
  created_at: string;
  updated_at: string;
}

export interface NotificationChannel {
  id: string;
  name: string;
  channel_type: "email" | "slack" | "webhook";
  configuration_reference: string;
  target_summary: string;
  severity_filters: string[];
  last_status: string;
  last_tested_at?: string | null;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface NotificationRecord {
  id: string;
  channel_id?: string | null;
  alert_fingerprint: string;
  title: string;
  message: string;
  category: string;
  severity: "critical" | "warning" | "info" | "resolved";
  status: string;
  is_read: boolean;
  read_at?: string | null;
  attempted_at: string;
  delivered_at?: string | null;
  error_summary?: string;
}

export interface AuditLog {
  id: string;
  actor_user_id?: string | null;
  actor_name: string;
  actor_email: string;
  action: string;
  resource_type: string;
  resource_id: string;
  source_ip: string;
  metadata: Record<string, unknown>;
  created_at: string;
}

export interface SystemIntegrations {
  postgres: { status: string; diagnostic: string };
  prometheus: { status: string; url: string; diagnostic: string };
  alertmanager: { status: string; url: string; diagnostic: string };
  grafana: {
    status: string;
    public_url: string;
    dashboards: GrafanaDashboardLink[];
    diagnostic: string;
  };
}
