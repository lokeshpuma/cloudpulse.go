import type {
  AlertDetailResponse,
  AlertRule,
  AlertView,
  AuditLog,
  ExplorerQueryResult,
  InfrastructureHealthResponse,
  MetricCatalogItem,
  NotificationChannel,
  NotificationRecord,
  Organization,
  OverviewResponse,
  Server,
  ServerDetailMetrics,
  ServerHistoryResponse,
  ServiceCheck,
  ServiceHistoryResponse,
  SystemIntegrations,
  TimeseriesResponse,
  User,
} from "@/types/api";

// Deterministic fixture data strictly for explicitly enabled DEMO MODE (PRD Section 26).
// Never mixed with live backend data.

const BASE_TIME = Date.now();
const isoOffset = (minsAgo: number) => new Date(BASE_TIME - minsAgo * 60_000).toISOString();

export const DEMO_ORG: Organization = {
  id: "demo-org-0001",
  name: "CloudPulse Demo Infrastructure [DEMO]",
  description: "Predictable demonstration dataset for exploring CloudPulse without live agents.",
  timezone: "UTC",
  display_units: "binary",
  date_format: "YYYY-MM-DD",
  time_format: "24h",
  default_scrape_interval: 15,
  default_monitoring_timeout: 5,
  agent_connection_timeout: 10,
  disk_exclusion_patterns: "/proc,/sys,/dev,/run",
  default_environment: "production",
  onboarding_completed: true,
  created_at: isoOffset(1440 * 14),
  updated_at: isoOffset(30),
};

export const DEMO_USER: User = {
  id: "demo-user-0001",
  organization_id: DEMO_ORG.id,
  name: "Demo Administrator",
  email: "demo@cloudpulse.dev",
  role: "admin",
  is_active: true,
  created_at: isoOffset(1440 * 14),
  updated_at: isoOffset(30),
};

export const DEMO_USERS: User[] = [
  DEMO_USER,
  {
    id: "demo-user-0002",
    organization_id: DEMO_ORG.id,
    name: "SRE Operator [DEMO]",
    email: "operator@cloudpulse.dev",
    role: "operator",
    is_active: true,
    created_at: isoOffset(1440 * 10),
    updated_at: isoOffset(120),
  },
  {
    id: "demo-user-0003",
    organization_id: DEMO_ORG.id,
    name: "Audit Viewer [DEMO]",
    email: "viewer@cloudpulse.dev",
    role: "viewer",
    is_active: true,
    created_at: isoOffset(1440 * 7),
    updated_at: isoOffset(300),
  },
];

export const DEMO_SERVERS: Server[] = [
  {
    id: "demo-srv-01",
    organization_id: DEMO_ORG.id,
    name: "prod-api-01",
    hostname: "prod-api-01.eu-west.internal",
    environment: "production",
    operating_system: "linux",
    architecture: "amd64",
    description: "Primary Go API cluster node [DEMO]",
    server_group: "api-cluster",
    scrape_address: "10.20.1.11:9180",
    monitoring_interval: 15,
    agent_version: "1.0.0",
    monitoring_enabled: true,
    last_seen_at: isoOffset(0.2),
    tags: [{ key: "tier", value: "backend" }, { key: "region", value: "eu-west-1" }],
    created_at: isoOffset(1440 * 12),
    updated_at: isoOffset(1),
    status: "UP",
    availability: "UP",
    cpu_percent: 38.4,
    memory_percent: 61.2,
    disk_percent: 54.0,
    is_stale: false,
    last_sample_at: isoOffset(0.2),
    data_source: "demo_fixture",
  },
  {
    id: "demo-srv-02",
    organization_id: DEMO_ORG.id,
    name: "prod-db-01",
    hostname: "prod-db-01.eu-west.internal",
    environment: "production",
    operating_system: "linux",
    architecture: "amd64",
    description: "Primary PostgreSQL database server [DEMO]",
    server_group: "database",
    scrape_address: "10.20.2.21:9180",
    monitoring_interval: 15,
    agent_version: "1.0.0",
    monitoring_enabled: true,
    last_seen_at: isoOffset(0.2),
    tags: [{ key: "role", value: "postgres-primary" }],
    created_at: isoOffset(1440 * 12),
    updated_at: isoOffset(1),
    status: "WARNING",
    availability: "UP",
    cpu_percent: 78.6,
    memory_percent: 82.4,
    disk_percent: 84.1,
    is_stale: false,
    last_sample_at: isoOffset(0.2),
    data_source: "demo_fixture",
  },
  {
    id: "demo-srv-03",
    organization_id: DEMO_ORG.id,
    name: "staging-worker-01",
    hostname: "staging-worker-01.internal",
    environment: "staging",
    operating_system: "linux",
    architecture: "arm64",
    description: "Background job processing worker [DEMO]",
    server_group: "workers",
    scrape_address: "10.20.4.31:9180",
    monitoring_interval: 15,
    agent_version: "1.0.0",
    monitoring_enabled: true,
    last_seen_at: isoOffset(0.2),
    tags: [{ key: "queue", value: "async" }],
    created_at: isoOffset(1440 * 5),
    updated_at: isoOffset(1),
    status: "UP",
    availability: "UP",
    cpu_percent: 24.1,
    memory_percent: 45.8,
    disk_percent: 37.5,
    is_stale: false,
    last_sample_at: isoOffset(0.2),
    data_source: "demo_fixture",
  },
  {
    id: "demo-srv-04",
    organization_id: DEMO_ORG.id,
    name: "dev-edge-02",
    hostname: "dev-edge-02.internal",
    environment: "development",
    operating_system: "linux",
    architecture: "amd64",
    description: "Edge proxy node simulating offline agent [DEMO]",
    server_group: "edge",
    scrape_address: "10.20.8.42:9180",
    monitoring_interval: 15,
    agent_version: "1.0.0",
    monitoring_enabled: true,
    last_seen_at: isoOffset(18),
    tags: [{ key: "edge", value: "nginx" }],
    created_at: isoOffset(1440 * 3),
    updated_at: isoOffset(18),
    status: "DOWN",
    availability: "DOWN",
    cpu_percent: null,
    memory_percent: null,
    disk_percent: null,
    is_stale: true,
    last_sample_at: isoOffset(18),
    data_source: "demo_fixture",
  },
];

export const DEMO_SERVICES: ServiceCheck[] = [
  {
    id: "demo-svc-01",
    server_id: "demo-srv-01",
    server_name: "prod-api-01",
    environment: "production",
    name: "cloudpulse-api",
    check_type: "http",
    target: "http://127.0.0.1:8080/healthz",
    interval_seconds: 30,
    timeout_seconds: 5,
    expected_status_code: 200,
    enabled: true,
    created_at: isoOffset(1440 * 10),
    updated_at: isoOffset(5),
    status: "UP",
    latency_ms: 14.2,
    http_status: 200,
    availability_24h: 99.96,
    valid_intervals_24h: 288,
    last_checked_at: isoOffset(0.3),
  },
  {
    id: "demo-svc-02",
    server_id: "demo-srv-02",
    server_name: "prod-db-01",
    environment: "production",
    name: "postgresql-primary",
    check_type: "tcp",
    target: "127.0.0.1:5432",
    interval_seconds: 30,
    timeout_seconds: 5,
    expected_status_code: 0,
    enabled: true,
    created_at: isoOffset(1440 * 10),
    updated_at: isoOffset(5),
    status: "UP",
    latency_ms: 3.1,
    availability_24h: 100.0,
    valid_intervals_24h: 288,
    last_checked_at: isoOffset(0.3),
  },
  {
    id: "demo-svc-03",
    server_id: "demo-srv-04",
    server_name: "dev-edge-02",
    environment: "development",
    name: "edge-gateway",
    check_type: "http",
    target: "https://edge-dev.internal/health",
    interval_seconds: 30,
    timeout_seconds: 5,
    expected_status_code: 200,
    enabled: true,
    created_at: isoOffset(1440 * 3),
    updated_at: isoOffset(18),
    status: "DOWN",
    latency_ms: null,
    http_status: 503,
    availability_24h: 91.32,
    valid_intervals_24h: 288,
    last_checked_at: isoOffset(1),
  },
];

export const DEMO_ALERTS: AlertView[] = [
  {
    fingerprint: "demo-fp-server-down",
    title: "Monitoring target down: dev-edge-02 [DEMO]",
    alert_name: "ServerDown",
    severity: "critical",
    description: "Prometheus failed to scrape CloudPulse agent on dev-edge-02 (10.20.8.42:9180) for more than 1 minute.",
    server_id: "demo-srv-04",
    server_name: "dev-edge-02",
    environment: "development",
    state: "active",
    triggered_at: isoOffset(17),
    updated_at: isoOffset(1),
    labels: { alertname: "ServerDown", severity: "critical", server_name: "dev-edge-02", environment: "development" },
    annotations: { summary: "Monitoring target down: dev-edge-02 [DEMO]" },
    acknowledged: false,
  },
  {
    fingerprint: "demo-fp-disk-warn",
    title: "High disk utilization on prod-db-01 (/var/lib/postgresql) [DEMO]",
    alert_name: "DiskUsageWarning",
    severity: "warning",
    description: "Disk mount /var/lib/postgresql on prod-db-01 is 84.1% full.",
    server_id: "demo-srv-02",
    server_name: "prod-db-01",
    environment: "production",
    state: "active",
    triggered_at: isoOffset(42),
    updated_at: isoOffset(2),
    labels: { alertname: "DiskUsageWarning", severity: "warning", server_name: "prod-db-01", mountpoint: "/var/lib/postgresql" },
    annotations: { summary: "High disk utilization on prod-db-01 [DEMO]" },
    acknowledged: true,
    acknowledgement: {
      id: "demo-ack-01",
      alert_fingerprint: "demo-fp-disk-warn",
      acknowledged_by_name: "Demo Administrator",
      acknowledged_at: isoOffset(25),
      note: "Scheduled WAL archive compaction at 03:00 UTC [DEMO]",
    },
  },
];

export const DEMO_ALERT_RULES: AlertRule[] = [
  { id: "rule-1", organization_id: DEMO_ORG.id, name: "CPU Usage Warning", description: "Trigger when CPU usage exceeds 75% for 5 minutes", expression: "cloudpulse_cpu_usage_percent > 75", metric_template: "cpu_usage", threshold: 75, severity: "warning", duration: "5m", environments: ["production", "staging", "development"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-2", organization_id: DEMO_ORG.id, name: "CPU Usage Critical", description: "Trigger when CPU usage exceeds 90% for 5 minutes", expression: "cloudpulse_cpu_usage_percent > 90", metric_template: "cpu_usage", threshold: 90, severity: "critical", duration: "5m", environments: ["production", "staging"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-3", organization_id: DEMO_ORG.id, name: "Memory Usage Warning", description: "Trigger when memory usage exceeds 80% for 5 minutes", expression: "cloudpulse_memory_usage_percent > 80", metric_template: "memory_usage", threshold: 80, severity: "warning", duration: "5m", environments: ["production", "staging"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-4", organization_id: DEMO_ORG.id, name: "Memory Usage Critical", description: "Trigger when memory usage exceeds 95% for 5 minutes", expression: "cloudpulse_memory_usage_percent > 95", metric_template: "memory_usage", threshold: 95, severity: "critical", duration: "5m", environments: ["production"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-5", organization_id: DEMO_ORG.id, name: "Disk Usage Warning", description: "Trigger when any monitored disk exceeds 80%", expression: "cloudpulse_disk_usage_percent > 80", metric_template: "disk_usage", threshold: 80, severity: "warning", duration: "5m", environments: ["production", "staging"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-6", organization_id: DEMO_ORG.id, name: "Disk Usage Critical", description: "Trigger when any monitored disk exceeds 90%", expression: "cloudpulse_disk_usage_percent > 90", metric_template: "disk_usage", threshold: 90, severity: "critical", duration: "5m", environments: ["production"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-7", organization_id: DEMO_ORG.id, name: "Server Down", description: "Trigger when Prometheus scrape target is unavailable", expression: 'up{job="cloudpulse-agents"} == 0', metric_template: "server_down", threshold: 0, severity: "critical", duration: "1m", environments: ["production", "staging", "development"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-8", organization_id: DEMO_ORG.id, name: "Service Down", description: "Trigger when HTTP or TCP service check fails", expression: "cloudpulse_service_up == 0", metric_template: "service_down", threshold: 0, severity: "critical", duration: "2m", environments: ["production", "staging"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
  { id: "rule-9", organization_id: DEMO_ORG.id, name: "High Network Errors", description: "Trigger when network error rate exceeds 5/s", expression: "rate(cloudpulse_network_receive_errors_total[5m]) + rate(cloudpulse_network_transmit_errors_total[5m]) > 5", metric_template: "network_errors", threshold: 5, severity: "warning", duration: "5m", environments: ["production"], is_default: true, enabled: true, last_eval_status: "active", created_at: isoOffset(14400), updated_at: isoOffset(60) },
];

export const DEMO_CHANNELS: NotificationChannel[] = [
  { id: "ch-1", name: "Ops Slack Alerts [DEMO]", channel_type: "slack", configuration_reference: "env:SLACK_WEBHOOK_URL", target_summary: "#infra-alerts", severity_filters: ["critical", "warning"], last_status: "delivered_via_alertmanager", last_tested_at: isoOffset(120), enabled: true, created_at: isoOffset(5000), updated_at: isoOffset(120) },
  { id: "ch-2", name: "On-Call Email [DEMO]", channel_type: "email", configuration_reference: "oncall@cloudpulse.dev", target_summary: "oncall@cloudpulse.dev", severity_filters: ["critical"], last_status: "delivered_via_alertmanager", last_tested_at: isoOffset(300), enabled: true, created_at: isoOffset(5000), updated_at: isoOffset(300) },
];

export const DEMO_NOTIFICATIONS: NotificationRecord[] = [
  { id: "n-1", alert_fingerprint: "demo-fp-server-down", title: "Agent disconnected: dev-edge-02 [DEMO]", message: "Prometheus failed to scrape 10.20.8.42:9180.", category: "alert", severity: "critical", status: "delivered", is_read: false, attempted_at: isoOffset(17), delivered_at: isoOffset(17) },
  { id: "n-2", alert_fingerprint: "demo-fp-disk-warn", title: "Disk threshold exceeded on prod-db-01 [DEMO]", message: "/var/lib/postgresql reached 84.1% utilization.", category: "alert", severity: "warning", status: "delivered", is_read: false, attempted_at: isoOffset(42), delivered_at: isoOffset(42) },
  { id: "n-3", alert_fingerprint: "", title: "New server registered: staging-worker-01 [DEMO]", message: "Added to Prometheus file_sd discovery.", category: "infrastructure", severity: "info", status: "delivered", is_read: true, attempted_at: isoOffset(360), delivered_at: isoOffset(360) },
];

export const DEMO_AUDIT_LOGS: AuditLog[] = [
  { id: "al-1", actor_name: "Demo Administrator", actor_email: "demo@cloudpulse.dev", action: "alert.acknowledged", resource_type: "alert", resource_id: "demo-fp-disk-warn", source_ip: "127.0.0.1", metadata: { note: "Scheduled WAL compaction [DEMO]" }, created_at: isoOffset(25) },
  { id: "al-2", actor_name: "Demo Administrator", actor_email: "demo@cloudpulse.dev", action: "server.registered", resource_type: "server", resource_id: "demo-srv-03", source_ip: "127.0.0.1", metadata: { name: "staging-worker-01", environment: "staging" }, created_at: isoOffset(360) },
  { id: "al-3", actor_name: "Demo Administrator", actor_email: "demo@cloudpulse.dev", action: "auth.login_success", resource_type: "user", resource_id: "demo-user-0001", source_ip: "127.0.0.1", metadata: { remember_me: true }, created_at: isoOffset(400) },
];

export const DEMO_METRIC_CATALOG: MetricCatalogItem[] = [
  { id: "cpu_usage", name: "CPU Usage", category: "CPU", unit: "%", description: "Aggregate host CPU utilization percentage", base_promql: "cloudpulse_cpu_usage_percent", metric_type: "gauge" },
  { id: "cpu_load_1m", name: "CPU Load (1m)", category: "CPU", unit: "load", description: "1-minute system load average", base_promql: "cloudpulse_cpu_load_1m", metric_type: "gauge" },
  { id: "memory_usage", name: "Memory Usage", category: "Memory", unit: "%", description: "Host memory utilization percentage", base_promql: "cloudpulse_memory_usage_percent", metric_type: "gauge" },
  { id: "memory_available", name: "Available Memory", category: "Memory", unit: "bytes", description: "OS-reported available memory", base_promql: "cloudpulse_memory_available_bytes", metric_type: "gauge" },
  { id: "disk_usage", name: "Disk Usage", category: "Disk", unit: "%", description: "Disk utilization by mountpoint", base_promql: "cloudpulse_disk_usage_percent", metric_type: "gauge" },
  { id: "disk_free", name: "Disk Free Space", category: "Disk", unit: "bytes", description: "Free disk space by mountpoint", base_promql: "cloudpulse_disk_free_bytes", metric_type: "gauge" },
  { id: "net_rx_rate", name: "Network Receive Rate", category: "Network", unit: "B/s", description: "5m receive throughput rate", base_promql: "rate(cloudpulse_network_receive_bytes_total[5m])", metric_type: "counter_rate" },
  { id: "net_tx_rate", name: "Network Transmit Rate", category: "Network", unit: "B/s", description: "5m transmit throughput rate", base_promql: "rate(cloudpulse_network_transmit_bytes_total[5m])", metric_type: "counter_rate" },
  { id: "net_packet_errors", name: "Packet Errors Rate", category: "Network", unit: "err/s", description: "Combined network error rate", base_promql: "rate(cloudpulse_network_receive_errors_total[5m]) + rate(cloudpulse_network_transmit_errors_total[5m])", metric_type: "counter_rate" },
  { id: "service_up", name: "Service Availability", category: "Services", unit: "state", description: "Service check status (1=UP, 0=DOWN)", base_promql: "cloudpulse_service_up", metric_type: "gauge" },
  { id: "service_latency", name: "Service Response Time", category: "Services", unit: "s", description: "Service response latency in seconds", base_promql: "cloudpulse_service_response_duration_seconds", metric_type: "gauge" },
  { id: "agent_health", name: "Agent Health", category: "Agent", unit: "state", description: "Prometheus target reachability", base_promql: 'up{job="cloudpulse-agents"}', metric_type: "gauge" },
];

export function buildDemoTimeseries(rangeKey = "1h"): TimeseriesResponse {
  const pts = Array.from({ length: 24 }, (_, idx) => {
    const minsAgo = (23 - idx) * 2.5;
    // Deterministic smooth wave (no Math.random())
    const wave = Math.sin(idx * 0.45);
    return {
      timestamp: isoOffset(minsAgo),
      cpu_percent: Number((45.5 + wave * 9.2 + (idx % 3) * 1.1).toFixed(2)),
      memory_percent: Number((62.4 + Math.cos(idx * 0.25) * 4.1).toFixed(2)),
      disk_percent: Number((58.5 + idx * 0.04).toFixed(2)),
    };
  });
  return {
    prometheus_available: true,
    range: rangeKey,
    start: pts[0].timestamp,
    end: pts[pts.length - 1].timestamp,
    step_seconds: 150,
    points: pts,
  };
}

export function buildDemoOverview(): OverviewResponse {
  const ts = buildDemoTimeseries("1h");
  return {
    prometheus_available: true,
    data_source: "demo_fixture",
    aggregation_method: "unweighted_mean_of_reporting_machines",
    generated_at: new Date().toISOString(),
    servers: { total: 4, up: 3, down: 1, unknown: 0, pending: 0, availability_percent: 75.0 },
    health: { healthy: 2, warning: 1, critical: 1, unknown: 0 },
    cpu: {
      average_percent: 47.03,
      change_percent: 2.4,
      included_count: 3,
      trend: ts.points.map((p) => ({ timestamp: p.timestamp, value: p.cpu_percent ?? 0 })),
    },
    memory: {
      average_percent: 63.13,
      used_bytes: 31.6 * 1024 ** 3,
      total_bytes: 48 * 1024 ** 3,
      available_bytes: 16.4 * 1024 ** 3,
      change_percent: 1.1,
      included_count: 3,
    },
    disk: {
      average_percent: 58.53,
      highest_server: "prod-db-01",
      highest_server_id: "demo-srv-02",
      highest_percent: 84.1,
      critical_count: 0,
      included_count: 3,
    },
    alerts: { total: 2, critical: 1, warning: 1, info: 0, unacknowledged: 1 },
    recent_events: [
      { id: "ev-1", action: "agent.offline", title: "Agent went offline: dev-edge-02 [DEMO]", resource_type: "server", resource_id: "demo-srv-04", actor: "Prometheus", severity: "critical", timestamp: isoOffset(17) },
      { id: "ev-2", action: "alert.acknowledged", title: "Alert acknowledged: DiskUsageWarning [DEMO]", resource_type: "alert", resource_id: "demo-fp-disk-warn", actor: "Demo Administrator", severity: "info", timestamp: isoOffset(25) },
      { id: "ev-3", action: "disk.threshold", title: "Disk threshold exceeded on prod-db-01 [DEMO]", resource_type: "server", resource_id: "demo-srv-02", actor: "Prometheus", severity: "warning", timestamp: isoOffset(42) },
    ],
    grafana_dashboards: DEMO_INTEGRATIONS.grafana.dashboards,
  };
}

export function buildDemoInfraHealth(): InfrastructureHealthResponse {
  return {
    prometheus_available: true,
    total_machines: DEMO_SERVERS.length,
    by_environment: { production: 2, staging: 1, development: 1 },
    by_operating_system: { linux: 4 },
    by_status: { UP: 2, WARNING: 1, DOWN: 1 },
    recently_added: DEMO_SERVERS,
    recently_disconnected: [DEMO_SERVERS[3]],
    grafana_dashboards: DEMO_INTEGRATIONS.grafana.dashboards,
  };
}

export function buildDemoServerDetail(serverId: string): ServerDetailMetrics {
  const srv = DEMO_SERVERS.find((s) => s.id === serverId) ?? DEMO_SERVERS[0];
  return {
    prometheus_available: true,
    server: srv,
    cpu: {
      usage_percent: srv.cpu_percent,
      core_count: 4,
      per_core_percent: { "0": 34.2, "1": 42.1, "2": 39.8, "3": 37.5 },
      load_1m: 1.42,
      load_5m: 1.28,
      load_15m: 1.15,
      load_supported: true,
      saturation_warning: (srv.cpu_percent ?? 0) >= 75,
      peak_percent: 68.4,
      average_percent: 41.2,
    },
    memory: {
      total_bytes: 16 * 1024 ** 3,
      used_bytes: 9.8 * 1024 ** 3,
      available_bytes: 6.2 * 1024 ** 3,
      cached_bytes: 3.1 * 1024 ** 3,
      free_bytes: 3.1 * 1024 ** 3,
      usage_percent: srv.memory_percent,
    },
    disks: [
      { mountpoint: "/", device: "/dev/nvme0n1p1", filesystem: "ext4", total_bytes: 120 * 1024 ** 3, used_bytes: 64.8 * 1024 ** 3, free_bytes: 55.2 * 1024 ** 3, usage_percent: 54.0, collected_at: isoOffset(0.2) },
      { mountpoint: "/var/lib/postgresql", device: "/dev/nvme1n1", filesystem: "xfs", total_bytes: 250 * 1024 ** 3, used_bytes: 210.2 * 1024 ** 3, free_bytes: 39.8 * 1024 ** 3, usage_percent: 84.1, collected_at: isoOffset(0.2) },
    ],
    networks: [
      { interface: "eth0", status: "up", receive_bytes_per_sec: 1_840_200, transmit_bytes_per_sec: 1_210_400, total_receive_bytes: 482 * 1024 ** 3, total_transmit_bytes: 319 * 1024 ** 3, receive_packets_total: 412_000_000, transmit_packets_total: 318_000_000, receive_errors_total: 0, transmit_errors_total: 0 },
    ],
    services: DEMO_SERVICES.filter((sc) => sc.server_id === srv.id),
    recent_activity: buildDemoOverview().recent_events,
    grafana_dashboards: DEMO_INTEGRATIONS.grafana.dashboards,
  };
}

export function buildDemoServerHistory(rangeKey = "1h"): ServerHistoryResponse {
  const ts = buildDemoTimeseries(rangeKey);
  return {
    prometheus_available: true,
    range: rangeKey,
    start: ts.start,
    end: ts.end,
    cpu: ts.points.map((p) => ({ timestamp: p.timestamp, value: p.cpu_percent ?? 0 })),
    memory: ts.points.map((p) => ({ timestamp: p.timestamp, value: p.memory_percent ?? 0 })),
    disk: ts.points.map((p) => ({ timestamp: p.timestamp, value: p.disk_percent ?? 0 })),
    network_rx_rate: ts.points.map((p, i) => ({ timestamp: p.timestamp, value: 1_500_000 + Math.sin(i * 0.5) * 400_000 })),
    network_tx_rate: ts.points.map((p, i) => ({ timestamp: p.timestamp, value: 980_000 + Math.cos(i * 0.5) * 280_000 })),
  };
}

export function buildDemoServiceHistory(serviceId: string): ServiceHistoryResponse {
  const svc = DEMO_SERVICES.find((s) => s.id === serviceId) ?? DEMO_SERVICES[0];
  const pts = Array.from({ length: 20 }, (_, i) => ({
    timestamp: isoOffset((19 - i) * 3),
    value: svc.status === "DOWN" && i > 15 ? 0 : 1,
  }));
  const lat = Array.from({ length: 20 }, (_, i) => ({
    timestamp: isoOffset((19 - i) * 3),
    value: Number((12 + Math.sin(i * 0.6) * 4).toFixed(2)),
  }));
  return {
    prometheus_available: true,
    service: svc,
    availability_percent: svc.availability_24h,
    valid_intervals: 20,
    failed_intervals: pts.filter((p) => p.value === 0).length,
    average_latency_ms: 13.4,
    up_timeline: pts,
    latency_timeline: lat,
  };
}

export function buildDemoExplorerResult(query: string, rangeKey = "1h"): ExplorerQueryResult {
  const ts = buildDemoTimeseries(rangeKey);
  const pts = ts.points.map((p) => ({ timestamp: p.timestamp, value: p.cpu_percent ?? 0 }));
  const vals = pts.map((p) => p.value);
  return {
    prometheus_available: true,
    query: query || "cloudpulse_cpu_usage_percent",
    range: rangeKey,
    start: ts.start,
    end: ts.end,
    series: [
      { label: "prod-api-01 [DEMO]", metric: { server_name: "prod-api-01", environment: "production" }, points: pts },
      { label: "prod-db-01 [DEMO]", metric: { server_name: "prod-db-01", environment: "production" }, points: pts.map((p) => ({ timestamp: p.timestamp, value: Number((p.value + 24).toFixed(2)) })) },
    ],
    summary: {
      min: Math.min(...vals),
      max: Math.max(...vals),
      avg: Number((vals.reduce((a, b) => a + b, 0) / vals.length).toFixed(2)),
      first: vals[0],
      last: vals[vals.length - 1],
      count: vals.length,
    },
    missing_data_points: 0,
  };
}

export function buildDemoAlertDetail(fp: string): AlertDetailResponse {
  const alert = DEMO_ALERTS.find((a) => a.fingerprint === fp) ?? DEMO_ALERTS[0];
  const ts = buildDemoTimeseries("1h");
  return {
    alert,
    acknowledgements: alert.acknowledgement ? [alert.acknowledgement] : [],
    related_metrics: ts.points.map((p) => ({ timestamp: p.timestamp, value: p.cpu_percent ?? 0 })),
    related_metric_label: "CPU Usage (%) [DEMO]",
    timeline: [
      { event: "Alert Triggered", actor: "Prometheus", detail: alert.description, timestamp: alert.triggered_at },
      ...(alert.acknowledgement
        ? [{ event: "Alert Acknowledged", actor: alert.acknowledgement.acknowledged_by_name ?? "Operator", detail: alert.acknowledgement.note, timestamp: alert.acknowledgement.acknowledged_at }]
        : []),
    ],
  };
}

export const DEMO_INTEGRATIONS: SystemIntegrations = {
  postgres: { status: "healthy", diagnostic: "Demo mode active — PostgreSQL bypassed with deterministic fixtures" },
  prometheus: { status: "healthy", url: "http://prometheus:9090", diagnostic: "Demo mode active — Prometheus queries simulated with deterministic fixtures" },
  alertmanager: { status: "healthy", url: "http://alertmanager:9093", diagnostic: "Demo mode active — Alertmanager routing simulated" },
  grafana: {
    status: "healthy",
    public_url: "http://localhost:3001",
    diagnostic: "Demo mode active — Grafana links point to local Docker Compose default",
    dashboards: [
      { uid: "cloudpulse-infra-overview", title: "Infrastructure Overview", description: "Fleet availability, CPU, Memory, Disk & Alerts", url: "http://localhost:3001/d/cloudpulse-infra-overview/infrastructure-overview" },
      { uid: "cloudpulse-server-perf", title: "Server Performance", description: "Per-server CPU, load, memory, mounts & network", url: "http://localhost:3001/d/cloudpulse-server-perf/server-performance" },
      { uid: "cloudpulse-service-mon", title: "Service Monitoring", description: "HTTP/TCP uptime, latency & failed checks", url: "http://localhost:3001/d/cloudpulse-service-mon/service-monitoring" },
      { uid: "cloudpulse-network-mon", title: "Network Monitoring", description: "Rx/Tx throughput, packets & interface errors", url: "http://localhost:3001/d/cloudpulse-network-mon/network-monitoring" },
    ],
  },
};
