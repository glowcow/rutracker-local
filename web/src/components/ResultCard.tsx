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
  // Every other row takes the paper-soft ground, so rows part without a rule.
  zebra?: boolean;
  // The page's last row closes the block and rounds the bottom corners.
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
    // Only the row itself: Enter on a nested control bubbles here, and
    // preventDefault would swallow that control's own action.
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
        // The zebra's resting tone; hover and focus are the same tint on both.
        zebra && "bg-[var(--color-paper-soft)]",
        last && "rounded-b-md",
        "hover:bg-[var(--color-row-hover)]",
        "focus-visible:outline-none focus-visible:bg-[var(--color-row-hover)]",
      )}
    >
      {/* The inset keeps the text off the row's edges. */}
      <div className="grid grid-cols-[1fr_auto] gap-x-4 sm:gap-x-6 items-start px-3 sm:px-4">
        {/* Left: title + forum (text-button). */}
        <div className="min-w-0 flex flex-col gap-1.5">
          {/* Two title lines are reserved even for a short title, so every row —
              and every stripe — is the same height. */}
          <h3 className="font-semibold text-[14px] sm:text-[15px] leading-snug tracking-[-0.01em] line-clamp-2 min-h-[2.75em] group-hover:text-[var(--color-accent)] transition-colors">
            {torrent.title}
          </h3>
          <ForumValue
            label={t.metaForum}
            value={torrent.forum_name}
            onClick={(e) => {
              e.stopPropagation();
              onForumClick(torrent.forum_id);
            }}
          />
        </div>

        {/* Right rail: star (compact) + size (top) + date (bottom).
            Tabular nums, right-aligned. */}
        <div className="shrink-0 flex flex-col items-end gap-1 text-[12px] text-[var(--color-ink-muted)] tabular-nums">
          <div className="flex items-center gap-2">
            <FavoriteStar torrent={torrent} />
            <span className="text-[var(--color-ink)] whitespace-nowrap font-medium">
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
        "text-[12px] text-[var(--color-ink-soft)]",
        "hover:text-[var(--color-accent)] transition-colors",
      )}
    >
      <span className="swiss-eyebrow text-[10px] shrink-0">{label}:</span>
      <span className="truncate">{value}</span>
    </button>
    </Tooltip>
  );
}
