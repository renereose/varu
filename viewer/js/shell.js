// The top bar and the tab bar every page shares: the drive's region, links
// to what is on it, the language and the theme.

import { availableLanguages, isDark, loadStatus, pick, pref, setLanguage, setPref, t } from "./common.js";
import { drawIcons, icon } from "./icons.js";

/** Fill in the bars. `status` is /api/status, fetched here when not given. */
export async function initShell(manifest, lang, status) {
  drawIcons();
  // "Varu – Estonia" names the region "Estonia".
  const name = pick(lang, manifest.name);
  const region = name.replace(/^Varu\s*[–-]\s*/, "");
  document.getElementById("region").textContent = region === name ? "" : region;

  const langs = document.getElementById("langs");
  const offered = availableLanguages(manifest);
  langs.replaceChildren(...(offered.length < 2 ? [] : offered.map((code) => {
    const button = Object.assign(document.createElement("button"), { type: "button", textContent: code });
    button.setAttribute("aria-pressed", String(code === lang));
    button.addEventListener("click", () => {
      setLanguage(code);
      location.reload();
    });
    return button;
  })));
  initTheme(lang);

  status ??= await loadStatus();
  for (const link of document.querySelectorAll('[data-needs="map"]')) link.hidden = !manifest.map;
  for (const link of document.querySelectorAll('[data-needs="library"]')) link.hidden = !status.library;
  return status;
}

// System, Light or Dark on a wide screen; on a phone one button that flips
// to the other one. A "varu-theme" event tells the page when it changed.
function initTheme(lang) {
  const root = document.documentElement;
  const toggle = document.getElementById("theme-toggle");
  const show = () => {
    const chosen = root.dataset.theme ?? "system";
    for (const button of document.querySelectorAll("[data-theme-choice]")) {
      button.setAttribute("aria-pressed", String(button.dataset.themeChoice === chosen));
    }
    toggle.replaceChildren(icon(isDark() ? "sun" : "moon"));
    toggle.setAttribute("aria-label", t(lang, isDark() ? "toLight" : "toDark"));
    window.VaruAndroid?.bars?.(isDark()); // the phone's own bars match
  };
  const choose = (theme) => {
    const wasDark = isDark();
    setPref("theme", theme);
    if (theme === "light" || theme === "dark") root.dataset.theme = theme; else delete root.dataset.theme;
    show();
    if (isDark() !== wasDark) document.dispatchEvent(new Event("varu-theme"));
  };
  for (const button of document.querySelectorAll("[data-theme-choice]")) {
    button.addEventListener("click", () => choose(button.dataset.themeChoice));
  }
  toggle.addEventListener("click", () => choose(isDark() ? "light" : "dark"));
  window.matchMedia("(prefers-color-scheme: dark)").addEventListener("change", () => {
    if (pref("theme") !== "light" && pref("theme") !== "dark") document.dispatchEvent(new Event("varu-theme"));
    show();
  });
  show();
}
