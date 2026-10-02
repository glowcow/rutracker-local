import { Search, Sun, Moon, X, Star, Database } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { useFavorites } from "../lib/favorites";
import { Tooltip } from "./Tooltip";

type Props = {
  // App measures the header to place the sticky meta-row under it.
  ref?: React.Ref<HTMLElement>;
  theme: "light" | "dark";
  onToggleTheme: () => void;
  query: string;
  onQueryChange: (v: string) => void;
  onReset: () => void;
  favOnly: boolean;
  onToggleFavOnly: () => void;
  onOpenAdmin: () => void;
};

// Swiss header — flat ground, one hairline rule along the bottom. No
// glass/blur. Brand is a small wordmark left, search is a flat filled input
// in the centre column (subtle 6px radius, no pill), controls are
// text-buttons on the right with caps tracking instead of icon-only pills.
export function Header({
  ref,
  theme,
  onToggleTheme,
  query,
  onQueryChange,
  onReset,
  favOnly,
  onToggleFavOnly,
  onOpenAdmin,
}: Props) {
  const { lang, setLang, t } = useLang();
  const { count: favCount } = useFavorites();

  return (
    // Outer <header> = full-bleed paper bg (sticky bar covers the viewport edge
    // to edge); inner container is max-w + <main>'s padding so wordmark/search
    // and the hairline align with the content rules. Hairline on the inner
    // container so it sits inside the type column, not edge-to-edge.
    <header
      ref={ref}
      className={cn(
        "sticky top-0 z-30",
        "bg-[var(--color-paper)] dark:bg-[var(--color-dark-paper)]",
        "pt-[env(safe-area-inset-top)]",
      )}
    >
      <div
        className={cn(
          "mx-auto max-w-[1400px] swiss-rule",
          "px-6 sm:px-10 lg:px-16",
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
        {/* Brand wordmark — flat text, no logo box. Click resets state. */}
        <button
          type="button"
          onClick={onReset}
          className="order-1 sm:order-none flex items-center gap-2 shrink-0 select-none cursor-pointer lg:justify-self-start"
        >
          <span className="font-semibold text-[17px] tracking-[-0.01em]">
            RuTracker
          </span>
        </button>

        {/* Search input — flat paper-soft fill, no border (the previous
            underline doubled awkwardly with the header's own bottom rule).
            Focus state lifts a 2px accent bar on the LEFT edge: subtle but
            unambiguous, fits the Swiss vocab. */}
        <div className="order-3 sm:order-none basis-full sm:basis-auto flex-1 min-w-0 lg:flex-none lg:w-[42rem] lg:max-w-full">
          <div
            className={cn(
              "group relative flex items-center gap-2 h-9 px-3 rounded-md",
              "bg-[var(--color-paper-soft)] dark:bg-[var(--color-dark-paper-soft)]",
              "border-l-2 border-transparent",
              "focus-within:border-[var(--color-accent)]",
              "transition-colors duration-150",
            )}
          >
            <Search className="size-4 text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)] shrink-0" />
            <input
              value={query}
              onChange={(e) => onQueryChange(e.target.value)}
              placeholder={t("search_placeholder")}
              name="search"
              autoComplete="off"
              className="flex-1 min-w-0 bg-transparent outline-none placeholder:text-[var(--color-ink-muted)] dark:placeholder:text-[var(--color-dark-ink-muted)] text-[14px]"
            />
            {query && (
              <button
                type="button"
                onClick={() => onQueryChange("")}
                aria-label={t("search_clear_aria")}
                className={cn(
                  "size-6 grid place-items-center shrink-0",
                  "text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]",
                  "hover:text-[var(--color-ink)] dark:hover:text-[var(--color-dark-ink)]",
                  "transition-colors",
                )}
              >
                <X className="size-3.5" />
              </button>
            )}
          </div>
        </div>

        {/* Right rail — text controls with caps tracking, separated by
            vertical hairlines. No pill / chip backgrounds. */}
        <div className="order-2 sm:order-none ml-auto sm:ml-0 flex items-center shrink-0 lg:justify-self-end">
          <HeaderButton
            onClick={onToggleFavOnly}
            ariaLabel={t("favorites_toggle_aria")}
            pressed={favOnly}
          >
            <Star
              className="size-3.5"
              strokeWidth={2}
              fill={favOnly ? "currentColor" : "none"}
            />
            <span className="hidden sm:inline">
              {t("favorites_title")}
            </span>
            {favCount > 0 && (
              <span className="tabular-nums text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)] ml-0.5">
                {favCount > 99 ? "99+" : favCount}
              </span>
            )}
          </HeaderButton>

          <VRule />

          <HeaderButton
            onClick={() => setLang(lang === "ru" ? "en" : "ru")}
            ariaLabel={t("lang_toggle_aria")}
          >
            <span className="tabular-nums">{lang === "ru" ? "RU" : "EN"}</span>
          </HeaderButton>

          <VRule />

          <HeaderButton
            onClick={onToggleTheme}
            ariaLabel={t("theme_toggle_aria")}
          >
            {theme === "dark" ? (
              <Sun className="size-3.5" />
            ) : (
              <Moon className="size-3.5" />
            )}
          </HeaderButton>

          <VRule />

          <HeaderButton
            onClick={onOpenAdmin}
            ariaLabel={t("admin_open_aria")}
          >
            <Database className="size-3.5" strokeWidth={2} />
          </HeaderButton>
        </div>
      </div>
    </header>
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
        "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)]",
        "hover:text-[var(--color-ink)] dark:hover:text-[var(--color-dark-ink)]",
        pressed && "text-[var(--color-accent)] dark:text-[var(--color-accent)]",
        "transition-colors",
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
      className="h-4 w-px bg-[var(--color-rule)] dark:bg-[var(--color-dark-rule)]"
    />
  );
}
