import { X } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { Tooltip } from "./Tooltip";

export type Chip = {
  id: string;
  label: string;
  value: string;
  // The full form of a value shown truncated, e.g. a forum's whole path.
  tooltip?: string;
};

type Props = {
  chips: Chip[];
  onRemove: (id: string) => void;
  onClearAll: () => void;
};

// A chip is plain text: eyebrow label, value, a cross to remove it.
export function FilterChips({ chips, onRemove, onClearAll }: Props) {
  const { t } = useLang();
  if (chips.length === 0) return null;
  return (
    <div className="flex flex-wrap items-center gap-x-4 gap-y-1.5 min-w-0">
      {chips.map((c) => (
        // Two triggers side by side, each with its own tooltip: Radix wants one
        // trigger per root.
        <div
          key={c.id}
          className="flex items-center gap-1 text-[12px] min-w-0 max-w-full text-[var(--color-ink)]"
        >
          <Tooltip text={c.tooltip}>
            <div className="flex items-baseline gap-2 min-w-0 cursor-default">
              {/* Label is dropped below lg — the value alone is unambiguous
                  there, and each result card repeats the forum anyway. */}
              <span className="swiss-eyebrow text-[10px] leading-none shrink-0 hidden lg:inline">
                {c.label}:
              </span>
              {/* Clips only once the row runs out of room; no fixed cap. */}
              <span className="font-medium leading-none truncate">{c.value}</span>
            </div>
          </Tooltip>
          <Tooltip text={t.chipRemoveFilter}>
            <button
              type="button"
              onClick={() => onRemove(c.id)}
              aria-label={t.chipRemoveFilter}
              // -my-1 shrinks the 20px hit area's layout box to the 12px
              // text line so the ✕ centres on the value instead of sitting low.
              className="group shrink-0 grid place-items-center size-5 -my-1"
            >
              {/* 14 px: at 12 the cross crowds the 12 px text beside it. */}
              <X
                strokeWidth={2.25}
                className={cn(
                  "size-3.5",
                  "text-[var(--color-ink-muted)]",
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
            "text-[var(--color-ink-soft)]",
            "hover:text-[var(--color-accent)] transition-colors",
          )}
        >
          {t.chipClearAll}
        </button>
      )}
    </div>
  );
}
