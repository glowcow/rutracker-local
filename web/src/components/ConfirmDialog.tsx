import { useEffect } from "react";
import { X } from "lucide-react";
import { useLang } from "../lib/i18n";
import { cn } from "../lib/cn";

type Props = {
  open: boolean;
  /** One line, set as an eyebrow. */
  title: string;
  /** What will happen, in a sentence. */
  body: string;
  /** The primary button says what it does, never "OK". */
  confirmLabel: string;
  onConfirm: () => void;
  onClose: () => void;
};

const BUTTON = cn(
  "flex-1 basis-0 h-11 px-4 rounded-md transition-colors duration-150",
  "text-[11px] font-semibold uppercase tracking-[0.08em]",
);

// The question before a destructive action. Always mounted and opened by CSS,
// like the other modals; closes on Escape and on a click outside.
export function ConfirmDialog({ open, title, body, confirmLabel, onConfirm, onClose }: Props) {
  const { t } = useLang();

  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") onClose();
    };
    document.addEventListener("keydown", onKey);
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.removeEventListener("keydown", onKey);
      document.body.style.overflow = prev;
    };
  }, [open, onClose]);

  return (
    <div
      inert={!open}
      className={cn(
        "fixed inset-0 z-40 grid place-items-center p-4",
        open ? "swiss-drawer-open" : "swiss-drawer-closed",
      )}
    >
      <div
        aria-hidden="true"
        onClick={onClose}
        className="swiss-drawer-backdrop absolute inset-0 bg-[var(--color-scrim)] backdrop-blur-xs"
      />
      <div
        role="alertdialog"
        aria-modal="true"
        aria-labelledby="confirm-title"
        aria-describedby="confirm-body"
        className={cn(
          "swiss-drawer-panel relative z-50 w-full max-w-md overflow-hidden rounded-lg",
          "border border-[var(--color-rule)] bg-[var(--color-paper)]",
        )}
      >
        <div className="swiss-rule h-14 sm:h-16 pl-5 sm:pl-6 pr-2 sm:pr-3 flex items-center justify-between gap-3">
          <h2 id="confirm-title" className="swiss-eyebrow truncate">
            {title}
          </h2>
          <button
            type="button"
            onClick={onClose}
            aria-label={t("cancel")}
            className="size-10 grid place-items-center shrink-0 rounded-md text-[var(--color-ink-soft)] hover:text-[var(--color-ink)] transition-colors duration-150"
          >
            <X aria-hidden="true" className="size-4 shrink-0" />
          </button>
        </div>
        <div className="p-5 sm:p-6">
          <p id="confirm-body" className="text-[14px] leading-relaxed">
            {body}
          </p>
          <div className="mt-6 flex gap-3">
            <button
              type="button"
              onClick={onClose}
              className={cn(
                BUTTON,
                "border border-[var(--color-rule)]",
                "hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]",
              )}
            >
              {t("cancel")}
            </button>
            <button
              type="button"
              onClick={() => {
                onConfirm();
                onClose();
              }}
              className={cn(
                BUTTON,
                "bg-[var(--color-accent)] text-[var(--color-on-accent)] hover:bg-[var(--color-accent-hover)]",
              )}
            >
              {confirmLabel}
            </button>
          </div>
        </div>
      </div>
    </div>
  );
}
