import { memo } from "react";
import { type Torrent } from "../lib/api";
import { peersStale } from "../lib/peers";
import { formatBytes, formatDate } from "../lib/format";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { FavoriteStar } from "./FavoriteStar";
import { PeerBadge } from "./PeerStats";
import { Tooltip } from "./Tooltip";

type Props = {
  torrent: Torrent;
  onSelect: (id: number) => void;
  onForumClick: (forumId: number) => void;
  // True for odd rows (1, 3, 5...) — applies a subtle paper-soft fill so
  // adjacent rows visually separate without a hairline per row. Swiss
  // editorial catalogues use this zebra pattern for dense list reading.
  zebra?: boolean;
  // True for the final row of the page — it closes the meta-row + list
  // block, so it alone rounds the bottom corners (the meta-row rounds the
  // top; everything between stays square so the stripes butt seamlessly).
  last?: boolean;
  // Mirrors the server's RT_PEERS_ENABLED; off → the peer badge line is
  // dropped rather than showing a cache that can no longer refresh.
  peersEnabled?: boolean;
};

// memo: App re-renders on every keystroke / favorites change; stable callbacks
// keep the 25-card list (each with its own tooltip roots) out of those commits.
export const ResultCard = memo(function ResultCard({
  torrent,
  onSelect,
  onForumClick,
  zebra = false,
  last = false,
  peersEnabled = false,
}: Props) {
  const { t } = useLang();

  const onClick = () => onSelect(torrent.id);

  const handleKey = (e: React.KeyboardEvent<HTMLDivElement>) => {
    // Only the row itself: Enter on a nested control (star, forum link)
    // bubbles here, and preventDefault would suppress the button's native
    // activation — opening the drawer instead of doing what the user asked.
    if (e.target !== e.currentTarget) return;
    if (e.key === "Enter" || e.key === " ") {
      e.preventDefault();
      onClick();
    }
  };

  return (
    <div
      role="button"
      tabIndex={0}
      onClick={onClick}
      onKeyDown={handleKey}
      className={cn(
        "group w-full text-left py-3 sm:py-4 cursor-pointer transition-colors",
        // Resting tone alternates for the zebra. Hover/focus is the same
        // accent tint regardless of phase — visible against both paper and
        // paper-soft, matches the accent-colored hover used everywhere else.
        zebra && "bg-[var(--color-paper-soft)] dark:bg-[var(--color-dark-paper-soft)]",
        last && "rounded-b-md",
        "hover:bg-accent/10 dark:hover:bg-accent/15",
        "focus-visible:outline-none focus-visible:bg-accent/15 dark:focus-visible:bg-accent/20",
      )}
    >
      {/* px inset so the title/size/date don't touch the rule's left/right
          edges. Rule lives on the outer div (which spans the full content
          column); this inner grid is the visible type area. */}
      <div className="grid grid-cols-[1fr_auto] gap-x-4 sm:gap-x-6 items-start px-3 sm:px-4">
        {/* Left: title + forum (text-button). */}
        <div className="min-w-0 flex flex-col gap-1.5">
          {/* Reserve two line-heights (leading-snug ×2) even for one-line
              titles: keeps every row — and so every zebra stripe — the same
              height instead of jumping with title length. line-clamp-2 caps
              the tall ones. */}
          <h3 className="font-semibold text-[14px] sm:text-[15px] leading-snug tracking-[-0.01em] line-clamp-2 min-h-[2.75em] group-hover:text-[var(--color-accent)] transition-colors">
            {torrent.title}
          </h3>
          <ForumValue
            label={t("meta_forum")}
            value={torrent.forum_name}
            onClick={(e) => {
              e.stopPropagation();
              onForumClick(torrent.forum_id);
            }}
          />
        </div>

        {/* Right rail: star (compact) + size (top) + date (bottom).
            Tabular nums, right-aligned. */}
        <div className="shrink-0 flex flex-col items-end gap-1 text-[12px] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)] tabular-nums">
          <div className="flex items-center gap-2">
            <FavoriteStar torrent={torrent} />
            <span className="text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] whitespace-nowrap font-medium">
              {formatBytes(torrent.size_bytes)}
            </span>
          </div>
          <span className="whitespace-nowrap">
            {formatDate(torrent.registered_at)}
          </span>
          {/* Third rail line: cached seeders/leechers ("—" until first
              checked), greyed once older than the refresh window. */}
          {peersEnabled && (
            <div className="h-4 flex items-center">
              <PeerBadge
                seeders={torrent.seeders ?? null}
                leechers={torrent.leechers ?? null}
                stale={peersStale(torrent.peers_checked_at)}
              />
            </div>
          )}
        </div>
      </div>
    </div>
  );
});

// Swiss forum link — small caps eyebrow + value. Click swallowed so the
// parent row's onClick (open drawer) doesn't also fire.
function ForumValue({
  label,
  value,
  onClick,
}: {
  label: string;
  value: string;
  onClick: (e: React.MouseEvent) => void;
}) {
  return (
    <Tooltip text={value}>
    <button
      type="button"
      onClick={onClick}
      className={cn(
        "flex items-baseline gap-2 min-w-0 max-w-full self-start",
        "text-[12px] text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)]",
        "hover:text-[var(--color-accent)] transition-colors",
      )}
    >
      <span className="swiss-eyebrow text-[10px] shrink-0">{label}:</span>
      <span className="truncate">{value}</span>
    </button>
    </Tooltip>
  );
}
