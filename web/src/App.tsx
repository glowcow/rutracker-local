import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import { Search, Star, Trash2 } from "lucide-react";
import { Header } from "./components/Header";
import { ForumStrip, STRIP_OPEN_KEY } from "./components/ForumStrip";
import { FilterChips, type Chip } from "./components/FilterChips";
import { SortControl, type Sort } from "./components/SortControl";
import { ResultCard } from "./components/ResultCard";
import { DetailDrawer } from "./components/DetailDrawer";
import { AdminPanel } from "./components/AdminPanel";
import { ConfirmDialog } from "./components/ConfirmDialog";
import { Pagination } from "./components/Pagination";
import { Footer } from "./components/Footer";
import { getForums, getStats, searchTorrents } from "./lib/api";
import { useDebounced } from "./lib/debounce";
import { useLang } from "./lib/i18n";
import { useFavorites } from "./lib/favorites";
import { cn } from "./lib/cn";

const PAGE_SIZE = 25;

// Hero line: one emoji per kind of catalogue content.
const HERO_EMOJI = ["🎵", "🎬", "📺", "📚", "🎓", "🎮", "💾", "⚽"];

/** Leaf segment of a " - "-joined breadcrumb. */
function leafName(name: string): string {
  const parts = name.split(" - ");
  return parts[parts.length - 1].trim();
}

function App() {
  const { t, p, locale } = useLang();
  const { items: favItems, count: favCount, clear: clearFavs } = useFavorites();
  const [query, setQuery] = useState("");
  const debouncedQuery = useDebounced(query, 300);
  // The last viewed id outlives the close, so the drawer keeps its content
  // through the closing transition; drawerOpen alone shows it.
  const [selectedId, setSelectedId] = useState<number | null>(null);
  const [drawerOpen, setDrawerOpen] = useState(false);
  const openTorrent = useCallback((id: number) => {
    setSelectedId(id);
    setDrawerOpen(true);
  }, []);
  const closeDrawer = useCallback(() => setDrawerOpen(false), []);
  const [sort, setSortState] = useState<Sort>({ key: "relevance", dir: "desc" });
  const [forumId, setForumIdState] = useState<number | undefined>(undefined);
  const [page, setPage] = useState(0);
  // The results show the starred torrents instead of a search. Exclusive with
  // the query and the forum filter: setting one clears the other.
  const [favOnly, setFavOnly] = useState(false);
  // Bumped to remount ForumStrip, which folds its open sections.
  const [forumsResetKey, setForumsResetKey] = useState(0);
  const [adminOpen, setAdminOpen] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const closeConfirm = useCallback(() => setConfirmClear(false), []);

  // A filter change resets the page, inside the setter — an effect may not
  // set state. Stable callbacks: the memoised children depend on them.
  const setQueryReset = useCallback((v: string) => {
    setQuery(v);
    setPage(0);
    if (v !== "") setFavOnly(false);
  }, []);
  const setSort = useCallback((s: Sort) => {
    setSortState(s);
    setPage(0);
  }, []);
  const setForumId = useCallback((id: number | undefined) => {
    setForumIdState(id);
    setPage(0);
    if (id !== undefined) setFavOnly(false);
  }, []);
  const toggleFavOnly = () => {
    // Side effects stay outside the updater: React may run an updater twice.
    const next = !favOnly;
    if (next) {
      // Favourites are an exclusive view: drop the search and the forum filter.
      setQuery("");
      setForumIdState(undefined);
      setPage(0);
      // ForumStrip reads this key on mount: write it before the remount below.
      localStorage.setItem(STRIP_OPEN_KEY, "false");
      setForumsResetKey((k) => k + 1);
    }
    setFavOnly(next);
  };

  // Don't run an unbounded SELECT on mount — wait until the user types or picks
  // a forum, else every page load scans ~10M rows for "newest 50".
  const hasFilter = debouncedQuery !== "" || forumId !== undefined;
  // favOnly decides the view: debouncedQuery lags the cleared query by 300 ms
  // and would bring the abandoned search back for a moment.
  const isFavView = favOnly;

  const { data, isFetching, error } = useQuery({
    queryKey: ["search", debouncedQuery, forumId, sort.key, sort.dir, page],
    queryFn: () =>
      searchTorrents({
        q: debouncedQuery || undefined,
        forum_id: forumId,
        sort: sort.key,
        dir: sort.dir,
        offset: page * PAGE_SIZE,
        limit: PAGE_SIZE,
      }),
    placeholderData: keepPreviousData,
    enabled: hasFilter && !isFavView,
  });

  // Peers feature gate (RT_PEERS_ENABLED). Shared with ForumStrip via the
  // same key — no extra request; off → cards drop their peer badge.
  const { data: stats } = useQuery({
    queryKey: ["stats"],
    queryFn: getStats,
  });
  const peersEnabled = stats?.peers_enabled ?? false;

  // The same query as ForumStrip's, so no second request: the chip needs the
  // forum's name, not its id.
  const { data: forumsData } = useQuery({
    queryKey: ["forums"],
    queryFn: getForums,
    staleTime: Infinity,
    gcTime: Infinity,
  });
  const selectedForum = useMemo(() => {
    if (forumId === undefined) return undefined;
    return forumsData?.items?.find((f) => f.id === forumId);
  }, [forumId, forumsData]);

  const chips: Chip[] = useMemo(() => {
    const cs: Chip[] = [];
    if (forumId !== undefined) {
      cs.push({
        id: "forum",
        label: t.metaForum,
        value: selectedForum ? leafName(selectedForum.name) : `#${forumId}`,
        // The tooltip carries the whole path a truncated chip cuts off.
        tooltip: selectedForum?.name.replace(/ - /g, " → "),
      });
    }
    return cs;
  }, [forumId, selectedForum, t]);

  // Favourites are sorted here, on a copy; relevance means newest starred first.
  const sortedFavs = useMemo(() => {
    if (!isFavView) return favItems;
    const arr = favItems.slice();
    const mul = sort.dir === "asc" ? 1 : -1;
    arr.sort((a, b) => {
      if (sort.key === "date") {
        return mul * (new Date(a.registered_at).getTime() - new Date(b.registered_at).getTime());
      }
      if (sort.key === "size") return mul * (a.size_bytes - b.size_bytes);
      // "relevance" maps to "newest starred first" in favorites view.
      return mul * (new Date(a.added_at).getTime() - new Date(b.added_at).getTime());
    });
    return arr;
  }, [isFavView, favItems, sort.key, sort.dir]);

  const total = isFavView ? favCount : data?.total ?? 0;
  // Clamped to the last page that still has rows: unstarring the last item of
  // the last page would otherwise show an empty list with no way back.
  const lastPage = Math.max(0, Math.ceil(total / PAGE_SIZE) - 1);
  const effPage = Math.min(page, lastPage);
  const items = isFavView
    ? sortedFavs.slice(effPage * PAGE_SIZE, (effPage + 1) * PAGE_SIZE)
    : data?.items ?? [];
  const showResults = hasFilter || isFavView;

  // Stuck is read off a zero-height sentinel above the meta row, not a
  // scroll listener.
  const headerRef = useRef<HTMLElement | null>(null);
  const stickyRef = useRef<HTMLDivElement | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const resultsPanelRef = useRef<HTMLDivElement | null>(null);
  // Stuck, the meta row drops its top rule: it would double the header's.
  const [isStuck, setIsStuck] = useState(false);

  // A page change scrolls the list's top to just under the stuck meta row.
  const onPageChange = useCallback((newPage: number) => {
    setPage(newPage);
    requestAnimationFrame(() => {
      const panel = resultsPanelRef.current;
      const sticky = stickyRef.current;
      if (!panel || !sticky) return;
      const stickyRect = sticky.getBoundingClientRect();
      const panelRect = panel.getBoundingClientRect();
      // 20 px ≈ main's `space-y-5` gap between meta and results panel.
      const target = stickyRect.bottom + 20;
      window.scrollBy({ top: panelRect.top - target, behavior: "smooth" });
    });
  }, []);
  // The observer's margin is the header's measured height; a ResizeObserver
  // re-arms it when the header changes rows.
  useEffect(() => {
    const node = sentinelRef.current;
    if (!node) return;
    let obs: IntersectionObserver | undefined;
    const arm = () => {
      obs?.disconnect();
      const h = Math.round(headerRef.current?.getBoundingClientRect().height ?? 64);
      obs = new IntersectionObserver(
        ([entry]) => setIsStuck(!entry.isIntersecting),
        { rootMargin: `-${h + 1}px 0px 0px 0px`, threshold: 0 },
      );
      obs.observe(node);
    };
    arm();
    const ro = new ResizeObserver(arm);
    if (headerRef.current) ro.observe(headerRef.current);
    return () => {
      ro.disconnect();
      obs?.disconnect();
    };
  }, [showResults]);

  return (
    // Header, a main that takes the slack, footer: the footer sits at the bottom.
    <div className="min-h-svh flex flex-col">
      <Header
        ref={headerRef}
        frost={!showResults}
        query={query}
        onQueryChange={setQueryReset}
        favOnly={favOnly}
        onToggleFavOnly={toggleFavOnly}
        onOpenAdmin={() => setAdminOpen(true)}
        onReset={() => {
          setQuery("");
          setForumIdState(undefined);
          setSortState({ key: "relevance", dir: "desc" });
          setPage(0);
          setDrawerOpen(false);
          setFavOnly(false);
          // ForumStrip reads this key on mount: write it before the remount below.
          localStorage.setItem(STRIP_OPEN_KEY, "false");
          setForumsResetKey((k) => k + 1);
          window.scrollTo({ top: 0, behavior: "smooth" });
        }}
      />

      <main className="flex-1 w-full mx-auto max-w-[1400px] px-6 sm:px-10 lg:px-16 pb-[env(safe-area-inset-bottom)]">
        {/* Forums + stats above everything — Swiss strip with hairlines, no
            outer panel. Lives flush against the header. */}
        {/* A new key remounts the strip, which folds its open sections. */}
        <ForumStrip
          key={forumsResetKey}
          selectedForumId={forumId}
          onSelectForum={setForumId}
        />

        {/* Hero on the empty state — emoji row; the localized phrase stays as
            the heading's accessible name. */}
        {!showResults && (
          <div className="pt-12 sm:pt-20 pb-8 sm:pb-12">
            <h1
              aria-label={t.heroFindSomething}
              className="flex flex-wrap gap-x-[0.25em] gap-y-2 leading-none select-none text-[40px] sm:text-[64px] lg:text-[88px]"
            >
              {HERO_EMOJI.map((e) => (
                <span key={e} aria-hidden="true">
                  {e}
                </span>
              ))}
            </h1>
          </div>
        )}

        {/* The sentinel is a sibling, so the observer has a stable node to watch. */}
        {showResults && (
          <>
            <div ref={sentinelRef} aria-hidden="true" className="h-0 w-full" />
            <div
              ref={stickyRef}
              className={cn(
                "sticky z-20 swiss-meta-enter",
                // The top rule is dropped while stuck: it would double the header's.
                "border-[var(--color-rule)]",
                "border-x border-b border-t",
                // The top border stays and fades: a width cannot be animated, a colour can.
                "transition-[border-radius,box-shadow,background-color,border-color,backdrop-filter] duration-300 ease-out",
                // Top corners are round only while the row floats; an empty list rounds
                // the bottom as well.
                isStuck ? "border-t-transparent rounded-t-none" : "rounded-t-md",
                items.length === 0 && "rounded-b-md",
                "top-[calc(env(safe-area-inset-top)+var(--header-h))]",
                // Frosted only while the list is under it.
                isStuck
                  ? "bg-[var(--color-paper)]/85 backdrop-blur-xs shadow-[var(--shadow-header)]"
                  : "bg-[var(--color-paper)]",
              )}
            >
              {/* Two rows below lg, one from lg: at sm the Russian labels still overflow. */}
              <div className="flex flex-col lg:flex-row lg:items-center lg:flex-nowrap gap-x-3 py-3 px-3 sm:px-4">
                <div className="flex items-center flex-nowrap gap-x-3 pb-2.5 lg:pb-0 lg:contents">
                <p className="text-[12px] text-[var(--color-ink-soft)] shrink-0 tabular-nums">
                  {isFavView ? (
                    <>
                      <span className="swiss-eyebrow text-[var(--color-accent)] mr-2">
                        {t.favoritesTitle}
                      </span>
                      <span className="text-[var(--color-rule)] mr-2">/</span>
                      <span className="font-medium text-[var(--color-ink)]">
                        {total.toLocaleString(locale)}
                      </span>{" "}
                      {p("results", total)}
                      {total > 0 && (
                        <>
                          <span className="text-[var(--color-rule)] mx-2">/</span>
                          <span className="font-medium text-[var(--color-ink)]">
                            {(effPage * PAGE_SIZE + 1).toLocaleString(locale)}–
                            {Math.min((effPage + 1) * PAGE_SIZE, total).toLocaleString(locale)}
                          </span>
                          {/* Page counter is dropped below lg — the pagination
                              control at the bottom of the list repeats it. */}
                          <span className="hidden lg:inline">
                            <span className="text-[var(--color-rule)] mx-2">/</span>
                            {t.paginationPage}{" "}
                            <span className="font-medium text-[var(--color-ink)]">
                              {effPage + 1}/{Math.max(1, Math.ceil(total / PAGE_SIZE))}
                            </span>
                          </span>
                        </>
                      )}
                    </>
                  ) : isFetching && !data ? (
                    t.searching
                  ) : (
                    <>
                      <span className="font-medium text-[var(--color-ink)]">
                        {total.toLocaleString(locale)}
                      </span>{" "}
                      {p("results", total)}
                      {total > 0 && (
                        <>
                          <span className="text-[var(--color-rule)] mx-2">/</span>
                          <span className="font-medium text-[var(--color-ink)]">
                            {(effPage * PAGE_SIZE + 1).toLocaleString(locale)}–
                            {Math.min((effPage + 1) * PAGE_SIZE, total).toLocaleString(locale)}
                          </span>
                          {/* Page counter is dropped below lg — the pagination
                              control at the bottom of the list repeats it. */}
                          <span className="hidden lg:inline">
                            <span className="text-[var(--color-rule)] mx-2">/</span>
                            {t.paginationPage}{" "}
                            <span className="font-medium text-[var(--color-ink)]">
                              {effPage + 1}/{Math.max(1, Math.ceil(total / PAGE_SIZE))}
                            </span>
                          </span>
                        </>
                      )}
                    </>
                  )}
                </p>
                {!isFavView && chips.length > 0 && (
                  <>
                    <span className="text-[var(--color-rule)] text-[14px] shrink-0">
                      /
                    </span>
                    {/* From lg the chip keeps its own width and shrinks only in a full row. */}
                    <div className="min-w-0 flex-1 lg:flex-initial">
                      <FilterChips
                        chips={chips}
                        onRemove={(id) => {
                          if (id === "forum") setForumId(undefined);
                        }}
                        onClearAll={() => setForumId(undefined)}
                      />
                    </div>
                  </>
                )}
                </div>
                {/* Hairline between the two stacked rows — mx-2 on top of the
                    chip's px-3 keeps it clear of the chip's own outline. */}
                <div
                  aria-hidden="true"
                  className="h-px mx-2 mb-2.5 bg-[var(--color-rule)] lg:hidden"
                />
                <div className="flex items-center gap-3 flex-nowrap shrink-0 lg:ml-auto">
                  <SortControl value={sort} onChange={setSort} />
                  {isFavView && favCount > 0 && (
                    <>
                      <span
                        aria-hidden="true"
                        className="h-4 w-px bg-[var(--color-rule)]"
                      />
                      <button
                        type="button"
                        onClick={() => setConfirmClear(true)}
                        aria-label={t.favoritesClearAll}
                        className="swiss-eyebrow hover:text-[var(--color-accent)] transition-colors duration-150"
                      >
                        {/* Icon-only below lg: the Russian label is 131px and
                            would not fit next to the sort options. */}
                        <Trash2 className="size-3.5 lg:hidden" />
                        <span className="hidden lg:inline">{t.favoritesClearAll}</span>
                      </button>
                    </>
                  )}
                </div>
              </div>
            </div>
          </>
        )}

        {error && (
          <div className="my-4 p-4 text-[13px] border-l-2 border-[var(--color-down)] bg-[var(--color-down)]/5 text-[var(--color-ink-soft)] rounded-md">
            {t.errorPrefix}
            {error instanceof Error ? error.message : t.errorGeneric}
          </div>
        )}

        {/* The cards fade in after the meta row has slid into place. */}
        <div ref={resultsPanelRef}>
          {showResults ? (
            <div className="swiss-results-enter">
              {items.map((it, i) => (
                <ResultCard
                  key={it.id}
                  torrent={it}
                  zebra={i % 2 === 0}
                  last={i === items.length - 1}
                  onSelect={openTorrent}
                  onForumClick={setForumId}
                  peersEnabled={peersEnabled}
                />
              ))}
              {isFavView && items.length === 0 && (
                <EmptyFavorites />
              )}
              {!isFavView && !isFetching && items.length === 0 && !error && (
                <div className="py-12 text-center text-[var(--color-ink-muted)] text-[14px]">
                  {t.noResults}
                </div>
              )}
              <Pagination
                page={effPage}
                pageSize={PAGE_SIZE}
                total={total}
                onChange={onPageChange}
              />
            </div>
          ) : (
            <EmptyState />
          )}
        </div>
      </main>

      <DetailDrawer
        torrentId={selectedId}
        open={drawerOpen}
        onClose={closeDrawer}
        onForumClick={setForumId}
      />

      <AdminPanel open={adminOpen} onClose={() => setAdminOpen(false)} />

      <ConfirmDialog
        open={confirmClear}
        title={t.favoritesClearAll}
        body={t.favoritesClearConfirm}
        confirmLabel={t.favoritesClearAll}
        onConfirm={() => {
          clearFavs();
          setPage(0);
        }}
        onClose={closeConfirm}
      />

      <Footer />
    </div>
  );
}

// Swiss empty state — flat text, no decoration. Eyebrow + body, top-aligned
// to a hairline above the section.
function EmptyState() {
  const { t } = useLang();
  return (
    <div className="swiss-rule-top py-8 sm:py-10 max-w-[42rem]">
      <div className="swiss-eyebrow mb-3 flex items-center gap-2">
        <Search className="size-3.5" strokeWidth={2.25} />
        <span>{t.emptyTitle}</span>
      </div>
      <p className="text-[14px] text-[var(--color-ink-soft)] leading-relaxed">
        {t.emptyHint}
      </p>
    </div>
  );
}

function EmptyFavorites() {
  const { t } = useLang();
  return (
    <div className="py-12 max-w-[42rem]">
      <div className="swiss-eyebrow mb-3 flex items-center gap-2 text-[var(--color-accent)]">
        <Star className="size-3.5" strokeWidth={2.25} fill="currentColor" />
        <span>{t.favoritesEmptyTitle}</span>
      </div>
      <p className="text-[14px] text-[var(--color-ink-soft)] leading-relaxed">
        {t.favoritesEmptyHint}
      </p>
    </div>
  );
}

export default App;
