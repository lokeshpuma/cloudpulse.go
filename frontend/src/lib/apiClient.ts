import type { ApiEnvelope } from "@/types/api";

// Centralized API client configuration (PRD Section 22.1 & 6.2)
// SECURITY: Access tokens are held ONLY in memory — never written to localStorage or sessionStorage.

const RAW_API_BASE = (import.meta.env.VITE_API_BASE_URL as string | undefined) ?? "";
export const API_BASE_URL = RAW_API_BASE.replace(/\/+$/, "");

export const ENV_DEMO_MODE =
  String(import.meta.env.VITE_DEMO_MODE ?? "false").toLowerCase() === "true";

let inMemoryAccessToken: string | null = null;
let inMemoryCsrfToken: string | null = null;

export function setAccessToken(token: string | null) {
  inMemoryAccessToken = token;
}

export function getAccessToken(): string | null {
  return inMemoryAccessToken;
}

export function setCsrfToken(token: string | null) {
  inMemoryCsrfToken = token;
}

export function readCsrfCookie(): string | null {
  if (inMemoryCsrfToken) return inMemoryCsrfToken;
  if (typeof document === "undefined") return null;
  const match = document.cookie
    .split("; ")
    .find((row) => row.startsWith("cloudpulse_csrf="));
  return match ? decodeURIComponent(match.split("=")[1] ?? "") : null;
}

export class ApiError extends Error {
  status: number;
  code: string;

  constructor(status: number, code: string, message: string) {
    super(message);
    this.status = status;
    this.code = code;
  }
}

interface RequestOptions {
  method?: string;
  body?: unknown;
  includeCsrf?: boolean;
  skipAuth?: boolean;
  signal?: AbortSignal;
}

export async function apiRequest<T>(path: string, opts: RequestOptions = {}): Promise<T> {
  const url = `${API_BASE_URL}${path}`;
  const headers: Record<string, string> = {};

  if (opts.body !== undefined) {
    headers["Content-Type"] = "application/json";
  }
  if (!opts.skipAuth && inMemoryAccessToken) {
    headers["Authorization"] = `Bearer ${inMemoryAccessToken}`;
  }
  if (opts.includeCsrf) {
    const csrf = readCsrfCookie();
    if (csrf) {
      headers["X-CSRF-Token"] = csrf;
    }
  }

  let resp: Response;
  try {
    resp = await fetch(url, {
      method: opts.method ?? "GET",
      headers,
      body: opts.body !== undefined ? JSON.stringify(opts.body) : undefined,
      credentials: "include",
      signal: opts.signal,
    });
  } catch {
    throw new ApiError(0, "NETWORK_ERROR", "Unable to reach CloudPulse API. Verify the backend server is running.");
  }

  const text = await resp.text();
  let parsed: ApiEnvelope<T> | null = null;
  if (text) {
    try {
      parsed = JSON.parse(text) as ApiEnvelope<T>;
    } catch {
      parsed = null;
    }
  }

  if (!resp.ok || (parsed && !parsed.success)) {
    const code = parsed?.error?.code ?? `HTTP_${resp.status}`;
    const message = parsed?.error?.message ?? `Request failed with status ${resp.status}`;
    throw new ApiError(resp.status, code, message);
  }

  return (parsed ? parsed.data : undefined) as T;
}

export async function apiDownloadBlob(path: string): Promise<{ blob: Blob; filename: string }> {
  const headers: Record<string, string> = {};
  if (inMemoryAccessToken) {
    headers["Authorization"] = `Bearer ${inMemoryAccessToken}`;
  }
  const resp = await fetch(`${API_BASE_URL}${path}`, {
    method: "GET",
    headers,
    credentials: "include",
  });
  if (!resp.ok) {
    throw new ApiError(resp.status, "EXPORT_FAILED", `Export failed (${resp.status})`);
  }
  const disp = resp.headers.get("Content-Disposition") ?? "";
  const match = disp.match(/filename="([^"]+)"/);
  const filename = match ? match[1] : "cloudpulse-report";
  return { blob: await resp.blob(), filename };
}
