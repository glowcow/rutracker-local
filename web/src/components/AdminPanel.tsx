import { useEffect, useRef, useState } from "react";
import { X, Play, Loader2, KeyRound, Check, ChevronDown } from "lucide-react";
import { cn } from "../lib/cn";
import { useLang } from "../lib/i18n";
import { formatBytes, formatDate } from "../lib/format";
import { useParseStream } from "../lib/useParse";
import {
  listDumps,
  getRecentRuns,
  startParse,
  getToken,
  setToken,
  clearToken,
  fmtDur,
  type DumpFile,
  type RunSummary,
  type ParseStatus,
} from "../lib/parse";

type Props = {
  open: boolean;
  onClose: () => void;
};

const BATCH_SIZES = [250, 500, 1000];

// AdminPanel — the in-app "load a dump" control. Read data (dumps / recent runs
// / live progress+logs via SSE) is open; starting a parse needs the token. The
// parse runs server-side, so progress survives reload / other devices.
export function AdminPanel({ open, onClose }: Props) {
  const { t, locale } = useLang();

  const [token, setTokenState] = useState(getToken());
  const [tokenDraft, setTokenDraft] = useState("");

  const [dumps, setDumps] = useState<DumpFile[]>([]);
  const [recent, setRecent] = useState<RunSummary[]>([]);
  const [source, setSource] = useState("");
  const [batchSize, setBatchSize] = useState(1000);
  const [sweep, setSweep] = useState(false);
  const [startErr, setStartErr] = useState<string | null>(null);

  const { status, progress, logs, running, begin } = useParseStream(open);
  const logBoxRef = useRef<HTMLDivElement | null>(null);

  // ESC to close. The parse keeps running server-side, so closing is safe.
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

  // Load dumps + recent runs when the panel opens.
  useEffect(() => {
    if (!open) return;
    listDumps()
      .then((d) => {
        setDumps(d.items);
        setSource((s) => s || d.items[0]?.name || "");
      })
      .catch(() => setDumps([]));
    getRecentRuns(3)
      .then((r) => setRecent(r.items))
      .catch(() => setRecent([]));
  }, [open]);

  // Refresh the recent list once a run finishes.
  useEffect(() => {
    if (status === "succeeded" || status === "failed") {
      getRecentRuns(3)
        .then((r) => setRecent(r.items))
        .catch(() => {});
    }
  }, [status]);

  // Auto-scroll the log tail to the newest line.
  useEffect(() => {
    const box = logBoxRef.current;
    if (box) box.scrollTop = box.scrollHeight;
  }, [logs]);

  const onStart = async () => {
    if (!token || running) return;
    setStartErr(null);
    try {
      await startParse({ source, batch_size: batchSize, sweep }, token);
      begin();
    } catch (e) {
      const code = e instanceof Error ? e.message : "";
      setStartErr(
        t(
          code === "conflict"
            ? "admin_err_conflict"
            : code === "unauthorized"
              ? "admin_err_unauthorized"
              : code === "disabled"
                ? "admin_err_disabled"
                : "admin_err_generic",
        ),
      );
    }
  };

  const saveToken = () => {
    const v = tokenDraft.trim();
    if (!v) return;
    setToken(v);
    setTokenState(v);
    setTokenDraft("");
  };

  const changeToken = () => {
    clearToken();
    setTokenState("");
  };

  return (
    <div className={cn(open ? "swiss-drawer-open" : "swiss-drawer-closed")}>
      <div
        onClick={onClose}
        className="swiss-drawer-backdrop fixed inset-0 z-40 bg-[var(--color-scrim)] backdrop-blur-xs"
      />
      <div
        className={cn(
          "fixed inset-0 z-50 grid place-items-center p-4 sm:p-6",
          "pt-[max(1rem,env(safe-area-inset-top))]",
          "pb-[max(1rem,env(safe-area-inset-bottom))]",
          "pointer-events-none",
        )}
      >
        <aside
          className={cn(
            "swiss-drawer-panel pointer-events-auto",
            "w-full max-w-3xl max-h-[calc(100svh-2rem)]",
            "bg-[var(--color-paper)]",
            "border border-[var(--color-rule)]",
            "rounded-lg overflow-hidden flex flex-col",
          )}
        >
          {/* Header */}
          <div className="flex items-center justify-between px-5 sm:px-6 h-14 sm:h-16 shrink-0 swiss-rule">
            <div className="swiss-eyebrow">
              {t("admin_title")} /{" "}
              <span className="normal-case text-[var(--color-ink-muted)]">
                {t("admin_subtitle")}
              </span>
            </div>
            <button
              onClick={onClose}
              aria-label={t("admin_close")}
              className={cn(
                "size-10 grid place-items-center -mr-2",
                "text-[var(--color-ink-soft)]",
                "hover:text-[var(--color-ink)]",
                "transition-colors",
              )}
            >
              <X className="size-4" />
            </button>
          </div>

          {/* Body */}
          <div className="flex-1 overflow-y-auto px-5 sm:px-6 py-5 sm:py-6 space-y-6">
            {/* Token */}
            {token ? (
              <div className="flex items-center gap-2 text-[12px] text-[var(--color-ink-muted)]">
                <KeyRound className="size-3.5 text-[var(--color-accent)]" />
                <span>{t("admin_token_saved")}</span>
                <span className="text-[var(--color-rule)]">·</span>
                <button
                  onClick={changeToken}
                  className="swiss-eyebrow hover:text-[var(--color-accent)] transition-colors"
                >
                  {t("admin_token_change")}
                </button>
              </div>
            ) : (
              <Field label={t("admin_token")}>
                <div className="flex gap-2">
                  <input
                    type="password"
                    value={tokenDraft}
                    onChange={(e) => setTokenDraft(e.target.value)}
                    onKeyDown={(e) => e.key === "Enter" && saveToken()}
                    placeholder={t("admin_token_placeholder")}
                    // Not a login form: `new-password` stops Chrome from
                    // offering saved passwords here and from autofilling a
                    // "username" into the nearest text field (the header
                    // search). data-*-ignore silences 1Password/LastPass too.
                    autoComplete="new-password"
                    data-1p-ignore="true"
                    data-lpignore="true"
                    className={cn(
                      "flex-1 min-w-0 h-10 px-3 rounded-md text-[14px]",
                      "bg-[var(--color-paper-soft)]",
                      "border-l-2 border-transparent focus:border-[var(--color-accent)] outline-none",
                      "transition-colors",
                    )}
                  />
                  <button
                    onClick={saveToken}
                    className={cn(
                      "h-10 px-4 rounded-md shrink-0",
                      "border border-[var(--color-rule)]",
                      "text-[11px] font-semibold uppercase tracking-[0.08em]",
                      "hover:border-[var(--color-accent)] hover:text-[var(--color-accent)] transition-colors",
                    )}
                  >
                    {t("admin_token_save")}
                  </button>
                </div>
              </Field>
            )}

            {/* Dump */}
            <Field label={t("admin_dump")}>
              {dumps.length === 0 ? (
                <div className="text-[13px] text-[var(--color-ink-muted)]">
                  {t("admin_no_dumps")}
                </div>
              ) : (
                <DumpSelect dumps={dumps} value={source} onChange={setSource} disabled={running} />
              )}
            </Field>

            {/* Batch size */}
            <Field label={t("admin_batch")}>
              <Segmented options={BATCH_SIZES} value={batchSize} onChange={setBatchSize} disabled={running} />
            </Field>

            {/* Sweep */}
            <Field label={t("admin_sweep")}>
              <div className="flex items-start gap-3">
                <Toggle checked={sweep} onChange={setSweep} disabled={running} />
                <p
                  className={cn(
                    "text-[12px] leading-snug flex-1",
                    sweep
                      ? "text-[var(--color-down)]"
                      : "text-[var(--color-ink-muted)]",
                  )}
                >
                  {t("admin_sweep_hint")}
                </p>
              </div>
            </Field>

            {/* Start */}
            <div>
              <button
                onClick={onStart}
                disabled={running || !token || dumps.length === 0}
                className={cn(
                  "w-full h-11 grid place-items-center rounded-md",
                  "bg-[var(--color-accent)] text-[var(--color-on-accent)]",
                  "text-[11px] font-semibold uppercase tracking-[0.08em]",
                  "hover:bg-[var(--color-accent-hover)] transition-colors",
                  "disabled:opacity-40 disabled:hover:bg-[var(--color-accent)]",
                )}
              >
                <span className="flex items-center gap-2">
                  {running ? <Loader2 className="size-4 animate-spin" /> : <Play className="size-4" />}
                  {running ? t("admin_running") : t("admin_start")}
                </span>
              </button>
              {!token && (
                <p className="mt-2 text-[11.5px] text-[var(--color-ink-muted)] text-center">
                  {t("admin_token_required")}
                </p>
              )}
              {startErr && (
                <p className="mt-2 text-[12px] text-[var(--color-down)] text-center">{startErr}</p>
              )}
            </div>

            {/* Recent loads — shown on the idle view; the live zone replaces it. */}
            {status === "idle" && (
              <div className="swiss-rule-top pt-5 space-y-2">
                <div className="swiss-eyebrow">{t("admin_recent")}</div>
                {recent.length === 0 ? (
                  <div className="text-[12px] text-[var(--color-ink-muted)]">
                    {t("admin_no_runs")}
                  </div>
                ) : (
                  <div className="rounded-md border border-[var(--color-rule)] divide-y divide-[var(--color-rule)]">
                    {recent.map((r) => (
                      <RunRow key={r.id} run={r} />
                    ))}
                  </div>
                )}
              </div>
            )}

            {/* Live zone */}
            {status !== "idle" && (
              <div className="swiss-rule-top pt-5 space-y-4">
                <div className="flex items-center justify-between">
                  <StatusPill status={status} />
                  {progress && (
                    <span className="text-[12px] tabular-nums text-[var(--color-ink-muted)]">
                      {t("admin_elapsed")} {fmtDur(progress.elapsed_s)}
                    </span>
                  )}
                </div>

                {/* Progress bar */}
                <div>
                  <div className="flex items-center justify-between mb-1.5">
                    <span className="text-[12px] tabular-nums font-medium">
                      {(progress?.pct ?? 0).toFixed(1)}%
                    </span>
                    {progress && status === "running" && (
                      <span className="text-[12px] tabular-nums text-[var(--color-ink-muted)]">
                        {t("admin_eta")} {fmtDur(progress.eta_s)}
                      </span>
                    )}
                  </div>
                  <div className="h-1.5 rounded-full bg-[var(--color-paper-soft)] overflow-hidden">
                    <div
                      className={cn(
                        "h-full rounded-full transition-[width] duration-300 ease-out",
                        status === "failed" ? "bg-[var(--color-down)]" : "bg-[var(--color-accent)]",
                      )}
                      style={{ width: `${progress?.pct ?? 0}%` }}
                    />
                  </div>
                </div>

                {/* Stats */}
                {progress && (
                  <div className="grid grid-cols-3 divide-x divide-[var(--color-rule)]">
                    <Stat label={t("admin_rows")} value={progress.rows.toLocaleString(locale)} />
                    <Stat label="MB" value={`${progress.read_mb} / ${progress.total_mb}`} />
                    <Stat label={t("admin_rate")} value={progress.rate_rows_s.toLocaleString(locale)} />
                  </div>
                )}

                {/* Logs */}
                <div>
                  <div className="swiss-eyebrow mb-2">{t("admin_logs")}</div>
                  <div
                    ref={logBoxRef}
                    className={cn(
                      "h-72 max-h-[45vh] overflow-y-auto rounded-md p-3",
                      "bg-[var(--color-paper-soft)]",
                      "font-mono text-[11.5px] leading-relaxed",
                    )}
                  >
                    {logs.map((l) => (
                      <div key={l.id} className="whitespace-pre-wrap break-words">
                        {l.ts && (
                          <span className="text-[var(--color-ink-muted)]/70">
                            {l.ts}
                            {"  "}
                          </span>
                        )}
                        <span
                          className={cn(
                            l.level === "error" && "text-[var(--color-down)]",
                            l.level === "warn" && "text-[var(--color-warn)]",
                            l.level === "info" && "text-[var(--color-ink)]",
                          )}
                        >
                          {l.msg}
                        </span>
                        {l.detail && (
                          <span className="text-[var(--color-ink-muted)]">
                            {"   "}
                            {l.detail}
                          </span>
                        )}
                      </div>
                    ))}
                    {status === "succeeded" && (
                      <div className="flex items-center gap-1.5 mt-1 text-[var(--color-accent)]">
                        <Check className="size-3.5" />
                        <span>done</span>
                      </div>
                    )}
                  </div>
                </div>
              </div>
            )}
          </div>
        </aside>
      </div>
    </div>
  );
}

function Field({ label, children }: { label: string; children: React.ReactNode }) {
  return (
    <div className="space-y-2">
      <div className="swiss-eyebrow">{label}</div>
      {children}
    </div>
  );
}

// Custom dropdown — the native <select> popup is OS-rendered (dark system menu
// on macOS) and un-styleable, which clashes with the flat Swiss look. This is
// a button + own popup list: click-outside and Escape close it.
function DumpSelect({
  dumps,
  value,
  onChange,
  disabled,
}: {
  dumps: DumpFile[];
  value: string;
  onChange: (v: string) => void;
  disabled?: boolean;
}) {
  const [open, setOpen] = useState(false);
  const ref = useRef<HTMLDivElement | null>(null);
  const selected = dumps.find((d) => d.name === value);

  useEffect(() => {
    if (!open) return;
    const onDown = (e: MouseEvent) => {
      if (ref.current && !ref.current.contains(e.target as Node)) setOpen(false);
    };
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    document.addEventListener("mousedown", onDown);
    document.addEventListener("keydown", onKey);
    return () => {
      document.removeEventListener("mousedown", onDown);
      document.removeEventListener("keydown", onKey);
    };
  }, [open]);

  return (
    <div ref={ref} className="relative">
      <button
        type="button"
        onClick={() => setOpen((o) => !o)}
        disabled={disabled}
        className={cn(
          "w-full h-10 pl-3 pr-9 rounded-md text-[14px] text-left flex items-center",
          "bg-[var(--color-paper-soft)]",
          "border-l-2 outline-none transition-colors disabled:opacity-50",
          open ? "border-[var(--color-accent)]" : "border-transparent",
        )}
      >
        <span className="truncate min-w-0">
          {selected && (
            <>
              <span className="font-medium">{selected.name}</span>
              <span className="text-[var(--color-ink-muted)]">
                {" — "}
                {formatBytes(selected.size_bytes)} · {formatDate(selected.mtime)}
              </span>
            </>
          )}
        </span>
        <ChevronDown
          className={cn(
            "pointer-events-none absolute right-3 top-1/2 -translate-y-1/2 size-4 transition-transform",
            "text-[var(--color-ink-muted)]",
            open && "rotate-180",
          )}
        />
      </button>

      {open && (
        <div
          className={cn(
            "absolute z-20 mt-1 w-full rounded-md overflow-hidden py-1",
            "bg-[var(--color-paper)]",
            "border border-[var(--color-rule)]",
            "shadow-[var(--shadow-menu)]",
          )}
        >
          {dumps.map((d) => {
            const active = d.name === value;
            return (
              <button
                key={d.name}
                type="button"
                onClick={() => {
                  onChange(d.name);
                  setOpen(false);
                }}
                className={cn(
                  "w-full text-left px-3 py-2 text-[13px] flex items-center gap-2",
                  "hover:bg-[var(--color-paper-soft)] transition-colors",
                )}
              >
                <Check className={cn("size-3.5 shrink-0 text-[var(--color-accent)]", !active && "invisible")} />
                <span className="truncate min-w-0">
                  <span className={cn("font-medium", active && "text-[var(--color-accent)]")}>{d.name}</span>
                  <span className="text-[var(--color-ink-muted)]">
                    {" — "}
                    {formatBytes(d.size_bytes)} · {formatDate(d.mtime)}
                  </span>
                </span>
              </button>
            );
          })}
        </div>
      )}
    </div>
  );
}

function Segmented({
  options,
  value,
  onChange,
  disabled,
}: {
  options: number[];
  value: number;
  onChange: (v: number) => void;
  disabled?: boolean;
}) {
  return (
    <div
      className={cn(
        "inline-flex gap-0.5 p-0.5 rounded-md",
        "bg-[var(--color-paper-soft)]",
        disabled && "opacity-50 pointer-events-none",
      )}
    >
      {options.map((o) => {
        const active = o === value;
        return (
          <button
            key={o}
            onClick={() => onChange(o)}
            className={cn(
              "px-4 h-8 rounded-[5px] text-[12px] font-medium tabular-nums transition-colors",
              active
                ? "bg-[var(--color-paper)] text-[var(--color-ink)] shadow-sm"
                : "text-[var(--color-ink-muted)] hover:text-[var(--color-ink)]",
            )}
          >
            {o}
          </button>
        );
      })}
    </div>
  );
}

function Toggle({
  checked,
  onChange,
  disabled,
}: {
  checked: boolean;
  onChange: (v: boolean) => void;
  disabled?: boolean;
}) {
  return (
    <button
      role="switch"
      aria-checked={checked}
      onClick={() => onChange(!checked)}
      disabled={disabled}
      className={cn(
        "relative w-9 h-5 rounded-full shrink-0 transition-colors disabled:opacity-50",
        checked ? "bg-[var(--color-down)]" : "bg-[var(--color-rule)]",
      )}
    >
      <span
        className={cn(
          "absolute top-0.5 left-0.5 size-4 rounded-full bg-[var(--color-paper)] transition-transform",
          checked && "translate-x-4",
        )}
      />
    </button>
  );
}

function Stat({ label, value }: { label: string; value: string }) {
  return (
    <div className="text-center px-2 space-y-1">
      <div className="swiss-eyebrow">{label}</div>
      <div className="text-[14px] tabular-nums font-medium">{value}</div>
    </div>
  );
}

function RunRow({ run }: { run: RunSummary }) {
  const { t, locale } = useLang();
  const dot =
    run.status === "succeeded" ? "bg-[var(--color-up)]" : run.status === "failed" ? "bg-[var(--color-down)]" : "bg-[var(--color-accent)]";
  return (
    <div className="flex items-center gap-3 px-3 py-2.5 text-[12px]">
      <span className={cn("size-2 rounded-full shrink-0", dot)} />
      <div className="min-w-0 flex-1">
        <div className="truncate font-medium">{run.source}</div>
        <div className="text-[var(--color-ink-muted)] tabular-nums">
          {formatDate(run.finished_at ?? run.started_at)}
        </div>
      </div>
      <div className="text-right tabular-nums shrink-0 leading-tight">
        <div className="font-medium">
          {run.rows.toLocaleString(locale)} {t("admin_rows")}
        </div>
        <div className="text-[var(--color-ink-muted)]">
          {run.sweep && <>sweep −{run.swept.toLocaleString(locale)} · </>}
          {fmtDur(run.duration_s)}
        </div>
      </div>
    </div>
  );
}

function StatusPill({ status }: { status: ParseStatus }) {
  const { t } = useLang();
  const map: Record<ParseStatus, { label: string; dot: string; text: string }> = {
    idle: {
      label: t("admin_status_idle"),
      dot: "bg-[var(--color-ink-muted)]",
      text: "text-[var(--color-ink-muted)]",
    },
    running: {
      label: t("admin_status_running"),
      dot: "bg-[var(--color-accent)] animate-pulse",
      text: "text-[var(--color-accent)]",
    },
    succeeded: {
      label: t("admin_status_succeeded"),
      dot: "bg-[var(--color-up)]",
      text: "text-[var(--color-up)]",
    },
    failed: {
      label: t("admin_status_failed"),
      dot: "bg-[var(--color-down)]",
      text: "text-[var(--color-down)]",
    },
  };
  const s = map[status];
  return (
    <span className={cn("inline-flex items-center gap-2 text-[11px] font-semibold uppercase tracking-[0.08em]", s.text)}>
      <span className={cn("size-2 rounded-full", s.dot)} />
      {s.label}
    </span>
  );
}
