import React, { useEffect, useState } from "react";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { ExternalLink, KeyRound, Plus, UserX } from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import { useWorkspace } from "@/app/ThemeContext";
import { Button, Card, Modal, Skeleton, StatusBadge, inputClassName } from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";
import type { UserRole } from "@/types/api";

type TabKey = "general" | "monitoring" | "users" | "integrations";

export const SettingsPage: React.FC = () => {
  const { organization, canManageInfra, refreshOrganization, isDemoMode } = useAuth();
  const { theme, setTheme } = useWorkspace();
  const { notify } = useToast();
  const queryClient = useQueryClient();

  const [tab, setTab] = useState<TabKey>("general");

  // General + Monitoring form state
  const [orgName, setOrgName] = useState(organization?.name ?? "");
  const [orgDesc, setOrgDesc] = useState(organization?.description ?? "");
  const [timezone, setTimezone] = useState(organization?.timezone ?? "UTC");
  const [dateFormat, setDateFormat] = useState(organization?.date_format ?? "YYYY-MM-DD");
  const [timeFormat, setTimeFormat] = useState<"12h" | "24h">(organization?.time_format ?? "24h");
  const [units, setUnits] = useState<"binary" | "decimal">(organization?.display_units ?? "binary");
  const [scrapeInt, setScrapeInt] = useState(organization?.default_scrape_interval ?? 15);
  const [monTimeout, setMonTimeout] = useState(organization?.default_monitoring_timeout ?? 5);
  const [agentTimeout, setAgentTimeout] = useState(organization?.agent_connection_timeout ?? 10);
  const [diskExcl, setDiskExcl] = useState(organization?.disk_exclusion_patterns ?? "/proc,/sys,/dev,/run");
  const [defaultEnv, setDefaultEnv] = useState(organization?.default_environment ?? "production");

  // New user modal
  const [userModal, setUserModal] = useState(false);
  const [uName, setUName] = useState("");
  const [uEmail, setUEmail] = useState("");
  const [uPass, setUPass] = useState("");
  const [uRole, setURole] = useState<UserRole>("operator");

  useEffect(() => {
    if (!organization) return;
    setOrgName(organization.name);
    setOrgDesc(organization.description);
    setTimezone(organization.timezone);
    setDateFormat(organization.date_format);
    setTimeFormat(organization.time_format);
    setUnits(organization.display_units);
    setScrapeInt(organization.default_scrape_interval);
    setMonTimeout(organization.default_monitoring_timeout);
    setAgentTimeout(organization.agent_connection_timeout);
    setDiskExcl(organization.disk_exclusion_patterns);
    setDefaultEnv(organization.default_environment);
  }, [organization]);

  const usersQ = useQuery({
    queryKey: ["users", isDemoMode],
    queryFn: () => api.listUsers(),
    enabled: tab === "users" && canManageInfra,
  });

  const integQ = useQuery({
    queryKey: ["system-integrations", isDemoMode],
    queryFn: () => api.getIntegrations(),
    enabled: tab === "integrations",
  });

  const saveOrgMut = useMutation({
    mutationFn: () =>
      api.updateOrg({
        name: orgName,
        description: orgDesc,
        timezone,
        date_format: dateFormat,
        time_format: timeFormat,
        display_units: units,
        default_scrape_interval: scrapeInt,
        default_monitoring_timeout: monTimeout,
        agent_connection_timeout: agentTimeout,
        disk_exclusion_patterns: diskExcl,
        default_environment: defaultEnv,
      }),
    onSuccess: async () => {
      await refreshOrganization();
      notify({ type: "success", title: "Settings persisted" });
    },
    onError: (err) => notify({ type: "error", title: "Could not save settings", description: err instanceof Error ? err.message : "" }),
  });

  const createUserMut = useMutation({
    mutationFn: () => api.createUser({ name: uName, email: uEmail, password: uPass, role: uRole }),
    onSuccess: () => {
      notify({ type: "success", title: "User account created" });
      setUserModal(false);
      setUName("");
      setUEmail("");
      setUPass("");
      void queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err) => notify({ type: "error", title: "Could not create user", description: err instanceof Error ? err.message : "" }),
  });

  const roleMut = useMutation({
    mutationFn: ({ id, role }: { id: string; role: string }) => api.updateUser(id, { role }),
    onSuccess: () => {
      notify({ type: "success", title: "User role updated" });
      void queryClient.invalidateQueries({ queryKey: ["users"] });
    },
  });

  const disableMut = useMutation({
    mutationFn: (id: string) => api.disableUser(id),
    onSuccess: () => {
      notify({ type: "info", title: "User account disabled and sessions revoked" });
      void queryClient.invalidateQueries({ queryKey: ["users"] });
    },
    onError: (err) => notify({ type: "error", title: "Action failed", description: err instanceof Error ? err.message : "" }),
  });

  const resetMut = useMutation({
    mutationFn: (id: string) => api.resetUserAccess(id),
    onSuccess: (res) => {
      notify({
        type: "info",
        title: "Sessions revoked & reset token issued",
        description: res.dev_reset_token ? `Dev token: ${res.dev_reset_token}` : "User must complete password reset.",
      });
    },
  });

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-xl font-bold text-slate-900 dark:text-white">Settings</h1>
        <p className="text-xs text-slate-500">Organization preferences, monitoring defaults, role-based access, and integration diagnostics</p>
      </div>

      <div className="flex flex-wrap gap-2 border-b border-slate-200 dark:border-slate-800 pb-3">
        {(
          [
            ["general", "General"],
            ["monitoring", "Monitoring"],
            ...(canManageInfra ? ([["users", "User Management"]] as const) : []),
            ["integrations", "Integrations"],
          ] as const
        ).map(([k, label]) => (
          <Button key={k} size="sm" variant={tab === k ? "primary" : "secondary"} onClick={() => setTab(k)}>
            {label}
          </Button>
        ))}
      </div>

      {tab === "general" && (
        <Card title="General Settings" subtitle="Workspace branding, localization, units, and display theme">
          <form onSubmit={(e) => { e.preventDefault(); saveOrgMut.mutate(); }} className="space-y-4 max-w-2xl">
            <div className="grid sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium mb-1">Organization Name</label>
                <input disabled={!canManageInfra} value={orgName} onChange={(e) => setOrgName(e.target.value)} className={inputClassName} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Default Time Zone</label>
                <select disabled={!canManageInfra} value={timezone} onChange={(e) => setTimezone(e.target.value)} className={inputClassName}>
                  <option value="UTC">UTC</option>
                  <option value="America/New_York">America/New_York</option>
                  <option value="America/Los_Angeles">America/Los_Angeles</option>
                  <option value="Europe/London">Europe/London</option>
                  <option value="Asia/Kolkata">Asia/Kolkata</option>
                  <option value="Asia/Tokyo">Asia/Tokyo</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Date Format</label>
                <select disabled={!canManageInfra} value={dateFormat} onChange={(e) => setDateFormat(e.target.value)} className={inputClassName}>
                  <option value="YYYY-MM-DD">YYYY-MM-DD</option>
                  <option value="MM/DD/YYYY">MM/DD/YYYY</option>
                  <option value="DD/MM/YYYY">DD/MM/YYYY</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Time Format</label>
                <select disabled={!canManageInfra} value={timeFormat} onChange={(e) => setTimeFormat(e.target.value as "12h" | "24h")} className={inputClassName}>
                  <option value="24h">24-hour</option>
                  <option value="12h">12-hour</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Preferred Units</label>
                <select disabled={!canManageInfra} value={units} onChange={(e) => setUnits(e.target.value as "binary" | "decimal")} className={inputClassName}>
                  <option value="binary">Binary (KiB / MiB / GiB)</option>
                  <option value="decimal">Decimal (KB / MB / GB)</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Interface Theme</label>
                <select value={theme} onChange={(e) => setTheme(e.target.value as "light" | "dark")} className={inputClassName}>
                  <option value="light">Light Workspace</option>
                  <option value="dark">Dark Workspace</option>
                </select>
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Organization Description</label>
              <textarea disabled={!canManageInfra} rows={2} value={orgDesc} onChange={(e) => setOrgDesc(e.target.value)} className={inputClassName} />
            </div>
            {canManageInfra && (
              <Button type="submit" disabled={saveOrgMut.isPending}>{saveOrgMut.isPending ? "Saving..." : "Save General Settings"}</Button>
            )}
          </form>
        </Card>
      )}

      {tab === "monitoring" && (
        <Card title="Monitoring Settings" subtitle="Default scrape intervals, timeouts, and Prometheus retention guidance">
          <form onSubmit={(e) => { e.preventDefault(); saveOrgMut.mutate(); }} className="space-y-4 max-w-2xl">
            <div className="grid sm:grid-cols-3 gap-4">
              <div>
                <label className="block text-xs font-medium mb-1">Default Scrape Interval (s)</label>
                <input disabled={!canManageInfra} type="number" min={5} value={scrapeInt} onChange={(e) => setScrapeInt(Number(e.target.value))} className={inputClassName} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Monitoring Timeout (s)</label>
                <input disabled={!canManageInfra} type="number" min={1} value={monTimeout} onChange={(e) => setMonTimeout(Number(e.target.value))} className={inputClassName} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Agent Connect Timeout (s)</label>
                <input disabled={!canManageInfra} type="number" min={1} value={agentTimeout} onChange={(e) => setAgentTimeout(Number(e.target.value))} className={inputClassName} />
              </div>
            </div>
            <div className="grid sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium mb-1">Disk Exclusion Patterns</label>
                <input disabled={!canManageInfra} value={diskExcl} onChange={(e) => setDiskExcl(e.target.value)} className={`${inputClassName} font-mono text-xs`} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Default Server Environment</label>
                <select disabled={!canManageInfra} value={defaultEnv} onChange={(e) => setDefaultEnv(e.target.value)} className={inputClassName}>
                  <option value="production">Production</option>
                  <option value="staging">Staging</option>
                  <option value="development">Development</option>
                  <option value="testing">Testing</option>
                </select>
              </div>
            </div>
            <div className="rounded-lg border border-sky-200 dark:border-sky-900 bg-sky-50/70 dark:bg-sky-950/30 p-3.5 text-xs text-sky-900 dark:text-sky-200">
              <strong>Metric Retention Policy Guidance:</strong> Prometheus time-series retention is governed by infrastructure configuration (<code className="font-mono">--storage.tsdb.retention.time=30d</code> in <code className="font-mono">docker-compose.yml</code>). Updating retention requires modifying Prometheus startup flags and restarting the Prometheus container.
            </div>
            {canManageInfra && (
              <Button type="submit" disabled={saveOrgMut.isPending}>Save Monitoring Settings</Button>
            )}
          </form>
        </Card>
      )}

      {tab === "users" && canManageInfra && (
        <Card
          title="User Management"
          subtitle="Manage organization roles (Administrator, Operator, Viewer)"
          action={
            <Button size="sm" onClick={() => setUserModal(true)}>
              <Plus className="h-4 w-4" /> Add User
            </Button>
          }
        >
          {usersQ.isLoading ? (
            <Skeleton className="h-40 w-full" />
          ) : (
            <div className="overflow-x-auto">
              <table className="w-full text-left text-xs">
                <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                  <tr>
                    <th className="py-2 pr-3">Name</th>
                    <th className="py-2 pr-3">Email</th>
                    <th className="py-2 pr-3">Role</th>
                    <th className="py-2 pr-3">Status</th>
                    <th className="py-2 text-right">Actions</th>
                  </tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {(usersQ.data ?? []).map((u) => (
                    <tr key={u.id}>
                      <td className="py-2.5 pr-3 font-semibold">{u.name}</td>
                      <td className="py-2.5 pr-3 font-mono text-slate-500">{u.email}</td>
                      <td className="py-2.5 pr-3">
                        <select
                          aria-label={`Role for ${u.name}`}
                          value={u.role}
                          onChange={(e) => roleMut.mutate({ id: u.id, role: e.target.value })}
                          className="rounded border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2 py-1 text-xs"
                        >
                          <option value="admin">Administrator</option>
                          <option value="operator">Operator</option>
                          <option value="viewer">Viewer</option>
                        </select>
                      </td>
                      <td className="py-2.5 pr-3">
                        <StatusBadge status={u.is_active ? "UP" : "DOWN"} />
                      </td>
                      <td className="py-2.5 text-right">
                        <div className="inline-flex gap-1">
                          <button type="button" onClick={() => resetMut.mutate(u.id)} title="Reset access & revoke sessions" className="rounded p-1.5 text-slate-500 hover:bg-slate-100 dark:hover:bg-navy-800">
                            <KeyRound className="h-4 w-4" />
                          </button>
                          {u.is_active && (
                            <button type="button" onClick={() => disableMut.mutate(u.id)} title="Disable account" className="rounded p-1.5 text-rose-500 hover:bg-rose-50">
                              <UserX className="h-4 w-4" />
                            </button>
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
      )}

      {tab === "integrations" && (
        <div className="space-y-4">
          {integQ.isLoading ? (
            <Skeleton className="h-56 w-full" />
          ) : integQ.data ? (
            <>
              <div className="grid sm:grid-cols-2 gap-4">
                {(
                  [
                    ["PostgreSQL Metadata Store", integQ.data.postgres.status, integQ.data.postgres.diagnostic, ""],
                    ["Prometheus TSDB", integQ.data.prometheus.status, integQ.data.prometheus.diagnostic, integQ.data.prometheus.url],
                    ["Alertmanager", integQ.data.alertmanager.status, integQ.data.alertmanager.diagnostic, integQ.data.alertmanager.url],
                    ["Grafana Visualization", integQ.data.grafana.status, integQ.data.grafana.diagnostic, integQ.data.grafana.public_url],
                  ] as const
                ).map(([title, status, diag, url]) => (
                  <Card key={title} title={title} action={<StatusBadge status={status === "healthy" ? "UP" : "UNAVAILABLE"} />}>
                    <p className="text-xs text-slate-600 dark:text-slate-300">{diag}</p>
                    {url && <p className="mt-2 font-mono text-[11px] text-slate-400">Endpoint: {url}</p>}
                  </Card>
                ))}
              </div>
              <Card title="Provisioned Grafana Dashboards" subtitle="Direct links without exposing Grafana admin credentials">
                <div className="grid sm:grid-cols-2 gap-3">
                  {(integQ.data.grafana.dashboards ?? []).map((d) => (
                    <a
                      key={d.uid}
                      href={d.url}
                      target="_blank"
                      rel="noreferrer"
                      className="flex items-center justify-between rounded-lg border border-slate-200 dark:border-slate-800 p-3 hover:border-brand-500"
                    >
                      <div>
                        <p className="text-xs font-semibold">{d.title}</p>
                        <p className="text-[11px] text-slate-500">{d.description}</p>
                      </div>
                      <ExternalLink className="h-4 w-4 text-slate-400 shrink-0" />
                    </a>
                  ))}
                </div>
              </Card>
            </>
          ) : null}
        </div>
      )}

      <Modal open={userModal} onClose={() => setUserModal(false)} title="Add Organization User">
        <form onSubmit={(e) => { e.preventDefault(); createUserMut.mutate(); }} className="space-y-4">
          <div>
            <label className="block text-xs font-medium mb-1">Full Name *</label>
            <input required value={uName} onChange={(e) => setUName(e.target.value)} className={inputClassName} />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Email *</label>
            <input required type="email" value={uEmail} onChange={(e) => setUEmail(e.target.value)} className={inputClassName} />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Initial Password * (8+ chars, upper, lower, digit)</label>
            <input required type="password" value={uPass} onChange={(e) => setUPass(e.target.value)} className={inputClassName} />
          </div>
          <div>
            <label className="block text-xs font-medium mb-1">Role *</label>
            <select value={uRole} onChange={(e) => setURole(e.target.value as UserRole)} className={inputClassName}>
              <option value="viewer">Viewer (Read-only)</option>
              <option value="operator">Operator (View + Acknowledge Alerts + Services)</option>
              <option value="admin">Administrator (Full access)</option>
            </select>
          </div>
          <div className="flex justify-end gap-2">
            <Button type="button" variant="secondary" onClick={() => setUserModal(false)}>Cancel</Button>
            <Button type="submit" disabled={createUserMut.isPending}>Create User</Button>
          </div>
        </form>
      </Modal>
    </div>
  );
};
