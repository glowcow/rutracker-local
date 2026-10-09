// The only fetch calls of the app: wrappers over the Go backend at /api/*.

export type Torrent = {
  id: number;
  title: string;
  forum_id: number;
  forum_name: string;
  size_bytes: number;
  registered_at: string; // ISO 8601
  hash: string;
  // The cached peer counts, joined on list reads; absent for a torrent that
  // was never checked.
  seeders?: number;
  leechers?: number;
  peers_checked_at?: string; // ISO 8601
};

export type TorrentDetail = Torrent & {
  content_html: string;
  // Size of the dump's file listing, so the drawer can label its collapsed
  // Files section without fetching the tree. Absent when the dump had none.
  files_count?: number;
};

export type SearchResponse = {
  items: Torrent[];
  total: number;
};

export type Stats = {
  torrents_total: number;
  total_size_bytes: number;
  forums_count: number;
  // How many torrents have a cached seeders/leechers snapshot. Grows as
  // browsing populates the peer cache; recomputed fresh server-side.
  peers_cached: number;
  // Server-side feature gate (RT_PEERS_ENABLED); off → the UI hides the
  // seeders/leechers layer entirely instead of showing frozen cache.
  peers_enabled: boolean;
  // ISO 8601 finish time of the last successful dump ingest. Absent until a
  // first sweep completes — the stats ribbon then drops its "@ date" stamp.
  dump_updated_at?: string;
};

export type Forum = {
  id: number;
  name: string;
  count: number;
};

export type SearchParams = {
  q?: string;
  forum_id?: number;
  sort?: "relevance" | "date" | "size";
  dir?: "asc" | "desc";
  offset?: number;
  limit?: number;
};

// A non-2xx throws the path and status, plus the body's `error` when it has one.
async function fail(path: string, res: Response): Promise<never> {
  let detail = "";
  try {
    const body: unknown = await res.json();
    if (body && typeof body === "object" && "error" in body && typeof body.error === "string") {
      detail = ` (${body.error})`;
    }
  } catch {
    // The body wasn't JSON.
  }
  throw new Error(`${path}: ${res.status}${detail}`);
}

/** The bare request, for a caller that reads the status itself. */
export function send(path: string, init?: RequestInit): Promise<Response> {
  return fetch(path, { ...init, headers: { Accept: "application/json", ...init?.headers } });
}

/** A mutation: any non-2xx throws. */
export async function sendOK(path: string, method: "POST" | "DELETE"): Promise<Response> {
  const res = await send(path, { method });
  if (!res.ok) await fail(path, res);
  return res;
}

export async function getJSON<T>(path: string): Promise<T> {
  const res = await send(path);
  if (!res.ok) await fail(path, res);
  return res.json() as Promise<T>;
}

/** null on 404: the endpoint's feature or record is absent. */
export async function getOptionalJSON<T>(path: string): Promise<T | null> {
  const res = await send(path);
  if (res.status === 404) return null;
  if (!res.ok) await fail(path, res);
  return res.json() as Promise<T>;
}

export function getStats() {
  return getJSON<Stats>("/api/stats");
}

export function getForums() {
  return getJSON<{ items: Forum[] }>("/api/forums");
}

export function getTorrent(id: number) {
  return getJSON<TorrentDetail>(`/api/torrents/${id}`);
}

// The dump's file list as [path, size] pairs. files_count is the real total:
// when truncated only the first 1000 are present. A 404 means no list.
export type TorrentFiles = {
  files_count: number;
  truncated: boolean;
  files: [string, number][];
};

export const getTorrentFiles = (id: number) =>
  getOptionalJSON<TorrentFiles>(`/api/torrents/${id}/files`);

// Transmission feature state: configured (endpoint set → render the button)
// and online (live probe → active vs greyed-out). Both false when off.
export type TransmissionStatus = { configured: boolean; online: boolean };

export function getTransmissionStatus() {
  return getJSON<TransmissionStatus>("/api/transmission/status");
}

// "added" = queued now, "duplicate" = the daemon already had it; a failed
// request throws.
export type DownloadResult = { status: "added" | "duplicate"; name?: string };

export async function sendToTransmission(id: number): Promise<DownloadResult> {
  const res = await sendOK(`/api/torrents/${id}/download`, "POST");
  return res.json() as Promise<DownloadResult>;
}

export function searchTorrents(p: SearchParams) {
  const qs = new URLSearchParams();
  if (p.q) qs.set("q", p.q);
  if (p.forum_id) qs.set("forum_id", String(p.forum_id));
  if (p.sort) qs.set("sort", p.sort);
  if (p.dir) qs.set("dir", p.dir);
  if (p.offset !== undefined) qs.set("offset", String(p.offset));
  if (p.limit !== undefined) qs.set("limit", String(p.limit));
  const url = qs.toString() ? `/api/search?${qs}` : "/api/search";
  return getJSON<SearchResponse>(url);
}
