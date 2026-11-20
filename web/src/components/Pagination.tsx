import { ChevronLeft, ChevronRight } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";

type Props = {
  page: number;          // 0-based
  pageSize: number;
  total: number;
  onChange: (page: number) => void;
};

// Swiss pagination — flat text-buttons with caps tracking, sharp corners,
// no chip backgrounds. Hairline above separates it from the row list.
export function Pagination({ page, pageSize, total, onChange }: Props) {
  const { t } = useLang();
  const lastPage = Math.max(0, Math.ceil(total / pageSize) - 1);
  if (lastPage === 0) return null;

  const pages = buildPageWindow(page, lastPage);

  const go = (target: number) => {
    const clamped = Math.max(0, Math.min(target, lastPage));
    if (clamped !== page) {
      onChange(clamped);
    }
  };

  return (
    <nav
      aria-label={t.paginationAria}
      className="flex flex-wrap items-center justify-center gap-3 py-5 px-3 sm:px-4 select-none"
    >
      <StepButton
        onClick={() => go(page - 1)}
        disabled={page === 0}
        aria-label={t.paginationPrev}
      >
        <ChevronLeft className="size-3.5" />
      </StepButton>

      {pages.map((entry, index) =>
        entry === "…" ? (
          <span
            key={`gap-${index}`}
            className="text-[12px] text-[var(--color-ink-muted)] tabular-nums"
            aria-hidden="true"
          >
            …
          </span>
        ) : (
          <PageButton key={entry} active={entry === page} onClick={() => go(entry)}>
            {entry + 1}
          </PageButton>
        ),
      )}

      <StepButton
        onClick={() => go(page + 1)}
        disabled={page === lastPage}
        aria-label={t.paginationNext}
      >
        <ChevronRight className="size-3.5" />
      </StepButton>
    </nav>
  );
}

function buildPageWindow(page: number, lastPage: number): (number | "…")[] {
  const totalPages = lastPage + 1;
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, index) => index);
  }
  const window: (number | "…")[] = [];
  const add = (entry: number | "…") => {
    if (typeof entry === "number" && (entry < 0 || entry > lastPage)) return;
    if (window[window.length - 1] === entry) return;
    window.push(entry);
  };

  add(0);
  if (page > 2) add("…");
  for (let near = page - 1; near <= page + 1; near++) add(near);
  if (page < lastPage - 2) add("…");
  add(lastPage);
  return window;
}

function PageButton({
  active,
  onClick,
  children,
}: {
  active: boolean;
  onClick: () => void;
  children: React.ReactNode;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      aria-current={active ? "page" : undefined}
      className={cn(
        "min-w-6 text-[13px] tabular-nums transition-colors",
        active
          ? "text-[var(--color-accent)] font-semibold"
          : "text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]",
      )}
    >
      {children}
    </button>
  );
}

function StepButton({
  onClick,
  disabled,
  children,
  ...rest
}: {
  onClick: () => void;
  disabled?: boolean;
  children: React.ReactNode;
  "aria-label"?: string;
}) {
  return (
    <button
      type="button"
      onClick={onClick}
      disabled={disabled}
      {...rest}
      className={cn(
        "size-7 grid place-items-center transition-colors",
        disabled
          ? "opacity-30 cursor-not-allowed"
          : "text-[var(--color-ink-soft)] hover:text-[var(--color-accent)]",
      )}
    >
      {children}
    </button>
  );
}
