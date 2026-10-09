import { useCallback, useEffect, useRef, useState } from "react";
import {
  getStatus,
  hhmmss,
  type LogLine,
  type ParseStatus,
  type Progress,
  type SSEvent,
} from "./parse";

// Subscribes to the parse stream while the panel is open. On open it asks
// /status and attaches to a running parse; begin() attaches after a start.
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
    // Fires on every reconnect too, and the server replays the whole run:
    // resetting here keeps log lines from doubling.
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

  // Closed: drop the connection. Open: ask /status, then attach or go idle —
  // in the async callback, since an effect body may not set state.
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
