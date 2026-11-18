import React from "react";
import { Link } from "react-router-dom";
import { useQuery } from "@tanstack/react-query";
import { ExternalLink, Layers, Monitor, Server as ServerIcon, WifiOff } from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import { useWorkspace } from "@/app/ThemeContext";
import { Card, EmptyState, ErrorState, Skeleton, StatusBadge } from "@/components/ui/Primitives";
import { api } from "@/services/api";
import { formatRelativeTime } from "@/utils/format";

export const InfrastructurePage: React.FC = () => {
  const { isDemoMode } = useAuth();
  const { refreshIntervalMs, refreshTick } = useWorkspace();

  const { data, isLoading, isError, error, refetch } = useQuery({
    queryKey: ["infrastructure-health", refreshTick, isDemoMode],
    queryFn: () => api.getInfrastructureHealth(),
    refetchInterval: refreshIntervalMs > 0 ? refreshIntervalMs : false,
  });

  if (isError) {
    return <ErrorState message={error instanceof Error ? error.message : "Failed to load infrastructure"} onRetry={() => void refetch()} />;
  }

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Infrastructure Summary</h1>
          <p className="text-xs text-slate-500">Fleet distribution across environments, operating systems, and operational health</p>
        </div>
        <Link to="/servers" className="text-xs font-medium text-brand-600 hover:underline">
          Manage Servers →
        </Link>
      </div>

      {isLoading ? (
        <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">
          {Array.from({ length: 4 }).map((_, i) => <Skeleton key={i} className="h-36" />)}
        </div>
      ) : (
        <>
          <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-4">
            <Card title="Total Machines">
              <div className="flex items-center justify-between">
                <span className="text-3xl font-bold">{data?.total_machines ?? 0}</span>
                <ServerIcon className="h-6 w-6 text-brand-500" />
              </div>
              <p className="mt-2 text-xs text-slate-500">
                Prometheus: {data?.prometheus_available ? "Connected" : "Unavailable"}
              </p>
            </Card>

            <Card title="By Environment">
              <ul className="space-y-1.5 text-xs">
                {Object.entries(data?.by_environment ?? {}).map(([k, v]) => (
                  <li key={k} className="flex justify-between">
                    <span className="capitalize text-slate-600 dark:text-slate-300">{k}</span>
                    <span className="font-semibold">{v}</span>
                  </li>
                ))}
                {Object.keys(data?.by_environment ?? {}).length === 0 && <li className="text-slate-400">No machines</li>}
              </ul>
            </Card>

            <Card title="By Operating System">
              <ul className="space-y-1.5 text-xs">
                {Object.entries(data?.by_operating_system ?? {}).map(([k, v]) => (
                  <li key={k} className="flex justify-between">
                    <span className="uppercase text-slate-600 dark:text-slate-300 flex items-center gap-1.5">
                      <Monitor className="h-3.5 w-3.5 text-slate-400" /> {k}
                    </span>
                    <span className="font-semibold">{v}</span>
                  </li>
                ))}
                {Object.keys(data?.by_operating_system ?? {}).length === 0 && <li className="text-slate-400">No machines</li>}
              </ul>
            </Card>

            <Card title="By Health Status">
              <ul className="space-y-1.5 text-xs">
                {Object.entries(data?.by_status ?? {}).map(([k, v]) => (
                  <li key={k} className="flex items-center justify-between">
                    <StatusBadge status={k} />
                    <span className="font-semibold">{v}</span>
                  </li>
                ))}
                {Object.keys(data?.by_status ?? {}).length === 0 && <li className="text-slate-400">No machines</li>}
              </ul>
            </Card>
          </div>

          <div className="grid lg:grid-cols-2 gap-6">
            <Card title="Recently Added Machines" subtitle="Latest registered hosts in your organization">
              {!data?.recently_added?.length ? (
                <EmptyState title="No registered servers" description="Register a server to populate infrastructure inventory." />
              ) : (
                <ul className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
                  {data.recently_added.map((s) => (
                    <li key={s.id} className="py-2.5 flex items-center justify-between gap-2">
                      <div>
                        <Link to={`/servers/${s.id}`} className="font-semibold text-brand-600 hover:underline">
                          {s.name}
                        </Link>
                        <p className="text-slate-500 font-mono">{s.hostname} · {s.environment}</p>
                      </div>
                      <StatusBadge status={s.status} isStale={s.is_stale} />
                    </li>
                  ))}
                </ul>
              )}
            </Card>

            <Card title="Recently Disconnected Machines" subtitle="Targets reporting DOWN or stale telemetry">
              {!data?.recently_disconnected?.length ? (
                <EmptyState
                  icon={<WifiOff className="h-5 w-5" />}
                  title="No disconnected machines"
                  description="All reporting agents are currently reachable by Prometheus."
                />
              ) : (
                <ul className="divide-y divide-slate-100 dark:divide-slate-800 text-xs">
                  {data.recently_disconnected.map((s) => (
                    <li key={s.id} className="py-2.5 flex items-center justify-between gap-2">
                      <div>
                        <Link to={`/servers/${s.id}`} className="font-semibold text-rose-600 hover:underline">
                          {s.name}
                        </Link>
                        <p className="text-slate-500">Last seen {formatRelativeTime(s.last_sample_at ?? s.last_seen_at)}</p>
                      </div>
                      <StatusBadge status={s.status} isStale={s.is_stale} />
                    </li>
                  ))}
                </ul>
              )}
            </Card>
          </div>

          {data?.grafana_dashboards && data.grafana_dashboards.length > 0 && (
            <Card title="Grafana Provisioned Dashboards" subtitle="Advanced visualization linked directly to your Prometheus TSDB">
              <div className="grid sm:grid-cols-2 lg:grid-cols-4 gap-3">
                {data.grafana_dashboards.map((d) => (
                  <a
                    key={d.uid}
                    href={d.url}
                    target="_blank"
                    rel="noreferrer"
                    className="rounded-lg border border-slate-200 dark:border-slate-800 p-3.5 hover:border-brand-500 transition-colors flex flex-col justify-between"
                  >
                    <div>
                      <div className="flex items-center justify-between text-xs font-semibold text-slate-900 dark:text-white">
                        <span className="flex items-center gap-1.5">
                          <Layers className="h-3.5 w-3.5 text-brand-500" /> {d.title}
                        </span>
                        <ExternalLink className="h-3.5 w-3.5 text-slate-400" />
                      </div>
                      <p className="mt-1.5 text-[11px] text-slate-500">{d.description}</p>
                    </div>
                  </a>
                ))}
              </div>
            </Card>
          )}
        </>
      )}
    </div>
  );
};
