import React, { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { useAuth } from "@/app/AuthContext";
import { Button, Card, EmptyState, ErrorState, Skeleton } from "@/components/ui/Primitives";
import { api } from "@/services/api";
import { formatTimestamp } from "@/utils/format";

export const ActivityLogsPage: React.FC = () => {
  const { organization, canOperateAlerts, isDemoMode } = useAuth();
  const tz = organization?.timezone ?? "UTC";
  const [action, setAction] = useState("");
  const [resourceType, setResourceType] = useState("");
  const [page, setPage] = useState(1);

  const logsQ = useQuery({
    queryKey: ["audit-logs", action, resourceType, page, isDemoMode],
    queryFn: () => api.listAuditLogs({ action, resource_type: resourceType, page }),
    enabled: canOperateAlerts,
  });

  if (!canOperateAlerts) {
    return (
      <EmptyState
        title="Audit logs restricted"
        description="Only Administrators and Operators can inspect organization audit trails."
      />
    );
  }

  if (logsQ.isError) {
    return <ErrorState message={logsQ.error instanceof Error ? logsQ.error.message : "Failed to load audit logs"} onRetry={() => void logsQ.refetch()} />;
  }

  const items = logsQ.data?.items ?? [];

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-white">Activity Logs & Audit Trail</h1>
        <p className="text-xs text-slate-500">
          Append-only record of security and infrastructure operations · Sensitive credentials are automatically redacted
        </p>
      </div>

      <Card
        action={
          <div className="flex flex-wrap gap-2 text-xs">
            <input
              placeholder="Filter action (e.g. server.)..."
              value={action}
              onChange={(e) => { setAction(e.target.value); setPage(1); }}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-navy-800 px-2.5 py-1.5"
            />
            <select
              value={resourceType}
              onChange={(e) => { setResourceType(e.target.value); setPage(1); }}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5"
            >
              <option value="">All Resources</option>
              <option value="user">User</option>
              <option value="server">Server</option>
              <option value="service_check">Service Check</option>
              <option value="alert">Alert</option>
              <option value="alert_rule">Alert Rule</option>
              <option value="notification_channel">Notification Channel</option>
              <option value="organization">Organization</option>
            </select>
          </div>
        }
      >
        {logsQ.isLoading ? (
          <Skeleton className="h-56 w-full" />
        ) : items.length === 0 ? (
          <EmptyState title="No audit events found" description="Try broadening your search filters." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                <tr>
                  <th className="py-2 pr-3">Timestamp ({tz})</th>
                  <th className="py-2 pr-3">Actor</th>
                  <th className="py-2 pr-3">Action</th>
                  <th className="py-2 pr-3">Resource</th>
                  <th className="py-2 pr-3">Source IP</th>
                  <th className="py-2">Metadata</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {items.map((l) => (
                  <tr key={l.id}>
                    <td className="py-2.5 pr-3 whitespace-nowrap text-slate-500">{formatTimestamp(l.created_at, tz)}</td>
                    <td className="py-2.5 pr-3 font-medium">{l.actor_name || l.actor_email || "System"}</td>
                    <td className="py-2.5 pr-3 font-mono text-brand-600 dark:text-sky-400">{l.action}</td>
                    <td className="py-2.5 pr-3">{l.resource_type} <span className="font-mono text-slate-400">{l.resource_id.slice(0, 8)}</span></td>
                    <td className="py-2.5 pr-3 font-mono text-slate-500">{l.source_ip || "-"}</td>
                    <td className="py-2.5 font-mono text-[11px] text-slate-500 max-w-xs truncate">{JSON.stringify(l.metadata)}</td>
                  </tr>
                ))}
              </tbody>
            </table>
            {(logsQ.data?.total_pages ?? 1) > 1 && (
              <div className="mt-4 flex items-center justify-between text-xs text-slate-500">
                <span>Page {logsQ.data?.page} of {logsQ.data?.total_pages}</span>
                <div className="flex gap-2">
                  <Button size="sm" variant="secondary" disabled={page <= 1} onClick={() => setPage((p) => p - 1)}>Prev</Button>
                  <Button size="sm" variant="secondary" disabled={page >= (logsQ.data?.total_pages ?? 1)} onClick={() => setPage((p) => p + 1)}>Next</Button>
                </div>
              </div>
            )}
          </div>
        )}
      </Card>
    </div>
  );
};
