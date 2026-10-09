import { Fragment, useEffect, useRef, useState } from "react";
import { Check, Monitor, Moon, Settings, Sun, type LucideIcon } from "lucide-react";
import { LANGS, useLang, type Dict } from "../lib/i18n";
import { useTheme, type Palette, type Theme } from "../lib/useTheme";
import { Tooltip } from "./Tooltip";
import { cn } from "../lib/cn";

const THEMES: { id: Theme; icon: LucideIcon; label: (t: Dict) => string }[] = [
  { id: "system", icon: Monitor, label: (t) => t.themeSystem },
  { id: "light", icon: Sun, label: (t) => t.themeLight },
  { id: "dark", icon: Moon, label: (t) => t.themeDark },
];

const PALETTES: { id: Palette; label: (t: Dict) => string }[] = [
  { id: "classic", label: (t) => t.paletteClassic },
  { id: "warm", label: (t) => t.paletteWarm },
];

const ROW = cn(
  "w-full h-9 flex items-center justify-between gap-3 rounded-md cursor-pointer",
  "text-[14px] text-left transition-colors duration-150 hover:text-[var(--color-accent)]",
);

// The gear at the end of the header and the panel under it: theme, colour
// scheme, language.
export function SettingsMenu() {
  const { t, lang, setLang } = useLang();
  const { theme, setTheme, palette, setPalette } = useTheme();
  const [open, setOpen] = useState(false);
  const rootRef = useRef<HTMLDivElement | null>(null);

  // Closes on Escape and on a press outside; the gear itself toggles.
  useEffect(() => {
    if (!open) return;
    const onKey = (e: KeyboardEvent) => {
      if (e.key === "Escape") setOpen(false);
    };
    const onPointer = (e: PointerEvent) => {
      if (!rootRef.current?.contains(e.target as Node)) setOpen(false);
    };
    document.addEventListener("keydown", onKey);
    document.addEventListener("pointerdown", onPointer);
    return () => {
      document.removeEventListener("keydown", onKey);
      document.removeEventListener("pointerdown", onPointer);
    };
  }, [open]);

  return (
    // The 13px pull puts the icon's edge, not the button's, on the column's end.
    <div ref={rootRef} className="relative shrink-0 -ml-[13px] -mr-[13px]">
      <Tooltip text={t.settings} suppressed={open} side="bottom" align="end">
        <button
          type="button"
          aria-label={t.settings}
          aria-expanded={open}
          aria-haspopup="true"
          onClick={() => setOpen((o) => !o)}
          className={cn(
            "size-10 grid place-items-center rounded-md cursor-pointer transition-colors duration-150",
            open ? "text-[var(--color-accent)]" : "text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]",
          )}
        >
          <Settings aria-hidden="true" className="size-3.5 shrink-0" />
        </button>
      </Tooltip>

      {/* Always mounted, opened by CSS like a modal. */}
      <div
        inert={!open}
        className={cn(
          "absolute right-[13px] top-full mt-1 z-50 w-max p-3 rounded-md",
          "border border-[var(--color-rule)] bg-[var(--color-paper)] shadow-[var(--shadow-menu)]",
          "origin-top-right transition-[opacity,translate,scale] duration-[180ms] ease-[cubic-bezier(0.4,0,0.2,1)]",
          open ? "opacity-100" : "opacity-0 pointer-events-none translate-y-2 scale-[0.98]",
        )}
      >
        <div className="swiss-eyebrow mb-1" id="settings-theme">
          {t.theme}
        </div>
        <div role="radiogroup" aria-labelledby="settings-theme" className="flex items-center -ml-2.5">
          {THEMES.map(({ id, icon: Icon, label }, i) => (
            <Fragment key={id}>
              {i > 0 && <span aria-hidden="true" className="w-px h-4 bg-[var(--color-rule)]" />}
              <Tooltip text={label(t)} side="bottom">
                <button
                  type="button"
                  role="radio"
                  aria-checked={theme === id}
                  aria-label={label(t)}
                  onClick={() => setTheme(id)}
                  className={cn(
                    "size-9 grid place-items-center rounded-md cursor-pointer transition-colors duration-150",
                    theme === id
                      ? "text-[var(--color-accent)]"
                      : "text-[var(--color-ink-soft)] hover:text-[var(--color-ink)]",
                  )}
                >
                  <Icon aria-hidden="true" className="size-4 shrink-0" />
                </button>
              </Tooltip>
            </Fragment>
          ))}
        </div>

        <div className="swiss-eyebrow mt-3 mb-1" id="settings-palette">
          {t.palette}
        </div>
        <div role="radiogroup" aria-labelledby="settings-palette">
          {PALETTES.map(({ id, label }) => (
            <button
              key={id}
              type="button"
              role="radio"
              aria-checked={palette === id}
              onClick={() => setPalette(id)}
              className={ROW}
            >
              {label(t)}
              {palette === id && <Check aria-hidden="true" className="size-4 shrink-0 text-[var(--color-accent)]" />}
            </button>
          ))}
        </div>

        <div className="swiss-eyebrow mt-3 mb-1" id="settings-language">
          {t.language}
        </div>
        <div role="radiogroup" aria-labelledby="settings-language">
          {LANGS.map(({ id, name }) => (
            <button
              key={id}
              type="button"
              role="radio"
              aria-checked={lang === id}
              lang={id}
              onClick={() => setLang(id)}
              className={ROW}
            >
              {name}
              {lang === id && <Check aria-hidden="true" className="size-4 shrink-0 text-[var(--color-accent)]" />}
            </button>
          ))}
        </div>
      </div>
    </div>
  );
}
