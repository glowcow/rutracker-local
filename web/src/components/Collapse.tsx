import type { ReactNode } from "react";
import { cn } from "../lib/cn";

// A body that folds both ways: the grid row animates between 1fr and 0fr, so
// no height is measured. Stays mounted; `inert` keeps it out of the tab order.
export function Collapse({
  open,
  children,
  className,
}: {
  open: boolean;
  children: ReactNode;
  /** Spacing of the folded body; it folds away with the content. */
  className?: string;
}) {
  return (
    <div
      inert={!open}
      className={cn(
        "grid transition-[grid-template-rows,opacity] duration-300 ease-[cubic-bezier(0.4,0,0.2,1)]",
        open ? "grid-rows-[1fr] opacity-100" : "grid-rows-[0fr] opacity-0",
      )}
    >
      {/* The side bleed keeps a focus ring at the edge out of the clip. */}
      <div className="min-h-0 overflow-hidden -mx-1 px-1">
        <div className={className}>{children}</div>
      </div>
    </div>
  );
}
