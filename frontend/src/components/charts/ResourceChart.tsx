import React, { useState } from "react";
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
import { Card, EmptyState, Skeleton } from "@/components/ui/Primitives";
import type { TimeRangeKey, TimeseriesPoint } from "@/types/api";
import { formatShortTime } from "@/utils/format";

interface ResourceChartProps {
  points: TimeseriesPoint[];
  loading: boolean;
  prometheusAvailable: boolean;
  timeRange: TimeRangeKey;
  onRangeChange: (r: TimeRangeKey) => void;
  timezone?: string;
}

const RANGES: TimeRangeKey[] = ["15m", "30m", "1h", "6h", "24h", "7d", "30d"];

export const ResourceChart: React.FC<ResourceChartProps> = ({
  points,
  loading,
  prometheusAvailable,
  timeRange,
  onRangeChange,
  timezone = "UTC",
}) => {
  const [showCpu, setShowCpu] = useState(true);
  const [showMem, setShowMem] = useState(true);
  const [showDisk, setShowDisk] = useState(true);

  const chartData = points.map((p) => ({
    time: formatShortTime(p.timestamp, timezone),
    rawTimestamp: p.timestamp,
    cpu: p.cpu_percent,
    memory: p.memory_percent,
    disk: p.disk_percent,
  }));

  return (
    <Card
      title="Resource Usage Timeline"
      subtitle="Live fleet CPU, Memory, and Disk utilization queried from Prometheus"
      action={
        <div className="flex flex-wrap items-center gap-2">
          <div className="flex items-center gap-1 rounded-lg bg-slate-100 dark:bg-navy-800 p-0.5">
            {RANGES.map((r) => (
              <button
                key={r}
                type="button"
                onClick={() => onRangeChange(r)}
                className={`rounded-md px-2 py-1 text-xs font-medium transition-colors ${
                  timeRange === r
                    ? "bg-white dark:bg-navy-700 text-brand-600 dark:text-sky-300 shadow-xs"
                    : "text-slate-600 dark:text-slate-400 hover:text-slate-900"
                }`}
              >
                {r}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-2 text-xs">
            <label className="inline-flex items-center gap-1 cursor-pointer">
              <input type="checkbox" checked={showCpu} onChange={(e) => setShowCpu(e.target.checked)} />
              <span className="text-sky-600 dark:text-sky-400 font-medium">CPU</span>
            </label>
            <label className="inline-flex items-center gap-1 cursor-pointer">
              <input type="checkbox" checked={showMem} onChange={(e) => setShowMem(e.target.checked)} />
              <span className="text-indigo-600 dark:text-indigo-400 font-medium">Memory</span>
            </label>
            <label className="inline-flex items-center gap-1 cursor-pointer">
              <input type="checkbox" checked={showDisk} onChange={(e) => setShowDisk(e.target.checked)} />
              <span className="text-amber-600 dark:text-amber-400 font-medium">Disk</span>
            </label>
          </div>
        </div>
      }
    >
      {loading ? (
        <Skeleton className="h-72 w-full" />
      ) : !prometheusAvailable ? (
        <EmptyState
          title="Prometheus time-series unavailable"
          description="CloudPulse cannot reach Prometheus right now. Missing data is never replaced with fabricated values."
        />
      ) : chartData.length === 0 ? (
        <EmptyState
          title="No time-series samples in this window"
          description="Register and start a CloudPulse agent so Prometheus begins scraping host metrics."
        />
      ) : (
        <div className="h-72 w-full" role="img" aria-label="Time-series chart of CPU, memory, and disk utilization">
          <ResponsiveContainer width="100%" height="100%">
            <LineChart data={chartData} margin={{ top: 8, right: 16, left: 0, bottom: 0 }}>
              <CartesianGrid strokeDasharray="3 3" stroke="#334155" opacity={0.2} />
              <XAxis dataKey="time" tick={{ fontSize: 11 }} stroke="#94a3b8" />
              <YAxis domain={[0, 100]} unit="%" tick={{ fontSize: 11 }} stroke="#94a3b8" />
              <Tooltip
                contentStyle={{
                  backgroundColor: "#0f172a",
                  border: "1px solid #334155",
                  borderRadius: "8px",
                  fontSize: "12px",
                  color: "#f8fafc",
                }}
              />
              <Legend wrapperStyle={{ fontSize: "12px" }} />
              {showCpu && (
                <Line
                  type="monotone"
                  dataKey="cpu"
                  name="CPU %"
                  stroke="#0284c7"
                  strokeWidth={2}
                  dot={false}
                  connectNulls={false}
                />
              )}
              {showMem && (
                <Line
                  type="monotone"
                  dataKey="memory"
                  name="Memory %"
                  stroke="#6366f1"
                  strokeWidth={2}
                  dot={false}
                  connectNulls={false}
                />
              )}
              {showDisk && (
                <Line
                  type="monotone"
                  dataKey="disk"
                  name="Disk %"
                  stroke="#f59e0b"
                  strokeWidth={2}
                  dot={false}
                  connectNulls={false}
                />
              )}
            </LineChart>
          </ResponsiveContainer>
        </div>
      )}
    </Card>
  );
};
