import { useLang } from "../lib/i18n";
import { Tooltip } from "./Tooltip";

// The build passes the git tag; the footer adds its own "v".
const VERSION = (import.meta.env.VITE_APP_VERSION ?? "dev").replace(/^v/, "");
const COMMIT = import.meta.env.VITE_APP_COMMIT ?? "local";
const BUILD_DATE = (import.meta.env.VITE_APP_BUILD_DATE ?? new Date().toISOString()).slice(0, 10);

// One hairline above the build line. No w-full: in the page's flex column the
// footer shrinks to its text, so the hairline is as wide as the line.
export function Footer() {
  const { t } = useLang();
  return (
    <footer className="mx-auto max-w-[1400px] px-6 sm:px-10 lg:px-16 mt-4 pb-[max(1.5rem,env(safe-area-inset-bottom))]">
      <div className="swiss-rule-top py-5 px-3 sm:px-4 flex flex-col items-center gap-y-1 text-[11.5px] text-[var(--color-ink-muted)] tabular-nums">
        <span className="flex items-center gap-x-2 sm:gap-x-3 whitespace-nowrap">
          <span className="font-semibold text-[13px] tracking-[-0.01em] text-[var(--color-ink)]">
            RuTracker Local:
          </span>
          <span>v{VERSION}</span>
          {/* The hash is the first stamp to go on a narrow screen. */}
          <span className="hidden sm:inline text-[var(--color-rule)]">/</span>
          <Tooltip text={COMMIT}>
            <span className="hidden sm:inline font-mono">{COMMIT.slice(0, 7)}</span>
          </Tooltip>
          <span className="text-[var(--color-rule)]">/</span>
          <span>
            <span className="hidden sm:inline">{t.footerBuilt} </span>
            {BUILD_DATE}
          </span>
        </span>
      </div>
    </footer>
  );
}
