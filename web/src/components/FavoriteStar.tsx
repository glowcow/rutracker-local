import { useState } from "react";
import { Star } from "lucide-react";
import { type Torrent } from "../lib/api";
import { useIsFavorite, useFavoriteToggle } from "../lib/favorites";
import { useLang } from "../lib/i18n";
import { cn } from "../lib/cn";
import { Tooltip } from "./Tooltip";

type Props = {
  torrent: Torrent;
  size?: "card" | "drawer";
};

// An icon button: bare in a card, framed in the drawer. Subscribes to its own
// boolean, so one toggle does not render the other stars.
export function FavoriteStar({ torrent, size = "card" }: Props) {
  const active = useIsFavorite(torrent.id);
  const toggle = useFavoriteToggle();
  const { t } = useLang();
  const [pulse, setPulse] = useState(0);

  const onClick = (e: React.MouseEvent) => {
    e.stopPropagation();
    toggle(torrent);
    setPulse((n) => n + 1);
  };

  const isDrawer = size === "drawer";
  // Drawer mode matches the action-row buttons (h-11, framed); card mode stays
  // the 24 px unframed icon — a border-frame ×25 in the dense list feels heavy.
  const btnSize = isDrawer ? "size-11" : "size-6";
  const iconSize = isDrawer ? "size-4" : "size-3.5";

  const label = active ? t.favoritesRemoveAria : t.favoritesAddAria;
  return (
    <Tooltip text={label}>
    <button
      type="button"
      onClick={onClick}
      aria-label={label}
      aria-pressed={active}
      className={cn(
        "shrink-0 grid place-items-center transition-colors",
        btnSize,
        isDrawer && [
          "rounded-md border border-[var(--color-rule)]",
          "hover:border-[var(--color-accent)]",
        ],
        active
          ? "text-[var(--color-accent)]"
          : "text-[var(--color-ink-muted)] hover:text-[var(--color-accent)]",
      )}
    >
      {/* key={pulse} remounts the span per click so the CSS pulse replays;
          pulse=0 (initial mount) skips the class — no pop on page load. */}
      <span
        key={pulse}
        className={cn("grid place-items-center", pulse > 0 && "swiss-star-pulse")}
      >
        <Star
          className={iconSize}
          strokeWidth={2}
          fill={active ? "currentColor" : "none"}
        />
      </span>
    </button>
    </Tooltip>
  );
}
