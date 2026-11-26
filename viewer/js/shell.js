// The top bar and the tab bar every page shares: the drive's region, links
// to what is on it, and the language switch.

import { availableLanguages, loadStatus, pick, setLanguage } from "./common.js";
import { drawIcons } from "./icons.js";

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

  status ??= await loadStatus();
  for (const link of document.querySelectorAll('[data-needs="map"]')) link.hidden = !manifest.map;
  for (const link of document.querySelectorAll('[data-needs="library"]')) link.hidden = !status.library;
  return status;
}
