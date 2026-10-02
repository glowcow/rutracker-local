import type { ReactNode } from "react";

// Label cell of the drawer's meta grid (first column = max-content, so every
// value aligns at the widest label). h-5 + self-start pins it to the value's
// first line; nothing here shrinks, so the icon can't be squashed away.
export function MetaLabel({ icon, label }: { icon: ReactNode; label: string }) {
  return (
    <dt className="flex items-center gap-1.5 h-5 self-start shrink-0 swiss-eyebrow whitespace-nowrap">
      {icon}
      <span>{label}:</span>
    </dt>
  );
}
