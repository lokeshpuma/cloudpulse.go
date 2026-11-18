import React, { useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Download, FileJson, FileSpreadsheet, Printer } from "lucide-react";
import { useAuth } from "@/app/AuthContext";
import { Button, Card, Skeleton, StatusBadge } from "@/components/ui/Primitives";
import { useToast } from "@/components/ui/ToastProvider";
import { api } from "@/services/api";
import { formatPercent } from "@/utils/format";

type ReportType = "infrastructure" | "resources" | "services" | "alerts";

export const ReportsPage: React.FC = () => {
  const { isDemoMode } = useAuth();
  const { notify } = useToast();

  const [reportType, setReportType] = useState<ReportType>("infrastructure");
  const [range, setRange] = useState("24h");
  const [environment, setEnvironment] = useState("");
  const [severity, setSeverity] = useState("");
  const [exporting, setExporting] = useState(false);

  const reportQ = useQuery({
    queryKey: ["report", reportType, range, environment, severity, isDemoMode],
    queryFn: () => api.getReport(reportType, { range, environment, severity }),
  });

  const handleExport = async (format: "csv" | "json" | "html") => {
    setExporting(true);
    try {
      const { blob, filename } = await api.exportReport(reportType, format, { range, environment, severity });
      const url = URL.createObjectURL(blob);
      if (format === "html") {
        window.open(url, "_blank", "noopener,noreferrer");
      } else {
        const a = document.createElement("a");
        a.href = url;
        a.download = filename;
        document.body.appendChild(a);
        a.click();
        a.remove();
      }
      window.setTimeout(() => URL.revokeObjectURL(url), 5000);
      notify({ type: "success", title: `Exported ${format.toUpperCase()} report` });
    } catch (err) {
      notify({ type: "error", title: "Export failed", description: err instanceof Error ? err.message : "" });
    } finally {
      setExporting(false);
    }
  };

  const data = reportQ.data as Record<string, unknown> | undefined;

  return (
    <div className="space-y-6">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <div>
          <h1 className="text-xl font-bold text-slate-900 dark:text-white">Reports & Analytics</h1>
          <p className="text-xs text-slate-500">Export formula-safe CSV, JSON, print-friendly HTML, or PDF reports</p>
        </div>
        <div className="flex flex-wrap gap-2 no-print">
          <Button size="sm" variant="secondary" disabled={exporting} onClick={() => void handleExport("csv")}>
            <FileSpreadsheet className="h-3.5 w-3.5" /> CSV
          </Button>
          <Button size="sm" variant="secondary" disabled={exporting} onClick={() => void handleExport("json")}>
            <FileJson className="h-3.5 w-3.5" /> JSON
          </Button>
          <Button size="sm" variant="secondary" disabled={exporting} onClick={() => void handleExport("html")}>
            <Download className="h-3.5 w-3.5" /> Printable HTML
          </Button>
          <Button size="sm" onClick={() => window.print()}>
            <Printer className="h-3.5 w-3.5" /> Print / PDF
          </Button>
        </div>
      </div>

      {/* Report Selector & Filters */}
      <Card className="no-print">
        <div className="flex flex-wrap items-center justify-between gap-3">
          <div className="flex flex-wrap gap-1.5">
            {(
              [
                ["infrastructure", "Infrastructure Health"],
                ["resources", "Resource Utilization"],
                ["services", "Service Availability"],
                ["alerts", "Alert History"],
              ] as const
            ).map(([key, label]) => (
              <Button
                key={key}
                size="sm"
                variant={reportType === key ? "primary" : "secondary"}
                onClick={() => setReportType(key)}
              >
                {label}
              </Button>
            ))}
          </div>

          <div className="flex flex-wrap gap-2 text-xs">
            <select value={range} onChange={(e) => setRange(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5">
              <option value="1h">Last 1h</option>
              <option value="24h">Last 24h</option>
              <option value="7d">Last 7 days</option>
              <option value="30d">Last 30 days</option>
            </select>
            <select value={environment} onChange={(e) => setEnvironment(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5">
              <option value="">All Environments</option>
              <option value="production">Production</option>
              <option value="staging">Staging</option>
              <option value="development">Development</option>
            </select>
            {reportType === "alerts" && (
              <select value={severity} onChange={(e) => setSeverity(e.target.value)} className="rounded-lg border border-slate-200 dark:border-slate-700 bg-white dark:bg-navy-800 px-2.5 py-1.5">
                <option value="">All Severities</option>
                <option value="critical">Critical</option>
                <option value="warning">Warning</option>
              </select>
            )}
          </div>
        </div>
      </Card>

      {/* Report Preview */}
      <Card title={`Report Preview: ${reportType.toUpperCase()}`} subtitle={`Window: ${range}`}>
        {reportQ.isLoading ? (
          <Skeleton className="h-64 w-full" />
        ) : !data ? null : (
          <div className="space-y-4 text-xs">
            {reportType === "infrastructure" && (
              <>
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Total Servers</p><p className="text-xl font-bold">{String(data.total_servers ?? 0)}</p></div>
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Up / Down</p><p className="text-xl font-bold">{String(data.up_servers ?? 0)} / {String(data.down_servers ?? 0)}</p></div>
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Availability</p><p className="text-xl font-bold">{formatPercent(data.availability_percent as number | null)}</p></div>
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Active Alerts</p><p className="text-xl font-bold">{String(data.active_alerts ?? 0)}</p></div>
                </div>
                <table className="w-full text-left">
                  <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                    <tr><th className="py-2">Server</th><th>Env</th><th>OS</th><th>CPU</th><th>Memory</th><th>Disk</th><th>Status</th></tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                    {((data.servers as Array<Record<string, unknown>>) ?? []).map((s) => (
                      <tr key={String(s.id)}>
                        <td className="py-2 font-semibold">{String(s.name)}</td>
                        <td className="capitalize">{String(s.environment)}</td>
                        <td className="uppercase">{String(s.operating_system)}</td>
                        <td className="font-mono">{formatPercent(s.cpu_percent as number | null)}</td>
                        <td className="font-mono">{formatPercent(s.memory_percent as number | null)}</td>
                        <td className="font-mono">{formatPercent(s.disk_percent as number | null)}</td>
                        <td><StatusBadge status={String(s.status)} /></td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </>
            )}

            {reportType === "resources" && (
              <table className="w-full text-left">
                <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                  <tr><th className="py-2">Server</th><th>Env</th><th>Avg CPU</th><th>Peak CPU</th><th>Avg Mem</th><th>Peak Mem</th><th>Disk</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800 font-mono">
                  {((data.rows as Array<Record<string, unknown>>) ?? []).map((r) => (
                    <tr key={String(r.server_id)}>
                      <td className="py-2 font-sans font-semibold">{String(r.server_name)}</td>
                      <td className="font-sans capitalize">{String(r.environment)}</td>
                      <td>{formatPercent(r.avg_cpu as number | null)}</td>
                      <td>{formatPercent(r.peak_cpu as number | null)}</td>
                      <td>{formatPercent(r.avg_memory as number | null)}</td>
                      <td>{formatPercent(r.peak_memory as number | null)}</td>
                      <td>{formatPercent(r.disk_utilization as number | null)}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}

            {reportType === "services" && (
              <table className="w-full text-left">
                <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                  <tr><th className="py-2">Service</th><th>Type</th><th>Server</th><th>Status</th><th>24h Uptime</th><th>Latency</th></tr>
                </thead>
                <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                  {((data.services as Array<Record<string, unknown>>) ?? []).map((s) => (
                    <tr key={String(s.id)}>
                      <td className="py-2 font-semibold">{String(s.name)}</td>
                      <td className="uppercase font-mono">{String(s.check_type)}</td>
                      <td>{String(s.server_name ?? "")}</td>
                      <td><StatusBadge status={String(s.status)} /></td>
                      <td className="font-mono">{formatPercent(s.availability_24h as number | null)}</td>
                      <td className="font-mono">{s.latency_ms !== null ? `${Number(s.latency_ms).toFixed(1)} ms` : "N/A"}</td>
                    </tr>
                  ))}
                </tbody>
              </table>
            )}

            {reportType === "alerts" && (
              <div className="space-y-4">
                <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Total</p><p className="text-xl font-bold">{String(data.total_alerts ?? 0)}</p></div>
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Critical</p><p className="text-xl font-bold text-rose-600">{String(data.critical_alerts ?? 0)}</p></div>
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Warning</p><p className="text-xl font-bold text-amber-600">{String(data.warning_alerts ?? 0)}</p></div>
                  <div className="rounded-lg bg-slate-50 dark:bg-navy-800 p-3"><p className="text-slate-500">Resolved</p><p className="text-xl font-bold text-emerald-600">{String(data.resolved_alerts ?? 0)}</p></div>
                </div>
                <table className="w-full text-left">
                  <thead className="border-b border-slate-200 dark:border-slate-800 text-slate-500 uppercase">
                    <tr><th className="py-2">Alert</th><th>Severity</th><th>Server</th><th>State</th><th>Ack</th></tr>
                  </thead>
                  <tbody className="divide-y divide-slate-100 dark:divide-slate-800">
                    {((data.alerts as Array<Record<string, unknown>>) ?? []).map((a) => (
                      <tr key={String(a.fingerprint)}>
                        <td className="py-2 font-semibold">{String(a.title)}</td>
                        <td><StatusBadge status={String(a.severity)} /></td>
                        <td>{String(a.server_name)}</td>
                        <td><StatusBadge status={String(a.state)} /></td>
                        <td>{a.acknowledged ? "Yes" : "No"}</td>
                      </tr>
                    ))}
                  </tbody>
                </table>
              </div>
            )}
          </div>
        )}
      </Card>
    </div>
  );
};
