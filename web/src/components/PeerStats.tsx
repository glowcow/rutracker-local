import { ArrowUp, ArrowDown, Loader2, Users } from "lucide-react";
import type { ReactNode } from "react";
import { cn } from "../lib/cn";
import { useLang, type Dict } from "../lib/i18n";
import type { PeersError, PeersState } from "../lib/peers";
import { MetaLabel } from "./MetaRow";

// Seeders in `up`, leechers in `down` while the value is current; both grey
// when a refresh was tried and failed — by `error`, not by age.

const greyCls = "text-[var(--color-ink-muted)]";

// "только что" / "5 ч назад" — short units so no RU pluralisation is needed.
function timeAgo(iso: string, t: Dict): string {
  const min = Math.max(0, Math.floor((Date.now() - new Date(iso).getTime()) / 60000));
  if (min < 1) return t.peersNow;
  if (min < 60) return `${min} ${t.peersMin} ${t.peersAgo}`;
  const h = Math.floor(min / 60);
  if (h < 24) return `${h} ${t.peersHour} ${t.peersAgo}`;
  return `${Math.floor(h / 24)} ${t.peersDay} ${t.peersAgo}`;
}

function reasonLabel(error: PeersError | null, t: Dict): string {
  if (error === "auth") return t.peersErrAuth;
  if (error === "unavailable") return t.peersErrUnavailable;
  return t.peersStale;
}

// Inline and em-sized, so the icons scale with the text and sit on the
// digits' baseline as one line.
const numIcon = "inline size-[1.05em] align-[-0.15em]";

function Numbers({ seeders, leechers, grey }: { seeders: number; leechers: number; grey?: boolean }) {
  // Plain inline text (no flex) — inherits the surrounding font-size (13px
  // drawer meta / 12px list rail) and sits on the same baseline as neighbours.
  return (
    <span className="tabular-nums font-medium whitespace-nowrap">
      <span className={grey ? greyCls : "text-[var(--color-up)]"}>
        <ArrowUp className={numIcon} strokeWidth={2.5} />
        {seeders}
      </span>
      <span className={cn("ml-2.5", grey ? greyCls : "text-[var(--color-down)]")}>
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
        {t.peersLoading}
      </span>
    );
  } else if (state.loading && has) {
    lead = <Numbers seeders={state.seeders!} leechers={state.leechers!} grey />;
    caption = t.peersLoading;
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
    caption = `${t.peersUpdated} ${state.checkedAt ? timeAgo(state.checkedAt, t) : t.peersNow}`;
  }

  return (
    // Two cells of the drawer's meta grid: numbers, then the freshness/reason
    // after a " / " separator (mirrors the drawer's "Торрент / #id" idiom).
    <>
      <MetaLabel icon={<Users className="size-3.5" />} label={t.peersLabel} />
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

// The list's badge reads the cache only: no loading state; `stale` greys
// a value past its window.
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
