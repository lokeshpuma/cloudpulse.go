import React, { useEffect, useState } from "react";
import { useNavigate } from "react-router-dom";
import { CheckCircle2, Copy, RefreshCw, Server as ServerIcon, Terminal } from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import { Button, Card, StatusBadge, inputClassName } from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";
import type { Server, ServerStatusCheck } from "@/types/api";
import { formatTimestamp } from "@/utils/format";

const STEPS = [
  "Organization Setup",
  "Monitoring Setup",
  "Register First Server",
  "Install Agent",
  "Verify Connection",
  "Finish Setup",
];

export const OnboardingPage: React.FC = () => {
  const { organization, refreshOrganization } = useAuth();
  const { notify } = useToast();
  const navigate = useNavigate();

  const [step, setStep] = useState(0);
  const [orgName, setOrgName] = useState(organization?.name ?? "");
  const [orgDesc, setOrgDesc] = useState(organization?.description ?? "");
  const [timezone, setTimezone] = useState(organization?.timezone ?? "UTC");
  const [units, setUnits] = useState<"binary" | "decimal">(organization?.display_units ?? "binary");

  // Step 3 server state
  const [srvName, setSrvName] = useState("app-server-01");
  const [hostname, setHostname] = useState("app-server-01.internal");
  const [env, setEnv] = useState("production");
  const [os, setOs] = useState("linux");
  const [scrapeAddr, setScrapeAddr] = useState("agent-node-1:9180");
  const [group, setGroup] = useState("core");
  const [tagsRaw, setTagsRaw] = useState("role=api,tier=backend");
  const [createdServer, setCreatedServer] = useState<Server | null>(null);
  const [statusCheck, setStatusCheck] = useState<ServerStatusCheck | null>(null);
  const [saving, setSaving] = useState(false);

  const checkStatus = async (serverId: string) => {
    try {
      const res = await api.getServerStatus(serverId);
      setStatusCheck(res);
    } catch {
      // ignore transient poll failures
    }
  };

  useEffect(() => {
    if (step !== 4 || !createdServer) return;
    void checkStatus(createdServer.id);
    const id = window.setInterval(() => void checkStatus(createdServer.id), 5000);
    return () => window.clearInterval(id);
  }, [step, createdServer]);

  const handleSaveOrg = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      await api.updateOrg({ name: orgName, description: orgDesc, timezone, display_units: units });
      await refreshOrganization();
      setStep(1);
    } catch (err) {
      notify({ type: "error", title: "Could not save organization", description: err instanceof Error ? err.message : "" });
    } finally {
      setSaving(false);
    }
  };

  const handleRegisterServer = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const tags = tagsRaw
        .split(",")
        .map((pair) => pair.trim())
        .filter(Boolean)
        .map((pair) => {
          const [key, value = "true"] = pair.split("=");
          return { key: key.trim(), value: value.trim() };
        });
      const srv = await api.createServer({
        name: srvName,
        hostname,
        environment: env,
        operating_system: os,
        server_group: group,
        scrape_address: scrapeAddr,
        monitoring_interval: 15,
        tags,
      });
      setCreatedServer(srv);
      notify({ type: "success", title: "Server registered (PENDING)", description: "Install the Go agent to begin live metric collection." });
      setStep(3);
    } catch (err) {
      notify({ type: "error", title: "Server registration failed", description: err instanceof Error ? err.message : "" });
    } finally {
      setSaving(false);
    }
  };

  const handleFinish = async () => {
    setSaving(true);
    try {
      await api.updateOrg({ onboarding_completed: true });
      await refreshOrganization();
      navigate("/");
    } finally {
      setSaving(false);
    }
  };

  const agentInstallSnippet = () => {
    const id = createdServer?.id ?? "server-001";
    const name = createdServer?.name ?? srvName;
    if (os === "windows") {
      return `# Windows (PowerShell)\n.\\cloudpulse-agent.exe --config .\\config.yaml\n# Ensure Windows Firewall allows Prometheus IP -> TCP 9180`;
    }
    if (os === "macos") {
      return `# macOS\nAGENT_ID="${id}" AGENT_NAME="${name}" AGENT_ENVIRONMENT="${env}" \\\n  ./cloudpulse-agent --config ./agent/config.example.yaml`;
    }
    return `# Linux binary / systemd\ncat <<'EOF' | sudo tee /etc/cloudpulse/config.yaml\nagent:\n  id: "${id}"\n  name: "${name}"\n  environment: "${env}"\nserver:\n  listen_address: "0.0.0.0:9180"\ncollection:\n  interval: "15s"\n  timeout: "5s"\nEOF\n\nsudo systemctl enable --now cloudpulse-agent\ncurl http://127.0.0.1:9180/health`;
  };

  const copySnippet = () => {
    void navigator.clipboard?.writeText(agentInstallSnippet());
    notify({ type: "info", title: "Copied installation commands" });
  };

  return (
    <div className="max-w-3xl mx-auto py-4 space-y-6">
      <div>
        <h1 className="text-2xl font-bold text-slate-900 dark:text-white">CloudPulse Onboarding</h1>
        <p className="text-sm text-slate-500 dark:text-slate-400">
          Complete the 6-step setup to start collecting real Prometheus metrics from your infrastructure.
        </p>
      </div>

      {/* Stepper */}
      <ol className="grid grid-cols-2 sm:grid-cols-6 gap-2">
        {STEPS.map((label, idx) => (
          <li
            key={label}
            className={`rounded-lg border px-2.5 py-2 text-xs font-medium ${
              idx === step
                ? "border-brand-500 bg-brand-50 text-brand-700 dark:bg-brand-900/30 dark:text-sky-300"
                : idx < step
                ? "border-emerald-300 bg-emerald-50 text-emerald-700 dark:border-emerald-800 dark:bg-emerald-950/30 dark:text-emerald-300"
                : "border-slate-200 dark:border-slate-800 text-slate-500"
            }`}
          >
            <span className="block text-[10px] opacity-75">Step {idx + 1}</span>
            <span className="truncate block">{label}</span>
          </li>
        ))}
      </ol>

      {step === 0 && (
        <Card title="Step 1: Organization Setup" subtitle="Configure your workspace defaults.">
          <form onSubmit={handleSaveOrg} className="space-y-4">
            <div>
              <label className="block text-xs font-medium mb-1">Organization Name</label>
              <input required value={orgName} onChange={(e) => setOrgName(e.target.value)} className={inputClassName} />
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Description</label>
              <textarea rows={2} value={orgDesc} onChange={(e) => setOrgDesc(e.target.value)} className={inputClassName} />
            </div>
            <div className="grid sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium mb-1">Time Zone</label>
                <select value={timezone} onChange={(e) => setTimezone(e.target.value)} className={inputClassName}>
                  <option value="UTC">UTC</option>
                  <option value="America/New_York">America/New_York</option>
                  <option value="America/Los_Angeles">America/Los_Angeles</option>
                  <option value="Europe/London">Europe/London</option>
                  <option value="Asia/Kolkata">Asia/Kolkata</option>
                  <option value="Asia/Tokyo">Asia/Tokyo</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Preferred Display Units</label>
                <select value={units} onChange={(e) => setUnits(e.target.value as "binary" | "decimal")} className={inputClassName}>
                  <option value="binary">Binary (KiB, MiB, GiB)</option>
                  <option value="decimal">Decimal (KB, MB, GB)</option>
                </select>
              </div>
            </div>
            <div className="flex justify-between pt-2">
              <Button type="button" variant="ghost" onClick={handleFinish}>
                Skip Onboarding
              </Button>
              <Button type="submit" disabled={saving}>
                Continue →
              </Button>
            </div>
          </form>
        </Card>
      )}

      {step === 1 && (
        <Card title="Step 2: How CloudPulse Monitoring Works" subtitle="Architecture and network path overview.">
          <div className="grid sm:grid-cols-2 gap-4 text-sm text-slate-600 dark:text-slate-300">
            <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-4">
              <h3 className="font-semibold text-slate-900 dark:text-white mb-1">1. Lightweight Go Agent</h3>
              <p className="text-xs">
                Runs on each host as <code className="font-mono">cloudpulse-agent</code>, collecting real CPU, memory, disk, network, and HTTP/TCP service metrics via gopsutil.
              </p>
            </div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-4">
              <h3 className="font-semibold text-slate-900 dark:text-white mb-1">2. Prometheus Scraping</h3>
              <p className="text-xs">
                CloudPulse atomically updates Prometheus <code className="font-mono">file_sd</code> targets whenever you register a machine. Prometheus scrapes <code className="font-mono">:9180/metrics</code>.
              </p>
            </div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-4">
              <h3 className="font-semibold text-slate-900 dark:text-white mb-1">3. Network Security</h3>
              <p className="text-xs">
                Bind agents to private interfaces or VPN tunnels (WireGuard/Tailscale). Never expose port 9180 to the public Internet without TLS and authentication.
              </p>
            </div>
            <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-4">
              <h3 className="font-semibold text-slate-900 dark:text-white mb-1">4. Grafana & Alertmanager</h3>
              <p className="text-xs">
                Alert rules evaluate in Prometheus and route through Alertmanager. Provisioned Grafana dashboards connect directly to Prometheus.
              </p>
            </div>
          </div>
          <div className="flex justify-between mt-5">
            <Button variant="secondary" onClick={() => setStep(0)}>← Back</Button>
            <Button onClick={() => setStep(2)}>Register First Server →</Button>
          </div>
        </Card>
      )}

      {step === 2 && (
        <Card title="Step 3: Register Your First Server" subtitle="Registered servers start in PENDING status until Prometheus completes a live scrape.">
          <form onSubmit={handleRegisterServer} className="space-y-4">
            <div className="grid sm:grid-cols-2 gap-4">
              <div>
                <label className="block text-xs font-medium mb-1">Server Display Name</label>
                <input required value={srvName} onChange={(e) => setSrvName(e.target.value)} className={inputClassName} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Hostname</label>
                <input required value={hostname} onChange={(e) => setHostname(e.target.value)} className={inputClassName} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Environment</label>
                <select value={env} onChange={(e) => setEnv(e.target.value)} className={inputClassName}>
                  <option value="production">Production</option>
                  <option value="staging">Staging</option>
                  <option value="development">Development</option>
                  <option value="testing">Testing</option>
                  <option value="other">Other</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Operating System</label>
                <select value={os} onChange={(e) => setOs(e.target.value)} className={inputClassName}>
                  <option value="linux">Linux</option>
                  <option value="macos">macOS</option>
                  <option value="windows">Windows</option>
                </select>
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Prometheus Scrape Address (host:port)</label>
                <input required value={scrapeAddr} onChange={(e) => setScrapeAddr(e.target.value)} placeholder="10.0.0.15:9180" className={inputClassName} />
              </div>
              <div>
                <label className="block text-xs font-medium mb-1">Server Group</label>
                <input required value={group} onChange={(e) => setGroup(e.target.value)} className={inputClassName} />
              </div>
            </div>
            <div>
              <label className="block text-xs font-medium mb-1">Optional Tags (key=value, comma-separated)</label>
              <input value={tagsRaw} onChange={(e) => setTagsRaw(e.target.value)} className={inputClassName} />
            </div>
            <div className="flex justify-between pt-2">
              <Button type="button" variant="secondary" onClick={() => setStep(1)}>← Back</Button>
              <Button type="submit" disabled={saving}>
                {saving ? "Registering..." : "Register Server →"}
              </Button>
            </div>
          </form>
        </Card>
      )}

      {step === 3 && (
        <Card
          title="Step 4: Install CloudPulse Go Agent"
          subtitle={`Generated setup instructions for ${os.toUpperCase()} (${createdServer?.name ?? srvName}).`}
          action={
            <Button size="sm" variant="secondary" onClick={copySnippet}>
              <Copy className="h-3.5 w-3.5" /> Copy
            </Button>
          }
        >
          <div className="rounded-lg bg-navy-900 p-4 text-xs font-mono text-sky-200 overflow-x-auto whitespace-pre">
            {agentInstallSnippet()}
          </div>
          <p className="mt-3 text-xs text-slate-500 dark:text-slate-400 flex items-center gap-1.5">
            <Terminal className="h-3.5 w-3.5" />
            Running locally via Docker Compose? Start built-in test agents with{" "}
            <code className="font-mono">docker compose --profile agents up -d</code>.
          </p>
          <div className="flex justify-between mt-5">
            <Button variant="secondary" onClick={() => setStep(2)}>← Back</Button>
            <Button onClick={() => setStep(4)}>Verify Connection →</Button>
          </div>
        </Card>
      )}

      {step === 4 && (
        <Card
          title="Step 5: Verify Live Prometheus Connection"
          subtitle="A server is marked connected only after Prometheus completes a real scrape."
          action={
            createdServer && (
              <Button size="sm" variant="secondary" onClick={() => void checkStatus(createdServer.id)}>
                <RefreshCw className="h-3.5 w-3.5" /> Check Now
              </Button>
            )
          }
        >
          <div className="space-y-4">
            <div className="grid sm:grid-cols-2 gap-3 text-sm">
              <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-3">
                <p className="text-xs text-slate-500">Connection Status</p>
                <div className="mt-1">
                  <StatusBadge status={statusCheck?.status ?? "PENDING"} isStale={statusCheck?.is_stale} />
                </div>
              </div>
              <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-3">
                <p className="text-xs text-slate-500">Hostname & Scrape Endpoint</p>
                <p className="mt-1 font-mono text-xs">{statusCheck?.hostname ?? hostname} ({statusCheck?.scrape_address ?? scrapeAddr})</p>
              </div>
              <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-3">
                <p className="text-xs text-slate-500">Last Successful Scrape</p>
                <p className="mt-1 text-xs">{formatTimestamp(statusCheck?.last_successful_scrape, timezone)}</p>
              </div>
              <div className="rounded-lg border border-slate-200 dark:border-slate-800 p-3">
                <p className="text-xs text-slate-500">Collected Metrics</p>
                <p className="mt-1 text-xs">
                  {statusCheck?.collected_metrics?.length ? statusCheck.collected_metrics.join(", ") : "Awaiting first scrape..."}
                </p>
              </div>
            </div>
            <div className="flex justify-between pt-2">
              <Button variant="secondary" onClick={() => setStep(3)}>← Back</Button>
              <Button onClick={() => setStep(5)}>
                {statusCheck?.connected ? "Connection Verified — Continue →" : "Continue to Finish →"}
              </Button>
            </div>
          </div>
        </Card>
      )}

      {step === 5 && (
        <Card title="Step 6: Setup Complete">
          <div className="flex flex-col items-center text-center py-6 space-y-3">
            <CheckCircle2 className="h-12 w-12 text-emerald-500" />
            <h2 className="text-lg font-semibold">CloudPulse is ready</h2>
            <p className="text-xs text-slate-500 max-w-md">
              You can register additional servers, configure service checks, customize alert rules, and explore live Prometheus metrics from the dashboard.
            </p>
            <Button size="lg" onClick={handleFinish} disabled={saving}>
              <ServerIcon className="h-4 w-4" /> Open Main Dashboard
            </Button>
          </div>
        </Card>
      )}
    </div>
  );
};
