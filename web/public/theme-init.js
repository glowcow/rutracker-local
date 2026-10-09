// Sets the theme and the colour scheme before the first paint; mirrors
// src/lib/useTheme.ts. A file, not inline: the CSP allows same-origin scripts only.
(function () {
  var dark = false;
  var warm = false;
  try {
    warm = localStorage.getItem("palette") === "warm";
    var stored = localStorage.getItem("theme");
    dark =
      stored === "dark" ||
      (stored !== "light" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  } catch (e) {
    // No storage: the classic light default stands until useTheme takes over.
  }
  var root = document.documentElement;
  root.classList.toggle("dark", dark);
  root.classList.toggle("warm", warm);
  root.style.colorScheme = dark ? "dark" : "light";
})();
