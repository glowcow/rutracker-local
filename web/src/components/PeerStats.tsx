import { ArrowUp, ArrowDown, Loader2, Users } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "../lib/cn";
import { useLang, type Dict } from "../lib/i18n";
import type { PeersError, PeersState } from "../lib/peers";
import { MetaLabel } from "./MetaRow";

// Live seeders/leechers layer (values from the 24h-TTL torrent_peers cache).
// Colour: seeders green / leechers red = trusted/current; both grey = a refresh
// was attempted and FAILED (value known-unreliable). Grey is driven by `error`,
// not age — a 5h-old cache we didn't re-fetch stays coloured.

type TFn = (k: keyof Dict) => string;

const greyCls = "text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]";

// "только что" / "5 ч назад" — short units so no RU pluralisation is needed.
function timeAgo(iso: string, t: TFn): string {
  const min = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60000));
  if (min < 1) return t("peers_now");
  if (min < 60) return `${min} ${t("peers_min")} ${t("peers_ago")}`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h} ${t("peers_hour")} ${t("peers_ago")}`;
  return `${Math.floor(h / 24)} ${t("peers_day")} ${t("peers_ago")}`;
}

function reasonLabel(error: PeersError | null, t: TFn): string {
  if (error === "auth") return t("peers_err_auth");
  if (error === "unavailable") return t("peers_err_unavailable");
  return t("peers_stale");
}

// The ↑seeders ↓leechers pair. `grey` desaturates both to signal a stale
// (failed-refresh) value.
// Icon shared class: inline + em-sized so it scales with the inherited font,
// nudged down onto the digits' baseline so the whole thing reads as one line
// of text (not a flex box that floats above the caption beside it).
const numIcon = "inline size-[1.05em] align-[-0.15em]";

function Numbers({ seeders, leechers, grey }: { seeders: number; leechers: number; grey?: boolean }) {
  // Plain inline text (no flex) — inherits the surrounding font-size (13px
  // drawer meta / 12px list rail) and sits on the same baseline as neighbours.
  return (
    <span className="tabular-nums font-medium whitespace-nowrap">
      <span className={grey ? greyCls : "text-emerald-600 dark:text-emerald-400"}>
        <ArrowUp className={numIcon} strokeWidth={2.5} />
        {seeders}
      </span>
      <span className={cn("ml-2.5", grey ? greyCls : "text-red-600 dark:text-red-400")}>
        <ArrowDown className={numIcon} strokeWidth={2.5} />
        {leechers}
      </span>
    </span>
  );
}

// Drawer variant — a row in the detail <dl>, matching the Folder/Size/Hash
// rows (eyebrow label + value), with a freshness/error caption beneath.
export function PeerStatsRow({ state }: { state: PeersState }) {
  const { t } = useLang();
  if (!state.configured) return null;

  const has = state.seeders !== null && state.leechers !== null;

  // lead = the numbers / dash / spinner; caption = the freshness or reason
  // text that follows after a " / " on the SAME line.
  let lead: ReactNode;
  let caption: string | null = null;

  if (state.loading && !has) {
    lead = (
      <span className={greyCls}>
        <Loader2 className={cn(numIcon, "mr-1.5 animate-spin")} />
        {t("peers_loading")}
      </span>
    );
  } else if (state.loading && has) {
    lead = <Numbers seeders={state.seeders!} leechers={state.leechers!} grey />;
    caption = t("peers_loading");
  } else if (!has) {
    // Nothing cached and (if error) the fresh fetch failed too.
    lead = <span className={greyCls}>—</span>;
    caption = state.error ? reasonLabel(state.error, t) : null;
  } else if (state.error) {
    // Cache present but the refresh failed → grey, with reason.
    lead = <Numbers seeders={state.seeders!} leechers={state.leechers!} grey />;
    caption = `${state.checkedAt ? `${timeAgo(state.checkedAt, t)} · ` : ""}${reasonLabel(state.error, t)}`;
  } else {
    // Live/current value.
    lead = <Numbers seeders={state.seeders!} leechers={state.leechers!} />;
    caption = `${t("peers_updated")} ${state.checkedAt ? timeAgo(state.checkedAt, t) : t("peers_now")}`;
  }

  return (
    // Two cells of the drawer's meta grid: numbers, then the freshness/reason
    // after a " / " separator (mirrors the drawer's "Торрент / #id" idiom).
    <>
      <MetaLabel icon={<Users className="size-3.5" />} label={t("peers_label")} />
      {/* One continuous inline line (not flex) — numbers, "/", and caption
          share a single text baseline; truncate clips the caption tail. */}
      <dd className="min-w-0 leading-5 truncate">
        {lead}
        {caption && (
          <span className={greyCls}>
            <span className="mx-2">/</span>
            {caption}
          </span>
        )}
      </dd>
    </>
  );
}

// List variant — compact badge for the ResultCard right rail. The list only
// ever reads cache (no fetch), so there's no loading state here; `stale`
// greys a value we know is past its window.
export function PeerBadge({
  seeders,
  leechers,
  stale,
}: {
  seeders: number | null;
  leechers: number | null;
  stale?: boolean;
}) {
  if (seeders === null || leechers === null) {
    return <span className={cn("tabular-nums", greyCls)}>—</span>;
  }
  return <Numbers seeders={seeders} leechers={leechers} grey={stale} />;
}
