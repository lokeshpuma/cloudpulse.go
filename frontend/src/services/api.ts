import { apiDownloadBlob, apiRequest } from "@/lib/apiClient";
import {
  buildDemoAlertDetail,
  buildDemoExplorerResult,
  buildDemoInfraHealth,
  buildDemoOverview,
  buildDemoServerDetail,
  buildDemoServerHistory,
  buildDemoServiceHistory,
  buildDemoTimeseries,
  DEMO_ALERT_RULES,
  DEMO_ALERTS,
  DEMO_AUDIT_LOGS,
  DEMO_CHANNELS,
  DEMO_INTEGRATIONS,
  DEMO_METRIC_CATALOG,
  DEMO_NOTIFICATIONS,
  DEMO_ORG,
  DEMO_SERVERS,
  DEMO_SERVICES,
  DEMO_USERS,
} from "@/lib/demoData";
import type {
  AlertDetailResponse,
  AlertRule,
  AlertView,
  AuditLog,
  AuthPayload,
  ExplorerQueryResult,
  InfrastructureHealthResponse,
  MetricCatalogItem,
  NotificationChannel,
  NotificationRecord,
  Organization,
  OverviewResponse,
  PaginatedResponse,
  QueryHistoryItem,
  Server,
  ServerDetailMetrics,
  ServerHistoryResponse,
  ServerStatusCheck,
  ServiceCheck,
  ServiceHistoryResponse,
  SystemIntegrations,
  TimeseriesResponse,
  User,
} from "@/types/api";

// Track whether the active session is in explicit Demo Mode (never mixed with live backend).
let demoActive = false;

export function setApiDemoMode(enabled: boolean) {
  demoActive = enabled;
}

export function isApiDemoMode(): boolean {
  return demoActive;
}

// Local mutable copies ONLY for Demo Mode interactive preview
let demoServers = [...DEMO_SERVERS];
let demoServices = [...DEMO_SERVICES];
let demoAlerts = [...DEMO_ALERTS];
let demoRules = [...DEMO_ALERT_RULES];
let demoChannels = [...DEMO_CHANNELS];
let demoNotifs = [...DEMO_NOTIFICATIONS];
let demoOrg = { ...DEMO_ORG };
let demoUsers = [...DEMO_USERS];

export const api = {
  // Authentication
  async register(body: { name: string; email: string; password: string; organization_name: string }) {
    return apiRequest<AuthPayload>("/api/v1/auth/register", { method: "POST", body, skipAuth: true });
  },
  async login(body: { email: string; password: string; remember_me: boolean }) {
    return apiRequest<AuthPayload>("/api/v1/auth/login", { method: "POST", body, skipAuth: true });
  },
  async refresh() {
    return apiRequest<AuthPayload>("/api/v1/auth/refresh", { method: "POST", includeCsrf: true, skipAuth: true });
  },
  async logout() {
    return apiRequest<{ logged_out: boolean }>("/api/v1/auth/logout", { method: "POST", includeCsrf: true });
  },
  async forgotPassword(email: string) {
    return apiRequest<{ instructions: string; dev_reset_token?: string }>("/api/v1/auth/forgot-password", {
      method: "POST",
      body: { email },
      skipAuth: true,
    });
  },
  async resetPassword(token: string, new_password: string) {
    return apiRequest<{ reset: boolean }>("/api/v1/auth/reset-password", {
      method: "POST",
      body: { token, new_password },
      skipAuth: true,
    });
  },
  async me() {
    return apiRequest<{ user: User; organization: Organization }>("/api/v1/auth/me");
  },

  // Organization & Users
  async getOrg(): Promise<Organization> {
    if (demoActive) return { ...demoOrg };
    return apiRequest<Organization>("/api/v1/organizations/current");
  },
  async updateOrg(patch: Partial<Organization>): Promise<Organization> {
    if (demoActive) {
      demoOrg = { ...demoOrg, ...patch, updated_at: new Date().toISOString() };
      return { ...demoOrg };
    }
    return apiRequest<Organization>("/api/v1/organizations/current", { method: "PATCH", body: patch });
  },
  async listUsers(): Promise<User[]> {
    if (demoActive) return [...demoUsers];
    return apiRequest<User[]>("/api/v1/users");
  },
  async createUser(body: { name: string; email: string; password: string; role: string }): Promise<User> {
    if (demoActive) {
      const u: User = {
        id: `demo-u-${Date.now()}`,
        organization_id: demoOrg.id,
        name: body.name,
        email: body.email,
        role: body.role as User["role"],
        is_active: true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      demoUsers = [...demoUsers, u];
      return u;
    }
    return apiRequest<User>("/api/v1/users", { method: "POST", body });
  },
  async updateUser(id: string, body: { name?: string; role?: string; is_active?: boolean }): Promise<User> {
    if (demoActive) {
      demoUsers = demoUsers.map((u) => (u.id === id ? { ...u, ...body, role: (body.role ?? u.role) as User["role"] } : u));
      return demoUsers.find((u) => u.id === id)!;
    }
    return apiRequest<User>(`/api/v1/users/${id}`, { method: "PATCH", body });
  },
  async disableUser(id: string): Promise<User> {
    if (demoActive) {
      demoUsers = demoUsers.map((u) => (u.id === id ? { ...u, is_active: false } : u));
      return demoUsers.find((u) => u.id === id)!;
    }
    return apiRequest<User>(`/api/v1/users/${id}/disable`, { method: "POST" });
  },
  async resetUserAccess(id: string) {
    if (demoActive) return { sessions_revoked: true, reset_issued: true };
    return apiRequest<{ sessions_revoked: boolean; reset_issued: boolean; dev_reset_token?: string }>(
      `/api/v1/users/${id}/reset-access`,
      { method: "POST" }
    );
  },

  // Monitoring Overview & Health & Timeseries
  async getOverview(range = "1h"): Promise<OverviewResponse> {
    if (demoActive) return buildDemoOverview();
    return apiRequest<OverviewResponse>(`/api/v1/monitoring/overview?range=${encodeURIComponent(range)}`);
  },
  async getInfrastructureHealth(): Promise<InfrastructureHealthResponse> {
    if (demoActive) return buildDemoInfraHealth();
    return apiRequest<InfrastructureHealthResponse>("/api/v1/monitoring/health");
  },
  async getTimeseries(params: { range?: string; start?: string; end?: string; server_id?: string; environment?: string }): Promise<TimeseriesResponse> {
    if (demoActive) return buildDemoTimeseries(params.range ?? "1h");
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v) q.set(k, v);
    });
    return apiRequest<TimeseriesResponse>(`/api/v1/monitoring/timeseries?${q.toString()}`);
  },
  async getMetricCatalog(): Promise<{ catalog: MetricCatalogItem[]; history: QueryHistoryItem[] }> {
    if (demoActive) return { catalog: DEMO_METRIC_CATALOG, history: [] };
    return apiRequest<{ catalog: MetricCatalogItem[]; history: QueryHistoryItem[] }>("/api/v1/monitoring/metrics");
  },
  async executeMetricQuery(params: {
    query?: string;
    metric?: string;
    server_id?: string;
    environment?: string;
    aggregation?: string;
    range?: string;
    start?: string;
    end?: string;
  }): Promise<{ catalog: MetricCatalogItem[]; result: ExplorerQueryResult; history: QueryHistoryItem[] }> {
    if (demoActive) {
      return {
        catalog: DEMO_METRIC_CATALOG,
        result: buildDemoExplorerResult(params.query || params.metric || "cloudpulse_cpu_usage_percent", params.range ?? "1h"),
        history: [{ id: "dh-1", metric_name: params.metric || "PromQL", query: params.query || "cloudpulse_cpu_usage_percent", time_range: params.range || "1h", created_at: new Date().toISOString() }],
      };
    }
    const q = new URLSearchParams({ execute: "true" });
    Object.entries(params).forEach(([k, v]) => {
      if (v) q.set(k, v);
    });
    return apiRequest(`/api/v1/monitoring/metrics?${q.toString()}`);
  },

  // Servers
  async listServers(params: {
    search?: string;
    environment?: string;
    health?: string;
    server_group?: string;
    page?: number;
    page_size?: number;
    sort_by?: string;
    sort_dir?: string;
  } = {}): Promise<PaginatedResponse<Server>> {
    if (demoActive) {
      let items = [...demoServers];
      if (params.search) {
        const s = params.search.toLowerCase();
        items = items.filter((i) => i.name.toLowerCase().includes(s) || i.hostname.toLowerCase().includes(s));
      }
      if (params.environment) items = items.filter((i) => i.environment === params.environment);
      if (params.health) items = items.filter((i) => i.status === params.health);
      if (params.server_group) items = items.filter((i) => i.server_group === params.server_group);
      return { items, total: items.length, page: 1, page_size: 25, total_pages: 1, prometheus_available: true };
    }
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v !== undefined && v !== "") q.set(k, String(v));
    });
    return apiRequest<PaginatedResponse<Server>>(`/api/v1/servers?${q.toString()}`);
  },
  async createServer(body: {
    name: string;
    hostname: string;
    environment: string;
    operating_system: string;
    architecture?: string;
    description?: string;
    server_group: string;
    scrape_address: string;
    monitoring_interval: number;
    tags?: { key: string; value: string }[];
  }): Promise<Server> {
    if (demoActive) {
      const created: Server = {
        id: `demo-srv-${Date.now()}`,
        organization_id: demoOrg.id,
        name: body.name,
        hostname: body.hostname,
        environment: body.environment,
        operating_system: body.operating_system,
        architecture: body.architecture ?? "amd64",
        description: body.description ?? "",
        server_group: body.server_group,
        scrape_address: body.scrape_address,
        monitoring_interval: body.monitoring_interval,
        agent_version: "",
        monitoring_enabled: true,
        tags: body.tags ?? [],
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
        status: "PENDING",
        availability: "UNKNOWN",
        cpu_percent: null,
        memory_percent: null,
        disk_percent: null,
        is_stale: false,
        data_source: "demo_fixture",
      };
      demoServers = [created, ...demoServers];
      return created;
    }
    return apiRequest<Server>("/api/v1/servers", { method: "POST", body });
  },
  async updateServer(id: string, body: Partial<Server>): Promise<Server> {
    if (demoActive) {
      demoServers = demoServers.map((s) => (s.id === id ? { ...s, ...body } : s));
      return demoServers.find((s) => s.id === id)!;
    }
    return apiRequest<Server>(`/api/v1/servers/${id}`, { method: "PATCH", body });
  },
  async deleteServer(id: string) {
    if (demoActive) {
      demoServers = demoServers.filter((s) => s.id !== id);
      return { deleted: true, historical_metrics_notice: "Removed from demo list." };
    }
    return apiRequest<{ deleted: boolean; historical_metrics_notice: string }>(`/api/v1/servers/${id}`, { method: "DELETE" });
  },
  async enableServer(id: string): Promise<Server> {
    if (demoActive) {
      demoServers = demoServers.map((s) => (s.id === id ? { ...s, monitoring_enabled: true } : s));
      return demoServers.find((s) => s.id === id)!;
    }
    return apiRequest<Server>(`/api/v1/servers/${id}/enable`, { method: "POST" });
  },
  async disableServer(id: string): Promise<Server> {
    if (demoActive) {
      demoServers = demoServers.map((s) => (s.id === id ? { ...s, monitoring_enabled: false, status: "UNKNOWN" } : s));
      return demoServers.find((s) => s.id === id)!;
    }
    return apiRequest<Server>(`/api/v1/servers/${id}/disable`, { method: "POST" });
  },
  async getServerStatus(id: string): Promise<ServerStatusCheck> {
    if (demoActive) {
      const s = demoServers.find((x) => x.id === id) ?? demoServers[0];
      return {
        server_id: s.id,
        name: s.name,
        hostname: s.hostname,
        scrape_address: s.scrape_address,
        status: s.status,
        availability: s.availability,
        connected: s.availability === "UP",
        is_stale: s.is_stale,
        agent_version: s.agent_version || "1.0.0",
        last_successful_scrape: s.last_sample_at,
        collected_metrics: s.availability === "UP" ? ["CPU", "Memory", "Disk"] : [],
        prometheus_available: true,
      };
    }
    return apiRequest<ServerStatusCheck>(`/api/v1/servers/${id}/status`);
  },
  async getServerMetrics(id: string): Promise<ServerDetailMetrics> {
    if (demoActive) return buildDemoServerDetail(id);
    return apiRequest<ServerDetailMetrics>(`/api/v1/servers/${id}/metrics`);
  },
  async getServerHistory(id: string, range = "1h"): Promise<ServerHistoryResponse> {
    if (demoActive) return buildDemoServerHistory(range);
    return apiRequest<ServerHistoryResponse>(`/api/v1/servers/${id}/history?range=${encodeURIComponent(range)}`);
  },

  // Services
  async listServices(serverId?: string): Promise<ServiceCheck[]> {
    if (demoActive) {
      return serverId ? demoServices.filter((s) => s.server_id === serverId) : [...demoServices];
    }
    const q = serverId ? `?server_id=${encodeURIComponent(serverId)}` : "";
    return apiRequest<ServiceCheck[]>(`/api/v1/services${q}`);
  },
  async createService(body: {
    server_id: string;
    name: string;
    check_type: "http" | "tcp";
    target: string;
    interval_seconds: number;
    timeout_seconds: number;
    expected_status_code: number;
  }): Promise<ServiceCheck> {
    if (demoActive) {
      const srv = demoServers.find((s) => s.id === body.server_id);
      const created: ServiceCheck = {
        ...body,
        id: `demo-svc-${Date.now()}`,
        server_name: srv?.name ?? "demo-server",
        environment: srv?.environment ?? "production",
        enabled: true,
        status: "UNKNOWN",
        latency_ms: null,
        availability_24h: null,
        valid_intervals_24h: 0,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      demoServices = [created, ...demoServices];
      return created;
    }
    return apiRequest<ServiceCheck>("/api/v1/services", { method: "POST", body });
  },
  async updateService(id: string, body: Partial<ServiceCheck>): Promise<ServiceCheck> {
    if (demoActive) {
      demoServices = demoServices.map((s) => (s.id === id ? { ...s, ...body } : s));
      return demoServices.find((s) => s.id === id)!;
    }
    return apiRequest<ServiceCheck>(`/api/v1/services/${id}`, { method: "PATCH", body });
  },
  async deleteService(id: string) {
    if (demoActive) {
      demoServices = demoServices.filter((s) => s.id !== id);
      return { deleted: true };
    }
    return apiRequest<{ deleted: boolean }>(`/api/v1/services/${id}`, { method: "DELETE" });
  },
  async getServiceHistory(id: string, range = "6h"): Promise<ServiceHistoryResponse> {
    if (demoActive) return buildDemoServiceHistory(id);
    return apiRequest<ServiceHistoryResponse>(`/api/v1/services/${id}/history?range=${encodeURIComponent(range)}`);
  },

  // Alerts & Alert Rules
  async listAlerts(params: { severity?: string; server?: string; environment?: string; state?: string; acknowledged?: string } = {}): Promise<AlertView[]> {
    if (demoActive) {
      return demoAlerts.filter((a) => {
        if (params.severity && a.severity !== params.severity) return false;
        if (params.environment && a.environment !== params.environment) return false;
        if (params.state && params.state !== "all" && a.state !== params.state) return false;
        if (params.acknowledged === "true" && !a.acknowledged) return false;
        if (params.acknowledged === "false" && a.acknowledged) return false;
        return true;
      });
    }
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v) q.set(k, v);
    });
    return apiRequest<AlertView[]>(`/api/v1/alerts?${q.toString()}`);
  },
  async getAlertDetail(fingerprint: string): Promise<AlertDetailResponse> {
    if (demoActive) return buildDemoAlertDetail(fingerprint);
    return apiRequest<AlertDetailResponse>(`/api/v1/alerts/${encodeURIComponent(fingerprint)}`);
  },
  async acknowledgeAlert(fingerprint: string, note: string) {
    if (demoActive) {
      demoAlerts = demoAlerts.map((a) =>
        a.fingerprint === fingerprint
          ? {
              ...a,
              acknowledged: true,
              acknowledgement: {
                id: `demo-ack-${Date.now()}`,
                alert_fingerprint: fingerprint,
                acknowledged_by_name: "Demo Administrator",
                acknowledged_at: new Date().toISOString(),
                note,
              },
            }
          : a
      );
      return demoAlerts.find((a) => a.fingerprint === fingerprint)?.acknowledgement;
    }
    return apiRequest(`/api/v1/alerts/${encodeURIComponent(fingerprint)}/acknowledge`, { method: "POST", body: { note } });
  },
  async listAlertRules(): Promise<AlertRule[]> {
    if (demoActive) return [...demoRules];
    return apiRequest<AlertRule[]>("/api/v1/alert-rules");
  },
  async createAlertRule(body: Partial<AlertRule>): Promise<AlertRule> {
    if (demoActive) {
      const rule: AlertRule = {
        id: `demo-rule-${Date.now()}`,
        organization_id: demoOrg.id,
        name: body.name ?? "Custom Rule",
        description: body.description ?? "",
        expression: body.expression ?? "cloudpulse_cpu_usage_percent > 80",
        metric_template: body.metric_template ?? "custom",
        threshold: body.threshold ?? 80,
        severity: body.severity ?? "warning",
        duration: body.duration ?? "5m",
        environments: body.environments ?? ["production"],
        is_default: false,
        enabled: body.enabled ?? true,
        last_eval_status: "validated",
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      demoRules = [...demoRules, rule];
      return rule;
    }
    return apiRequest<AlertRule>("/api/v1/alert-rules", { method: "POST", body });
  },
  async updateAlertRule(id: string, body: Partial<AlertRule>): Promise<AlertRule> {
    if (demoActive) {
      demoRules = demoRules.map((r) => (r.id === id ? { ...r, ...body } : r));
      return demoRules.find((r) => r.id === id)!;
    }
    return apiRequest<AlertRule>(`/api/v1/alert-rules/${id}`, { method: "PATCH", body });
  },
  async deleteAlertRule(id: string) {
    if (demoActive) {
      demoRules = demoRules.filter((r) => r.id !== id);
      return { deleted: true };
    }
    return apiRequest<{ deleted: boolean }>(`/api/v1/alert-rules/${id}`, { method: "DELETE" });
  },
  async validateAlertRule(id: string) {
    if (demoActive) return { valid: true, prometheus_evaluated: true, matching_series: 2 };
    return apiRequest<{ valid: boolean; prometheus_evaluated?: boolean; matching_series?: number; error?: string; diagnostic?: string }>(
      `/api/v1/alert-rules/${id}/validate`,
      { method: "POST" }
    );
  },

  // Notifications & Channels
  async listNotifications(params: { severity?: string; unread_only?: boolean; page?: number } = {}): Promise<PaginatedResponse<NotificationRecord>> {
    if (demoActive) {
      let items = [...demoNotifs];
      if (params.severity) items = items.filter((n) => n.severity === params.severity);
      if (params.unread_only) items = items.filter((n) => !n.is_read);
      return { items, total: items.length, page: 1, page_size: 25, total_pages: 1 };
    }
    const q = new URLSearchParams();
    if (params.severity) q.set("severity", params.severity);
    if (params.unread_only) q.set("unread_only", "true");
    if (params.page) q.set("page", String(params.page));
    return apiRequest<PaginatedResponse<NotificationRecord>>(`/api/v1/notifications?${q.toString()}`);
  },
  async markNotificationRead(id: string) {
    if (demoActive) {
      demoNotifs = demoNotifs.map((n) => (n.id === id ? { ...n, is_read: true } : n));
      return { read: true };
    }
    return apiRequest<{ read: boolean }>(`/api/v1/notifications/${id}/read`, { method: "PATCH" });
  },
  async markAllNotificationsRead() {
    if (demoActive) {
      demoNotifs = demoNotifs.map((n) => ({ ...n, is_read: true }));
      return { read_all: true };
    }
    return apiRequest<{ read_all: boolean }>("/api/v1/notifications/read-all", { method: "POST" });
  },
  async listNotificationChannels(): Promise<NotificationChannel[]> {
    if (demoActive) return [...demoChannels];
    return apiRequest<NotificationChannel[]>("/api/v1/notification-channels");
  },
  async createNotificationChannel(body: Partial<NotificationChannel>): Promise<NotificationChannel> {
    if (demoActive) {
      const ch: NotificationChannel = {
        id: `demo-ch-${Date.now()}`,
        name: body.name ?? "New Channel",
        channel_type: body.channel_type ?? "webhook",
        configuration_reference: body.configuration_reference ?? "env:GENERIC_WEBHOOK_URL",
        target_summary: body.target_summary ?? body.configuration_reference ?? "",
        severity_filters: body.severity_filters ?? ["critical", "warning"],
        last_status: "never_tested",
        enabled: body.enabled ?? true,
        created_at: new Date().toISOString(),
        updated_at: new Date().toISOString(),
      };
      demoChannels = [ch, ...demoChannels];
      return ch;
    }
    return apiRequest<NotificationChannel>("/api/v1/notification-channels", { method: "POST", body });
  },
  async updateNotificationChannel(id: string, body: Partial<NotificationChannel>): Promise<NotificationChannel> {
    if (demoActive) {
      demoChannels = demoChannels.map((c) => (c.id === id ? { ...c, ...body } : c));
      return demoChannels.find((c) => c.id === id)!;
    }
    return apiRequest<NotificationChannel>(`/api/v1/notification-channels/${id}`, { method: "PATCH", body });
  },
  async deleteNotificationChannel(id: string) {
    if (demoActive) {
      demoChannels = demoChannels.filter((c) => c.id !== id);
      return { deleted: true };
    }
    return apiRequest<{ deleted: boolean }>(`/api/v1/notification-channels/${id}`, { method: "DELETE" });
  },
  async testNotificationChannel(id: string) {
    if (demoActive) {
      demoChannels = demoChannels.map((c) => (c.id === id ? { ...c, last_status: "delivered_via_alertmanager", last_tested_at: new Date().toISOString() } : c));
      return { status: "delivered_via_alertmanager", tested_at: new Date().toISOString() };
    }
    return apiRequest<{ status: string; tested_at: string }>(`/api/v1/notification-channels/${id}/test`, { method: "POST" });
  },

  // Reports
  async getReport(type: "infrastructure" | "resources" | "services" | "alerts", params: { range?: string; environment?: string; server_group?: string; severity?: string } = {}) {
    if (demoActive) {
      if (type === "infrastructure") {
        return {
          generated_at: new Date().toISOString(),
          range: params.range ?? "24h",
          total_servers: demoServers.length,
          up_servers: demoServers.filter((s) => s.availability === "UP").length,
          down_servers: demoServers.filter((s) => s.availability === "DOWN").length,
          availability_percent: 75.0,
          active_alerts: demoAlerts.length,
          health_distribution: { UP: 2, WARNING: 1, DOWN: 1 },
          servers: demoServers,
        };
      }
      if (type === "resources") {
        return {
          generated_at: new Date().toISOString(),
          range: params.range ?? "24h",
          rows: demoServers.map((s) => ({
            server_id: s.id,
            server_name: s.name,
            environment: s.environment,
            server_group: s.server_group,
            avg_cpu: s.cpu_percent,
            peak_cpu: s.cpu_percent !== null ? Math.min(100, s.cpu_percent + 12) : null,
            avg_memory: s.memory_percent,
            peak_memory: s.memory_percent !== null ? Math.min(100, s.memory_percent + 6) : null,
            disk_utilization: s.disk_percent,
            network_rx_bps: s.availability === "UP" ? 1840000 : null,
            network_tx_bps: s.availability === "UP" ? 1210000 : null,
          })),
        };
      }
      if (type === "services") {
        return { generated_at: new Date().toISOString(), range: params.range ?? "24h", services: demoServices };
      }
      return {
        generated_at: new Date().toISOString(),
        range: params.range ?? "24h",
        total_alerts: demoAlerts.length,
        critical_alerts: demoAlerts.filter((a) => a.severity === "critical").length,
        warning_alerts: demoAlerts.filter((a) => a.severity === "warning").length,
        resolved_alerts: 0,
        most_affected_servers: { "dev-edge-02": 1, "prod-db-01": 1 },
        alerts: demoAlerts,
      };
    }
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v) q.set(k, v);
    });
    return apiRequest<Record<string, unknown>>(`/api/v1/reports/${type}?${q.toString()}`);
  },
  async exportReport(type: string, format: "csv" | "json" | "html", params: { range?: string; environment?: string; server_group?: string; severity?: string } = {}) {
    if (demoActive) {
      const data = await this.getReport(type as "infrastructure", params);
      const content = format === "json" ? JSON.stringify(data, null, 2) : `Report,Type,Mode\nCloudPulse,${type},DEMO\n`;
      const mime = format === "json" ? "application/json" : format === "html" ? "text/html" : "text/csv";
      return { blob: new Blob([content], { type: mime }), filename: `cloudpulse-demo-${type}.${format}` };
    }
    const q = new URLSearchParams({ type, format });
    Object.entries(params).forEach(([k, v]) => {
      if (v) q.set(k, v);
    });
    return apiDownloadBlob(`/api/v1/reports/export?${q.toString()}`);
  },

  // Audit Logs & Integrations
  async listAuditLogs(params: { action?: string; resource_type?: string; page?: number } = {}): Promise<PaginatedResponse<AuditLog>> {
    if (demoActive) {
      return { items: DEMO_AUDIT_LOGS, total: DEMO_AUDIT_LOGS.length, page: 1, page_size: 30, total_pages: 1 };
    }
    const q = new URLSearchParams();
    Object.entries(params).forEach(([k, v]) => {
      if (v) q.set(k, String(v));
    });
    return apiRequest<PaginatedResponse<AuditLog>>(`/api/v1/audit-logs?${q.toString()}`);
  },
  async getIntegrations(): Promise<SystemIntegrations> {
    if (demoActive) return DEMO_INTEGRATIONS;
    return apiRequest<SystemIntegrations>("/api/v1/system/integrations");
  },
};
