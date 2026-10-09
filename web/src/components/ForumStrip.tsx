import { memo, useMemo, useState } from "react";
import { useQuery } from "@tanstack/react-query";
import { Database, HardDrive, Users, ChevronRight, ChevronDown, ChevronUp } from "lucide-react";
import { getForums, getStats, type Forum } from "../lib/api";
import { cn } from "../lib/cn";
import { formatBytes, formatDate } from "../lib/format";
import { useLang } from "../lib/i18n";
import { Collapse } from "./Collapse";
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
  // A 2-level forum whose leaf names an existing 3-level middle: the general
  // forum of that subcategory, folded into it and pinned first.
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

// Russian first, then English; numeric, so "Top 9" sorts before "Top 10".
const ALPHA = new Intl.Collator(["ru", "en"], { numeric: true });

// Builds top → mid → leaf from the flat list: first collect the middle
// names per top, then bucket each forum. Deeper levels fold into the leaf.
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
        // Folded into the matching middle as its general entry; the label is a
        // fallback — the chip renders it from the dictionary.
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

// The strip's open state, remembered across visits; it starts folded. Only its
// own button opens it. Exported: App's reset paths write "closed" under it.
export const STRIP_OPEN_KEY = "forums_open";

// memo: an open strip holds hundreds of chips, and App renders on every
// keystroke. Requires a stable onSelectForum from App.
export const ForumStrip = memo(function ForumStrip({ selectedForumId, onSelectForum }: Props) {
  const { t, p, pAbbr } = useLang();
  // The last open top category: kept after the close so the fold has
  // content to animate over; topOpen says whether it is open.
  const [expandedTop, setExpandedTop] = useState<string | null>(null);
  const [topOpen, setTopOpen] = useState(false);
  // Sections start folded; direct sub-forums are always shown.
  const [expandedMids, setExpandedMids] = useState<Set<string>>(new Set());
  // A forum filter does not open the strip: a tap on a card's forum line
  // would throw the results down the page.
  const [stripOpen, setStripOpen] = useState<boolean>(
    () => localStorage.getItem(STRIP_OPEN_KEY) === "true",
  );

  const toggleStrip = () => {
    const next = !stripOpen;
    setStripOpen(next);
    localStorage.setItem(STRIP_OPEN_KEY, next ? "true" : "false");
  };

  // Noun matching formatCount's display: abbreviated K/M phrases take the
  // genitive plural (pAbbr), exact sub-1000 counts use real plural rules.
  const nounFor = (key: "torrents" | "forums", n: number) =>
    n >= 1000 ? pAbbr(key) : p(key, n);

  // Forums and stats do not change while the tab is open: cached for good.
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

  // A second click folds the open top; another top swaps the content and
  // resets its sections.
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
    // No outer rules: the header above and the meta row below draw their own.
    <div>
      {/* The numbers carry the weight, their nouns sit soft beside them. */}
      {/* One row at every width: a cluster that does not fit fades out under
          the toggle instead of wrapping. */}
      <div className="flex items-baseline gap-3 sm:gap-4 text-[14px] sm:text-[17px] leading-tight text-[var(--color-ink-soft)] py-3 sm:py-4 px-3 sm:px-4">
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
              <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)]">
                {formatCount(stats.torrents_total)}
              </span>{" "}
              <span className="text-[11px] sm:text-[12px]">
                {nounFor("torrents", stats.torrents_total)}
              </span>
            </span>
            {/* Hidden below sm: only two clusters fit beside the toggle, and the dump's
                date says more than a total size. */}
            <span className="hidden sm:flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
              <span className="text-[var(--color-rule)]">/</span>
              <HardDrive className="size-3.5 sm:size-4 translate-y-[2px] shrink-0" />
              <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)]">
                {formatBytes(stats.total_size_bytes)}
              </span>
            </span>
            {/* Hidden on mobile: the wider Russian toggle label left this
                cluster half-masked, reading as a truncated "1". */}
            <span className="hidden sm:flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
              <span className="text-[var(--color-rule)]">/</span>
              <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)]">
                {formatCount(stats.forums_count)}
              </span>{" "}
              <span className="text-[11px] sm:text-[12px]">
                {nounFor("forums", stats.forums_count)}
              </span>
            </span>
            {/* When the last successful load finished; shown at every width. */}
            {stats.dump_updated_at && (
              <Tooltip text={t.statsDumpUpdated}>
                {/* No slash before it: the @ is the separator. The negative margin trims
                    the row's gap to the cluster's own, so the @ has equal air on both sides. */}
                <span className="flex items-baseline gap-1.5 sm:gap-2 -ml-0.5 sm:-ml-2 whitespace-nowrap shrink-0">
                  {/* Inter draws @ short and low: nudged up and enlarged to sit with the digits. */}
                  <span className="text-[1.1em] leading-none -translate-y-[2px] text-[var(--color-rule)]">
                    @
                  </span>
                  <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)]">
                    {formatDate(stats.dump_updated_at)}
                  </span>
                </span>
              </Tooltip>
            )}
            {/* Peer cluster only when the feature is on — a frozen count of a
                cache that can no longer refresh is worse than no number. */}
            {stats.peers_enabled && (
              <span className="flex items-baseline gap-1.5 sm:gap-2 whitespace-nowrap shrink-0">
                <span className="text-[var(--color-rule)]">/</span>
                <Users className="size-3.5 sm:size-4 translate-y-[2px] shrink-0" />
                <span className="tabular-nums font-semibold tracking-[-0.01em] text-[var(--color-ink)]">
                  {formatCount(stats.peers_cached)}
                </span>{" "}
                <span className="text-[11px] sm:text-[12px]">
                  {t.peersCachedLabel}
                </span>
              </span>
            )}
          </div>
        ) : (
          /* Bars as tall as the text line, so the row keeps its height when the
             numbers land. */
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
            "text-[var(--color-ink-soft)]",
            "hover:text-[var(--color-ink)]",
            "transition-colors leading-none",
          )}
        >
          <span className="leading-none">{stripOpen ? t.forumsHide : t.forumsShow}</span>
          {stripOpen ? (
            <ChevronUp className="size-3 sm:size-3.5 shrink-0" />
          ) : (
            <ChevronDown className="size-3 sm:size-3.5 shrink-0" />
          )}
        </button>
      </div>

      <Collapse open={stripOpen}>
            {/* rounded-b only — the top edge carries the hairline rule and
                stays square against it. */}
            <div className="swiss-rule-top rounded-b-md pt-3 pb-4 px-3 sm:px-4 bg-[var(--color-paper-soft)]">
              {/* Row 1 — top-level categories */}
        <div className="flex flex-wrap gap-x-4 gap-y-2">
          {isLoading && (
            /* Uneven widths, so it does not read as a ruler; first load only. */
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

        {/* Row 2 — the open top group's sections, each folding its own leaves.
            displayGroup keeps the last group rendered while it folds away. */}
        {displayGroup && (
          <Collapse open={midsOpen} className="mt-3 pt-3 swiss-rule-top space-y-3">
          {displayGroup.mids.map((mid) => {
            const key = `${displayGroup.top}/${mid.midName}`;
            const isDirect = mid.midName === "";
            // Direct (2-level) forums are always shown; a section starts folded.
            const expanded = isDirect || expandedMids.has(key);
            return (
              <div key={key} className="space-y-1.5">
                {isDirect ? (
                  <div className="swiss-eyebrow flex items-baseline gap-2">
                    <span>{t.forumsDirect}</span>
                    <span className="tabular-nums normal-case tracking-normal text-[var(--color-ink-muted)]">
                      ({formatCount(mid.totalCount)})
                    </span>
                  </div>
                ) : (
                  <button
                    onClick={() => toggleMid(key)}
                    className="flex items-center gap-2 swiss-eyebrow hover:text-[var(--color-ink)] transition-colors py-0.5"
                  >
                    <ChevronRight
                      className={cn(
                        "size-3 transition-transform",
                        expanded && "rotate-90",
                      )}
                    />
                    <span>{mid.midName}</span>
                    <span className="normal-case tracking-normal tabular-nums text-[var(--color-ink-muted)]">
                      {formatCount(mid.totalCount)}
                    </span>
                  </button>
                )}
                <Collapse
                  open={expanded}
                  className={cn("flex flex-wrap gap-x-4 gap-y-1.5", !isDirect && "pl-4")}
                >
                      {mid.leaves.map((l) => (
                        <SubChip
                          key={l.forum.id}
                          forum={l.forum}
                          leafLabel={l.isUmbrella ? t.forumsUmbrella : l.leafLabel}
                          active={l.forum.id === selectedForumId}
                          onClick={() =>
                            onSelectForum(
                              l.forum.id === selectedForumId ? undefined : l.forum.id,
                            )
                          }
                        />
                      ))}
                </Collapse>
              </div>
            );
          })}
          </Collapse>
        )}
            </div>
            {/* Air under the panel; inside the fold, so it folds away too. */}
            <div aria-hidden="true" className="h-3 sm:h-4" />
      </Collapse>
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
          : "text-[var(--color-ink)] hover:text-[var(--color-accent)]",
      )}
    >
      <span className="max-w-[220px] truncate">{label}</span>
      <span
        className={cn(
          "text-[11px] tabular-nums",
          expanded
            ? "text-[var(--color-accent)]"
            : "text-[var(--color-ink-muted)]",
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
  active,
  onClick,
}: {
  forum: Forum;
  leafLabel: string;
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
          : "text-[var(--color-ink)] hover:text-[var(--color-accent)]",
      )}
    >
      <span className="max-w-[260px] truncate">{leafLabel}</span>
      <span
        className={cn(
          "text-[11px] tabular-nums",
          active
            ? "text-[var(--color-accent)]"
            : "text-[var(--color-ink-muted)]",
        )}
      >
        {formatCount(forum.count)}
      </span>
    </button>
    </Tooltip>
  );
}
