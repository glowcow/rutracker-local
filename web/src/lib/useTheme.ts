import { useCallback, useEffect, useState, useSyncExternalStore } from "react";

/** system = follow the OS and store nothing. */
export type Theme = "system" | "light" | "dark";
/** classic is the default and stores nothing. */
export type Palette = "classic" | "warm";

const THEME_KEY = "theme";
const PALETTE_KEY = "palette";
const DARK_QUERY = "(prefers-color-scheme: dark)";

function read(key: string): string | null {
  try {
    return localStorage.getItem(key);
  } catch {
    return null;
  }
}

// Storage unavailable: the choice holds for this page only.
function write(key: string, value: string | null) {
  try {
    if (value === null) localStorage.removeItem(key);
    else localStorage.setItem(key, value);
  } catch {
    return;
  }
}

function readTheme(): Theme {
  const v = read(THEME_KEY);
  return v === "light" || v === "dark" ? v : "system";
}

const readPalette = (): Palette => (read(PALETTE_KEY) === "warm" ? "warm" : "classic");

function subscribeOS(onChange: () => void) {
  const mq = window.matchMedia(DARK_QUERY);
  mq.addEventListener("change", onChange);
  return () => mq.removeEventListener("change", onChange);
}

const osDark = () => window.matchMedia(DARK_QUERY).matches;

/** The viewer's theme and colour scheme; keeps the classes on <html> in step. */
export function useTheme() {
  const [theme, setThemeState] = useState<Theme>(readTheme);
  const [palette, setPaletteState] = useState<Palette>(readPalette);
  const systemDark = useSyncExternalStore(subscribeOS, osDark);
  const dark = theme === "dark" || (theme === "system" && systemDark);

  useEffect(() => {
    const root = document.documentElement;
    root.classList.toggle("dark", dark);
    root.classList.toggle("warm", palette === "warm");
    root.style.colorScheme = dark ? "dark" : "light";
    // The browser's own chrome takes the page ground. Chrome on Android
    // drops a near-white theme colour, so a light theme hands it the soft tone.
    const tone = getComputedStyle(root).getPropertyValue(dark ? "--color-paper" : "--color-paper-soft").trim();
    for (const meta of document.querySelectorAll('meta[name="theme-color"]')) {
      meta.setAttribute("content", tone);
    }
  }, [dark, palette]);

  const setTheme = useCallback((next: Theme) => {
    setThemeState(next);
    write(THEME_KEY, next === "system" ? null : next);
  }, []);

  const setPalette = useCallback((next: Palette) => {
    setPaletteState(next);
    write(PALETTE_KEY, next === "classic" ? null : next);
  }, []);

  return { theme, setTheme, palette, setPalette };
}
