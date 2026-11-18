import React, { useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { BarChart3, Edit2, Plus, Power, PowerOff, Trash2 } from "lucide-react";
import { CartesianGrid, Line, LineChart, ResponsiveContainer, Tooltip, XAxis, YAxis } from "recharts";
import { useAuth } from "@/app/AuthContext";
import { useWorkspace } from "@/app/ThemeContext";
import {
  Button,
  Card,
  ConfirmDialog,
  EmptyState,
  ErrorState,
  Modal,
  Skeleton,
  StatusBadge,
  inputClassName,
} from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";
import type { ServiceCheck } from "@/types/api";
import { formatPercent, formatRelativeTime, formatShortTime } from "@/utils/format";

export const ServicesPage: React.FC = () => {
  const { canOperateAlerts, isDemoMode } = useAuth();
  const { refreshIntervalMs, refreshTick } = useWorkspace();
  const { notify } = useToast();
  const queryClient = useQueryClient();

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<ServiceCheck | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<ServiceCheck | null>(null);
  const [historyTarget, setHistoryTarget] = useState<ServiceCheck | null>(null);

  const [serverId, setServerId] = useState("");
  const [name, setName] = useState("");
  const [checkType, setCheckType] = useState<"http" | "tcp">("http");
  const [target, setTarget] = useState("");
  const [intervalSec, setIntervalSec] = useState(30);
  const [timeoutSec, setTimeoutSec] = useState(5);
  const [expectedStatus, setExpectedStatus] = useState(200);

  const servicesQ = useQuery({
    queryKey: ["services", refreshTick, isDemoMode],
    queryFn: () => api.listServices(),
    refetchInterval: refreshIntervalMs > 0 ? refreshIntervalMs : false,
  });

  const serversQ = useQuery({
    queryKey: ["servers-for-services", isDemoMode],
    queryFn: () => api.listServers({ page: 1, page_size: 100 }),
  });

  const historyQ = useQuery({
    queryKey: ["service-history", historyTarget?.id, isDemoMode],
    queryFn: () => api.getServiceHistory(historyTarget!.id, "6h"),
    enabled: Boolean(historyTarget),
  });

  const saveMut = useMutation({
    mutationFn: () =>
      editing
        ? api.updateService(editing.id, {
            name,
            check_type: checkType,
            target,
            interval_seconds: intervalSec,
            timeout_seconds: timeoutSec,
            expected_status_code: expectedStatus,
          })
        : api.createService({
            server_id: serverId,
            name,
            check_type: checkType,
            target,
            interval_seconds: intervalSec,
            timeout_seconds: timeoutSec,
            expected_status_code: expectedStatus,
          }),
    onSuccess: () => {
      notify({ type: "success", title: editing ? "Service check updated" : "Service check added" });
      setFormOpen(false);
      setEditing(null);
      void queryClient.invalidateQueries({ queryKey: ["services"] });
    },
    onError: (err) => notify({ type: "error", title: "Save failed", description: err instanceof Error ? err.message : "" }),
  });

  const toggleMut = useMutation({
    mutationFn: (sc: ServiceCheck) => api.updateService(sc.id, { enabled: !sc.enabled }),
    onSuccess: () => {
      notify({ type: "success", title: "Service check status updated" });
      void queryClient.invalidateQueries({ queryKey: ["services"] });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteService(id),
    onSuccess: () => {
      notify({ type: "info", title: "Service check removed" });
      setDeleteTarget(null);
      void queryClient.invalidateQueries({ queryKey: ["services"] });
    },
  });

  const openCreate = () => {
    setEditing(null);
    setServerId(serversQ.data?.items?.[0]?.id ?? "");
    setName("");
    setCheckType("http");
    setTarget("");
    setIntervalSec(30);
    setTimeoutSec(5);
    setExpectedStatus(200);
    setFormOpen(true);
  };

  const openEdit = (sc: ServiceCheck) => {
    setEditing(sc);
    setServerId(sc.server_id);
    setName(sc.name);
    setCheckType(sc.check_type);
    setTarget(sc.target);
    setIntervalSec(sc.interval_seconds);
    setTimeoutSec(sc.timeout_seconds);
    setExpectedStatus(sc.expected_status_code);
    setFormOpen(true);
  };

  if (servicesQ.isError) {
    return <ErrorState message={servicesQ.error instanceof Error ? servicesQ.error.message : "Failed to load services"} onRetry={() => void servicesQ.refetch()} />;
  }

  const list = servicesQ.data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Service Availability Monitoring</h1>
          <p className="text-xs text-slate-500">
            HTTP and TCP endpoint health tracked independently of host availability · Missing samples are never treated as UP
          </p>
        </div>
        {canOperateAlerts && (
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" /> Add Service Check
          </Button>
        )}
      </div>

      <Card title="Service Inventory">
        {servicesQ.isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : list.length === 0 ? (
          <EmptyState
            title="No service checks configured"
            description="Create HTTP or TCP service checks to track uptime percentage, HTTP status codes, and response latency."
            action={canOperateAlerts && <Button onClick={openCreate}>Add Service Check</Button>}
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                <tr>
                  <th className="py-2.5 pr-3">Service</th>
                  <th className="py-2.5 pr-3">Type</th>
                  <th className="py-2.5 pr-3">Target</th>
                  <th className="py-2.5 pr-3">Server</th>
                  <th className="py-2.5 pr-3">Status</th>
                  <th className="py-2.5 pr-3">Latency</th>
                  <th className="py-2.5 pr-3">24h Uptime</th>
                  <th className="py-2.5 pr-3">Last Check</th>
                  <th className="py-2.5 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {list.map((sc) => (
                  <tr key={sc.id}>
                    <td className="py-3 pr-3 font-semibold">{sc.name}</td>
                    <td className="py-3 pr-3 uppercase font-mono">{sc.check_type}</td>
                    <td className="py-3 pr-3 font-mono text-slate-600 dark:text-slate-300">{sc.target}</td>
                    <td className="py-3 pr-3">{sc.server_name || sc.server_id}</td>
                    <td className="py-3 pr-3"><StatusBadge status={sc.status} /></td>
                    <td className="py-3 pr-3 font-mono">{sc.latency_ms !== null ? `${sc.latency_ms.toFixed(1)} ms` : "N/A"}</td>
                    <td className="py-3 pr-3 font-mono">
                      {formatPercent(sc.availability_24h)}
                      {sc.valid_intervals_24h > 0 && <span className="text-slate-400 ml-1">({sc.valid_intervals_24h} pts)</span>}
                    </td>
                    <td className="py-3 pr-3 text-slate-500">{formatRelativeTime(sc.last_checked_at)}</td>
                    <td className="py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <button type="button" onClick={() => setHistoryTarget(sc)} aria-label="View service history" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                          <BarChart3 className="h-4 w-4" />
                        </button>
                        {canOperateAlerts && (
                          <>
                            <button type="button" onClick={() => openEdit(sc)} aria-label="Edit service" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                              <Edit2 className="h-4 w-4" />
                            </button>
                            <button type="button" onClick={() => toggleMut.mutate(sc)} aria-label="Toggle service check" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                              {sc.enabled ? <PowerOff className="h-4 w-4 text-amber-500" /> : <Power className="h-4 w-4 text-emerald-500" />}
                            </button>
                            <button type="button" onClick={() => setDeleteTarget(sc)} aria-label="Delete service" className="rounded p-1.5 text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40">
                              <Trash2 className="h-4 w-4" />
                            </button>
                          </>
                        )}
                      </div>
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      {/* Add/Edit Service Modal */}
      <Modal open={formOpen} onClose={() => setFormOpen(false)} title={editing ? "Edit Service Check" : "Add Service Check"}>
        <form
          onSubmit={(e) => {
            e.preventDefault();
            saveMut.mutate();
          }}
          className="space-y-4"
        >
          {!editing && (
            <div>
              <label className="block text-xs font-medium mb-1">Associated Server *</label>
              <select required value={serverId} onChange={(e) => setServerId(e.target.value)} className={inputClassName}>
                <option value="">Select server...</option>
                {(serversQ.data?.items ?? []).map((s) => (
                  <option key={s.id} value={s.id}>{s.name} ({s.environment})</option>
                ))}
              </select>
            </div>
          )}
          <div className="grid sm:grid-cols-2 gap-4">
            <div>
              <label className="block text-xs font-medium mb-1">Service Name *</label>
              <input required value={name} onChange={(e) => setName(e.target.value)} placeholder="application-api" className={inputClassName} />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Check Type *</label>
              <select value={checkType} onChange={(e) => setCheckType(e.target.value as "http" | "tcp")} className={inputClassName}>
                <option value="http">HTTP / HTTPS</option>
                <option value="tcp">TCP Port</option>
              </select>
            </div>
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Target Endpoint * (no embedded passwords)</label>
            <input required value={target} onChange={(e) => setTarget(e.target.value)} placeholder={checkType === "http" ? "http://127.0.0.1:8080/healthz" : "127.0.0.1:5432"} className={inputClassName} />
          </div>
          <div className="grid grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-medium mb-1">Interval (s)</label>
              <input type="number" min={5} value={intervalSec} onChange={(e) => setIntervalSec(Number(e.target.value))} className={inputClassName} />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Timeout (s)</label>
              <input type="number" min={1} value={timeoutSec} onChange={(e) => setTimeoutSec(Number(e.target.value))} className={inputClassName} />
            </div>
            {checkType === "http" && (
              <div>
                <label className="block text-xs font-medium mb-1">Expected Status</label>
                <input type="number" min={100} max={599} value={expectedStatus} onChange={(e) => setExpectedStatus(Number(e.target.value))} className={inputClassName} />
              </div>
            )}
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setFormOpen(false)}>Cancel</Button>
            <Button type="submit" disabled={saveMut.isPending}>{saveMut.isPending ? "Saving..." : "Save Service"}</Button>
          </div>
        </form>
      </Modal>

      {/* Historical Availability Modal */}
      <Modal
        open={Boolean(historyTarget)}
        onClose={() => setHistoryTarget(null)}
        title={`Service History: ${historyTarget?.name}`}
        description="Calculated strictly from valid collected Prometheus intervals."
        size="lg"
      >
        {historyQ.isLoading ? (
          <Skeleton className="h-52 w-full" />
        ) : (
          <div className="space-y-4">
            <div className="grid grid-cols-3 gap-3 text-xs">
              <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3">
                <p className="text-slate-500">Availability (6h)</p>
                <p className="text-lg font-bold mt-0.5">{formatPercent(historyQ.data?.availability_percent)}</p>
              </div>
              <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3">
                <p className="text-slate-500">Valid / Failed Checks</p>
                <p className="text-lg font-bold mt-0.5">{historyQ.data?.valid_intervals ?? 0} / {historyQ.data?.failed_intervals ?? 0}</p>
              </div>
              <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3">
                <p className="text-slate-500">Avg Latency</p>
                <p className="text-lg font-bold mt-0.5">
                  {historyQ.data?.average_latency_ms !== null && historyQ.data?.average_latency_ms !== undefined
                    ? `${historyQ.data.average_latency_ms.toFixed(1)} ms`
                    : "N/A"}
                </p>
              </div>
            </div>
            {(historyQ.data?.latency_timeline?.length ?? 0) > 0 ? (
              <div className="h-56">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={historyQ.data!.latency_timeline.map((p) => ({ time: formatShortTime(p.timestamp), ms: p.value }))}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
                    <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                    <YAxis unit="ms" tick={{ fontSize: 11 }} />
                    <Tooltip />
                    <Line type="monotone" dataKey="ms" name="Response Time (ms)" stroke="#0284c7" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <EmptyState title="No historical service samples" description="Ensure this service check is also configured on the host's cloudpulse-agent." />
            )}
          </div>
        )}
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMut.mutate(deleteTarget.id)}
        title="Remove Service Check?"
        message={`Delete service check "${deleteTarget?.name}"?`}
        confirmLabel="Delete"
        loading={deleteMut.isPending}
      />
    </div>
  );
};
