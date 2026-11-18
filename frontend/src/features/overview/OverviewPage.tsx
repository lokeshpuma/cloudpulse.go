import React, { useState } from "react";
import { Link, useNavigate } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Activity,
  AlertTriangle,
  CheckCircle2,
  Cpu,
  Database,
  ExternalLink,
  HardDrive,
  Plus,
  Server as ServerIcon,
  ShieldAlert,
} from "lucide-react";
import { Line, LineChart, ResponsiveContainer } from "recharts";
import { useAuth } from "@/app/AuthContext";
import { useWorkspace } from "@/app/ThemeContext";
import { ResourceChart } from "@/components/charts/ResourceChart";
import {
  Button,
  Card,
  EmptyState,
  ErrorState,
  Modal,
  Skeleton,
  StatusBadge,
  inputClassName,
} from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";
import type { AlertView } from "@/types/api";
import { formatBytes, formatPercent, formatRelativeTime } from "@/utils/format";

export const OverviewPage: React.FC = () => {
  const { organization, canManageInfra, canOperateAlerts, isDemoMode } = useAuth();
  const { timeRange, setTimeRange, refreshIntervalMs, refreshTick } = useWorkspace();
  const { notify } = useToast();
  const navigate = useNavigate();
  const queryClient = useQueryClient();

  const [search, setSearch] = useState("");
  const [envFilter, setEnvFilter] = useState("");
  const [healthFilter, setHealthFilter] = useState("");
  const [groupFilter, setGroupFilter] = useState("");
  const [page, setPage] = useState(1);
  const [ackTarget, setAckTarget] = useState<AlertView | null>(null);
  const [ackNote, setAckNote] = useState("");

  const refetchInterval = refreshIntervalMs > 0 ? refreshIntervalMs : false;

  const overviewQ = useQuery({
    queryKey: ["overview", timeRange, refreshTick, isDemoMode],
    queryFn: () => api.getOverview(timeRange),
    refetchInterval,
  });

  const tsQ = useQuery({
    queryKey: ["timeseries", timeRange, refreshTick, isDemoMode],
    queryFn: () => api.getTimeseries({ range: timeRange }),
    refetchInterval,
  });

  const serversQ = useQuery({
    queryKey: ["overview-servers", search, envFilter, healthFilter, groupFilter, page, refreshTick, isDemoMode],
    queryFn: () =>
      api.listServers({
        search,
        environment: envFilter,
        health: healthFilter,
        server_group: groupFilter,
        page,
        page_size: 10,
      }),
    refetchInterval,
  });

  const alertsQ = useQuery({
    queryKey: ["overview-alerts", refreshTick, isDemoMode],
    queryFn: () => api.listAlerts({ state: "active" }),
    refetchInterval,
  });

  const ackMutation = useMutation({
    mutationFn: ({ fp, note }: { fp: string; note: string }) => api.acknowledgeAlert(fp, note),
    onSuccess: () => {
      notify({ type: "success", title: "Alert acknowledged", description: "Recorded in CloudPulse without silencing Prometheus." });
      setAckTarget(null);
      setAckNote("");
      void queryClient.invalidateQueries({ queryKey: ["overview-alerts"] });
      void queryClient.invalidateQueries({ queryKey: ["overview"] });
    },
    onError: (err) => {
      notify({ type: "error", title: "Acknowledgement failed", description: err instanceof Error ? err.message : "" });
    },
  });

  const units = organization?.display_units ?? "binary";
  const tz = organization?.timezone ?? "UTC";

  if (overviewQ.isError) {
    return (
      <ErrorState
        message={overviewQ.error instanceof Error ? overviewQ.error.message : "Failed to load dashboard overview"}
        onRetry={() => void overviewQ.refetch()}
      />
    );
  }

  const ov = overviewQ.data;
  const servers = serversQ.data?.items ?? [];
  const alerts = alertsQ.data ?? [];

  return (
    <div className="space-y-6">
      {/* Header Row */}
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <div className="flex items-center gap-2">
            <h1 className="text-xl font-bold text-slate-900 dark:text-white">Infrastructure Overview</h1>
            {isDemoMode && (
              <span className="rounded bg-amber-100 text-amber-800 dark:bg-amber-900/40 dark:text-amber-300 px-2 py-0.5 text-[11px] font-semibold">
                DEMO DATA
              </span>
            )}
          </div>
          <p className="text-xs text-slate-500 dark:text-slate-400">
            Real-time operational metrics from Prometheus · Averages are unweighted across reporting machines
          </p>
        </div>

        <div className="flex items-center gap-2">
          {ov?.grafana_dashboards?.[0] && (
            <a
              href={ov.grafana_dashboards[0].url}
              target="_blank"
              rel="noreferrer"
              className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-3 py-2 text-xs font-medium text-slate-700 dark:text-slate-200 hover:bg-slate-50 dark:hover:bg-navy-700"
            >
              Open in Grafana <ExternalLink className="h-3.5 w-3.5" />
            </a>
          )}
          {canManageInfra && (
            <Link to="/servers">
              <Button size="sm">
                <Plus className="h-4 w-4" /> Register Server
              </Button>
            </Link>
          )}
        </div>
      </div>

      {/* Prometheus Outage Warning */}
      {ov && !ov.prometheus_available && (
        <div
          role="alert"
          className="flex items-center gap-3 rounded-xl border border-rose-300 dark:border-rose-900 bg-rose-50 dark:bg-rose-950/30 px-4 py-3 text-xs text-rose-800 dark:text-rose-200"
        >
          <AlertTriangle className="h-5 w-5 shrink-0 text-rose-600" />
          <div>
            <strong>Monitoring Data Unavailable:</strong> Prometheus cannot currently be reached. Live metrics show as unavailable rather than fabricated values.
          </div>
        </div>
      )}

      {/* Production Onboarding Empty State when 0 servers */}
      {!overviewQ.isLoading && !isDemoMode && ov && ov.servers.total === 0 && (
        <EmptyState
          icon={<ServerIcon className="h-6 w-6" />}
          title="No servers registered yet"
          description="Register your first server and deploy cloudpulse-agent so Prometheus can begin collecting live CPU, memory, disk, and network metrics."
          action={
            <div className="flex items-center gap-2">
              <Link to="/onboarding">
                <Button>Start Onboarding Wizard</Button>
              </Link>
              {canManageInfra && (
                <Link to="/servers">
                  <Button variant="secondary">Register Server Directly</Button>
                </Link>
              )}
            </div>
          }
        />
      )}

      {/* 6 Dashboard KPI Cards (PRD Section 7.1) */}
      <div className="grid grid-cols-1 sm:grid-cols-2 xl:grid-cols-3 gap-4">
        {overviewQ.isLoading ? (
          Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-32 rounded-xl" />)
        ) : (
          <>
            {/* Card 1: Total Servers */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-500">Total Servers</span>
                <ServerIcon className="h-4 w-4 text-brand-500" />
              </div>
              <div className="mt-2 flex items-baseline justify-between">
                <span className="text-2xl font-bold">{ov?.servers.total ?? 0}</span>
                <span className="text-xs font-medium text-emerald-600 dark:text-emerald-400">
                  {ov?.servers.availability_percent !== null && ov?.servers.availability_percent !== undefined
                    ? `${ov.servers.availability_percent.toFixed(1)}% available`
                    : "Awaiting scrapes"}
                </span>
              </div>
              <div className="mt-3 flex gap-3 text-xs text-slate-500">
                <span className="text-emerald-600 font-medium">{ov?.servers.up ?? 0} Up</span>
                <span className="text-rose-600 font-medium">{ov?.servers.down ?? 0} Down</span>
                <span>{(ov?.servers.pending ?? 0) + (ov?.servers.unknown ?? 0)} Pending/Unknown</span>
              </div>
            </div>

            {/* Card 2: Healthy Infrastructure */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-500">Healthy Infrastructure</span>
                <CheckCircle2 className="h-4 w-4 text-emerald-500" />
              </div>
              <div className="mt-2 flex items-baseline gap-2">
                <span className="text-2xl font-bold text-emerald-600 dark:text-emerald-400">
                  {ov?.health.healthy ?? 0}
                </span>
                <span className="text-xs text-slate-500">healthy servers</span>
              </div>
              <div className="mt-3 flex flex-wrap gap-2 text-xs">
                <span className="rounded bg-amber-50 dark:bg-amber-950/50 px-2 py-0.5 text-amber-700 dark:text-amber-300">
                  {ov?.health.warning ?? 0} Warning
                </span>
                <span className="rounded bg-rose-50 dark:bg-rose-950/50 px-2 py-0.5 text-rose-700 dark:text-rose-300">
                  {ov?.health.critical ?? 0} Critical
                </span>
                <span className="rounded bg-slate-100 dark:bg-slate-800 px-2 py-0.5 text-slate-600 dark:text-slate-400">
                  {ov?.health.unknown ?? 0} Unknown
                </span>
              </div>
            </div>

            {/* Card 3: Average CPU Usage */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-500">Average CPU Usage</span>
                <Cpu className="h-4 w-4 text-sky-500" />
              </div>
              <div className="mt-1 flex items-center justify-between gap-2">
                <div>
                  <span className="text-2xl font-bold">{formatPercent(ov?.cpu.average_percent)}</span>
                  <p className="text-xs text-slate-500 mt-0.5">
                    {ov?.cpu.change_percent !== null && ov?.cpu.change_percent !== undefined
                      ? `${ov.cpu.change_percent >= 0 ? "+" : ""}${ov.cpu.change_percent.toFixed(1)}% vs start`
                      : "No trend delta"} · {ov?.cpu.included_count ?? 0} hosts
                  </p>
                </div>
                {ov?.cpu.trend && ov.cpu.trend.length > 1 && (
                  <div className="h-12 w-24">
                    <ResponsiveContainer width="100%" height="100%">
                      <LineChart data={ov.cpu.trend}>
                        <Line type="monotone" dataKey="value" stroke="#0284c7" strokeWidth={2} dot={false} />
                      </LineChart>
                    </ResponsiveContainer>
                  </div>
                )}
              </div>
            </div>

            {/* Card 4: Average Memory Usage */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-500">Average Memory Usage</span>
                <Database className="h-4 w-4 text-indigo-500" />
              </div>
              <div className="mt-2 flex items-baseline justify-between">
                <span className="text-2xl font-bold">{formatPercent(ov?.memory.average_percent)}</span>
                <span className="text-xs text-slate-500">
                  {ov?.memory.included_count ?? 0} hosts reporting
                </span>
              </div>
              <div className="mt-2 text-xs text-slate-500 flex justify-between">
                <span>Used: {formatBytes(ov?.memory.used_bytes, units)}</span>
                <span>Avail: {formatBytes(ov?.memory.available_bytes, units)}</span>
              </div>
            </div>

            {/* Card 5: Disk Utilization */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-500">Disk Utilization</span>
                <HardDrive className="h-4 w-4 text-amber-500" />
              </div>
              <div className="mt-2 flex items-baseline justify-between">
                <span className="text-2xl font-bold">{formatPercent(ov?.disk.average_percent)}</span>
                <span className="text-xs font-medium text-rose-600">
                  {ov?.disk.critical_count ?? 0} critical (&ge;90%)
                </span>
              </div>
              <p className="mt-2 text-xs text-slate-500 truncate">
                Highest:{" "}
                {ov?.disk.highest_server
                  ? `${ov.disk.highest_server} (${formatPercent(ov.disk.highest_percent)})`
                  : "Unavailable"}
              </p>
            </div>

            {/* Card 6: Active Alerts */}
            <div className="rounded-xl border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-4 shadow-sm">
              <div className="flex items-center justify-between">
                <span className="text-xs font-medium text-slate-500">Active Alerts</span>
                <ShieldAlert className="h-4 w-4 text-rose-500" />
              </div>
              <div className="mt-2 flex items-baseline justify-between">
                <span className="text-2xl font-bold">{ov?.alerts.total ?? 0}</span>
                <span className="text-xs font-medium text-amber-600">
                  {ov?.alerts.unacknowledged ?? 0} unacknowledged
                </span>
              </div>
              <div className="mt-3 flex gap-3 text-xs">
                <span className="text-rose-600 font-medium">{ov?.alerts.critical ?? 0} Critical</span>
                <span className="text-amber-600 font-medium">{ov?.alerts.warning ?? 0} Warning</span>
                <span className="text-sky-600 font-medium">{ov?.alerts.info ?? 0} Info</span>
              </div>
            </div>
          </>
        )}
      </div>

      {/* Main Resource Usage Chart */}
      <ResourceChart
        points={tsQ.data?.points ?? []}
        loading={tsQ.isLoading}
        prometheusAvailable={tsQ.data?.prometheus_available ?? true}
        timeRange={timeRange}
        onRangeChange={setTimeRange}
        timezone={tz}
      />

      {/* Server Status Overview Table (PRD Section 7.3) */}
      <Card
        title="Server Status Overview"
        subtitle="Click any server row to inspect CPU, memory, disk mounts, network throughput, and services"
        action={
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="search"
              placeholder="Filter servers..."
              aria-label="Filter servers"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(1);
              }}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-navy-800 px-2.5 py-1.5 text-xs"
            />
            <select
              aria-label="Filter by environment"
              value={envFilter}
              onChange={(e) => {
                setEnvFilter(e.target.value);
                setPage(1);
              }}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5 text-xs"
            >
              <option value="">All Envs</option>
              <option value="production">Production</option>
              <option value="staging">Staging</option>
              <option value="development">Development</option>
              <option value="testing">Testing</option>
            </select>
            <select
              aria-label="Filter by health"
              value={healthFilter}
              onChange={(e) => {
                setHealthFilter(e.target.value);
                setPage(1);
              }}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5 text-xs"
            >
              <option value="">All Health</option>
              <option value="UP">Healthy</option>
              <option value="WARNING">Warning</option>
              <option value="CRITICAL">Critical</option>
              <option value="DOWN">Down</option>
              <option value="PENDING">Pending</option>
              <option value="UNKNOWN">Unknown</option>
            </select>
            <input
              placeholder="Group..."
              aria-label="Filter by group"
              value={groupFilter}
              onChange={(e) => setGroupFilter(e.target.value)}
              className="w-24 rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-navy-800 px-2 py-1.5 text-xs"
            />
          </div>
        }
      >
        {serversQ.isLoading ? (
          <Skeleton className="h-44 w-full" />
        ) : servers.length === 0 ? (
          <EmptyState
            title="No matching servers"
            description="Adjust your filters or register a new machine to begin monitoring."
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase tracking-wider">
                <tr>
                  <th className="py-2.5 pr-4">Server</th>
                  <th className="py-2.5 pr-4">Hostname</th>
                  <th className="py-2.5 pr-4">Environment</th>
                  <th className="py-2.5 pr-4">CPU</th>
                  <th className="py-2.5 pr-4">Memory</th>
                  <th className="py-2.5 pr-4">Disk</th>
                  <th className="py-2.5 pr-4">Availability</th>
                  <th className="py-2.5 pr-4">Last Seen</th>
                  <th className="py-2.5">Status</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {servers.map((srv) => (
                  <tr
                    key={srv.id}
                    onClick={() => navigate(`/servers/${srv.id}`)}
                    className="cursor-pointer hover:bg-slate-50 dark:hover:bg-navy-800/60 transition-colors"
                  >
                    <td className="py-3 pr-4 font-semibold text-slate-900 dark:text-white">{srv.name}</td>
                    <td className="py-3 pr-4 font-mono text-slate-600 dark:text-slate-300">{srv.hostname}</td>
                    <td className="py-3 pr-4 capitalize">{srv.environment}</td>
                    <td className="py-3 pr-4 font-mono">{formatPercent(srv.cpu_percent)}</td>
                    <td className="py-3 pr-4 font-mono">{formatPercent(srv.memory_percent)}</td>
                    <td className="py-3 pr-4 font-mono">{formatPercent(srv.disk_percent)}</td>
                    <td className="py-3 pr-4">{srv.availability}</td>
                    <td className="py-3 pr-4 text-slate-500">{formatRelativeTime(srv.last_sample_at ?? srv.last_seen_at)}</td>
                    <td className="py-3">
                      <StatusBadge status={srv.status} isStale={srv.is_stale} />
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
            {(serversQ.data?.total_pages ?? 1) > 1 && (
              <div className="mt-4 flex items-center justify-between text-xs text-slate-500">
                <span>
                  Page {serversQ.data?.page} of {serversQ.data?.total_pages} ({serversQ.data?.total} servers)
                </span>
                <div className="flex gap-2">
                  <Button size="sm" variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>
                    Previous
                  </Button>
                  <Button
                    size="sm"
                    variant="secondary"
                    disabled={page >= (serversQ.data?.total_pages ?? 1)}
                    onClick={() => setPage((p) => p + 1)}
                  >
                    Next
                  </Button>
                </div>
              </div>
            )}
          </div>
        )}
      </Card>

      {/* Bottom Row: Recent Alerts & Infrastructure Events */}
      <div className="grid grid-cols-1 lg:grid-cols-2 gap-6">
        <Card
          title="Recent Active Alerts"
          subtitle="Live alerts from Prometheus & Alertmanager"
          action={
            <Link to="/alerts" className="text-xs font-medium text-brand-600 hover:underline">
              View all →
            </Link>
          }
        >
          {alerts.length === 0 ? (
            <EmptyState
              title="No active alerts"
              description="All evaluated Prometheus alert rules are currently clear."
            />
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-800">
              {alerts.slice(0, 6).map((al) => (
                <li key={al.fingerprint} className="py-3 flex items-start justify-between gap-3">
                  <div className="min-w-0">
                    <div className="flex items-center gap-2 flex-wrap">
                      <StatusBadge status={al.severity} />
                      <Link
                        to={`/alerts/${encodeURIComponent(al.fingerprint)}`}
                        className="text-sm font-medium text-slate-900 dark:text-white hover:text-brand-600 truncate"
                      >
                        {al.title}
                      </Link>
                    </div>
                    <p className="mt-1 text-xs text-slate-500">
                      {al.server_name} · {formatRelativeTime(al.triggered_at)} ·{" "}
                      {al.acknowledged ? `Acknowledged` : "Unacknowledged"}
                    </p>
                  </div>
                  {canOperateAlerts && !al.acknowledged && (
                    <Button size="sm" variant="secondary" onClick={() => setAckTarget(al)}>
                      Acknowledge
                    </Button>
                  )}
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card
          title="Infrastructure Events"
          subtitle="Recent audit and infrastructure activity"
          action={
            <Link to="/activity-logs" className="text-xs font-medium text-brand-600 hover:underline">
              Audit trail →
            </Link>
          }
        >
          {!ov?.recent_events?.length ? (
            <EmptyState title="No recent events" description="Server registrations, alerts, and configuration changes appear here." />
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-800">
              {ov.recent_events.slice(0, 8).map((ev) => (
                <li key={ev.id} className="py-2.5 flex items-center justify-between gap-3 text-xs">
                  <div className="flex items-center gap-2.5 min-w-0">
                    <Activity className="h-3.5 w-3.5 text-brand-500 shrink-0" />
                    <div className="min-w-0">
                      <p className="font-medium text-slate-800 dark:text-slate-200 truncate">{ev.title}</p>
                      <p className="text-[11px] text-slate-500">{ev.actor} · {ev.resource_type}</p>
                    </div>
                  </div>
                  <span className="text-slate-400 shrink-0">{formatRelativeTime(ev.timestamp)}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>

      {/* Acknowledge Alert Modal */}
      <Modal
        open={Boolean(ackTarget)}
        onClose={() => setAckTarget(null)}
        title="Acknowledge Alert"
        description="Acknowledgement is recorded in PostgreSQL and does not silence the Prometheus alert."
        size="sm"
      >
        <div className="space-y-4">
          <p className="text-xs font-medium text-slate-700 dark:text-slate-300">{ackTarget?.title}</p>
          <div>
            <label htmlFor="ack-note" className="block text-xs font-medium mb-1">
              Optional Note
            </label>
            <textarea
              id="ack-note"
              rows={3}
              value={ackNote}
              onChange={(e) => setAckNote(e.target.value)}
              placeholder="e.g. Investigating high CPU spike after deploy..."
              className={inputClassName}
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setAckTarget(null)}>
              Cancel
            </Button>
            <Button
              disabled={ackMutation.isPending}
              onClick={() => ackTarget && ackMutation.mutate({ fp: ackTarget.fingerprint, note: ackNote })}
            >
              {ackMutation.isPending ? "Saving..." : "Confirm Acknowledgement"}
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
};
