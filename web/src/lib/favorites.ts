import { useCallback, useEffect, useMemo } from "react";
import { useMutation, useQuery, useQueryClient, type QueryClient } from "@tanstack/react-query";
import { getJSON, sendOK, type Torrent } from "./api";

// Favourites live on the server, one set for everyone. Items left in
// localStorage by an older version are uploaded once and removed.

const STORAGE_KEY = "rt_favorites";

export type FavoriteItem = Torrent & {
  // ISO 8601, from the server.
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

// Best effort: an id that fails to upload is dropped, and the key is
// cleared after the pass so it does not run again.
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

// Module-level: the migration runs once per tab, not once per hook instance.
let migrationStarted = false;

// Snapshots the list, applies the change, returns a rollback. Synchronous:
// onMutate must return before the request starts.
function optimisticUpdate(
  qc: QueryClient,
  mutator: (cur: FavoriteItem[]) => FavoriteItem[],
) {
  const prev = qc.getQueryData<FavoriteItem[]>(["favorites"]);
  qc.setQueryData<FavoriteItem[]>(["favorites"], (cur) => mutator(cur ?? []));
  return () => qc.setQueryData(["favorites"], prev);
}

// One boolean off the shared list: a star renders only when its own
// membership flips.
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

// Mutations without a list subscription. The optimistic update is exact,
// so only an error refetches.
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

// The whole list, for the favourites view and the header's counter; a star
// uses useIsFavorite and useFavoriteToggle.
export function useFavorites() {
  const qc = useQueryClient();

  const { data } = useQuery({
    queryKey: ["favorites"],
    queryFn: apiList,
    staleTime: 30_000,
  });
  // One reference while data is undefined: a fresh [] on every render would
  // churn every callback that depends on items.
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
