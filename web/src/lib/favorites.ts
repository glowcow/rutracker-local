import { useCallback, useEffect, useMemo } from "react";
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { getJSON, sendOK, type Torrent } from "./api";

// Server-side global favorites (shared service, no per-user accounts → one set).
// On first mount we migrate any leftover localStorage items up to the server and
// clear the local copy so a future browser change can't resurrect old picks.

const STORAGE_KEY = "rt_favorites";

export type FavoriteItem = Torrent & {
  // ISO 8601 from the server (added_at). The legacy localStorage shape stored
  // an `addedAt` epoch number — kept here as a separate optional field for
  // the one-shot migration path.
  added_at: string;
};

type ListResponse = { items: FavoriteItem[] };

async function apiList(): Promise<FavoriteItem[]> {
  const body = await getJSON<ListResponse>("/api/favorites");
  return body.items ?? [];
}

async function apiAdd(id: number): Promise<void> {
  await sendOK(`/api/favorites/${id}`, "POST");
}

async function apiRemove(id: number): Promise<void> {
  await sendOK(`/api/favorites/${id}`, "DELETE");
}

async function apiClear(): Promise<void> {
  await sendOK("/api/favorites", "DELETE");
}

// One-shot migration of legacy localStorage favorites. Best-effort: any
// per-id POST that fails (network blip, swept torrent → FK violation) is
// dropped silently so the user isn't blocked by a half-broken upload. After
// a successful sweep across the list we clear the key so we don't try again.
async function migrateLegacyLocal(): Promise<boolean> {
  try {
    const raw = localStorage.getItem(STORAGE_KEY);
    if (!raw) return false;
    const parsed = JSON.parse(raw);
    if (!Array.isArray(parsed) || parsed.length === 0) {
      localStorage.removeItem(STORAGE_KEY);
      return false;
    }
    await Promise.all(
      parsed
        .filter((x: unknown): x is { id: number } => {
          return (
            !!x && typeof (x as { id?: unknown }).id === "number"
          );
        })
        .map((x) => apiAdd(x.id).catch(() => undefined)),
    );
    localStorage.removeItem(STORAGE_KEY);
    return true;
  } catch {
    return false;
  }
}

// Module-level so the migration really runs once per tab. A useRef guard
// (the previous shape) is per-hook-instance — Header, App and every star
// mount their own copy, and on startup several of them raced the localStorage
// read before any had removed the key, double-POSTing every legacy id.
let migrationStarted = false;

// Optimistic-update helper: snapshot the list, apply the mutator locally, return
// a rollback fn. Stays synchronous — TanStack onMutate runs before the mutation
// fn and must return without awaiting; the network call is the caller's job.
function optimisticUpdate(
  qc: QueryClient,
  mutator: (cur: FavoriteItem[]) => FavoriteItem[],
) {
  const prev = qc.getQueryData<FavoriteItem[]>(["favorites"]);
  qc.setQueryData<FavoriteItem[]>(["favorites"], (cur) => mutator(cur ?? []));
  return () => qc.setQueryData(["favorites"], prev);
}

// Membership-only subscription for the star buttons. `select` narrows the
// shared ["favorites"] cache entry down to one boolean, so a star re-renders
// only when *its* membership flips — not 25 cards on every list change.
export function useIsFavorite(id: number): boolean {
  const { data } = useQuery({
    queryKey: ["favorites"],
    queryFn: apiList,
    staleTime: 30_000,
    select: useCallback(
      (items: FavoriteItem[]) => items.some((x) => x.id === id),
      [id],
    ),
  });
  return data ?? false;
}

// Add/remove mutations without a list subscription — membership is read from
// the query cache at call time. The optimistic update is exact, so success
// doesn't re-fetch; only an error invalidates to re-sync with server truth
// after the rollback.
export function useFavoriteToggle() {
  const qc = useQueryClient();

  const addMut = useMutation({
    mutationFn: (t: Torrent) => apiAdd(t.id),
    onMutate: async (t) => {
      await qc.cancelQueries({ queryKey: ["favorites"] });
      const rollback = optimisticUpdate(qc, (cur) => {
        if (cur.some((x) => x.id === t.id)) return cur;
        const stamp = new Date().toISOString();
        return [{ ...t, added_at: stamp }, ...cur];
      });
      return { rollback };
    },
    onError: (_e, _t, ctx) => {
      ctx?.rollback?.();
      qc.invalidateQueries({ queryKey: ["favorites"] });
    },
  });

  const removeMut = useMutation({
    mutationFn: (id: number) => apiRemove(id),
    onMutate: async (id) => {
      await qc.cancelQueries({ queryKey: ["favorites"] });
      const rollback = optimisticUpdate(qc, (cur) =>
        cur.filter((x) => x.id !== id),
      );
      return { rollback };
    },
    onError: (_e, _id, ctx) => {
      ctx?.rollback?.();
      qc.invalidateQueries({ queryKey: ["favorites"] });
    },
  });

  return useCallback(
    (t: Torrent) => {
      const cur = qc.getQueryData<FavoriteItem[]>(["favorites"]) ?? [];
      if (cur.some((x) => x.id === t.id)) removeMut.mutate(t.id);
      else addMut.mutate(t);
    },
    [qc, addMut, removeMut],
  );
}

// Full-list hook for App (favorites view) and Header (counter). Star buttons
// should use useIsFavorite/useFavoriteToggle instead — they don't need the
// whole list re-rendering them.
export function useFavorites() {
  const qc = useQueryClient();

  const { data } = useQuery({
    queryKey: ["favorites"],
    queryFn: apiList,
    staleTime: 30_000,
  });
  // Stable identity per data reference — a bare `data ?? []` returns a fresh
  // empty array on every render while data is undefined, which makes every
  // downstream useCallback that depends on `items` (isFavorite) churn its
  // identity too. useMemo pins it to the same reference until data flips.
  const items: FavoriteItem[] = useMemo(() => data ?? [], [data]);

  useEffect(() => {
    if (migrationStarted) return;
    migrationStarted = true;
    migrateLegacyLocal().then((did) => {
      if (did) qc.invalidateQueries({ queryKey: ["favorites"] });
    });
  }, [qc]);

  const clearMut = useMutation({
    mutationFn: () => apiClear(),
    onMutate: async () => {
      await qc.cancelQueries({ queryKey: ["favorites"] });
      const rollback = optimisticUpdate(qc, () => []);
      return { rollback };
    },
    onError: (_e, _v, ctx) => {
      ctx?.rollback?.();
      qc.invalidateQueries({ queryKey: ["favorites"] });
    },
  });

  const isFavorite = useCallback(
    (id: number) => items.some((x) => x.id === id),
    [items],
  );

  const clear = useCallback(() => clearMut.mutate(), [clearMut]);

  return { items, isFavorite, clear, count: items.length };
}
