// Client for the live seeders/leechers endpoint (GET /api/torrents/:id/peers).
// The server returns cache when fresh (< 24h) and otherwise scrapes rutracker;
// on a failed scrape it hands back the stale cache plus an `error` reason.

export type PeersError = "auth" | "unavailable";

// The drawer's view-model. `loading` is client-side (request in flight);
// everything else mirrors the server response.
export type PeersState = {
  // false when the server has no rutracker credentials → the UI hides the row.
  configured: boolean;
  seeders: number | null;
  leechers: number | null;
  checkedAt: string | null; // ISO of the cached value's fetch time
  error: PeersError | null; // set when the latest refresh attempt failed
  loading?: boolean;
};

type PeersJSON = {
  configured: boolean;
  seeders: number | null;
  leechers: number | null;
  checked_at: string | null;
  error?: PeersError;
};

export async function getPeers(id: number): Promise<PeersState> {
  const r = await fetch(`/api/torrents/${id}/peers`);
  if (!r.ok) throw new Error(`peers HTTP ${r.status}`);
  const j = (await r.json()) as PeersJSON;
  return {
    configured: j.configured,
    seeders: j.seeders ?? null,
    leechers: j.leechers ?? null,
    checkedAt: j.checked_at ?? null,
    error: j.error ?? null,
  };
}

// A list-cached value is "stale" once it's older than the refresh window; the
// badge greys it to signal it's not fresh (matches the drawer's grey rule).
const PEERS_TTL_MS = 24 * 60 * 60 * 1000;

export function peersStale(checkedAt: string | undefined): boolean {
  if (!checkedAt) return false;
  return Date.now() - new Date(checkedAt).getTime() > PEERS_TTL_MS;
}
