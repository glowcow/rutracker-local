import { getJSON, send } from "./api";

// Types and calls of the in-app dump loader. Reading is open; starting a
// parse needs the bearer token. Types mirror the backend's JSON.

export type DumpFile = {
  name: string;
  size_bytes: number;
  mtime: string; // RFC3339
};

export type ParseStatus = "idle" | "running" | "succeeded" | "failed";

// Progress is byte-based: pct/eta come from bytes read (the parser doesn't know
// the total row count up front); rows is a running count with no denominator.
export type Progress = {
  pct: number;
  rows: number;
  read_mb: number;
  total_mb: number;
  rate_rows_s: number;
  eta_s: number;
  elapsed_s: number;
};

// LogLine is a UI-side view derived from a kind=log SSE event.
export type LogLine = {
  id: number;
  ts?: string; // HH:MM:SS
  level: "info" | "warn" | "error";
  msg: string;
  detail?: string;
};

export type RunSummary = {
  id: number;
  started_at: string;
  finished_at: string | null;
  source: string;
  rows: number;
  swept: number;
  sweep: boolean;
  status: "running" | "succeeded" | "failed";
  duration_s: number;
};

// SSEvent mirrors internal/ingest.Event — one item on /parse/stream.
export type SSEvent = {
  kind: "status" | "log" | "progress";
  ts: string;
  status?: string;
  run_id?: number;
  level?: "info" | "warn" | "error";
  msg?: string;
  detail?: string;
  progress?: Progress;
};

// ── Token (shared-secret, LAN-only). Per-browser in localStorage; sent as
// `Authorization: Bearer` on the mutating call. Verified server-side. ──
const TOKEN_KEY = "rt_admin_token";
export const getToken = (): string => localStorage.getItem(TOKEN_KEY) ?? "";
export const setToken = (v: string): void => localStorage.setItem(TOKEN_KEY, v);
export const clearToken = (): void => localStorage.removeItem(TOKEN_KEY);

// ── Read API (open) ──
export const listDumps = () => getJSON<{ items: DumpFile[] }>("/api/admin/dumps");

export const getRecentRuns = (limit = 3) =>
  getJSON<{ items: RunSummary[] }>(`/api/admin/parse/runs?limit=${limit}`);

export type StatusResp = { running: boolean; run: RunSummary | null };
export const getStatus = () => getJSON<StatusResp>("/api/admin/parse/status");

// ── Write API (bearer token) ──
export type StartOpts = { source: string; batch_size: number; sweep: boolean };

// Resolves on 202; otherwise throws Error(code): "conflict", "unauthorized",
// "disabled" or "http" — the panel maps the code to a message.
export async function startParse(opts: StartOpts, token: string): Promise<void> {
  const r = await send("/api/admin/parse", {
    method: "POST",
    headers: { "Content-Type": "application/json", Authorization: `Bearer ${token}` },
    body: JSON.stringify(opts),
  });
  if (r.status === 202) return;
  const code =
    r.status === 409 ? "conflict" :
    r.status === 401 ? "unauthorized" :
    r.status === 503 ? "disabled" :
    "http";
  throw new Error(code);
}

// fmtDur renders seconds as m:ss (or h:mm:ss past an hour).
export function fmtDur(s: number): string {
  s = Math.max(0, Math.floor(s));
  const h = Math.floor(s / 3600);
  const m = Math.floor((s % 3600) / 60);
  const sec = s % 60;
  const mm = h > 0 ? String(m).padStart(2, "0") : String(m);
  const ss = String(sec).padStart(2, "0");
  return h > 0 ? `${h}:${mm}:${ss}` : `${mm}:${ss}`;
}

// hhmmss pulls HH:MM:SS out of an RFC3339 timestamp for compact log display.
export function hhmmss(rfc3339: string): string {
  return rfc3339.length >= 19 ? rfc3339.slice(11, 19) : rfc3339;
}
