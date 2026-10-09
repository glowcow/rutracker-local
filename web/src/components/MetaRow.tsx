import type { ReactNode } from "react";

// The label cell of a meta grid: the column fits its widest label, and
// nothing here shrinks, so the icon cannot be squashed.
export function MetaLabel({ icon, label }: { icon: ReactNode; label: string }) {
  return (
    <dt className="flex items-center gap-1.5 h-5 self-start shrink-0 swiss-eyebrow whitespace-nowrap">
      {icon}
      <span>{label}:</span>
    </dt>
  );
}
