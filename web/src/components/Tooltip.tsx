import * as TooltipPrimitive from "@radix-ui/react-tooltip";
import type { ReactNode } from "react";
import { cn } from "../lib/cn";

// Flat paper, hairline border, no arrow; the delays are set in main.tsx.
// Renders its children bare when the text is empty.
export function Tooltip({
  text,
  children,
  side = "top",
  align = "center",
  suppressed = false,
}: {
  text: string | undefined;
  children: ReactNode;
  side?: "top" | "right" | "bottom" | "left";
  align?: "start" | "center" | "end";
  /** Held shut without unmounting the trigger. */
  suppressed?: boolean;
}) {
  if (!text) return <>{children}</>;
  return (
    <TooltipPrimitive.Root open={suppressed ? false : undefined}>
      <TooltipPrimitive.Trigger asChild>{children}</TooltipPrimitive.Trigger>
      <TooltipPrimitive.Portal>
        <TooltipPrimitive.Content
          side={side}
          align={align}
          sideOffset={6}
          collisionPadding={8}
          className={cn(
            "z-50 max-w-xs px-2 py-1 rounded-[4px] shadow-sm",
            "border border-[var(--color-rule)] bg-[var(--color-paper)]",
            "text-[11px] leading-snug text-[var(--color-ink)]",
            "select-none break-words whitespace-pre-line",
          )}
        >
          {text}
        </TooltipPrimitive.Content>
      </TooltipPrimitive.Portal>
    </TooltipPrimitive.Root>
  );
}
