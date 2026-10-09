import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { QueryClient, QueryClientProvider } from "@tanstack/react-query";
import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import "./index.css";
import App from "./App.tsx";
import { LangProvider } from "./lib/LangProvider";

const queryClient = new QueryClient({
  defaultOptions: {
    queries: {
      // The dump is immutable between loads: what was read never goes stale.
      // Favourites, which do change, set their own staleTime.
      staleTime: Infinity,
      // Half an hour: going back a page is instant, a long session hoards nothing.
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
        {/* 400 ms before the first tooltip, 100 ms between neighbours. */}
        <TooltipPrimitive.Provider delayDuration={400} skipDelayDuration={100}>
          <App />
        </TooltipPrimitive.Provider>
      </LangProvider>
    </QueryClientProvider>
  </StrictMode>,
);
