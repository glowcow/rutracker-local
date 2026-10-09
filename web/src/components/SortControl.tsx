import { ArrowDown, ArrowUp } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang, type Dict } from "../lib/i18n";

export type SortKey = "relevance" | "date" | "size";
export type SortDir = "desc" | "asc";
export type Sort = { key: SortKey; dir: SortDir };

const OPTIONS: { key: SortKey; label: (t: Dict) => string }[] = [
  { key: "relevance", label: (t) => t.sortRelevance },
  { key: "date", label: (t) => t.sortDate },
  // Seeders aren't in the rutracker XML dump (dump = static metadata, no
  // live tracker stats), so we sort by Size — which we do have.
  { key: "size", label: (t) => t.sortSize },
];

type Props = {
  value: Sort;
  onChange: (sort: Sort) => void;
};

// Text buttons with rules between; the active one is accent and carries an
// arrow when it has a direction. Never wraps: a rule would end a line.
export function SortControl({ value, onChange }: Props) {
  const { t } = useLang();
  return (
    <div className="flex items-center gap-3 flex-nowrap">
      <span className="swiss-eyebrow hidden lg:inline">{t.sortLabel}</span>
      <div className="flex items-center gap-3">
        {OPTIONS.map((opt, i) => {
          const active = value.key === opt.key;
          const directional = opt.key !== "relevance";
          return (
            <div key={opt.key} className="flex items-center gap-3">
              {i > 0 && (
                <span
                  aria-hidden="true"
                  className="h-3 w-px bg-[var(--color-rule)]"
                />
              )}
              <button
                onClick={() => {
                  if (active && directional) {
                    onChange({
                      key: opt.key,
                      dir: value.dir === "desc" ? "asc" : "desc",
                    });
                  } else if (!active) {
                    onChange({ key: opt.key, dir: "desc" });
                  }
                }}
                // The eyebrow's type with its own colour: idle options are ink, not muted.
                className={cn(
                  "flex items-center gap-1 transition-colors",
                  "text-[11px] font-semibold uppercase tracking-[0.08em]",
                  active
                    ? "text-[var(--color-accent)]"
                    : "text-[var(--color-ink)] hover:text-[var(--color-accent)]",
                )}
              >
                <span>{opt.label(t)}</span>
                {active && directional && (value.dir === "desc" ? (
                  <ArrowDown className="size-3" />
                ) : (
                  <ArrowUp className="size-3" />
                ))}
              </button>
            </div>
          );
        })}
      </div>
    </div>
  );
}
