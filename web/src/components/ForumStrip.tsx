import { memo, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Database, HardDrive, Users, ChevronRight, ChevronDown, ChevronUp } from "lucide-react";
import { getForums, getStats, type Forum } from "../lib/api";
import { cn } from "../lib/cn";
import { formatBytes, formatDate } from "../lib/format";
import { useLang } from "../lib/i18n";
import { Tooltip } from "./Tooltip";

function formatCount(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1000) return `${(n / 1000).toFixed(1)}K`;
  return String(n);
}

function splitPath(name: string): string[] {
  return name
    .split(" - ")
    .map((s) => s.trim())
    .filter(Boolean);
}

type LeafForum = {
  forum: Forum;
  leafLabel: string;
  // The "общий" entry of a subcategory: a 2-level forum whose leaf name matches
  // an existing 3-level middle (e.g. forum 314's leaf is the umbrella of a middle
  // that also has sub-leaves). Folded into that middle and pinned first.
  isUmbrella: boolean;
};

type Mid = {
  midName: string; // "" when the forum is only 2 levels deep
  totalCount: number;
  leaves: LeafForum[];
};

type Group = {
  top: string;
  totalCount: number;
  mids: Mid[];
};

// Collator: Russian primary, English secondary. Handles `numeric: true` so
// e.g. "Top 9" sorts before "Top 10" even though the alphabet check would
// otherwise reverse them.
const ALPHA = new Intl.Collator(["ru", "en"], { numeric: true });

// Build a 3-tier (top → mid → leaf) structure from the flat forum list. Two
// passes: (1) collect middle names appearing in 3+ level forums per top (to tell
// a 2-level forum's leaf "umbrella of a known subcategory" from a standalone
// sub-forum); (2) bucket each forum. 4-level forums (7 in the dump) fold past level 2.
function buildGroups(forums: Forum[]): Group[] {
  const knownMids = new Map<string, Set<string>>();
  for (const f of forums) {
    const parts = splitPath(f.name);
    if (parts.length < 3) continue;
    const top = parts[0];
    let set = knownMids.get(top);
    if (!set) {
      set = new Set();
      knownMids.set(top, set);
    }
    set.add(parts[1]);
  }

  const tops = new Map<string, Map<string, LeafForum[]>>();
  for (const f of forums) {
    const parts = splitPath(f.name);
    const top = parts[0] || f.name;

    let mid: string;
    let leafLabel: string;
    let isUmbrella = false;

    if (parts.length >= 3) {
      mid = parts[1];
      leafLabel = parts.slice(2).join(" / ");
    } else if (parts.length === 2) {
      const candidate = parts[1];
      if (knownMids.get(top)?.has(candidate)) {
        // Fold this 2-level forum into the matching middle as its umbrella
        // ("общий") entry. The label here is a fallback — SubChip renders
        // umbrella entries via t("forums_umbrella") for the EN locale.
        mid = candidate;
        leafLabel = "Общий";
        isUmbrella = true;
      } else {
        mid = "";
        leafLabel = candidate;
      }
    } else {
      mid = "";
      leafLabel = parts[0];
    }

    let midMap = tops.get(top);
    if (!midMap) {
      midMap = new Map();
      tops.set(top, midMap);
    }
    let arr = midMap.get(mid);
    if (!arr) {
      arr = [];
      midMap.set(mid, arr);
    }
    arr.push({ forum: f, leafLabel, isUmbrella });
  }

  return Array.from(tops.entries())
    .map(([top, midMap]) => {
      const mids: Mid[] = Array.from(midMap.entries()).map(([midName, leaves]) => ({
        midName,
        totalCount: leaves.reduce((s, l) => s + l.forum.count, 0),
        leaves: leaves.sort((a, b) => {
          // "Общий" pinned first within its mid.
          if (a.isUmbrella !== b.isUmbrella) return a.isUmbrella ? -1 : 1;
          return ALPHA.compare(a.leafLabel, b.leafLabel);
        }),
      }));
      mids.sort((a, b) => {
        // Empty-mid first (truly direct sub-forums), then alphabetically.
        if (a.midName === "" && b.midName !== "") return -1;
        if (a.midName !== "" && b.midName === "") return 1;
        return ALPHA.compare(a.midName, b.midName);
      });
      return {
        top,
        totalCount: mids.reduce((s, m) => s + m.totalCount, 0),
        mids,
      };
    })
    .sort((a, b) => ALPHA.compare(a.top, b.top));
}

type Props = {
  selectedForumId?: number;
  onSelectForum: (id: number | undefined) => void;
};

// localStorage flag for the strip's open/collapsed state. The panel is heavy
// vertical space and most sessions are search-driven, so it starts collapsed
// unless opened last time (picking a forum auto-opens it). Exported because
// App's reset paths write "closed" — one constant, not a literal in two files.
export const STRIP_OPEN_KEY = "forums_open";

// memo: with a forum selected the strip is force-open while the user types a
// refining query — without memo every keystroke re-rendered hundreds of chips.
// Requires a stable onSelectForum from App.
export const ForumStrip = memo(function ForumStrip({ selectedForumId, onSelectForum }: Props) {
  const { t, p, pAbbr } = useLang();
  // expandedTop = LAST expanded top category; it survives closing so the
  // grid-rows collapse still has content to animate over. topOpen alone
  // says whether that section is currently expanded.
  const [expandedTop, setExpandedTop] = useState<string | null>(null);
  const [topOpen, setTopOpen] = useState(false);
  // Mid sections start collapsed — user expands what they want via the
  // chevron. Empty-mid (direct sub-forums) sits separately and is always
  // shown (see below).
  const [expandedMids, setExpandedMids] = useState<Set<string>>(new Set());
  const [persistedOpen, setPersistedOpen] = useState<boolean>(
    () => localStorage.getItem(STRIP_OPEN_KEY) === "true",
  );
  // Explicit override on top of the derived state, keyed to the selection it
  // was made under. Without it "Hide forums" was a no-op while a forum filter
  // force-opened the strip. A stale override (sel no longer matching) just stops
  // applying, so picking another forum re-engages auto-open without an effect.
  const [override, setOverride] = useState<{ sel: number | undefined; open: boolean } | null>(null);

  // Derived open state: override for this exact selection wins; else the
  // persisted preference OR an active forum filter auto-opens the strip.
  const stripOpen =
    override && override.sel === selectedForumId
      ? override.open
      : persistedOpen || selectedForumId !== undefined;

  const toggleStrip = () => {
    const next = !stripOpen;
    setOverride({ sel: selectedForumId, open: next });
    setPersistedOpen(next);
    localStorage.setItem(STRIP_OPEN_KEY, next ? "true" : "false");
  };

  // Noun matching formatCount's display: abbreviated K/M phrases take the
  // genitive plural (pAbbr), exact sub-1000 counts use real plural rules.
  const nounFor = (key: "torrents" | "forums", n: number) =>
    n >= 1000 ? pAbbr(key) : p(key, n);

  // Forums + stats are immutable for the session — a new dump arrives via
  // a redeploy, not while the tab is open. Cache forever so we don't
  // refetch them on every re-mount / tab-focus / 30s-stale window.
  const { data: forumsData, isLoading } = useQuery({
    queryKey: ["forums"],
    queryFn: getForums,
    staleTime: Infinity,
    gcTime: Infinity,
  });
  const { data: stats } = useQuery({
    queryKey: ["stats"],
    queryFn: getStats,
    staleTime: Infinity,
    gcTime: Infinity,
  });

  const groups: Group[] = useMemo(
    () => buildGroups(forumsData?.items ?? []),
    [forumsData?.items],
  );

  // The group whose mid-list is rendered. Because expandedTop is kept on
  // close, this stays set during the collapse animation.
  const displayGroup = expandedTop
    ? groups.find((g) => g.top === expandedTop)
    : undefined;
  const midsOpen = topOpen && displayGroup !== undefined;

  const toggleMid = (key: string) => {
    setExpandedMids((prev) => {
      const next = new Set(prev);
      if (next.has(key)) next.delete(key);
      else next.add(key);
      return next;
    });
  };

  // Re-clicking the open top folds it; picking another swaps the content
  // (no tween for the swap — open/close keep the animation) and resets the
  // mid expansion so the next category starts clean.
  const toggleTop = (top: string) => {
    if (topOpen && top === expandedTop) {
      setTopOpen(false);
      return;
    }
    setExpandedTop(top);
    setTopOpen(true);
    setExpandedMids(new Set());
  };

  return (
    // Swiss strip — no outer rules; the header above and the meta-row below
    // each provide their own boundary, so adding ours here doubled the
    // hairline. Internal hairlines separate the stats row from the chips
    // panel when expanded.
    <div>
      {/* Stats line — Swiss editorial sub-display: numbers are the data,
          they get the weight; the noun ("torrents", "forums") sits soft
          next to them. Sized between body text and the hero so it reads
          as a data ribbon rather than a control row. */}
      {/* Mobile shrinks stats so the row fits stats + toggle in one line
          (and the toggle stops looking awkwardly orphaned). sm+ keeps the
          full editorial 22px ribbon. Each [slash + cluster] is grouped via
          whitespace-nowrap so a slash never lands alone at the start of a
          wrapped row.

          Stats inner div is flex-nowrap + overflow-hidden with a right-edge
          mask-gradient: when the viewport is too narrow to hold every
          cluster, the trailing cluster fades out under the toggle button
          instead of wrapping onto a second row. Mask is benign when content
          fits — only the empty 24px gutter on the right gets faded. */}
      <div className="flex items-baseline gap-3 sm:gap-4 text-[14px] sm:text-[17px] leading-tight text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)] py-3 sm:py-4 px-3 sm:px-4">
        {stats ? (
          <div
            className="flex-1 min-w-0 flex items-baseline gap-x-2 sm:gap-x-4 flex-nowrap overflow-hidden"
            style={{
              maskImage:
                "linear-gradient(to right, black calc(100% - 24px), transparent)",
              WebkitMaskImage:
                "linear-gradient(to right, black calc(100% - 24px), transparent)",
            }}
          >
            <span className="flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
              <Database className="size-3.5 sm:size-4 translate-y-[2px] shrink-0" />
              <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]">
                {formatCount(stats.torrents_total)}
              </span>{" "}
              <span className="text-[11px] sm:text-[12px]">
                {nounFor("torrents", stats.torrents_total)}
              </span>
            </span>
            {/* Hidden on mobile so the dump stamp below stays visible: at
                390px only two clusters fit next to the toggle, and "when was
                this dump loaded" beats a static total size. */}
            <span className="hidden sm:flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
              <span className="text-[var(--color-rule)] dark:text-[var(--color-dark-rule)]">/</span>
              <HardDrive className="size-3.5 sm:size-4 translate-y-[2px] shrink-0" />
              <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]">
                {formatBytes(stats.total_size_bytes)}
              </span>
            </span>
            {/* Hidden on mobile: the wider Russian toggle label left this
                cluster half-masked, reading as a truncated "1". */}
            <span className="hidden sm:flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
              <span className="text-[var(--color-rule)] dark:text-[var(--color-dark-rule)]">/</span>
              <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]">
                {formatCount(stats.forums_count)}
              </span>{" "}
              <span className="text-[11px] sm:text-[12px]">
                {nounFor("forums", stats.forums_count)}
              </span>
            </span>
            {/* Dump freshness — when the last successful ingest finished.
                Kept visible at every width: it's what tells the reader how
                current everything else in this ribbon is. */}
            {stats.dump_updated_at && (
              <Tooltip text={t("stats_dump_updated")}>
                {/* No slash before it: the "@" is itself the separator, and
                    the date is a value, so it carries the same weight and
                    size as the other numbers in the ribbon. The negative ml
                    trims the row's gap down to this cluster's own, so the "@"
                    keeps equal — and tight — air on both sides. */}
                <span className="flex items-baseline gap-1.5 sm:gap-2 -ml-0.5 sm:-ml-2 whitespace-nowrap shrink-0">
                  {/* Inter's "@" is drawn short and hangs below the baseline,
                      so it reads low next to lining digits — nudged up and
                      slightly enlarged to sit on the digits' optical centre. */}
                  <span className="text-[1.1em] leading-none -translate-y-[2px] text-[var(--color-rule)] dark:text-[var(--color-dark-rule)]">
                    @
                  </span>
                  <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]">
                    {formatDate(stats.dump_updated_at)}
                  </span>
                </span>
              </Tooltip>
            )}
            {/* Peer cluster only when the feature is on — a frozen count of a
                cache that can no longer refresh is worse than no number. */}
            {stats.peers_enabled && (
              <span className="flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
                <span className="text-[var(--color-rule)] dark:text-[var(--color-dark-rule)]">/</span>
                <Users className="size-3.5 sm:size-4 translate-y-[2px] shrink-0" />
                <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]">
                  {formatCount(stats.peers_cached)}
                </span>{" "}
                <span className="text-[11px] sm:text-[12px]">
                  {t("peers_cached_label")}
                </span>
              </span>
            )}
          </div>
        ) : (
          /* Skeleton placeholder — bars are sized to the actual text
             line-height (14px×1.25 ≈ 18px on mobile, 17px×1.25 ≈ 22px
             on sm+) so the row's height stays put when real stats land.
             Otherwise the layout would jump 10-12 px downward as the
             cluster grew, which the user reads as a "bounce". */
          <div className="flex-1 min-w-0 flex items-center gap-x-2 sm:gap-x-4 flex-nowrap overflow-hidden">
            <span className="swiss-skeleton h-[18px] sm:h-[22px] w-24 sm:w-40 shrink-0" />
            <span className="swiss-skeleton h-[18px] sm:h-[22px] w-16 sm:w-24 shrink-0" />
            <span className="swiss-skeleton h-[18px] sm:h-[22px] w-20 sm:w-32 shrink-0" />
          </div>
        )}
        <button
          type="button"
          onClick={toggleStrip}
          aria-expanded={stripOpen}
          className={cn(
            "ml-auto self-center shrink-0 flex items-center gap-1.5",
            "text-[10px] sm:text-[11px] font-semibold uppercase tracking-[0.08em]",
            "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)]",
            "hover:text-[var(--color-ink)] dark:hover:text-[var(--color-dark-ink)]",
            "transition-colors leading-none",
          )}
        >
          <span className="leading-none">{stripOpen ? t("forums_hide") : t("forums_show")}</span>
          {stripOpen ? (
            <ChevronUp className="size-3 sm:size-3.5 shrink-0" />
          ) : (
            <ChevronDown className="size-3 sm:size-3.5 shrink-0" />
          )}
        </button>
      </div>

      {/* Forum chips section — animated expand/collapse via the CSS
          grid-rows 0fr↔1fr trick (the modern height:auto animation; runs on
          the compositor). Content stays mounted while closed —
          the row collapses and the inner overflow-hidden clips it; `inert`
          keeps the hidden chips out of tab order and hit-testing. */}
      <div
        inert={!stripOpen}
        className={cn(
          "grid swiss-collapse",
          stripOpen ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
        )}
      >
        <div className="overflow-hidden min-h-0">
            {/* rounded-b only — the top edge carries the hairline rule and
                stays square against it. */}
            <div className="swiss-rule-top rounded-b-md pt-3 pb-4 px-3 sm:px-4 bg-[var(--color-paper-soft)] dark:bg-[var(--color-dark-paper-soft)]">
              {/* Row 1 — top-level categories */}
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {isLoading && (
            /* Skeleton row for the top-level forum chips — random-ish
               widths so it doesn't look like an evenly-spaced ruler.
               First load only; once cached (staleTime:Infinity) the
               panel opens with real chips immediately. */
            <>
              <span className="swiss-skeleton h-4 w-28" />
              <span className="swiss-skeleton h-4 w-36" />
              <span className="swiss-skeleton h-4 w-20" />
              <span className="swiss-skeleton h-4 w-32" />
              <span className="swiss-skeleton h-4 w-24" />
              <span className="swiss-skeleton h-4 w-40" />
              <span className="swiss-skeleton h-4 w-28" />
              <span className="swiss-skeleton h-4 w-24" />
            </>
          )}
          {groups.map((g) => (
            <TopChip
              key={g.top}
              label={g.top}
              count={g.totalCount}
              expanded={topOpen && expandedTop === g.top}
              onClick={() => toggleTop(g.top)}
            />
          ))}
        </div>

        {/* Row 2 — middle sections + leaf chips for the expanded top group.
            Same grid-rows collapse, two nested layers: (1) the whole
            mid-list when a top toggles, (2) each mid's leaf chips when its
            chevron toggles. displayGroup keeps the LAST expanded group
            rendered while collapsing so the close still has content to
            animate over; switching directly between tops swaps content
            without a tween (acceptable: open/close keep the animation). */}
        {displayGroup && (
          <div
            inert={!midsOpen}
            className={cn(
              "grid swiss-collapse",
              midsOpen ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
            )}
          >
            <div className="overflow-hidden min-h-0">
              <div className="mt-3 pt-3 swiss-rule-top space-y-3">
          {displayGroup.mids.map((mid) => {
            const key = `${displayGroup.top}/${mid.midName}`;
            const isDirect = mid.midName === "";
            // Direct (2-level) forums: quiet italic label, always shown (small
            // count, useful quick-picks). Proper mid sections start collapsed.
            const expanded = isDirect || expandedMids.has(key);
            return (
              <div key={key} className="space-y-1.5">
                {isDirect ? (
                  <div className="swiss-eyebrow flex items-baseline gap-2">
                    <span>{t("forums_direct")}</span>
                    <span className="tabular-nums normal-case tracking-normal text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]">
                      ({formatCount(mid.totalCount)})
                    </span>
                  </div>
                ) : (
                  <button
                    onClick={() => toggleMid(key)}
                    className="flex items-center gap-2 swiss-eyebrow hover:text-[var(--color-ink)] dark:hover:text-[var(--color-dark-ink)] transition-colors py-0.5"
                  >
                    <ChevronRight
                      className={cn(
                        "size-3 transition-transform",
                        expanded && "rotate-90",
                      )}
                    />
                    <span>{mid.midName}</span>
                    <span className="normal-case tracking-normal tabular-nums text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]">
                      {formatCount(mid.totalCount)}
                    </span>
                  </button>
                )}
                <div
                  inert={!expanded}
                  className={cn(
                    "grid swiss-collapse",
                    expanded ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
                  )}
                >
                  <div className="overflow-hidden min-h-0">
                    <div className={cn("flex flex-wrap gap-x-4 gap-y-1.5", !isDirect && "pl-4")}>
                      {mid.leaves.map((l) => (
                        <SubChip
                          key={l.forum.id}
                          forum={l.forum}
                          leafLabel={l.isUmbrella ? t("forums_umbrella") : l.leafLabel}
                          isUmbrella={l.isUmbrella}
                          active={l.forum.id === selectedForumId}
                          onClick={() =>
                            onSelectForum(
                              l.forum.id === selectedForumId ? undefined : l.forum.id,
                            )
                          }
                        />
                      ))}
                    </div>
                  </div>
                </div>
              </div>
            );
          })}
              </div>
            </div>
          </div>
        )}
            </div>
            {/* Plain-paper spacer below the panel — gives the meta-row (or
                hero) clear breathing room from the last chip row regardless
                of expansion depth. Inside the collapsing row so it folds
                with the height tween. */}
            <div aria-hidden="true" className="h-3 sm:h-4" />
        </div>
      </div>
    </div>
  );
});

function TopChip({
  label,
  count,
  expanded,
  onClick,
}: {
  label: string;
  count: number;
  expanded: boolean;
  onClick: () => void;
}) {
  // items-center (not baseline): the lucide chevron is an SVG with no text
  // baseline, so baseline-align floated it high. Center keeps the row even.
  return (
    <Tooltip text={label}>
    <button
      onClick={onClick}
      className={cn(
        "shrink-0 flex items-center gap-1.5 text-[13px] transition-colors",
        expanded
          ? "text-[var(--color-accent)] font-semibold"
          : "text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] hover:text-[var(--color-accent)]",
      )}
    >
      <span className="max-w-[220px] truncate">{label}</span>
      <span
        className={cn(
          "text-[11px] tabular-nums",
          expanded
            ? "text-[var(--color-accent)]"
            : "text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]",
        )}
      >
        {formatCount(count)}
      </span>
      <ChevronRight
        className={cn(
          "size-3 transition-transform shrink-0",
          expanded && "rotate-90",
        )}
      />
    </button>
    </Tooltip>
  );
}

function SubChip({
  forum,
  leafLabel,
  isUmbrella,
  active,
  onClick,
}: {
  forum: Forum;
  leafLabel: string;
  isUmbrella?: boolean;
  active: boolean;
  onClick: () => void;
}) {
  return (
    <Tooltip text={forum.name}>
    <button
      onClick={onClick}
      className={cn(
        "shrink-0 flex items-baseline gap-1.5 text-[12.5px] transition-colors",
        active
          ? "text-[var(--color-accent)] font-semibold"
          : isUmbrella
            ? "italic text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] hover:text-[var(--color-accent)]"
            : "text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] hover:text-[var(--color-accent)]",
      )}
    >
      <span className="max-w-[260px] truncate">{leafLabel}</span>
      <span
        className={cn(
          "text-[11px] tabular-nums",
          active
            ? "text-[var(--color-accent)]"
            : "text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]",
        )}
      >
        {formatCount(forum.count)}
      </span>
    </button>
    </Tooltip>
  );
}
