export function formatBytes(
  bytes: number | null | undefined,
  units: "binary" | "decimal" = "binary"
): string {
  if (bytes === null || bytes === undefined || Number.isNaN(bytes)) return "Unavailable";
  if (bytes === 0) return "0 B";
  const base = units === "binary" ? 1024 : 1000;
  const labels =
    units === "binary"
      ? ["B", "KiB", "MiB", "GiB", "TiB", "PiB"]
      : ["B", "KB", "MB", "GB", "TB", "PB"];
  const idx = Math.min(Math.floor(Math.log(Math.abs(bytes)) / Math.log(base)), labels.length - 1);
  const value = bytes / Math.pow(base, idx);
  return `${value.toFixed(idx === 0 ? 0 : 1)} ${labels[idx]}`;
}

export function formatBytesPerSec(
  bps: number | null | undefined,
  units: "binary" | "decimal" = "binary"
): string {
  if (bps === null || bps === undefined || Number.isNaN(bps)) return "Unavailable";
  return `${formatBytes(bps, units)}/s`;
}

export function formatPercent(v: number | null | undefined): string {
  if (v === null || v === undefined || Number.isNaN(v)) return "N/A";
  return `${v.toFixed(1)}%`;
}

export function formatTimestamp(
  iso: string | null | undefined,
  timezone = "UTC",
  timeFormat: "12h" | "24h" = "24h"
): string {
  if (!iso) return "Never";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "Invalid date";
  try {
    return new Intl.DateTimeFormat("en-US", {
      timeZone: timezone || "UTC",
      year: "numeric",
      month: "short",
      day: "2-digit",
      hour: "2-digit",
      minute: "2-digit",
      second: "2-digit",
      hour12: timeFormat === "12h",
    }).format(date);
  } catch {
    return date.toISOString();
  }
}

export function formatShortTime(iso: string, timezone = "UTC"): string {
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return iso;
  try {
    return new Intl.DateTimeFormat("en-US", {
      timeZone: timezone || "UTC",
      hour: "2-digit",
      minute: "2-digit",
      hour12: false,
    }).format(date);
  } catch {
    return date.toISOString().slice(11, 16);
  }
}

export function formatRelativeTime(iso: string | null | undefined): string {
  if (!iso) return "Never";
  const date = new Date(iso);
  if (Number.isNaN(date.getTime())) return "Unknown";
  const diffSec = Math.max(0, Math.floor((Date.now() - date.getTime()) / 1000));
  if (diffSec < 15) return "Just now";
  if (diffSec < 60) return `${diffSec}s ago`;
  if (diffSec < 3600) return `${Math.floor(diffSec / 60)}m ago`;
  if (diffSec < 86400) return `${Math.floor(diffSec / 3600)}h ago`;
  return `${Math.floor(diffSec / 86400)}d ago`;
}

export function cn(...classes: Array<string | false | null | undefined>): string {
  return classes.filter(Boolean).join(" ");
}
