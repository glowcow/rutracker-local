import { useEffect, useRef, useState } from "react";
import { Search, X, Star, Database } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { useFavorites } from "../lib/favorites";
import { SettingsMenu } from "./SettingsMenu";
import { Tooltip } from "./Tooltip";

type Props = {
  // App measures the header to place the sticky meta-row under it.
  ref?: React.Ref<HTMLElement>;
  // False while a sticky bar below the header does the frosting instead.
  frost: boolean;
  query: string;
  onQueryChange: (v: string) => void;
  onReset: () => void;
  favOnly: boolean;
  onToggleFavOnly: () => void;
  onOpenAdmin: () => void;
};

// The wordmark, the search field, then favourites, the parser panel and the gear.
export function Header({
  ref,
  frost,
  query,
  onQueryChange,
  onReset,
  favOnly,
  onToggleFavOnly,
  onOpenAdmin,
}: Props) {
  const { t } = useLang();
  const { count: favCount } = useFavorites();

  // Stuck is read off a zero-height sentinel above the bar, not a scroll
  // listener; the 1px margin keeps it inside the viewport at rest.
  const sentinelRef = useRef<HTMLDivElement | null>(null);
  const [isStuck, setIsStuck] = useState(false);

  useEffect(() => {
    const node = sentinelRef.current;
    if (!node) return;
    const obs = new IntersectionObserver(
      ([entry]) => setIsStuck(!entry.isIntersecting),
      { rootMargin: "1px 0px 0px 0px", threshold: 0 },
    );
    obs.observe(node);
    return () => obs.disconnect();
  }, []);

  return (
    <>
    <div ref={sentinelRef} aria-hidden="true" className="h-0 w-full" />
    {/* Full-bleed ground; the inner container carries the hairline, so the
        rule ends where the text does. */}
    <header
      ref={ref}
      className={cn(
        "sticky top-0 z-30 pt-[env(safe-area-inset-top)]",
        "transition-[background-color,box-shadow,backdrop-filter] duration-300 ease-out",
        // Frosted only while the page is under it.
        frost && isStuck
          ? "bg-[var(--color-paper)]/85 backdrop-blur-xs shadow-[var(--shadow-header)]"
          : "bg-[var(--color-paper)]",
      )}
    >
      <div
        className={cn(
          "mx-auto max-w-[1400px] swiss-rule",
          "pl-[max(1.5rem,env(safe-area-inset-left))]",
          "pr-[max(1.5rem,env(safe-area-inset-right))]",
          "sm:pl-[max(2.5rem,env(safe-area-inset-left))]",
          "sm:pr-[max(2.5rem,env(safe-area-inset-right))]",
          "lg:pl-[max(4rem,env(safe-area-inset-left))]",
          "lg:pr-[max(4rem,env(safe-area-inset-right))]",
          // Mobile wraps into two rows (see --header-h): brand + controls on
          // the first, search on the second — one row left it ~110px wide.
          "h-[var(--header-h)] gap-x-3 gap-y-2 sm:gap-6 items-center content-center",
          "flex flex-wrap sm:flex-nowrap lg:grid lg:grid-cols-[1fr_auto_1fr]",
        )}
      >
        {/* The wordmark resets the page. */}
        <button
          type="button"
          onClick={onReset}
          className="order-1 sm:order-none flex items-center gap-2 shrink-0 select-none cursor-pointer lg:justify-self-start"
        >
          <span className="font-semibold text-[17px] tracking-[-0.01em]">
            RuTracker
          </span>
        </button>

        {/* The field shows focus itself: a 2px accent bar on its left edge. */}
        <div className="order-3 sm:order-none basis-full sm:basis-auto flex-1 min-w-0 lg:flex-none lg:w-[42rem] lg:max-w-full">
          <div
            className={cn(
              "group relative flex items-center gap-2 h-9 px-3 rounded-md",
              "bg-[var(--color-paper-soft)]",
              "border-l-2 border-transparent",
              "focus-within:border-[var(--color-accent)]",
              "transition-colors duration-150",
            )}
          >
            <Search className="size-4 text-[var(--color-ink-muted)] shrink-0" />
            <input
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              placeholder={t.searchPlaceholder}
              name="search"
              autoComplete="off"
              className="flex-1 min-w-0 bg-transparent outline-none placeholder:text-[var(--color-ink-muted)] text-[14px]"
            />
            {query && (
              <button
                type="button"
                onClick={() => onQueryChange("")}
                aria-label={t.searchClearAria}
                className={cn(
                  "size-6 grid place-items-center shrink-0",
                  "text-[var(--color-ink-muted)]",
                  "hover:text-[var(--color-ink)]",
                  "transition-colors",
                )}
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>
        </div>

        <div className="order-2 sm:order-none ml-auto sm:ml-0 flex items-center shrink-0 lg:justify-self-end">
          <HeaderButton
            onClick={onToggleFavOnly}
            ariaLabel={t.favoritesToggleAria}
            pressed={favOnly}
          >
            <Star
              className="size-3.5"
              strokeWidth={2}
              fill={favOnly ? "currentColor" : "none"}
            />
            <span className="hidden sm:inline">
              {t.favoritesTitle}
            </span>
            {favCount > 0 && (
              <span className="tabular-nums text-[var(--color-ink-muted)] ml-0.5">
                {favCount > 99 ? "99+" : favCount}
              </span>
            )}
          </HeaderButton>

          <VRule />

          <HeaderButton
            onClick={onOpenAdmin}
            ariaLabel={t.adminOpenAria}
          >
            <Database className="size-3.5" strokeWidth={2} />
          </HeaderButton>

          {/* The rule stands 12/16px before the gear's icon, as before a text button. */}
          <span aria-hidden="true" className="h-4 w-px mr-3 sm:mr-4 bg-[var(--color-rule)]" />
          <SettingsMenu />
        </div>
      </div>
    </header>
    </>
  );
}

function HeaderButton({
  onClick,
  ariaLabel,
  pressed,
  children,
}: {
  onClick: () => void;
  ariaLabel: string;
  pressed?: boolean;
  children: React.ReactNode;
}) {
  return (
    <Tooltip text={ariaLabel}>
    <button
      type="button"
      onClick={onClick}
      aria-label={ariaLabel}
      aria-pressed={pressed}
      className={cn(
        "flex items-center gap-1.5 h-10 px-3 sm:px-4",
        "text-[11px] font-semibold uppercase tracking-[0.08em]",
        "text-[var(--color-ink-soft)]",
        "hover:text-[var(--color-ink)]",
        pressed && "text-[var(--color-accent)] hover:text-[var(--color-accent)]",
        "transition-colors duration-150",
      )}
    >
      {children}
    </button>
    </Tooltip>
  );
}

function VRule() {
  return (
    <span
      aria-hidden="true"
      className="h-4 w-px bg-[var(--color-rule)]"
    />
  );
}
