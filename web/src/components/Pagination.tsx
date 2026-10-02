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

  const go = (p: number) => {
    const clamped = Math.max(0, Math.min(p, lastPage));
    if (clamped !== page) {
      onChange(clamped);
    }
  };

  return (
    <nav
      aria-label={t("pagination_aria")}
      className="flex flex-wrap items-center justify-center gap-3 py-5 px-3 sm:px-4 select-none"
    >
      <StepButton
        onClick={() => go(page - 1)}
        disabled={page === 0}
        aria-label={t("pagination_prev")}
      >
        <ChevronLeft className="size-3.5" />
      </StepButton>

      {pages.map((p, i) =>
        p === "…" ? (
          <span
            key={`gap-${i}`}
            className="text-[12px] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)] tabular-nums"
            aria-hidden="true"
          >
            …
          </span>
        ) : (
          <PageButton key={p} active={p === page} onClick={() => go(p)}>
            {p + 1}
          </PageButton>
        ),
      )}

      <StepButton
        onClick={() => go(page + 1)}
        disabled={page === lastPage}
        aria-label={t("pagination_next")}
      >
        <ChevronRight className="size-3.5" />
      </StepButton>
    </nav>
  );
}

function buildPageWindow(page: number, lastPage: number): (number | "…")[] {
  const totalPages = lastPage + 1;
  if (totalPages <= 7) {
    return Array.from({ length: totalPages }, (_, i) => i);
  }
  const window: (number | "…")[] = [];
  const add = (p: number | "…") => {
    if (typeof p === "number" && (p < 0 || p > lastPage)) return;
    if (window[window.length - 1] === p) return;
    window.push(p);
  };

  add(0);
  if (page > 2) add("…");
  for (let p = page - 1; p <= page + 1; p++) add(p);
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
          : "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)] hover:text-[var(--color-ink)] dark:hover:text-[var(--color-dark-ink)]",
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
          : "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)] hover:text-[var(--color-accent)]",
      )}
    >
      {children}
    </button>
  );
}
