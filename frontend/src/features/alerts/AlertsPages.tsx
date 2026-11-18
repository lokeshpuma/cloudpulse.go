import React, { useState } from "react";
import { Link, useParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ArrowLeft, CheckCircle2, Edit2, Plus, Power, PowerOff, ShieldCheck, Trash2 } from "lucide-react";
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
import type { AlertRule, AlertView } from "@/types/api";
import { formatRelativeTime, formatShortTime, formatTimestamp } from "@/utils/format";

export const AlertsPage: React.FC = () => {
  const { canOperateAlerts, isDemoMode } = useAuth();
  const { refreshIntervalMs, refreshTick } = useWorkspace();
  const { notify } = useToast();
  const queryClient = useQueryClient();

  const [severity, setSeverity] = useState("");
  const [environment, setEnvironment] = useState("");
  const [state, setState] = useState("active");
  const [acknowledged, setAcknowledged] = useState("");
  const [serverFilter, setServerFilter] = useState("");
  const [ackTarget, setAckTarget] = useState<AlertView | null>(null);
  const [ackNote, setAckNote] = useState("");

  const alertsQ = useQuery({
    queryKey: ["alerts", severity, environment, state, acknowledged, serverFilter, refreshTick, isDemoMode],
    queryFn: () =>
      api.listAlerts({ severity, environment, state, acknowledged, server: serverFilter }),
    refetchInterval: refreshIntervalMs > 0 ? refreshIntervalMs : false,
  });

  const ackMut = useMutation({
    mutationFn: ({ fp, note }: { fp: string; note: string }) => api.acknowledgeAlert(fp, note),
    onSuccess: () => {
      notify({
        type: "success",
        title: "Alert acknowledged",
        description: "Recorded in PostgreSQL. Prometheus alert remains active until condition recovers.",
      });
      setAckTarget(null);
      setAckNote("");
      void queryClient.invalidateQueries({ queryKey: ["alerts"] });
    },
    onError: (err) => notify({ type: "error", title: "Failed", description: err instanceof Error ? err.message : "" }),
  });

  if (alertsQ.isError) {
    return <ErrorState message={alertsQ.error instanceof Error ? alertsQ.error.message : "Failed to load alerts"} onRetry={() => void alertsQ.refetch()} />;
  }

  const list = alertsQ.data ?? [];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Active Alerts</h1>
          <p className="text-xs text-slate-500">Live alerts from Prometheus & Alertmanager joined with CloudPulse acknowledgements</p>
        </div>
        <Link to="/alert-rules">
          <Button variant="secondary" size="sm">Manage Alert Rules →</Button>
        </Link>
      </div>

      <Card
        action={
          <div className="flex flex-wrap items-center gap-2">
            <input
              placeholder="Filter server..."
              value={serverFilter}
              onChange={(e) => setServerFilter(e.target.value)}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-navy-800 px-2.5 py-1.5 text-xs"
            />
            <select value={severity} onChange={(e) => setSeverity(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5 text-xs">
              <option value="">All Severities</option>
              <option value="critical">Critical</option>
              <option value="warning">Warning</option>
              <option value="info">Info</option>
            </select>
            <select value={environment} onChange={(e) => setEnvironment(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5 text-xs">
              <option value="">All Envs</option>
              <option value="production">Production</option>
              <option value="staging">Staging</option>
              <option value="development">Development</option>
            </select>
            <select value={state} onChange={(e) => setState(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5 text-xs">
              <option value="active">Active</option>
              <option value="resolved">Resolved</option>
              <option value="all">All States</option>
            </select>
            <select value={acknowledged} onChange={(e) => setAcknowledged(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1.5 text-xs">
              <option value="">All Ack</option>
              <option value="false">Unacknowledged</option>
              <option value="true">Acknowledged</option>
            </select>
          </div>
        }
      >
        {alertsQ.isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : list.length === 0 ? (
          <EmptyState title="No matching alerts" description="No active or historical alerts match your selected filters." />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                <tr>
                  <th className="py-2.5 pr-3">Alert</th>
                  <th className="py-2.5 pr-3">Severity</th>
                  <th className="py-2.5 pr-3">Server</th>
                  <th className="py-2.5 pr-3">State</th>
                  <th className="py-2.5 pr-3">Triggered</th>
                  <th className="py-2.5 pr-3">Acknowledgement</th>
                  <th className="py-2.5 text-right">Action</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {list.map((al) => (
                  <tr key={al.fingerprint}>
                    <td className="py-3 pr-3">
                      <Link to={`/alerts/${encodeURIComponent(al.fingerprint)}`} className="font-semibold text-brand-600 hover:underline">
                        {al.title}
                      </Link>
                      <p className="text-[11px] text-slate-500 mt-0.5 line-clamp-1">{al.description}</p>
                    </td>
                    <td className="py-3 pr-3"><StatusBadge status={al.severity} /></td>
                    <td className="py-3 pr-3">{al.server_name} <span className="text-slate-400">({al.environment})</span></td>
                    <td className="py-3 pr-3"><StatusBadge status={al.state} /></td>
                    <td className="py-3 pr-3 text-slate-500">{formatRelativeTime(al.triggered_at)}</td>
                    <td className="py-3 pr-3">
                      {al.acknowledged ? (
                        <span className="text-emerald-600 font-medium">
                          Ack by {al.acknowledgement?.acknowledged_by_name || "Operator"}
                        </span>
                      ) : (
                        <span className="text-slate-400">Unacknowledged</span>
                      )}
                    </td>
                    <td className="py-3 text-right">
                      {canOperateAlerts && !al.acknowledged && (
                        <Button size="sm" variant="secondary" onClick={() => setAckTarget(al)}>
                          Acknowledge
                        </Button>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal
        open={Boolean(ackTarget)}
        onClose={() => setAckTarget(null)}
        title="Acknowledge Alert"
        description="Acknowledging logs ownership in CloudPulse without silencing the underlying Prometheus alert."
        size="sm"
      >
        <div className="space-y-4">
          <p className="text-xs font-medium">{ackTarget?.title}</p>
          <div>
            <label className="block text-xs font-medium mb-1">Note (optional)</label>
            <textarea rows={3} value={ackNote} onChange={(e) => setAckNote(e.target.value)} className={inputClassName} />
          </div>
          <div className="flex justify-end gap-2">
            <Button variant="secondary" onClick={() => setAckTarget(null)}>Cancel</Button>
            <Button disabled={ackMut.isPending} onClick={() => ackTarget && ackMut.mutate({ fp: ackTarget.fingerprint, note: ackNote })}>
              Acknowledge
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
};

export const AlertDetailPage: React.FC = () => {
  const { fingerprint = "" } = useParams();
  const { organization, canOperateAlerts, isDemoMode } = useAuth();
  const { notify } = useToast();
  const queryClient = useQueryClient();
  const [note, setNote] = useState("");
  const tz = organization?.timezone ?? "UTC";

  const detailQ = useQuery({
    queryKey: ["alert-detail", fingerprint, isDemoMode],
    queryFn: () => api.getAlertDetail(fingerprint),
    enabled: Boolean(fingerprint),
  });

  const ackMut = useMutation({
    mutationFn: () => api.acknowledgeAlert(fingerprint, note),
    onSuccess: () => {
      notify({ type: "success", title: "Alert acknowledged" });
      setNote("");
      void queryClient.invalidateQueries({ queryKey: ["alert-detail", fingerprint] });
    },
  });

  if (detailQ.isLoading) return <Skeleton className="h-96 w-full" />;
  if (detailQ.isError || !detailQ.data) {
    return <ErrorState message={detailQ.error instanceof Error ? detailQ.error.message : "Alert not found"} />;
  }

  const { alert, related_metrics, related_metric_label, timeline } = detailQ.data;

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Link to="/alerts" className="rounded-lg border border-slate-200 dark:border-slate-700 p-2">
          <ArrowLeft className="h-4 w-4" />
        </Link>
        <div>
          <div className="flex items-center gap-2 flex-wrap">
            <h1 className="text-xl font-bold">{alert.title}</h1>
            <StatusBadge status={alert.severity} />
            <StatusBadge status={alert.state} />
          </div>
          <p className="text-xs text-slate-500">Fingerprint: <code className="font-mono">{alert.fingerprint}</code> · Server: {alert.server_name}</p>
        </div>
      </div>

      <div className="grid lg:grid-cols-3 gap-6">
        <div className="lg:col-span-2 space-y-6">
          <Card title="Description & Related Metric">
            <p className="text-sm text-slate-700 dark:text-slate-300 mb-4">{alert.description}</p>
            {related_metrics.length > 0 ? (
              <div className="h-56">
                <ResponsiveContainer width="100%" height="100%">
                  <LineChart data={related_metrics.map((p) => ({ time: formatShortTime(p.timestamp, tz), val: p.value }))}>
                    <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
                    <XAxis dataKey="time" tick={{ fontSize: 11 }} />
                    <YAxis tick={{ fontSize: 11 }} />
                    <Tooltip />
                    <Line type="monotone" dataKey="val" name={related_metric_label} stroke="#ef4444" strokeWidth={2} dot={false} />
                  </LineChart>
                </ResponsiveContainer>
              </div>
            ) : (
              <p className="text-xs text-slate-500">No related metric series available for this alert.</p>
            )}
          </Card>

          <Card title="Labels & Annotations">
            <div className="grid sm:grid-cols-2 gap-4 text-xs">
              <div>
                <h3 className="font-semibold mb-2">Prometheus Labels</h3>
                <div className="flex flex-wrap gap-1.5">
                  {Object.entries(alert.labels ?? {}).map(([k, v]) => (
                    <span key={k} className="rounded bg-slate-100 dark:bg-navy-800 px-2 py-1 font-mono">
                      {k}={v}
                    </span>
                  ))}
                </div>
              </div>
              <div>
                <h3 className="font-semibold mb-2">Annotations</h3>
                <ul className="space-y-1">
                  {Object.entries(alert.annotations ?? {}).map(([k, v]) => (
                    <li key={k}><span className="font-semibold">{k}:</span> {v}</li>
                  ))}
                </ul>
              </div>
            </div>
          </Card>
        </div>

        <div className="space-y-6">
          {canOperateAlerts && (
            <Card title="Acknowledge Alert" subtitle="Records acknowledgement in PostgreSQL without silencing Prometheus">
              <div className="space-y-3">
                <textarea
                  rows={3}
                  value={note}
                  onChange={(e) => setNote(e.target.value)}
                  placeholder="Investigation note..."
                  className={inputClassName}
                />
                <Button className="w-full" onClick={() => ackMut.mutate()} disabled={ackMut.isPending}>
                  <CheckCircle2 className="h-4 w-4" /> Record Acknowledgement
                </Button>
              </div>
            </Card>
          )}

          <Card title="Alert Timeline">
            <ol className="space-y-3 text-xs">
              {timeline.map((ev, idx) => (
                <li key={idx} className="border-l-2 border-brand-500 pl-3">
                  <p className="font-semibold">{ev.event}</p>
                  <p className="text-slate-500">{ev.detail}</p>
                  <p className="text-[11px] text-slate-400 mt-0.5">{ev.actor} · {formatTimestamp(ev.timestamp, tz)}</p>
                </li>
              ))}
            </ol>
          </Card>
        </div>
      </div>
    </div>
  );
};

export const AlertRulesPage: React.FC = () => {
  const { canManageInfra, isDemoMode } = useAuth();
  const { notify } = useToast();
  const queryClient = useQueryClient();

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<AlertRule | null>(null);
  const [deleteTarget, setDeleteTarget] = useState<AlertRule | null>(null);

  const [name, setName] = useState("");
  const [description, setDescription] = useState("");
  const [template, setTemplate] = useState("cpu_usage");
  const [threshold, setThreshold] = useState(80);
  const [expression, setExpression] = useState("");
  const [severity, setSeverity] = useState<"critical" | "warning" | "info">("warning");
  const [duration, setDuration] = useState("5m");

  const rulesQ = useQuery({
    queryKey: ["alert-rules", isDemoMode],
    queryFn: () => api.listAlertRules(),
  });

  const saveMut = useMutation({
    mutationFn: () =>
      editing
        ? api.updateAlertRule(editing.id, { name, description, expression, threshold, severity, duration })
        : api.createAlertRule({
            name,
            description,
            metric_template: template,
            threshold,
            expression: expression.trim() || undefined,
            severity,
            duration,
          }),
    onSuccess: () => {
      notify({ type: "success", title: editing ? "Alert rule updated & synced to Prometheus" : "Custom alert rule created" });
      setFormOpen(false);
      setEditing(null);
      void queryClient.invalidateQueries({ queryKey: ["alert-rules"] });
    },
    onError: (err) => notify({ type: "error", title: "Validation error", description: err instanceof Error ? err.message : "" }),
  });

  const toggleMut = useMutation({
    mutationFn: (r: AlertRule) => api.updateAlertRule(r.id, { enabled: !r.enabled }),
    onSuccess: () => {
      notify({ type: "success", title: "Alert rule toggled" });
      void queryClient.invalidateQueries({ queryKey: ["alert-rules"] });
    },
  });

  const validateMut = useMutation({
    mutationFn: (id: string) => api.validateAlertRule(id),
    onSuccess: (res) => {
      notify({
        type: res.valid ? "success" : "error",
        title: res.valid ? "Expression valid" : "Expression invalid",
        description: res.valid
          ? res.prometheus_evaluated
            ? `Evaluated against Prometheus (${res.matching_series ?? 0} matching series).`
            : res.diagnostic
          : res.error,
      });
    },
  });

  const deleteMut = useMutation({
    mutationFn: (id: string) => api.deleteAlertRule(id),
    onSuccess: () => {
      notify({ type: "info", title: "Custom rule deleted" });
      setDeleteTarget(null);
      void queryClient.invalidateQueries({ queryKey: ["alert-rules"] });
    },
  });

  const openCreate = () => {
    setEditing(null);
    setName("");
    setDescription("");
    setTemplate("cpu_usage");
    setThreshold(85);
    setExpression("");
    setSeverity("warning");
    setDuration("5m");
    setFormOpen(true);
  };

  const openEdit = (r: AlertRule) => {
    setEditing(r);
    setName(r.name);
    setDescription(r.description);
    setTemplate(r.metric_template);
    setThreshold(r.threshold);
    setExpression(r.expression);
    setSeverity(r.severity);
    setDuration(r.duration);
    setFormOpen(true);
  };

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Alert Rules</h1>
          <p className="text-xs text-slate-500">Configurable Prometheus alert policy thresholds and custom PromQL rules</p>
        </div>
        {canManageInfra && (
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" /> Create Alert Rule
          </Button>
        )}
      </div>

      <Card>
        {rulesQ.isLoading ? (
          <Skeleton className="h-60 w-full" />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                <tr>
                  <th className="py-2.5 pr-3">Rule</th>
                  <th className="py-2.5 pr-3">PromQL Expression</th>
                  <th className="py-2.5 pr-3">Duration</th>
                  <th className="py-2.5 pr-3">Severity</th>
                  <th className="py-2.5 pr-3">State</th>
                  <th className="py-2.5 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {(rulesQ.data ?? []).map((r) => (
                  <tr key={r.id}>
                    <td className="py-3 pr-3">
                      <p className="font-semibold">{r.name} {r.is_default && <span className="ml-1 rounded bg-slate-100 dark:bg-navy-800 px-1.5 py-0.5 text-[10px]">Default</span>}</p>
                      <p className="text-[11px] text-slate-500">{r.description}</p>
                    </td>
                    <td className="py-3 pr-3 font-mono text-[11px] text-brand-600 dark:text-sky-400">{r.expression}</td>
                    <td className="py-3 pr-3 font-mono">{r.duration}</td>
                    <td className="py-3 pr-3"><StatusBadge status={r.severity} /></td>
                    <td className="py-3 pr-3">{r.enabled ? "Enabled" : "Disabled"}</td>
                    <td className="py-3 text-right">
                      {canManageInfra && (
                        <div className="inline-flex items-center gap-1">
                          <button type="button" onClick={() => validateMut.mutate(r.id)} title="Validate expression" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                            <ShieldCheck className="h-4 w-4 text-sky-500" />
                          </button>
                          <button type="button" onClick={() => openEdit(r)} title="Edit rule" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                            <Edit2 className="h-4 w-4" />
                          </button>
                          <button type="button" onClick={() => toggleMut.mutate(r)} title="Toggle enabled" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                            {r.enabled ? <PowerOff className="h-4 w-4 text-amber-500" /> : <Power className="h-4 w-4 text-emerald-500" />}
                          </button>
                          {!r.is_default && (
                            <button type="button" onClick={() => setDeleteTarget(r)} title="Delete rule" className="rounded p-1.5 text-rose-500 hover:bg-rose-50">
                              <Trash2 className="h-4 w-4" />
                            </button>
                          )}
                        </div>
                      )}
                    </td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        )}
      </Card>

      <Modal open={formOpen} onClose={() => setFormOpen(false)} title={editing ? "Edit Alert Rule" : "Create Custom Alert Rule"}>
        <form onSubmit={(e) => { e.preventDefault(); saveMut.mutate(); }} className="space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Rule Name *</label>
            <input required value={name} onChange={(e) => setName(e.target.value)} className={inputClassName} />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Description</label>
            <input value={description} onChange={(e) => setDescription(e.target.value)} className={inputClassName} />
          </div>
          <div className="grid sm:grid-cols-3 gap-3">
            <div>
              <label className="block text-xs font-medium mb-1">Metric Template</label>
              <select value={template} onChange={(e) => setTemplate(e.target.value)} className={inputClassName}>
                <option value="cpu_usage">CPU Usage (%)</option>
                <option value="memory_usage">Memory Usage (%)</option>
                <option value="disk_usage">Disk Usage (%)</option>
                <option value="server_down">Server Down</option>
                <option value="service_down">Service Down</option>
                <option value="network_errors">Network Errors</option>
                <option value="custom">Custom PromQL</option>
              </select>
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Threshold</label>
              <input type="number" step="0.1" value={threshold} onChange={(e) => setThreshold(Number(e.target.value))} className={inputClassName} />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">For Duration</label>
              <input required value={duration} onChange={(e) => setDuration(e.target.value)} placeholder="5m" className={inputClassName} />
            </div>
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Severity</label>
            <select value={severity} onChange={(e) => setSeverity(e.target.value as "critical" | "warning" | "info")} className={inputClassName}>
              <option value="warning">Warning</option>
              <option value="critical">Critical</option>
              <option value="info">Info</option>
            </select>
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">PromQL Expression (optional if using template)</label>
            <input value={expression} onChange={(e) => setExpression(e.target.value)} placeholder="cloudpulse_cpu_usage_percent > 85" className={`${inputClassName} font-mono text-xs`} />
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setFormOpen(false)}>Cancel</Button>
            <Button type="submit" disabled={saveMut.isPending}>{saveMut.isPending ? "Saving..." : "Save Rule"}</Button>
          </div>
        </form>
      </Modal>

      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMut.mutate(deleteTarget.id)}
        title="Delete Alert Rule?"
        message={`Delete custom alert rule "${deleteTarget?.name}"?`}
        confirmLabel="Delete"
        loading={deleteMut.isPending}
      />
    </div>
  );
};
