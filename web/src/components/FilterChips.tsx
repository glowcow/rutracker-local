import { X } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { Tooltip } from "./Tooltip";

export type Chip = {
  id: string;
  label: string;
  value: string;
  // Optional full-text tooltip — set when value is a truncated display
  // form (e.g. forum leaf name) and the user might want to see the
  // complete path (full forum breadcrumb) on hover.
  tooltip?: string;
};

type Props = {
  chips: Chip[];
  onRemove: (id: string) => void;
  onClearAll: () => void;
};

// Swiss filter chips — eyebrow label / colon / value as plain text with an
// inline ✕ to remove. No background, no rounded pill. Active state reads
// as "small typographic tag" rather than a UI control.
export function FilterChips({ chips, onRemove, onClearAll }: Props) {
  const { t } = useLang();
  if (chips.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 min-w-0">
      {chips.map((c) => (
        // Chip = two surfaces, each with its own tooltip: label+value shows the
        // full breadcrumb (informational), the ✕ is the remove action. Sibling
        // Tooltips (not nested) — Radix wants one trigger per Tooltip.Root.
        <div
          key={c.id}
          className="flex items-center gap-1 text-[12px] min-w-0 max-w-full text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]"
        >
          <Tooltip text={c.tooltip}>
            <div className="flex items-baseline gap-2 min-w-0 cursor-default">
              {/* Label is dropped below lg — the value alone is unambiguous
                  there, and each result card repeats the forum anyway. */}
              <span className="swiss-eyebrow text-[10px] leading-none shrink-0 hidden lg:inline">
                {c.label}:
              </span>
              {/* Value clips with ellipsis only once the row actually runs
                  out of room (min-w-0 chain above) — no fixed cap, which used
                  to truncate at 260px with half the row still empty. */}
              <span className="font-medium leading-none truncate">{c.value}</span>
            </div>
          </Tooltip>
          <Tooltip text={t("chip_remove_filter")}>
            <button
              type="button"
              onClick={() => onRemove(c.id)}
              aria-label={t("chip_remove_filter")}
              // -my-1 shrinks the 20px hit area's layout box to the 12px
              // text line so the ✕ centres on the value instead of sitting low.
              className="group shrink-0 grid place-items-center size-5 -my-1"
            >
              {/* ✕ sized 14 px (size-3.5) — 12 px read as crowded next to
                  12 px text, and lucide's X strokes at size-3 felt visually
                  off-axis. */}
              <X
                strokeWidth={2.25}
                className={cn(
                  "size-3.5",
                  "text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]",
                  "group-hover:text-[var(--color-accent)] transition-colors",
                )}
              />
            </button>
          </Tooltip>
        </div>
      ))}
      {chips.length > 1 && (
        <button
          onClick={onClearAll}
          className={cn(
            "swiss-eyebrow",
            "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)]",
            "hover:text-[var(--color-accent)] transition-colors",
          )}
        >
          {t("chip_clear_all")}
        </button>
      )}
    </div>
  );
}
