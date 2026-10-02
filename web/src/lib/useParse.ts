import { useCallback, useEffect, useRef, useState } from "react";
import {
  getStatus,
  hhmmss,
  type LogLine,
  type ParseStatus,
  type Progress,
  type SSEvent,
} from "./parse";

// useParseStream subscribes to the server's parse SSE while the panel is open.
// The parse lives server-side, so resume is transparent: on open it asks /status
// and, if running, attaches (the stream replays the run so reloads/other devices
// see live progress). begin() attaches optimistically right after a start.
export function useParseStream(open: boolean) {
  const [status, setStatus] = useState<ParseStatus>("idle");
  const [progress, setProgress] = useState<Progress | null>(null);
  const [logs, setLogs] = useState<LogLine[]>([]);

  const esRef = useRef<EventSource | null>(null);
  const logId = useRef(0);

  const disconnect = useCallback(() => {
    esRef.current?.close();
    esRef.current = null;
  }, []);

  const connect = useCallback(() => {
    if (esRef.current) return;
    const es = new EventSource("/api/admin/parse/stream");
    esRef.current = es;
    // onopen fires on the initial connect AND on every auto-reconnect. The
    // server replays the whole run buffer on each connect, so resetting here
    // keeps a reconnect from duplicating log lines.
    es.onopen = () => {
      setLogs([]);
      setProgress(null);
      logId.current = 0;
    };
    es.onmessage = (e) => {
      let ev: SSEvent;
      try {
        ev = JSON.parse(e.data);
      } catch {
        return;
      }
      if (ev.kind === "status" && ev.status) {
        setStatus(ev.status as ParseStatus);
      } else if (ev.kind === "progress" && ev.progress) {
        setProgress(ev.progress);
      } else if (ev.kind === "log") {
        setLogs((prev) => [
          ...prev.slice(-299),
          {
            id: logId.current++,
            ts: ev.ts ? hhmmss(ev.ts) : undefined,
            level: ev.level ?? "info",
            msg: ev.msg ?? "",
            detail: ev.detail,
          },
        ]);
      }
    };
    // onerror: EventSource auto-reconnects on its own — nothing to do.
  }, []);

  // On close: drop the connection (state stays put, hidden by CSS, refreshed on
  // the next open). On open: ask /status and either attach to a running parse
  // (resume) or reset to the idle view. Resets live in the async callback — the
  // lint (and React) forbid synchronous setState in an effect body.
  useEffect(() => {
    if (!open) {
      disconnect();
      return;
    }
    let cancelled = false;
    getStatus()
      .then((s) => {
        if (cancelled) return;
        if (s.running) {
          setStatus("running");
          connect();
        } else {
          setStatus("idle");
          setProgress(null);
          setLogs([]);
        }
      })
      .catch(() => {});
    return () => {
      cancelled = true;
    };
  }, [open, connect, disconnect]);

  useEffect(() => () => disconnect(), [disconnect]);

  // begin resets local state and attaches — call after a successful POST.
  const begin = useCallback(() => {
    setLogs([]);
    setProgress(null);
    logId.current = 0;
    setStatus("running");
    connect();
  }, [connect]);

  return { status, progress, logs, running: status === "running", begin };
}
