const UNITS = ["B", "KB", "MB", "GB", "TB"];

export function formatBytes(bytes: number): string {
  if (!Number.isFinite(bytes) || bytes <= 0) return "0 B";
  const i = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), UNITS.length - 1);
  const value = bytes / Math.pow(1024, i);
  const formatted = value >= 100 || i === 0 ? value.toFixed(0) : value.toFixed(1);
  return `${formatted} ${UNITS[i]}`;
}

// Accepts the API's ISO 8601 registered_at directly — the previous unix-
// timestamp signature forced both call sites to keep a private toUnixts()
// helper whose result this immediately multiplied back into a Date.
export function formatDate(iso: string): string {
  const d = new Date(iso);
  const dd = String(d.getDate()).padStart(2, "0");
  const mm = String(d.getMonth() + 1).padStart(2, "0");
  const yyyy = d.getFullYear();
  return `${dd}-${mm}-${yyyy}`;
}
