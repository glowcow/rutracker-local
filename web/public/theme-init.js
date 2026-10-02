// Runs before first paint — a blocking <script> in <head>, ahead of the
// deferred module bundle — so a dark-pinned or dark-OS user never flashes the
// light "paper" background on load (notably in Safari, where the bundle paints
// too late). CSP `script-src 'self'` allows this same-origin file but forbids
// an inline <script>, so the logic lives here rather than in index.html.
// Mirror of the theme resolution in src/App.tsx (useTheme).
(function () {
  try {
    var stored = localStorage.getItem("theme");
    var dark =
      stored === "dark" ||
      (stored !== "light" &&
        window.matchMedia("(prefers-color-scheme: dark)").matches);
    var root = document.documentElement;
    root.classList.toggle("dark", dark);
    // Also flip the UA color-scheme so native surfaces (scrollbars, the
    // pre-CSS default background) match immediately.
    root.style.colorScheme = dark ? "dark" : "light";
  } catch (e) {
    // localStorage/matchMedia unavailable — App's useTheme effect catches up.
  }
})();
