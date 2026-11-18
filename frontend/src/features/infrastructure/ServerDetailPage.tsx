import React from "react";
import { Link, useParams } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import {
  AlertTriangle,
  ArrowLeft,
  Cpu,
  Database,
  ExternalLink,
  HardDrive,
  Network,
} from "lucide-react";
import {
  CartesianGrid,
  Legend,
  Line,
  LineChart,
  ResponsiveContainer,
  Tooltip,
  XAxis,
  YAxis,
} from "recharts";
import { useAuth } from "@/app/AuthContext";
import { useWorkspace } from "@/app/ThemeContext";
import { Card, EmptyState, ErrorState, Skeleton, StatusBadge } from "@/components/ui/Primitives";
import { api } from "@/services/api";
import {
  formatBytes,
  formatBytesPerSec,
  formatPercent,
  formatRelativeTime,
  formatShortTime,
  formatTimestamp,
} from "@/utils/format";

export const ServerDetailPage: React.FC = () => {
  const { id = "" } = useParams();
  const { organization, isDemoMode } = useAuth();
  const { timeRange, refreshIntervalMs, refreshTick } = useWorkspace();

  const units = organization?.display_units ?? "binary";
  const tz = organization?.timezone ?? "UTC";
  const refetchInterval = refreshIntervalMs > 0 ? refreshIntervalMs : false;

  const metricsQ = useQuery({
    queryKey: ["server-metrics", id, refreshTick, isDemoMode],
    queryFn: () => api.getServerMetrics(id),
    enabled: Boolean(id),
    refetchInterval,
  });

  const historyQ = useQuery({
    queryKey: ["server-history", id, timeRange, refreshTick, isDemoMode],
    queryFn: () => api.getServerHistory(id, timeRange),
    enabled: Boolean(id),
    refetchInterval,
  });

  if (metricsQ.isError) {
    return (
      <ErrorState
        message={metricsQ.error instanceof Error ? metricsQ.error.message : "Could not load server details"}
        onRetry={() => void metricsQ.refetch()}
      />
    );
  }

  if (metricsQ.isLoading || !metricsQ.data) {
    return (
      <div className="space-y-4">
        <Skeleton className="h-12 w-64" />
        <Skeleton className="h-44 w-full" />
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }

  const data = metricsQ.data;
  const srv = data.server;
  const hist = historyQ.data;

  const cpuMemData = (hist?.cpu ?? []).map((p, idx) => ({
    time: formatShortTime(p.timestamp, tz),
    cpu: p.value,
    memory: hist?.memory?.[idx]?.value ?? null,
    disk: hist?.disk?.[idx]?.value ?? null,
  }));

  const netData = (hist?.network_rx_rate ?? []).map((p, idx) => ({
    time: formatShortTime(p.timestamp, tz),
    rx: Math.round(p.value / 1024),
    tx: Math.round((hist?.network_tx_rate?.[idx]?.value ?? 0) / 1024),
  }));

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div className="flex items-center gap-3">
          <Link to="/servers" className="rounded-lg border border-slate-200 dark:border-slate-700 p-2 hover:bg-slate-100 dark:hover:bg-navy-800">
            <ArrowLeft className="h-4 w-4" />
          </Link>
          <div>
            <div className="flex items-center gap-2.5 flex-wrap">
              <h1 className="text-xl font-bold text-slate-900 dark:text-white">{srv.name}</h1>
              <StatusBadge status={srv.status} isStale={srv.is_stale} />
            </div>
            <p className="text-xs text-slate-500 font-mono">
              {srv.hostname} · {srv.scrape_address} · {srv.environment}
            </p>
          </div>
        </div>
        {data.grafana_dashboards?.[1] && (
          <a
            href={data.grafana_dashboards[1].url}
            target="_blank"
            rel="noreferrer"
            className="inline-flex items-center gap-1.5 rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-3 py-2 text-xs font-medium"
          >
            Server Performance in Grafana <ExternalLink className="h-3.5 w-3.5" />
          </a>
        )}
      </div>

      {/* Section A: Server Information */}
      <Card title="Section A: Server Information">
        <dl className="grid grid-cols-2 sm:grid-cols-3 lg:grid-cols-5 gap-4 text-xs">
          <div><dt className="text-slate-500">Hostname</dt><dd className="mt-1 font-mono font-medium">{srv.hostname}</dd></div>
          <div><dt className="text-slate-500">Scrape Endpoint</dt><dd className="mt-1 font-mono font-medium">{srv.scrape_address}</dd></div>
          <div><dt className="text-slate-500">OS / Arch</dt><dd className="mt-1 font-medium uppercase">{srv.operating_system} / {srv.architecture}</dd></div>
          <div><dt className="text-slate-500">Server Group</dt><dd className="mt-1 font-medium">{srv.server_group}</dd></div>
          <div><dt className="text-slate-500">Environment</dt><dd className="mt-1 capitalize font-medium">{srv.environment}</dd></div>
          <div><dt className="text-slate-500">Agent Version</dt><dd className="mt-1 font-mono">{srv.agent_version || "Pending"}</dd></div>
          <div><dt className="text-slate-500">Last Successful Scrape</dt><dd className="mt-1">{formatTimestamp(srv.last_sample_at ?? srv.last_seen_at, tz)}</dd></div>
          <div><dt className="text-slate-500">Availability</dt><dd className="mt-1 font-semibold">{srv.availability}</dd></div>
          <div><dt className="text-slate-500">Monitoring</dt><dd className="mt-1">{srv.monitoring_enabled ? "Enabled" : "Disabled"}</dd></div>
          <div>
            <dt className="text-slate-500">Tags</dt>
            <dd className="mt-1 flex flex-wrap gap-1">
              {(srv.tags ?? []).length ? srv.tags.map((t) => (
                <span key={t.key} className="rounded bg-slate-100 dark:bg-navy-800 px-1.5 py-0.5 font-mono text-[11px]">
                  {t.key}={t.value}
                </span>
              )) : "None"}
            </dd>
          </div>
        </dl>
      </Card>

      {/* Section B: Resource Overview */}
      <div className="grid grid-cols-1 sm:grid-cols-2 lg:grid-cols-4 gap-4">
        <Card>
          <div className="flex items-center justify-between text-xs text-slate-500"><span>CPU Usage</span><Cpu className="h-4 w-4 text-sky-500" /></div>
          <p className="mt-2 text-2xl font-bold">{formatPercent(data.cpu.usage_percent)}</p>
          <p className="mt-1 text-xs text-slate-500">{data.cpu.core_count ?? "N/A"} logical cores</p>
        </Card>
        <Card>
          <div className="flex items-center justify-between text-xs text-slate-500"><span>Memory Utilization</span><Database className="h-4 w-4 text-indigo-500" /></div>
          <p className="mt-2 text-2xl font-bold">{formatPercent(data.memory.usage_percent)}</p>
          <p className="mt-1 text-xs text-slate-500">
            {formatBytes(data.memory.used_bytes, units)} used / {formatBytes(data.memory.available_bytes, units)} avail
          </p>
        </Card>
        <Card>
          <div className="flex items-center justify-between text-xs text-slate-500"><span>Highest Disk</span><HardDrive className="h-4 w-4 text-amber-500" /></div>
          <p className="mt-2 text-2xl font-bold">{formatPercent(srv.disk_percent)}</p>
          <p className="mt-1 text-xs text-slate-500">{data.disks.length} monitored mount(s)</p>
        </Card>
        <Card>
          <div className="flex items-center justify-between text-xs text-slate-500"><span>Network Throughput</span><Network className="h-4 w-4 text-emerald-500" /></div>
          <p className="mt-2 text-sm font-bold font-mono">
            Rx: {data.networks[0] ? formatBytesPerSec(data.networks[0].receive_bytes_per_sec, units) : "N/A"}
          </p>
          <p className="mt-1 text-xs font-mono text-slate-500">
            Tx: {data.networks[0] ? formatBytesPerSec(data.networks[0].transmit_bytes_per_sec, units) : "N/A"}
          </p>
        </Card>
      </div>

      {/* Section C & D: CPU & Memory Analytics */}
      <div className="grid lg:grid-cols-2 gap-6">
        <Card
          title="Section C: CPU Analytics"
          subtitle="Aggregate utilization, per-core breakdown, and load averages"
          action={
            data.cpu.saturation_warning && (
              <span className="inline-flex items-center gap-1 rounded-full bg-amber-100 text-amber-800 px-2.5 py-0.5 text-xs font-medium">
                <AlertTriangle className="h-3.5 w-3.5" /> Saturation Warning
              </span>
            )
          }
        >
          <div className="grid grid-cols-3 gap-3 mb-4 text-xs">
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5">
              <p className="text-slate-500">1h Average</p>
              <p className="font-bold text-sm mt-0.5">{formatPercent(data.cpu.average_percent)}</p>
            </div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5">
              <p className="text-slate-500">1h Peak</p>
              <p className="font-bold text-sm mt-0.5">{formatPercent(data.cpu.peak_percent)}</p>
            </div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5">
              <p className="text-slate-500">Load (1m/5m/15m)</p>
              <p className="font-mono text-xs mt-1">
                {data.cpu.load_supported
                  ? `${data.cpu.load_1m ?? "-"} / ${data.cpu.load_5m ?? "-"} / ${data.cpu.load_15m ?? "-"}`
                  : "Unsupported on OS"}
              </p>
            </div>
          </div>
          {Object.keys(data.cpu.per_core_percent).length > 0 && (
            <div className="mb-4">
              <p className="text-xs font-medium text-slate-500 mb-2">Per-Core Utilization</p>
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-2">
                {Object.entries(data.cpu.per_core_percent).map(([core, pct]) => (
                  <div key={core} className="rounded border border-slate-200 dark:border-slate-800 p-2 text-xs">
                    <div className="flex justify-between font-mono">
                      <span>Core {core}</span>
                      <span>{pct.toFixed(1)}%</span>
                    </div>
                    <div className="mt-1 h-1.5 rounded-full bg-slate-200 dark:bg-slate-800 overflow-hidden">
                      <div className="h-full bg-sky-500" style={{ width: `${Math.min(100, pct)}%` }} />
                    </div>
                  </div>
                ))}
              </div>
            </div>
          )}
          <div className="h-56">
            {cpuMemData.length === 0 ? (
              <EmptyState title="No CPU history yet" description="Prometheus samples will plot here after scraping." />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={cpuMemData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
                  <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                  <YAxis domain={[0, 100]} unit="%" tick={{ fontSize: 11 }} />
                  <Tooltip />
                  <Legend />
                  <Line type="monotone" dataKey="cpu" name="CPU %" stroke="#0284c7" strokeWidth={2} dot={false} />
                </LineChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>

        <Card title="Section D: Memory Analytics" subtitle="OS-reported available, used, cached/buffers, and free memory">
          <div className="grid grid-cols-2 sm:grid-cols-3 gap-3 mb-4 text-xs">
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5"><p className="text-slate-500">Total</p><p className="font-bold mt-0.5">{formatBytes(data.memory.total_bytes, units)}</p></div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5"><p className="text-slate-500">Used</p><p className="font-bold mt-0.5">{formatBytes(data.memory.used_bytes, units)}</p></div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5"><p className="text-slate-500">Available</p><p className="font-bold mt-0.5">{formatBytes(data.memory.available_bytes, units)}</p></div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5"><p className="text-slate-500">Cached & Buffers</p><p className="font-bold mt-0.5">{formatBytes(data.memory.cached_bytes, units)}</p></div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5"><p className="text-slate-500">Free</p><p className="font-bold mt-0.5">{formatBytes(data.memory.free_bytes, units)}</p></div>
            <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-2.5"><p className="text-slate-500">Utilization</p><p className="font-bold mt-0.5">{formatPercent(data.memory.usage_percent)}</p></div>
          </div>
          <div className="h-56">
            {cpuMemData.length === 0 ? (
              <EmptyState title="No memory history yet" description="Waiting for Prometheus samples." />
            ) : (
              <ResponsiveContainer width="100%" height="100%">
                <LineChart data={cpuMemData}>
                  <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
                  <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                  <YAxis domain={[0, 100]} unit="%" tick={{ fontSize: 11 }} />
                  <Tooltip />
                  <Legend />
                  <Line type="monotone" dataKey="memory" name="Memory %" stroke="#6366f1" strokeWidth={2} dot={false} />
                </LineChart>
              </ResponsiveContainer>
            )}
          </div>
        </Card>
      </div>

      {/* Section E: Disk Analytics */}
      <Card title="Section E: Disk Analytics" subtitle="Monitored mount points (pseudo-filesystems excluded)">
        {data.disks.length === 0 ? (
          <EmptyState title="No disk metrics reported" description="Disk metrics appear once cloudpulse-agent completes collection." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                <tr>
                  <th className="py-2 pr-3">Mount Point</th>
                  <th className="py-2 pr-3">Device</th>
                  <th className="py-2 pr-3">Filesystem</th>
                  <th className="py-2 pr-3">Total</th>
                  <th className="py-2 pr-3">Used</th>
                  <th className="py-2 pr-3">Free</th>
                  <th className="py-2 pr-3">Utilization</th>
                  <th className="py-2">Collected</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {data.disks.map((d) => (
                  <tr key={d.mountpoint}>
                    <td className="py-2.5 pr-3 font-mono font-semibold">{d.mountpoint}</td>
                    <td className="py-2.5 pr-3 font-mono text-slate-500">{d.device}</td>
                    <td className="py-2.5 pr-3 uppercase">{d.filesystem}</td>
                    <td className="py-2.5 pr-3">{formatBytes(d.total_bytes, units)}</td>
                    <td className="py-2.5 pr-3">{formatBytes(d.used_bytes, units)}</td>
                    <td className="py-2.5 pr-3">{formatBytes(d.free_bytes, units)}</td>
                    <td className="py-2.5 pr-3 font-mono font-semibold">{d.usage_percent.toFixed(1)}%</td>
                    <td className="py-2.5 text-slate-500">{formatRelativeTime(d.collected_at)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Section F: Network Analytics */}
      <Card
        title="Section F: Network Analytics"
        subtitle="Instantaneous 5m PromQL rate() throughput separated from cumulative interface byte counters"
      >
        {data.networks.length === 0 ? (
          <EmptyState title="No network interface metrics yet" description="Waiting for cloudpulse-agent network counters." />
        ) : (
          <div className="space-y-4">
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                  <tr>
                    <th className="py-2 pr-3">Interface</th>
                    <th className="py-2 pr-3">Rx Rate (B/s)</th>
                    <th className="py-2 pr-3">Tx Rate (B/s)</th>
                    <th className="py-2 pr-3">Total Received</th>
                    <th className="py-2 pr-3">Total Transmitted</th>
                    <th className="py-2 pr-3">Packets (Rx / Tx)</th>
                    <th className="py-2">Errors (Rx / Tx)</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {data.networks.map((n) => (
                    <tr key={n.interface}>
                      <td className="py-2.5 pr-3 font-mono font-semibold">{n.interface}</td>
                      <td className="py-2.5 pr-3 font-mono text-emerald-600">{formatBytesPerSec(n.receive_bytes_per_sec, units)}</td>
                      <td className="py-2.5 pr-3 font-mono text-sky-600">{formatBytesPerSec(n.transmit_bytes_per_sec, units)}</td>
                      <td className="py-2.5 pr-3 font-mono">{formatBytes(n.total_receive_bytes, units)}</td>
                      <td className="py-2.5 pr-3 font-mono">{formatBytes(n.total_transmit_bytes, units)}</td>
                      <td className="py-2.5 pr-3 font-mono">{n.receive_packets_total.toLocaleString()} / {n.transmit_packets_total.toLocaleString()}</td>
                      <td className="py-2.5 font-mono">{n.receive_errors_total} / {n.transmit_errors_total}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            </div>
            {netData.length > 0 && (
              <div className="h-52">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={netData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
                    <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                    <YAxis unit=" KB/s" tick={{ fontSize: 11 }} />
                    <Tooltip />
                    <Legend />
                    <Line type="monotone" dataKey="rx" name="Receive KB/s" stroke="#10b981" strokeWidth={2} dot={false} />
                    <Line type="monotone" dataKey="tx" name="Transmit KB/s" stroke="#0284c7" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            )}
          </div>
        )}
      </Card>

      {/* Section G & H: Service Health & Recent Activity */}
      <div className="grid lg:grid-cols-2 gap-6">
        <Card title="Section G: Service Health" subtitle="HTTP & TCP service checks configured for this machine">
          {data.services.length === 0 ? (
            <EmptyState title="No service checks configured" description="Add HTTP or TCP service checks on the Services page." />
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
              {data.services.map((sc) => (
                <li key={sc.id} className="py-2.5 flex items-center justify-between gap-2">
                  <div>
                    <p className="font-semibold">{sc.name} <span className="uppercase text-slate-400">({sc.check_type})</span></p>
                    <p className="font-mono text-slate-500">{sc.target}</p>
                  </div>
                  <div className="text-right">
                    <StatusBadge status={sc.status} />
                    <p className="mt-1 text-[11px] text-slate-500">
                      {sc.latency_ms !== null ? `${sc.latency_ms.toFixed(1)} ms` : "No latency"}
                    </p>
                  </div>
                </li>
              ))}
            </ul>
          )}
        </Card>

        <Card title="Section H: Recent Activity" subtitle="Configuration updates, alerts, and connection events">
          {data.recent_activity.length === 0 ? (
            <EmptyState title="No recent activity" description="Events for this machine will appear here." />
          ) : (
            <ul className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
              {data.recent_activity.map((ev) => (
                <li key={ev.id} className="py-2.5 flex items-center justify-between">
                  <div>
                    <p className="font-medium">{ev.title}</p>
                    <p className="text-slate-500">{ev.actor}</p>
                  </div>
                  <span className="text-slate-400">{formatRelativeTime(ev.timestamp)}</span>
                </li>
              ))}
            </ul>
          )}
        </Card>
      </div>
    </div>
  );
};
