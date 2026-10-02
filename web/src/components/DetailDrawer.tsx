import { useQuery, useQueryClient } from "@tanstack/react-query";
import { X, Download, Copy, Hash, Folder, HardDrive, Calendar, Check, ChevronRight, ExternalLink, HardDriveDownload, Loader2 } from "lucide-react";
import { useEffect, useRef, useState } from "react";
import { getStats, getTorrent, getTorrentFiles, getTransmissionStatus, sendToTransmission, type SearchResponse } from "../lib/api";
import { type FavoriteItem } from "../lib/favorites";
import { getPeers, type PeersState } from "../lib/peers";
import { formatBytes, formatDate } from "../lib/format";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { FavoriteStar } from "./FavoriteStar";
import { FileTree } from "./FileTree";
import { MetaLabel } from "./MetaRow";
import { PeerStatsRow } from "./PeerStats";
import { Tooltip } from "./Tooltip";

type Props = {
  // Last viewed torrent — App keeps it set after close so the drawer's
  // content stays rendered while the CSS close transition plays; `open`
  // alone drives visibility.
  torrentId: number | null;
  open: boolean;
  onClose: () => void;
  // When set, clicking the Forum row inside the drawer triggers this with
  // the forum's id — App wires it to setForumId(+close) so the underlying
  // list is filtered as soon as the drawer dismisses.
  onForumClick: (forumId: number) => void;
};

// Transmission button lifecycle: idle → sending → (added | duplicate | error),
// then back to idle after a short window.
type TxState = "idle" | "sending" | "added" | "duplicate" | "error";

export function DetailDrawer({ torrentId, open, onClose, onForumClick }: Props) {
  // `copiedFor` instead of a boolean: the indicator is tied to the torrent
  // it was copied FROM, so reopening a different torrent within the 1.6 s
  // window can't inherit a stale "Copied" state.
  const [copiedFor, setCopiedFor] = useState<number | null>(null);
  const copiedTimer = useRef<number | null>(null);
  // Transmission send state, tied to the torrent it fired for (same reasoning
  // as copiedFor — reopening another torrent mustn't inherit a stale badge).
  const [tx, setTx] = useState<{ id: number; state: TxState }>({ id: -1, state: "idle" });
  const txTimer = useRef<number | null>(null);
  const { t } = useLang();
  const queryClient = useQueryClient();

  const { data, isLoading, error } = useQuery({
    queryKey: ["torrent", torrentId],
    queryFn: () => getTorrent(torrentId!),
    enabled: torrentId !== null,
  });

  // File listing — collapsed by default (the description below is what most
  // opens are for), so the tree is fetched only once someone expands it.
  // Keyed by torrent id, like copiedFor/tx above: switching torrents collapses
  // the section without an effect that resets state.
  const [filesOpenFor, setFilesOpenFor] = useState<number | null>(null);
  const filesOpen = torrentId !== null && filesOpenFor === torrentId;
  const {
    data: files,
    isLoading: filesLoading,
    error: filesError,
  } = useQuery({
    queryKey: ["files", torrentId],
    queryFn: () => getTorrentFiles(torrentId!),
    enabled: torrentId !== null && open && filesOpen,
  });

  // Peers feature gate (RT_PEERS_ENABLED), read off the shared stats query —
  // off → no request goes out and the row never flashes a spinner.
  const { data: stats } = useQuery({ queryKey: ["stats"], queryFn: getStats });
  const peersEnabled = stats?.peers_enabled ?? false;

  // Live peers load independently so the detail (from local PG) paints
  // instantly while the scrape — cache hit or a fresh rutracker fetch —
  // resolves in the background.
  const { data: peers } = useQuery({
    queryKey: ["peers", torrentId],
    queryFn: () => getPeers(torrentId!),
    enabled: torrentId !== null && peersEnabled,
  });
  const peersState: PeersState = peers
    ? { ...peers, loading: false }
    : { configured: peersEnabled, seeders: null, leechers: null, checkedAt: null, error: null, loading: true };

  // Transmission feature state — `configured` gates rendering the button;
  // `online` (a live probe cached ~20s server-side) toggles active vs greyed.
  // enabled:open + staleTime re-probe on each meaningful open, so a daemon
  // moved elsewhere greys the button instead of failing on click.
  const { data: txStatus } = useQuery({
    queryKey: ["transmission-status"],
    queryFn: getTransmissionStatus,
    enabled: open,
    staleTime: 15_000,
  });
  const txConfigured = txStatus?.configured ?? false;
  const txOffline = txConfigured && !(txStatus?.online ?? false);

  // Once a scrape resolves with numbers, patch every cached search page that
  // holds this torrent so its zebra badge appears the moment the drawer
  // closes — the list query is staleTime:Infinity and won't refetch on its own
  // (this was why the row stayed empty until a manual page reload).
  useEffect(() => {
    if (torrentId === null || !peers || peers.seeders === null || peers.leechers === null) {
      return;
    }
    queryClient.setQueriesData<SearchResponse>({ queryKey: ["search"] }, (old) => {
      if (!old) return old;
      let changed = false;
      const items = old.items.map((it) => {
        if (it.id !== torrentId) return it;
        changed = true;
        return {
          ...it,
          seeders: peers.seeders ?? undefined,
          leechers: peers.leechers ?? undefined,
          peers_checked_at: peers.checkedAt ?? undefined,
        };
      });
      return changed ? { ...old, items } : old;
    });
    // Same patch for the favorites list (its cache is a plain array, not the
    // {items,total} search shape) so a starred torrent's badge updates too.
    queryClient.setQueryData<FavoriteItem[]>(["favorites"], (old) => {
      if (!old) return old;
      let changed = false;
      const next = old.map((it) => {
        if (it.id !== torrentId) return it;
        changed = true;
        return {
          ...it,
          seeders: peers.seeders ?? undefined,
          leechers: peers.leechers ?? undefined,
          peers_checked_at: peers.checkedAt ?? undefined,
        };
      });
      return changed ? next : old;
    });
    // The "cached with peers" dashboard count may have just grown — refresh
    // it (cheap: the heavy stats stay server-cached, only the count re-counts).
    queryClient.invalidateQueries({ queryKey: ["stats"] });
  }, [peers, torrentId, queryClient]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape" && open) onClose();
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [onClose, open]);

  useEffect(() => {
    if (!open) return;
    const prev = document.body.style.overflow;
    document.body.style.overflow = "hidden";
    return () => {
      document.body.style.overflow = prev;
    };
  }, [open]);

  // Hash comes from the untrusted dump — only a clean 40-hex-char info-hash
  // may reach the magnet link; anything else (extra &tr= params smuggled
  // into the string, truncated values) hides the Download/magnet actions.
  const validHash = data ? /^[0-9a-f]{40}$/i.test(data.hash) : false;
  const magnet = data && validHash ? buildMagnet(data.hash, data.title) : "";
  const copied = copiedFor !== null && copiedFor === torrentId;

  const copyMagnet = async () => {
    if (!magnet || torrentId === null) return;
    await navigator.clipboard.writeText(magnet);
    setCopiedFor(torrentId);
    // Re-copy within the window restarts the timer instead of letting the
    // first one clear the fresh indicator early.
    if (copiedTimer.current !== null) window.clearTimeout(copiedTimer.current);
    copiedTimer.current = window.setTimeout(() => setCopiedFor(null), 1600);
  };

  // The tx badge only applies to the torrent it fired for; otherwise idle.
  const txState: TxState = tx.id === torrentId ? tx.state : "idle";
  const sendTx = async () => {
    if (torrentId === null || txState === "sending") return;
    setTx({ id: torrentId, state: "sending" });
    let next: TxState;
    try {
      const res = await sendToTransmission(torrentId);
      next = res.status === "duplicate" ? "duplicate" : "added";
    } catch {
      next = "error";
      // A failed send may mean the daemon went away — re-probe so the button
      // greys out rather than staying clickable.
      queryClient.invalidateQueries({ queryKey: ["transmission-status"] });
    }
    setTx({ id: torrentId, state: next });
    // Latest send owns the timer — clear the previous so its reset can't wipe
    // a fresher badge.
    if (txTimer.current !== null) window.clearTimeout(txTimer.current);
    txTimer.current = window.setTimeout(() => setTx({ id: -1, state: "idle" }), 2200);
  };

  return (
    // Always mounted; open/close is pure CSS (see swiss-drawer classes).
    // visibility:hidden in the closed state removes the subtree from
    // hit-testing and the a11y tree, so no `inert` needed here.
    <div className={cn(open ? "swiss-drawer-open" : "swiss-drawer-closed")}>
      {/* backdrop-blur fades with the backdrop's own opacity transition —
          the filtered backdrop is part of the element's paint, so the blur
          eases in/out together with the dim. */}
      <div
        onClick={onClose}
        className="swiss-drawer-backdrop fixed inset-0 z-40 bg-[var(--color-ink)]/60 backdrop-blur-xs"
      />
      {/* Centred modal. The outer grid handles centering at any viewport
          size; inner card has max-w + max-h so it never spans the whole
          screen even on huge monitors. */}
      <div
        className={cn(
          "fixed inset-0 z-50 grid place-items-center",
          "p-4 sm:p-6",
          "pt-[max(1rem,env(safe-area-inset-top))]",
          "pb-[max(1rem,env(safe-area-inset-bottom))]",
          "pointer-events-none" // wrapper doesn't catch clicks — backdrop does
        )}
      >
        <aside
          className={cn(
            "swiss-drawer-panel",
            "pointer-events-auto",
            // max-w-4xl (896px) — was max-w-2xl (672px); the card read too
            // narrow on desktop. ~+33% per user request 2026-06-10.
            "w-full max-w-4xl max-h-[calc(100svh-2rem)]",
            "bg-[var(--color-paper)] dark:bg-[var(--color-dark-paper)]",
            "border border-[var(--color-rule)] dark:border-[var(--color-dark-rule)]",
            // overflow-hidden also clips the square header/scroll children
            // to the 8px radius.
            "rounded-lg overflow-hidden",
            "flex flex-col"
          )}
        >
              <div className="flex items-center justify-between px-5 sm:px-6 h-14 sm:h-16 shrink-0 swiss-rule">
                <div className="swiss-eyebrow">
                  {t("drawer_torrent")} / <span className="tabular-nums normal-case">#{torrentId}</span>
                </div>
                <button
                  onClick={onClose}
                  aria-label={t("drawer_close")}
                  className={cn(
                    "size-10 grid place-items-center -mr-2",
                    "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)]",
                    "hover:text-[var(--color-ink)] dark:hover:text-[var(--color-dark-ink)]",
                    "transition-colors"
                  )}
                >
                  <X className="size-4" />
                </button>
              </div>

              <div className="flex-1 overflow-y-auto px-5 sm:px-6 py-5 sm:py-6 space-y-5 sm:space-y-6">
                {isLoading && (
                  <div className="text-[14px] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]">{t("drawer_loading")}</div>
                )}

                {error && (
                  <div className="p-4 text-[13px] border-l-2 border-red-600 bg-red-600/5 text-red-700 dark:text-red-300 rounded-md">
                    {error instanceof Error ? error.message : t("drawer_load_error")}
                  </div>
                )}

                {data && (
                  <>
                    <h2 className="text-[22px] sm:text-[28px] font-bold leading-tight tracking-[-0.02em]">
                      {data.title}
                    </h2>

                    <div className="swiss-rule-top swiss-rule py-4">
                      {/* Label column is max-content: it fits the widest
                          label exactly, so values share one edge in both
                          languages and no label has to squash its icon. */}
                      <dl className="grid grid-cols-[max-content_minmax(0,1fr)] gap-x-3 gap-y-3 text-[13px]">
                        <PeerStatsRow state={peersState} />
                        <Row
                          icon={<Folder className="size-3.5" />}
                          label={t("meta_forum")}
                          value={data.forum_name}
                          onClick={() => {
                            onForumClick(data.forum_id);
                            onClose();
                          }}
                        />
                        <Row icon={<HardDrive className="size-3.5" />} label={t("meta_size")} value={formatBytes(data.size_bytes)} mono />
                        <Row icon={<Calendar className="size-3.5" />} label={t("meta_registered")} value={formatDate(data.registered_at)} mono />
                        <Row icon={<Hash className="size-3.5" />} label={t("meta_hash")} value={data.hash} mono small />
                      </dl>
                    </div>

                    {/* Swiss action row — Download (accent CTA) → Transmission
                        (send magnet to the server queue) → magnet copy →
                        RuTracker; the star closes the row, kept square. */}
                    <div className="flex gap-2 flex-wrap">
                      {magnet && (
                      <Tooltip text={t("drawer_download_title")}>
                      <a
                        href={magnet}
                        className={cn(
                          "flex-1 basis-0 min-w-[120px] h-11 grid place-items-center rounded-md",
                          "bg-[var(--color-accent)] text-[var(--color-ink)]",
                          "text-[11px] font-semibold uppercase tracking-[0.08em]",
                          "hover:bg-[var(--color-accent-hover)] transition-colors"
                        )}
                      >
                        <span className="flex items-center gap-2">
                          <Download className="size-4" />
                          {t("drawer_download")}
                        </span>
                      </a>
                      </Tooltip>
                      )}
                      {txConfigured && magnet && (
                      <Tooltip text={txOffline ? t("drawer_transmission_offline") : t("drawer_transmission_title")}>
                      <button
                        type="button"
                        onClick={sendTx}
                        disabled={txState === "sending" || txOffline}
                        aria-busy={txState === "sending"}
                        className={cn(
                          "flex-1 basis-0 min-w-[120px] h-11 rounded-md border",
                          "text-[11px] font-semibold uppercase tracking-[0.08em]",
                          "transition-colors grid place-items-center",
                          txOffline
                            ? "border-[var(--color-rule)] dark:border-[var(--color-dark-rule)] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)] opacity-60 cursor-not-allowed"
                            : txState === "error"
                              ? "border-red-600/60 text-red-600 dark:text-red-400"
                              : txState === "added" || txState === "duplicate"
                                ? "border-[var(--color-accent)] text-[var(--color-accent)]"
                                : "border-[var(--color-rule)] dark:border-[var(--color-dark-rule)] text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]"
                        )}
                      >
                        <span className="flex items-center gap-2">
                          {txState === "sending" ? (
                            <Loader2 className="size-4 animate-spin" />
                          ) : txState === "added" || txState === "duplicate" ? (
                            <Check className="size-4" />
                          ) : (
                            <HardDriveDownload className="size-4" />
                          )}
                          <span>
                            {txState === "added"
                              ? t("drawer_transmission_added")
                              : txState === "duplicate"
                                ? t("drawer_transmission_duplicate")
                                : txState === "error"
                                  ? t("drawer_transmission_error")
                                  : t("drawer_transmission")}
                          </span>
                        </span>
                      </button>
                      </Tooltip>
                      )}
                      {magnet && (
                      <Tooltip text={t("drawer_magnet_title")}>
                      <button
                        onClick={copyMagnet}
                        className={cn(
                          "flex-1 basis-0 min-w-[120px] h-11 rounded-md",
                          "border border-[var(--color-rule)] dark:border-[var(--color-dark-rule)]",
                          "text-[11px] font-semibold uppercase tracking-[0.08em]",
                          "text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]",
                          "hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]",
                          "transition-colors grid place-items-center"
                        )}
                      >
                        {/* Both labels share one grid cell; the hidden one
                            still reserves width, so swapping "magnet" ↔
                            "Скопировано" can't resize the button and shove
                            its neighbours around. */}
                        <span
                          className={cn(
                            "col-start-1 row-start-1 flex items-center gap-2",
                            !copied && "invisible"
                          )}
                        >
                          <Check className="size-4 text-[var(--color-accent)]" />
                          <span>{t("drawer_magnet_copied")}</span>
                        </span>
                        <span
                          className={cn(
                            "col-start-1 row-start-1 flex items-center gap-2",
                            copied && "invisible"
                          )}
                        >
                          <Copy className="size-4" />
                          magnet
                        </span>
                      </button>
                      </Tooltip>
                      )}
                      <Tooltip text={`${t("drawer_topic_title")} — rutracker.org/forum/viewtopic.php?t=${data.id}`}>
                      <a
                        href={`https://rutracker.org/forum/viewtopic.php?t=${data.id}`}
                        target="_blank"
                        rel="noopener noreferrer"
                        aria-label={t("drawer_topic_aria")}
                        className={cn(
                          "flex-1 basis-0 min-w-[120px] h-11 rounded-md",
                          "border border-[var(--color-rule)] dark:border-[var(--color-dark-rule)]",
                          "text-[11px] font-semibold uppercase tracking-[0.08em]",
                          "text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]",
                          "hover:border-[var(--color-accent)] hover:text-[var(--color-accent)]",
                          "transition-colors flex items-center justify-center gap-2"
                        )}
                      >
                        <ExternalLink className="size-4" />
                        <span>{t("drawer_topic")}</span>
                      </a>
                      </Tooltip>
                      <FavoriteStar torrent={data} size="drawer" />
                    </div>

                    {data.files_count !== undefined && (
                      // Collapsed, this section is a single line between two
                      // hairlines: the top gap must match the container's
                      // space-y below it, or the label sits high.
                      <section className="swiss-rule-top pt-5 sm:pt-6">
                        <button
                          type="button"
                          onClick={() => setFilesOpenFor(filesOpen ? null : data.id)}
                          aria-expanded={filesOpen}
                          className="w-full flex items-center gap-2 swiss-eyebrow hover:text-[var(--color-accent)] transition-colors cursor-pointer"
                        >
                          <ChevronRight
                            className={cn("size-3 shrink-0 transition-transform", filesOpen && "rotate-90")}
                          />
                          <span>
                            {t("drawer_files")}: {data.files_count}
                          </span>
                          {files?.truncated && (
                            <span className="normal-case tracking-normal text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]">
                              {t("drawer_files_truncated").replace(
                                "{n}",
                                String(files.files_count - files.files.length)
                              )}
                            </span>
                          )}
                        </button>
                        {filesOpen && (
                          <div className="mt-3">
                            {filesLoading ? (
                              <div className="text-[12.5px] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]">
                                {t("drawer_files_loading")}
                              </div>
                            ) : filesError ? (
                              <div className="text-[12.5px] text-red-700 dark:text-red-300">
                                {t("drawer_files_error")}
                              </div>
                            ) : files ? (
                              <FileTree files={files.files} />
                            ) : (
                              <div className="text-[12.5px] text-[var(--color-ink-muted)] dark:text-[var(--color-dark-ink-muted)]">
                                {t("drawer_files_none")}
                              </div>
                            )}
                          </div>
                        )}
                      </section>
                    )}

                    <section className="swiss-rule-top pt-4">
                      <div className="swiss-eyebrow mb-3">
                        {t("drawer_description")}
                      </div>
                      <div
                        className={cn(
                          "text-[13.5px] leading-relaxed",
                          "text-[var(--color-ink-soft)] dark:text-[var(--color-dark-ink-soft)]",
                          "[&_strong]:font-semibold [&_strong]:text-[var(--color-ink)] dark:[&_strong]:text-[var(--color-dark-ink)]",
                          "[&_a]:text-[var(--color-accent)] [&_a]:underline [&_a]:underline-offset-2",
                          "[&_pre]:font-mono [&_pre]:text-[12px] [&_pre]:bg-[var(--color-paper-soft)] dark:[&_pre]:bg-[var(--color-dark-paper-soft)] [&_pre]:p-3 [&_pre]:my-3 [&_pre]:overflow-x-auto [&_pre]:rounded-md",
                          "[&_blockquote]:border-l-2 [&_blockquote]:border-[var(--color-accent)] [&_blockquote]:pl-3 [&_blockquote]:my-3 [&_blockquote]:text-[var(--color-ink-muted)] dark:[&_blockquote]:text-[var(--color-dark-ink-muted)]",
                          "[&_blockquote_cite]:block [&_blockquote_cite]:text-[11px] [&_blockquote_cite]:text-[var(--color-accent)] [&_blockquote_cite]:mb-1",
                          "[&_details]:my-3 [&_details]:p-3 [&_details]:bg-[var(--color-paper-soft)] dark:[&_details]:bg-[var(--color-dark-paper-soft)] [&_details]:rounded-md",
                          "[&_summary]:cursor-pointer [&_summary]:font-medium [&_summary]:text-[12.5px]",
                          "[&_ul]:list-disc [&_ul]:pl-5 [&_ul]:my-2",
                          "[&_li]:my-1"
                        )}
                        dangerouslySetInnerHTML={{ __html: data.content_html }}
                      />
                    </section>
                  </>
                )}
              </div>
        </aside>
      </div>
    </div>
  );
}

// Trackers baked into every magnet — mirror of internal/server/download.go.
// bt[1-4].t-ru.org are rutracker's own (`?magnet` marks magnet-originated
// announces, as the site does); the udp:// ones are open-trackers kept as a
// fallback. opentrackr/openbittorrent were dropped — unreachable from the daemon.
const TRACKERS = [
  "http://bt.t-ru.org/ann?magnet",
  "http://bt2.t-ru.org/ann?magnet",
  "http://bt3.t-ru.org/ann?magnet",
  "http://bt4.t-ru.org/ann?magnet",
  "udp://exodus.desync.com:6969/announce",
  "udp://tracker.torrent.eu.org:451/announce",
  "udp://open.demonii.com:1337/announce",
];

function buildMagnet(hash: string, title: string): string {
  const trs = TRACKERS.map((u) => `tr=${encodeURIComponent(u)}`).join("&");
  return `magnet:?xt=urn:btih:${hash}&dn=${encodeURIComponent(title)}&${trs}`;
}

// Two grid cells, not a self-contained row — the parent <dl> owns the columns.
function Row({
  icon,
  label,
  value,
  mono,
  small,
  onClick,
}: {
  icon: React.ReactNode;
  label: string;
  value: string;
  mono?: boolean;
  small?: boolean;
  // When set, the value becomes a tappable link that hovers to accent.
  // Used by the Forum row to jump the underlying list to that forum.
  onClick?: () => void;
}) {
  const valueClass = cn(
    // leading-5 matches MetaLabel's h-5, so label and value sit on one line.
    "block leading-5 text-[var(--color-ink)] dark:text-[var(--color-dark-ink)] break-all",
    mono && "tabular-nums",
    small && "text-[11.5px] font-mono"
  );
  return (
    <>
      <MetaLabel icon={icon} label={label} />
      <dd className="min-w-0">
        {onClick ? (
          <button
            type="button"
            onClick={onClick}
            className={cn(
              valueClass,
              "text-left hover:text-[var(--color-accent)] transition-colors cursor-pointer"
            )}
          >
            {value}
          </button>
        ) : (
          <span className={valueClass}>{value}</span>
        )}
      </dd>
    </>
  );
}
