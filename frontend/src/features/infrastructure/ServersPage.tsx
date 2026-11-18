import React, { useState } from "react";
import { Link, useSearchParams } from "react-router-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { Edit2, Eye, Plus, Power, PowerOff, Terminal, Trash2 } from "lucide-react";
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
import type { Server } from "@/types/api";
import { formatPercent, formatRelativeTime } from "@/utils/format";

interface ServerFormState {
  name: string;
  hostname: string;
  environment: string;
  operating_system: string;
  architecture: string;
  description: string;
  server_group: string;
  scrape_address: string;
  monitoring_interval: number;
  tagsRaw: string;
}

const EMPTY_FORM: ServerFormState = {
  name: "",
  hostname: "",
  environment: "production",
  operating_system: "linux",
  architecture: "amd64",
  description: "",
  server_group: "default",
  scrape_address: "",
  monitoring_interval: 15,
  tagsRaw: "",
};

export const ServersPage: React.FC = () => {
  const { canManageInfra, isDemoMode } = useAuth();
  const { refreshIntervalMs, refreshTick } = useWorkspace();
  const { notify } = useToast();
  const queryClient = useQueryClient();
  const [urlParams] = useSearchParams();

  const [search, setSearch] = useState(urlParams.get("search") ?? "");
  const [envFilter, setEnvFilter] = useState("");
  const [healthFilter, setHealthFilter] = useState("");
  const [page, setPage] = useState(1);

  const [formOpen, setFormOpen] = useState(false);
  const [editing, setEditing] = useState<Server | null>(null);
  const [form, setForm] = useState<ServerFormState>(EMPTY_FORM);
  const [deleteTarget, setDeleteTarget] = useState<Server | null>(null);
  const [connectModal, setConnectModal] = useState<Server | null>(null);

  const serversQ = useQuery({
    queryKey: ["servers", search, envFilter, healthFilter, page, refreshTick, isDemoMode],
    queryFn: () =>
      api.listServers({ search, environment: envFilter, health: healthFilter, page, page_size: 20 }),
    refetchInterval: refreshIntervalMs > 0 ? refreshIntervalMs : false,
  });

  const invalidate = () => {
    void queryClient.invalidateQueries({ queryKey: ["servers"] });
    void queryClient.invalidateQueries({ queryKey: ["overview"] });
  };

  const saveMutation = useMutation({
    mutationFn: async (st: ServerFormState) => {
      const tags = st.tagsRaw
        .split(",")
        .map((p) => p.trim())
        .filter(Boolean)
        .map((p) => {
          const [key, value = "true"] = p.split("=");
          return { key: key.trim(), value: value.trim() };
        });
      const payload = {
        name: st.name,
        hostname: st.hostname,
        environment: st.environment,
        operating_system: st.operating_system,
        architecture: st.architecture,
        description: st.description,
        server_group: st.server_group,
        scrape_address: st.scrape_address,
        monitoring_interval: Number(st.monitoring_interval) || 15,
        tags,
      };
      return editing ? api.updateServer(editing.id, payload) : api.createServer(payload);
    },
    onSuccess: (srv) => {
      notify({
        type: "success",
        title: editing ? `Updated ${srv.name}` : `Registered ${srv.name} (PENDING)`,
        description: editing ? "Prometheus discovery updated." : "Awaiting first Prometheus scrape from cloudpulse-agent.",
      });
      setFormOpen(false);
      setEditing(null);
      setForm(EMPTY_FORM);
      invalidate();
    },
    onError: (err) => {
      notify({ type: "error", title: "Could not save server", description: err instanceof Error ? err.message : "" });
    },
  });

  const deleteMutation = useMutation({
    mutationFn: (id: string) => api.deleteServer(id),
    onSuccess: (res) => {
      notify({
        type: "info",
        title: "Server removed from discovery",
        description: res.historical_metrics_notice,
      });
      setDeleteTarget(null);
      invalidate();
    },
    onError: (err) => {
      notify({ type: "error", title: "Delete failed", description: err instanceof Error ? err.message : "" });
    },
  });

  const toggleMutation = useMutation({
    mutationFn: (srv: Server) => (srv.monitoring_enabled ? api.disableServer(srv.id) : api.enableServer(srv.id)),
    onSuccess: (srv) => {
      notify({
        type: "success",
        title: srv.monitoring_enabled ? `Monitoring enabled for ${srv.name}` : `Monitoring disabled for ${srv.name}`,
      });
      invalidate();
    },
  });

  const openCreate = () => {
    setEditing(null);
    setForm(EMPTY_FORM);
    setFormOpen(true);
  };

  const openEdit = (srv: Server) => {
    setEditing(srv);
    setForm({
      name: srv.name,
      hostname: srv.hostname,
      environment: srv.environment,
      operating_system: srv.operating_system,
      architecture: srv.architecture,
      description: srv.description,
      server_group: srv.server_group,
      scrape_address: srv.scrape_address,
      monitoring_interval: srv.monitoring_interval,
      tagsRaw: (srv.tags ?? []).map((t) => `${t.key}=${t.value}`).join(", "),
    });
    setFormOpen(true);
  };

  if (serversQ.isError) {
    return (
      <ErrorState
        message={serversQ.error instanceof Error ? serversQ.error.message : "Failed to load servers"}
        onRetry={() => void serversQ.refetch()}
      />
    );
  }

  const items = serversQ.data?.items ?? [];

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Servers</h1>
          <p className="text-xs text-slate-500">
            Manage registered machines synchronized with Prometheus file-based service discovery
          </p>
        </div>
        {canManageInfra && (
          <Button onClick={openCreate}>
            <Plus className="h-4 w-4" /> Register Server
          </Button>
        )}
      </div>

      <Card
        action={
          <div className="flex flex-wrap items-center gap-2">
            <input
              type="search"
              value={search}
              onChange={(e) => {
                setSearch(e.target.value);
                setPage(1);
              }}
              placeholder="Search name, hostname, IP..."
              aria-label="Search servers"
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-slate-50 dark:bg-navy-800 px-3 py-1.5 text-xs"
            />
            <select
              aria-label="Filter environment"
              value={envFilter}
              onChange={(e) => setEnvFilter(e.target.value)}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5 text-xs"
            >
              <option value="">All Environments</option>
              <option value="production">Production</option>
              <option value="staging">Staging</option>
              <option value="development">Development</option>
              <option value="testing">Testing</option>
              <option value="other">Other</option>
            </select>
            <select
              aria-label="Filter health"
              value={healthFilter}
              onChange={(e) => setHealthFilter(e.target.value)}
              className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5 text-xs"
            >
              <option value="">All Statuses</option>
              <option value="UP">Healthy</option>
              <option value="WARNING">Warning</option>
              <option value="CRITICAL">Critical</option>
              <option value="DOWN">Down</option>
              <option value="PENDING">Pending</option>
              <option value="UNKNOWN">Unknown</option>
            </select>
          </div>
        }
      >
        {serversQ.isLoading ? (
          <Skeleton className="h-48 w-full" />
        ) : items.length === 0 ? (
          <EmptyState
            title="No servers registered"
            description="Register your first server to generate Prometheus file_sd targets and monitor real host metrics."
            action={canManageInfra && <Button onClick={openCreate}>Register Server</Button>}
          />
        ) : (
          <div className="overflow-x-auto">
            <table className="w-full text-left text-xs">
              <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                <tr>
                  <th className="py-2.5 pr-3">Server</th>
                  <th className="py-2.5 pr-3">Scrape Address</th>
                  <th className="py-2.5 pr-3">Env / Group</th>
                  <th className="py-2.5 pr-3">CPU / Mem / Disk</th>
                  <th className="py-2.5 pr-3">Last Seen</th>
                  <th className="py-2.5 pr-3">Status</th>
                  <th className="py-2.5 text-right">Actions</th>
                </tr>
              </thead>
              <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                {items.map((srv) => (
                  <tr key={srv.id} className="hover:bg-slate-50 dark:hover:bg-navy-800/40">
                    <td className="py-3 pr-3">
                      <Link to={`/servers/${srv.id}`} className="font-semibold text-brand-600 hover:underline">
                        {srv.name}
                      </Link>
                      <p className="text-[11px] font-mono text-slate-500">{srv.hostname}</p>
                    </td>
                    <td className="py-3 pr-3 font-mono text-slate-600 dark:text-slate-300">{srv.scrape_address}</td>
                    <td className="py-3 pr-3">
                      <span className="capitalize font-medium">{srv.environment}</span>
                      <span className="text-slate-400"> / {srv.server_group}</span>
                    </td>
                    <td className="py-3 pr-3 font-mono">
                      {formatPercent(srv.cpu_percent)} / {formatPercent(srv.memory_percent)} / {formatPercent(srv.disk_percent)}
                    </td>
                    <td className="py-3 pr-3 text-slate-500">{formatRelativeTime(srv.last_sample_at ?? srv.last_seen_at)}</td>
                    <td className="py-3 pr-3">
                      <StatusBadge status={srv.status} isStale={srv.is_stale} />
                    </td>
                    <td className="py-3 text-right">
                      <div className="inline-flex items-center gap-1">
                        <Link
                          to={`/servers/${srv.id}`}
                          aria-label={`View ${srv.name}`}
                          className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800"
                        >
                          <Eye className="h-4 w-4" />
                        </Link>
                        <button
                          type="button"
                          onClick={() => setConnectModal(srv)}
                          aria-label={`Connection details for ${srv.name}`}
                          className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800"
                        >
                          <Terminal className="h-4 w-4" />
                        </button>
                        {canManageInfra && (
                          <>
                            <button
                              type="button"
                              onClick={() => openEdit(srv)}
                              aria-label={`Edit ${srv.name}`}
                              className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800"
                            >
                              <Edit2 className="h-4 w-4" />
                            </button>
                            <button
                              type="button"
                              onClick={() => toggleMutation.mutate(srv)}
                              aria-label={srv.monitoring_enabled ? `Disable monitoring for ${srv.name}` : `Enable monitoring for ${srv.name}`}
                              title={srv.monitoring_enabled ? "Disable monitoring" : "Enable monitoring"}
                              className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800"
                            >
                              {srv.monitoring_enabled ? (
                                <PowerOff className="h-4 w-4 text-amber-500" />
                              ) : (
                                <Power className="h-4 w-4 text-emerald-500" />
                              )}
                            </button>
                            <button
                              type="button"
                              onClick={() => setDeleteTarget(srv)}
                              aria-label={`Delete ${srv.name}`}
                              className="rounded p-1.5 text-rose-500 hover:bg-rose-50 dark:hover:bg-rose-950/40"
                            >
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

      {/* Register / Edit Server Modal */}
      <Modal
        open={formOpen}
        onClose={() => setFormOpen(false)}
        title={editing ? `Edit Server: ${editing.name}` : "Register New Server"}
        description="Registered servers start in PENDING state until Prometheus successfully scrapes cloudpulse-agent."
        size="lg"
      >
        <form
          onSubmit={(e) => {
            e.preventDefault();
            saveMutation.mutate(form);
          }}
          className="space-y-4"
        >
          <div className="grid sm:grid-cols-2 gap-4">
            <div>
              <label htmlFor="srv-name" className="block text-xs font-medium mb-1">Server Display Name *</label>
              <input id="srv-name" required value={form.name} onChange={(e) => setForm({ ...form, name: e.target.value })} placeholder="prod-api-01" className={inputClassName} />
            </div>
            <div>
              <label htmlFor="srv-host" className="block text-xs font-medium mb-1">Hostname *</label>
              <input id="srv-host" required value={form.hostname} onChange={(e) => setForm({ ...form, hostname: e.target.value })} placeholder="prod-api-01.internal" className={inputClassName} />
            </div>
            <div>
              <label htmlFor="srv-addr" className="block text-xs font-medium mb-1">Prometheus Scrape Address (host:port) *</label>
              <input id="srv-addr" required value={form.scrape_address} onChange={(e) => setForm({ ...form, scrape_address: e.target.value })} placeholder="agent-node-1:9180" className={inputClassName} />
            </div>
            <div>
              <label htmlFor="srv-env" className="block text-xs font-medium mb-1">Environment *</label>
              <select id="srv-env" value={form.environment} onChange={(e) => setForm({ ...form, environment: e.target.value })} className={inputClassName}>
                <option value="production">Production</option>
                <option value="staging">Staging</option>
                <option value="development">Development</option>
                <option value="testing">Testing</option>
                <option value="other">Other</option>
              </select>
            </div>
            <div>
              <label htmlFor="srv-os" className="block text-xs font-medium mb-1">Operating System</label>
              <select id="srv-os" value={form.operating_system} onChange={(e) => setForm({ ...form, operating_system: e.target.value })} className={inputClassName}>
                <option value="linux">Linux</option>
                <option value="macos">macOS</option>
                <option value="windows">Windows</option>
              </select>
            </div>
            <div>
              <label htmlFor="srv-group" className="block text-xs font-medium mb-1">Server Group</label>
              <input id="srv-group" value={form.server_group} onChange={(e) => setForm({ ...form, server_group: e.target.value })} className={inputClassName} />
            </div>
            <div>
              <label htmlFor="srv-int" className="block text-xs font-medium mb-1">Monitoring Interval (seconds)</label>
              <input id="srv-int" type="number" min={5} max={3600} value={form.monitoring_interval} onChange={(e) => setForm({ ...form, monitoring_interval: Number(e.target.value) })} className={inputClassName} />
            </div>
            <div>
              <label htmlFor="srv-tags" className="block text-xs font-medium mb-1">Optional Tags (key=value, ...)</label>
              <input id="srv-tags" value={form.tagsRaw} onChange={(e) => setForm({ ...form, tagsRaw: e.target.value })} placeholder="role=api, region=us-east" className={inputClassName} />
            </div>
          </div>
          <div>
            <label htmlFor="srv-desc" className="block text-xs font-medium mb-1">Description</label>
            <textarea id="srv-desc" rows={2} value={form.description} onChange={(e) => setForm({ ...form, description: e.target.value })} className={inputClassName} />
          </div>
          <div className="flex justify-end gap-2 pt-2">
            <Button type="button" variant="secondary" onClick={() => setFormOpen(false)}>Cancel</Button>
            <Button type="submit" disabled={saveMutation.isPending}>
              {saveMutation.isPending ? "Saving..." : editing ? "Save Changes" : "Register Server"}
            </Button>
          </div>
        </form>
      </Modal>

      {/* Connection Details Modal */}
      <Modal
        open={Boolean(connectModal)}
        onClose={() => setConnectModal(null)}
        title={`Agent Connection Details: ${connectModal?.name}`}
        description="Configure cloudpulse-agent on this machine so Prometheus can scrape it."
      >
        {connectModal && (
          <div className="space-y-3 text-xs">
            <p>
              <strong>Server ID:</strong> <code className="font-mono">{connectModal.id}</code>
            </p>
            <p>
              <strong>Prometheus Target:</strong> <code className="font-mono">http://{connectModal.scrape_address}/metrics</code>
            </p>
            <pre className="rounded-lg bg-navy-900 p-3 text-sky-200 overflow-x-auto font-mono">
{`agent:
  id: "${connectModal.id}"
  name: "${connectModal.name}"
  environment: "${connectModal.environment}"
server:
  listen_address: "0.0.0.0:9180"
collection:
  interval: "${connectModal.monitoring_interval}s"
  timeout: "5s"`}
            </pre>
            <p className="text-slate-500">
              For remote servers behind NAT, connect via private VPN (WireGuard/Tailscale) rather than exposing port 9180 publicly.
            </p>
          </div>
        )}
      </Modal>

      {/* Delete Confirmation Dialog */}
      <ConfirmDialog
        open={Boolean(deleteTarget)}
        onClose={() => setDeleteTarget(null)}
        onConfirm={() => deleteTarget && deleteMutation.mutate(deleteTarget.id)}
        title="Remove Server from Monitoring?"
        message={`Removing "${deleteTarget?.name}" removes it from Prometheus file_sd discovery. Existing historical time-series samples in Prometheus remain governed by the configured retention policy.`}
        confirmLabel="Remove Server"
        loading={deleteMutation.isPending}
      />
    </div>
  );
};
