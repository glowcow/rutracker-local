import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import type { ReactNode } from "react";
import { cn } from "../lib/cn";

// Swiss tooltip — flat paper rect, hairline border, 4px corners, no arrow.
// delayDuration is set globally in main.tsx. Falls through to bare children
// when text is empty, so callers can pass an optional label unconditionally.
export function Tooltip({
  text,
  children,
  side = "top",
  align = "center",
}: {
  text: string | undefined;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
}) {
  if (!text) return <>{children}</>;
  return (
    <TooltipPrimitive.Root>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          align={align}
          sideOffset={6}
          collisionPadding={8}
          className={cn(
            "z-50 max-w-xs px-2 py-1 rounded-sm",
            "border border-[var(--color-rule)] dark:border-[var(--color-dark-rule)]",
            "bg-[var(--color-paper)] dark:bg-[var(--color-dark-paper)]",
            "text-[11px] leading-snug",
            "text-[var(--color-ink)] dark:text-[var(--color-dark-ink)]",
            "shadow-sm",
            "select-none break-words",
          )}
        >
          {text}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
