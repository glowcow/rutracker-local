import { ArrowDown, ArrowUp } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang, type Dict } from "../lib/i18n";

export type SortKey = "relevance" | "date" | "size";
export type SortDir = "desc" | "asc";
export type Sort = { key: SortKey; dir: SortDir };

const OPTIONS: { key: SortKey; tKey: keyof Dict }[] = [
  { key: "relevance", tKey: "sort_relevance" },
  { key: "date", tKey: "sort_date" },
  // Seeders aren't in the rutracker XML dump (dump = static metadata, no
  // live tracker stats), so we sort by Size — which we do have.
  { key: "size", tKey: "sort_size" },
];

type Props = {
  value: Sort;
  onChange: (sort: Sort) => void;
};

// Swiss sort — eyebrow label, text-button options with caps tracking,
// vertical hairlines between. Active uses the accent; an arrow appears
// only on directional sorts. flex-nowrap: wrapping stranded a bare
// separator at a line end once the Russian labels overflowed.
export function SortControl({ value, onChange }: Props) {
  const { t } = useLang();
  return (
    <div className="flex items-center gap-3 flex-nowrap">
      <span className="swiss-eyebrow hidden lg:inline">{t("sort_label")}</span>
      <div className="flex items-center gap-3">
        {OPTIONS.map((opt, i) => {
          const active = value.key === opt.key;
          const directional = opt.key !== "relevance";
          return (
            <div key={opt.key} className="flex items-center gap-3">
              {i > 0 && (
                <span
                  aria-hidden="true"
                  className="h-3 w-px bg-[var(--color-rule)] dark:bg-[var(--color-dark-rule)]"
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
                // Eyebrow typography inlined (not .swiss-eyebrow, which forces
                // muted) so inactive options use high-contrast ink like the forum
                // chips. The muted .swiss-eyebrow is kept for the static "SORT".
                className={cn(
                  "flex items-center gap-1 transition-colors",
                  "text-[11px] font-semibold uppercase tracking-[0.08em]",
                  active
                    ? "text-[var(--color-accent)]"
                    : "text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] hover:text-[var(--color-accent)]",
                )}
              >
                <span>{t(opt.tKey)}</span>
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
