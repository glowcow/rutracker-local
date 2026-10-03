import { useLang } from "../lib/i18n";
import { cn } from "../lib/cn";
import { Tooltip } from "./Tooltip";

// The build passes the git tag; the footer adds its own "v".
const VERSION = (import.meta.env.VITE_APP_VERSION ?? "dev").replace(/^v/, "");
const COMMIT = import.meta.env.VITE_APP_COMMIT ?? "local";
const BUILD_DATE = (import.meta.env.VITE_APP_BUILD_DATE ?? new Date().toISOString()).slice(0, 10);

// Swiss footer — single hairline above, slash-separated metadata, eyebrow
// caps for the host wordmark and tabular numbers for the build stamps.
export function Footer() {
  const { t } = useLang();
  return (
    <footer
      className={cn(
        "mx-auto max-w-[1400px] px-6 sm:px-10 lg:px-16 mt-4",
        "pb-[max(1.5rem,env(safe-area-inset-bottom))]",
      )}
    >
      <div className="swiss-rule-top py-5 px-3 sm:px-4 flex flex-wrap items-center justify-between gap-x-2 sm:gap-x-4 gap-y-1.5 text-[11.5px] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)] tabular-nums">
        <span className="font-semibold text-[13px] tracking-[-0.01em] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]">
          RuTracker Local:
        </span>
        <div className="flex flex-wrap items-center gap-x-2 sm:gap-x-3 gap-y-1">
          <span>v{VERSION}</span>
          <span className="text-[var(--color-rule)] dark:text-[var(--color-dark-rule)]">/</span>
          <Tooltip text={COMMIT}>
            <span className="font-mono">{COMMIT.slice(0, 7)}</span>
          </Tooltip>
          <span className="text-[var(--color-rule)] dark:text-[var(--color-dark-rule)]">/</span>
          {/* "built" label is dropped on mobile — the Russian "собрано" is
              20px wider than it and pushed the whole row onto a second line. */}
          <span>
            <span className="hidden sm:inline">{t("footer_built")} </span>
            {BUILD_DATE}
          </span>
        </div>
      </div>
    </footer>
  );
}
