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
  // selectedId survives drawer close (it's "last viewed", not "currently
  // open") so the drawer keeps its content rendered during the CSS close
  // transition; drawerOpen alone drives visibility.
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
  // favOnly turns the results panel into a local-snapshot view of starred
  // torrents. Mutually exclusive with the search query / forum filter: any of
  // those entering active state turns favOnly off, and vice versa, so the
  // results meta-row never describes two competing sources of truth.
  const [favOnly, setFavOnly] = useState(false);
  // Bumped to signal ForumStrip to collapse its expanded top/mid sections.
  // Triggers: brand-button reset, ⭐ click that enters favOnly. Local state
  // for chip expansion stays in the strip; this is just a fold-up signal.
  const [forumsResetKey, setForumsResetKey] = useState(0);
  const [adminOpen, setAdminOpen] = useState(false);
  const [confirmClear, setConfirmClear] = useState(false);
  const closeConfirm = useCallback(() => setConfirmClear(false), []);

  // Any filter change resets the cursor (staying on page 17 after picking a
  // forum would land on a phantom page). Reset is folded into the setters, not
  // a useEffect — react-hooks forbids sync setState in effects. Setters are
  // useCallback'd because memoized children (ForumStrip, ResultCard) depend on them.
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
    // Side effects live outside the setFavOnly updater on purpose: React
    // runs updaters during render (twice in StrictMode), so anything
    // non-idempotent inside one silently doubles.
    const next = !favOnly;
    if (next) {
      // Entering favorites mode is an exclusive view, so drop any active
      // search/forum filter — otherwise the meta-row count, the sort
      // control, and the pagination would all be inconsistent with the
      // local snapshot list we're about to render.
      setQuery("");
      setForumIdState(undefined);
      setPage(0);
      // Force the strip back to its default-closed state (panel + all
      // expanded categories). ForumStrip reads this key on mount, so we
      // write before the remount triggered by forumsResetKey below.
      localStorage.setItem(STRIP_OPEN_KEY, "false");
      setForumsResetKey((k) => k + 1);
    }
    setFavOnly(next);
  };

  // Don't run an unbounded SELECT on mount — wait until the user types or picks
  // a forum, else every page load scans ~10M rows for "newest 50".
  const hasFilter = debouncedQuery !== "" || forumId !== undefined;
  // favOnly is authoritative for the search-vs-favorites view: toggleFavOnly
  // clears query/forum synchronously, but debouncedQuery lags 300 ms — deriving
  // the view from it briefly resurrected the abandoned search (stale cards + refetch).
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

  // Forum lookup so the active chip can show the leaf name instead of a
  // bare id. The query is shared with ForumStrip via the same key — no
  // extra request. Same session-long cache settings as ForumStrip so
  // both observers agree the data is fresh forever.
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
        label: t("meta_forum"),
        value: selectedForum ? leafName(selectedForum.name) : `#${forumId}`,
        // Full breadcrumb (top → mid → leaf) goes into the tooltip so
        // truncated chips still expose the whole path on hover. The raw
        // dump uses " - " as the level separator; swap to " → " for the
        // tooltip so it reads as a real path, not three dashed phrases.
        tooltip: selectedForum?.name.replace(/ - /g, " → "),
      });
    }
    return cs;
  }, [forumId, selectedForum, t]);

  // Favorites are sorted client-side via the same SortControl. relevance maps
  // to "newest added first" because relevance has no meaning outside of a
  // text search. We sort over a *copy* so the underlying favorites array
  // (used by the Header counter etc.) stays stable.
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
  // Clamp the cursor to the last page that still has data. Unstarring the
  // only item on the last favorites page shrinks the list under a stale
  // `page` — without the clamp that rendered a false "no favorites" state
  // with the pagination gone, leaving no way back to the surviving items.
  const lastPage = Math.max(0, Math.ceil(total / PAGE_SIZE) - 1);
  const effPage = Math.min(page, lastPage);
  const items = isFavView
    ? sortedFavs.slice(effPage * PAGE_SIZE, (effPage + 1) * PAGE_SIZE)
    : data?.items ?? [];
  const showResults = hasFilter || isFavView;

  // Sticky-meta stuck detection via a zero-height sentinel above it: once the
  // sentinel scrolls past the header's bottom (rootMargin = -header height) it
  // stops intersecting → toggle the bar styles. No per-frame scroll listener.
  const headerRef = useRef<HTMLElement | null>(null);
  const stickyRef = useRef<HTMLDivElement | null>(null);
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const resultsPanelRef = useRef<HTMLDivElement | null>(null);
  // isStuck is used to hide meta-row's top hairline when it's pinned under
  // the header — otherwise the meta's top rule and the header's bottom rule
  // sit at the same Y and visually thicken into a 2px line.
  const [isStuck, setIsStuck] = useState(false);

  // Pagination: scroll the results panel's top under the stuck meta-row so the
  // new page's first card lands where the user left off (else they keep their
  // bottom scroll and see only the tail, or get yanked up to the forum strip).
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
  // Observe the sentinel; when it leaves the top region the meta-row is stuck.
  // Re-runs on showResults (the sentinel only mounts when results are visible).
  // rootMargin is the header's measured height (two rows on mobile, one from
  // sm up) + 1px slack; a ResizeObserver re-arms it when that height changes.
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
    // Flexbox sticky footer: root is a vertical column of the viewport
    // height, <main> grows to absorb the slack, footer sits at the bottom
    // whether the content is a tall result list or a single hero line.
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
          // Full strip reset: panel closed + all expanded categories
          // dropped. localStorage write happens before the remount so the
          // strip reads "closed" on its fresh mount.
          localStorage.setItem(STRIP_OPEN_KEY, "false");
          setForumsResetKey((k) => k + 1);
          window.scrollTo({ top: 0, behavior: "smooth" });
        }}
      />

      <main className="flex-1 w-full mx-auto max-w-[1400px] px-6 sm:px-10 lg:px-16 pb-[env(safe-area-inset-bottom)]">
        {/* Forums + stats above everything — Swiss strip with hairlines, no
            outer panel. Lives flush against the header. */}
        {/* key={forumsResetKey} — bumping it forces a remount which
            collapses any expanded top/mid sections. Cheaper than lifting
            the expansion state into App; forum + stats queries are cached
            with staleTime:Infinity so no refetch happens on remount. */}
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
              aria-label={t("hero_find_something")}
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

        {/* Sticky meta-row — flat hairline rectangle. At rest it sits in
            the flow; once the sentinel above scrolls past the header's
            bottom, position:sticky pins it. Top rule is dropped while
            stuck so it doesn't stack with the header's bottom rule into
            a visibly thicker 2px line — see isStuck below. */}
        {/* Sentinel + sticky meta-row. Animation is a CSS keyframe (see
            index.css `.swiss-meta-enter`) — runs on the compositor and
            doesn't drop frames when React commits ~25 ResultCards on the
            same tick. Sentinel is rendered as a sibling so the
            IntersectionObserver still has a stable mount point. */}
        {showResults && (
          <>
            <div ref={sentinelRef} aria-hidden="true" className="h-0 w-full" />
            <div
              ref={stickyRef}
              className={cn(
                "sticky z-20 swiss-meta-enter",
                // Full hairline outline — left/right/bottom always, top
                // dropped when stuck under the header (header's own bottom
                // rule provides that edge, otherwise the two 1px lines
                // would stack into a visibly thicker line).
                "border-[var(--color-rule)]",
                "border-x border-b border-t",
                // Docking animates: radius/border/bg/shadow tween 300 ms instead
                // of snapping. Top border is always present but fades transparent
                // when stuck (border-width isn't animatable, border-color is).
                "transition-[border-radius,box-shadow,background-color,border-color,backdrop-filter] duration-300 ease-out",
                // Top corners soft only while floating; flush/square when stuck.
                // Bottom square while the list continues below (its last card
                // rounds it) — but a zero-row meta is standalone and closes itself.
                isStuck ? "border-t-transparent rounded-t-none" : "rounded-t-md",
                items.length === 0 && "rounded-b-md",
                "top-[calc(env(safe-area-inset-top)+var(--header-h))]",
                // Stuck: near-opaque frosted bg — the list scrolling away
                // beneath shows through faintly blurred — plus a soft drop
                // shadow so the row reads as floating above the rows.
                isStuck
                  ? "bg-[var(--color-paper)]/85 backdrop-blur-xs shadow-[var(--shadow-header)]"
                  : "bg-[var(--color-paper)]",
              )}
            >
              {/* Below lg the row stacks into counts / controls, split by an
                  inset hairline; lg:contents dissolves the wrapper so wide
                  viewports keep the original single row. The breakpoint is lg,
                  not sm: at sm the Russian labels still overflow and wrapped
                  into two rows with no divider between them. */}
              <div className="flex flex-col lg:flex-row lg:items-center lg:flex-nowrap gap-x-3 py-3 px-3 sm:px-4">
                <div className="flex items-center flex-nowrap gap-x-3 pb-2.5 lg:pb-0 lg:contents">
                <p className="text-[12px] text-[var(--color-ink-soft)] shrink-0 tabular-nums">
                  {isFavView ? (
                    <>
                      <span className="swiss-eyebrow text-[var(--color-accent)] mr-2">
                        {t("favorites_title")}
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
                            {t("pagination_page")}{" "}
                            <span className="font-medium text-[var(--color-ink)]">
                              {effPage + 1}/{Math.max(1, Math.ceil(total / PAGE_SIZE))}
                            </span>
                          </span>
                        </>
                      )}
                    </>
                  ) : isFetching && !data ? (
                    t("searching")
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
                            {t("pagination_page")}{" "}
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
                    {/* lg:flex-initial, not flex-none: the chip takes its
                        natural width and only shrinks when the row is full,
                        so ml-auto still parks the sort control on the right. */}
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
                        aria-label={t("favorites_clear_all")}
                        className="swiss-eyebrow hover:text-[var(--color-accent)] transition-colors duration-150"
                      >
                        {/* Icon-only below lg: the Russian label is 131px and
                            would not fit next to the sort options. */}
                        <Trash2 className="size-3.5 lg:hidden" />
                        <span className="hidden lg:inline">{t("favorites_clear_all")}</span>
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
            {t("error_prefix")}
            {error instanceof Error ? error.message : t("error_generic")}
          </div>
        )}

        {/* Swiss results panel — no rounded outer surface. Just a flat
            container; the cards inside provide their own hairline rules.
            Entry fade is a CSS keyframe (see index.css
            `.swiss-results-enter`) with a 120 ms delay so the meta-row's
            slide-in plays first and the cards proper land on already-
            settled meta. CSS keeps animation on the compositor thread,
            so the heavy card-render commit can't drop frames here. */}
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
                  {t("no_results")}
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
        title={t("favorites_clear_all")}
        body={t("favorites_clear_confirm")}
        confirmLabel={t("favorites_clear_all")}
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
        <span>{t("empty_title")}</span>
      </div>
      <p className="text-[14px] text-[var(--color-ink-soft)] leading-relaxed">
        {t("empty_hint")}
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
        <span>{t("favorites_empty_title")}</span>
      </div>
      <p className="text-[14px] text-[var(--color-ink-soft)] leading-relaxed">
        {t("favorites_empty_hint")}
      </p>
    </div>
  );
}

export default App;
