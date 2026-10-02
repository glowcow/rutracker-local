import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import "./index.css";
import App from "./App.tsx";
import { LangProvider } from "./lib/LangProvider";

// The initial dark class is applied pre-paint by the blocking
// /theme-init.js in index.html's <head> (CSP forbids an inline script, so it
// lives in a same-origin file) — that's what kills the light-background flash
// on reload. App's useTheme effect keeps it in sync afterwards.

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // The dump is immutable between deploys, so search/torrent/forum data
      // can't go stale within a session — never refetch what we've seen.
      // Mutable data (favorites) opts out with its own staleTime.
      staleTime: Infinity,
      // Keep unobserved pages around for 30 min so "back to page 1" after a
      // long read is instant, while a long session doesn't hoard every page
      // ever visited.
      gcTime: 30 * 60_000,
      retry: 1,
      refetchOnWindowFocus: false,
    },
  },
});

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <QueryClientProvider client={queryClient}>
      <LangProvider>
        {/* 400 ms delay before any tooltip opens — still ~2× faster than
            the browser's native title (~700–1000 ms) without feeling
            twitchy on pointer fly-throughs. skipDelay keeps the wait
            short when moving between adjacent tooltipped siblings (e.g.
            the icon row in the drawer). */}
        <TooltipPrimitive.Provider delayDuration={400} skipDelayDuration={100}>
          <App />
        </TooltipPrimitive.Provider>
      </LangProvider>
    </QueryClientProvider>
  </StrictMode>,
);
