const key = "nut-allergy-theme";

export function themeMode() {
  return localStorage.getItem(key) || "auto";
}

export function applyTheme(mode = themeMode()) {
  const dark =
    mode === "dark" ||
    (mode !== "light" && window.matchMedia("(prefers-color-scheme: dark)").matches);
  document.documentElement.classList.toggle("dark", dark);
}

export function setTheme(mode) {
  localStorage.setItem(key, mode);
  applyTheme(mode);
}

export function cycleTheme() {
  const order = ["auto", "light", "dark"];
  const next = order[(order.indexOf(themeMode()) + 1) % order.length];
  setTheme(next);
  return next;
}

window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => applyTheme());
