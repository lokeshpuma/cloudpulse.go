import React, { useState } from "react";
import { useMutation, useQuery } from "@tanstack/react-query";
import { History, Play, Search, Table as TableIcon, TrendingUp } from "lucide-react";
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
import { Button, Card, EmptyState, Skeleton, inputClassName } from "@/components/ui/Primitives";
import { api } from "@/services/api";
import type { ExplorerQueryResult } from "@/types/api";
import { formatShortTime, formatTimestamp } from "@/utils/format";

const SERIES_COLORS = ["#0284c7", "#6366f1", "#10b981", "#f59e0b", "#ec4899", "#06b6d4"];

export const MetricsExplorerPage: React.FC = () => {
  const { organization, isDemoMode } = useAuth();
  const { timeRange } = useWorkspace();
  const tz = organization?.timezone ?? "UTC";

  const [catalogSearch, setCatalogSearch] = useState("");
  const [selectedMetric, setSelectedMetric] = useState("cpu_usage");
  const [serverId, setServerId] = useState("");
  const [environment, setEnvironment] = useState("");
  const [aggregation, setAggregation] = useState("");
  const [range, setRange] = useState<string>(timeRange === "custom" ? "1h" : timeRange);
  const [customExpr, setCustomExpr] = useState("");
  const [viewMode, setViewMode] = useState<"chart" | "table">("chart");
  const [result, setResult] = useState<ExplorerQueryResult | null>(null);
  const [queryError, setQueryError] = useState<string | null>(null);

  const catalogQ = useQuery({
    queryKey: ["metric-catalog", isDemoMode],
    queryFn: () => api.getMetricCatalog(),
  });

  const serversQ = useQuery({
    queryKey: ["explorer-servers", isDemoMode],
    queryFn: () => api.listServers({ page: 1, page_size: 100 }),
  });

  const execMut = useMutation({
    mutationFn: () =>
      api.executeMetricQuery({
        query: customExpr.trim() || undefined,
        metric: selectedMetric,
        server_id: serverId || undefined,
        environment: environment || undefined,
        aggregation: aggregation || undefined,
        range,
      }),
    onSuccess: (res) => {
      setQueryError(res.result.error || null);
      setResult(res.result);
    },
    onError: (err) => {
      setQueryError(err instanceof Error ? err.message : "Query execution failed");
    },
  });

  const catalog = (catalogQ.data?.catalog ?? []).filter(
    (m) =>
      m.name.toLowerCase().includes(catalogSearch.toLowerCase()) ||
      m.category.toLowerCase().includes(catalogSearch.toLowerCase()) ||
      m.base_promql.toLowerCase().includes(catalogSearch.toLowerCase())
  );

  // Merge series timestamps for Recharts multi-line visualization
  const mergedChartData = (() => {
    if (!result || result.series.length === 0) return [];
    const byTime = new Map<string, Record<string, string | number>>();
    result.series.forEach((ser) => {
      ser.points.forEach((p) => {
        const key = p.timestamp;
        if (!byTime.has(key)) {
          byTime.set(key, { time: formatShortTime(key, tz), rawTime: key });
        }
        byTime.get(key)![ser.label] = p.value;
      });
    });
    return Array.from(byTime.values());
  })();

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-white">Metrics Explorer</h1>
        <p className="text-xs text-slate-500">
          Query supported CloudPulse metrics from Prometheus with controlled PromQL validation · Timestamps shown in {tz}
        </p>
      </div>

      <div className="grid lg:grid-cols-12 gap-6">
        {/* Metric Catalog Sidebar */}
        <div className="lg:col-span-4 space-y-4">
          <Card title="Metric Catalog" subtitle="Click any metric to populate the query builder">
            <div className="relative mb-3">
              <Search className="pointer-events-none absolute left-2.5 top-2.5 h-3.5 w-3.5 text-slate-400" />
              <input
                value={catalogSearch}
                onChange={(e) => setCatalogSearch(e.target.value)}
                placeholder="Search metrics..."
                className={`${inputClassName} pl-8 text-xs`}
              />
            </div>
            {catalogQ.isLoading ? (
              <Skeleton className="h-64 w-full" />
            ) : (
              <ul className="space-y-1.5 max-h-[380px] overflow-y-auto pr-1">
                {catalog.map((m) => (
                  <li key={m.id}>
                    <button
                      type="button"
                      onClick={() => {
                        setSelectedMetric(m.id);
                        setCustomExpr("");
                      }}
                      className={`w-full text-left rounded-lg border px-3 py-2 text-xs transition-colors ${
                        selectedMetric === m.id && !customExpr
                          ? "border-brand-500 bg-brand-50 dark:bg-brand-900/25"
                          : "border-slate-200 dark:border-slate-800 hover:bg-slate-50 dark:hover:bg-navy-800"
                      }`}
                    >
                      <div className="flex items-center justify-between">
                        <span className="font-semibold text-slate-900 dark:text-white">{m.name}</span>
                        <span className="rounded bg-slate-100 dark:bg-navy-800 px-1.5 py-0.5 text-[10px] text-slate-500">
                          {m.category}
                        </span>
                      </div>
                      <p className="mt-0.5 text-[11px] text-slate-500">{m.description}</p>
                      <code className="block mt-1 text-[10px] font-mono text-brand-600 dark:text-sky-400 truncate">
                        {m.base_promql}
                      </code>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Card>

          <Card title="Query History" subtitle="Your recent Metrics Explorer runs">
            {!catalogQ.data?.history?.length ? (
              <p className="text-xs text-slate-500">Executed queries will appear here.</p>
            ) : (
              <ul className="space-y-2 text-xs max-h-48 overflow-y-auto">
                {catalogQ.data.history.map((h) => (
                  <li key={h.id}>
                    <button
                      type="button"
                      onClick={() => setCustomExpr(h.query)}
                      className="w-full text-left rounded border border-slate-200 dark:border-slate-800 p-2 hover:bg-slate-50 dark:hover:bg-navy-800"
                    >
                      <div className="flex items-center gap-1.5 text-[11px] font-medium">
                        <History className="h-3 w-3 text-slate-400" />
                        <span>{h.metric_name}</span>
                        <span className="ml-auto text-slate-400">{h.time_range}</span>
                      </div>
                      <code className="block mt-1 font-mono text-[10px] text-slate-500 truncate">{h.query}</code>
                    </button>
                  </li>
                ))}
              </ul>
            )}
          </Card>
        </div>

        {/* Query Builder & Results */}
        <div className="lg:col-span-8 space-y-4">
          <Card title="Query Builder & Controlled PromQL Editor">
            <div className="grid sm:grid-cols-4 gap-3 mb-3">
              <div>
                <label className="block text-xs font-medium mb-1">Server Filter</label>
                <select value={serverId} onChange={(e) => setServerId(e.target.value)} className={inputClassName}>
                  <option value="">All Servers</option>
                  {(serversQ.data?.items ?? []).map((s) => (
                    <option key={s.id} value={s.id}>{s.name}</option>
                  ))}
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Environment</label>
                <select value={environment} onChange={(e) => setEnvironment(e.target.value)} className={inputClassName}>
                  <option value="">All Envs</option>
                  <option value="production">Production</option>
                  <option value="staging">Staging</option>
                  <option value="development">Development</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Aggregation</label>
                <select value={aggregation} onChange={(e) => setAggregation(e.target.value)} className={inputClassName}>
                  <option value="">Per Series</option>
                  <option value="avg">Average (avg)</option>
                  <option value="max">Maximum (max)</option>
                  <option value="min">Minimum (min)</option>
                  <option value="sum">Sum (sum)</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Time Range</label>
                <select value={range} onChange={(e) => setRange(e.target.value)} className={inputClassName}>
                  <option value="15m">Last 15m</option>
                  <option value="30m">Last 30m</option>
                  <option value="1h">Last 1h</option>
                  <option value="6h">Last 6h</option>
                  <option value="24h">Last 24h</option>
                  <option value="7d">Last 7d</option>
                  <option value="30d">Last 30d</option>
                </select>
              </div>
            </div>

            <div className="mb-3">
              <label htmlFor="promql-input" className="block text-xs font-medium mb-1">
                PromQL Override (optional — leave blank to auto-build from selected catalog metric)
              </label>
              <input
                id="promql-input"
                value={customExpr}
                onChange={(e) => setCustomExpr(e.target.value)}
                placeholder="e.g. cloudpulse_cpu_usage_percent or rate(cloudpulse_network_receive_bytes_total[5m])"
                className={`${inputClassName} font-mono text-xs`}
              />
            </div>

            {queryError && (
              <div role="alert" className="mb-3 rounded-lg border border-rose-200 bg-rose-50 dark:bg-rose-950/40 dark:border-rose-900 px-3 py-2 text-xs text-rose-700 dark:text-rose-300">
                {queryError}
              </div>
            )}

            <div className="flex items-center justify-between">
              <div className="flex gap-1 rounded-lg bg-slate-100 dark:bg-navy-800 p-0.5">
                <button
                  type="button"
                  onClick={() => setViewMode("chart")}
                  className={`inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-medium ${viewMode === "chart" ? "bg-white dark:bg-navy-700 text-brand-600" : "text-slate-500"}`}
                >
                  <TrendingUp className="h-3.5 w-3.5" /> Chart
                </button>
                <button
                  type="button"
                  onClick={() => setViewMode("table")}
                  className={`inline-flex items-center gap-1 rounded-md px-2.5 py-1 text-xs font-medium ${viewMode === "table" ? "bg-white dark:bg-navy-700 text-brand-600" : "text-slate-500"}`}
                >
                  <TableIcon className="h-3.5 w-3.5" /> Table
                </button>
              </div>

              <Button onClick={() => execMut.mutate()} disabled={execMut.isPending}>
                <Play className="h-3.5 w-3.5" /> {execMut.isPending ? "Running..." : "Execute Query"}
              </Button>
            </div>
          </Card>

          {/* Summary Statistics */}
          {result && (
            <div className="grid grid-cols-2 sm:grid-cols-6 gap-2.5 text-xs">
              {[
                ["Min", result.summary.min],
                ["Max", result.summary.max],
                ["Average", result.summary.avg],
                ["First", result.summary.first],
                ["Last", result.summary.last],
                ["Missing Pts", result.missing_data_points],
              ].map(([label, val]) => (
                <div key={String(label)} className="rounded-lg border border-slate-200 dark:border-slate-800 bg-white dark:bg-navy-900 p-3">
                  <p className="text-[11px] text-slate-500">{label}</p>
                  <p className="font-mono font-bold mt-0.5">{val !== null && val !== undefined ? String(val) : "N/A"}</p>
                </div>
              ))}
            </div>
          )}

          {/* Visualization Card */}
          <Card
            title="Query Results"
            subtitle={result ? `Executed: ${result.query}` : "Select a metric and click Execute Query"}
          >
            {execMut.isPending ? (
              <Skeleton className="h-72 w-full" />
            ) : !result ? (
              <EmptyState title="Ready to query Prometheus" description="Choose a metric from the catalog or enter a controlled PromQL expression, then click Execute Query." />
            ) : result.series.length === 0 ? (
              <EmptyState title="No data points returned" description="No Prometheus series matched the expression in this time range." />
            ) : viewMode === "chart" ? (
              <div className="h-72">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={mergedChartData}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
                    <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                    <YAxis tick={{ fontSize: 11 }} />
                    <Tooltip />
                    <Legend wrapperStyle={{ fontSize: "12px" }} />
                    {result.series.map((s, idx) => (
                      <Line
                        key={s.label}
                        type="monotone"
                        dataKey={s.label}
                        stroke={SERIES_COLORS[idx % SERIES_COLORS.length]}
                        strokeWidth={2}
                        dot={false}
                      />
                    ))}
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <div className="overflow-x-auto max-h-80">
                <table className="w-full text-left text-xs">
                  <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                    <tr>
                      <th className="py-2 pr-3">Series</th>
                      <th className="py-2 pr-3">Timestamp ({tz})</th>
                      <th className="py-2">Value</th>
                    </tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800 font-mono">
                    {result.series.flatMap((ser) =>
                      ser.points.slice(-50).map((p, idx) => (
                        <tr key={`${ser.label}-${idx}`}>
                          <td className="py-1.5 pr-3">{ser.label}</td>
                          <td className="py-1.5 pr-3 text-slate-500">{formatTimestamp(p.timestamp, tz)}</td>
                          <td className="py-1.5 font-semibold">{p.value}</td>
                        </tr>
                      ))
                    )}
                  </tbody>
                </table>
              </div>
            )}
          </Card>
        </div>
      </div>
    </div>
  );
};
