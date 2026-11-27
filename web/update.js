// In the web app's pages (sw.js adds it): when Varu opens with internet,
// check for a newer version and update in the background. A small update
// (new app code, a new road network) downloads by itself; a large one (a
// new map) asks first, as it may be on mobile data. download.js writes the
// new files beside the ones in use and switches over at the end, so Varu
// keeps working throughout; the next page opened is the new version.
(() => {
  "use strict";
  const scope = document.currentScript.dataset.scope;
  const AUTO = 50e6; // bytes fetched without asking
  const QUIET_MS = 10 * 60 * 1000; // between checks, unless one is under way
  const STRINGS = {
    en: {
      available: "A new version of Varu is ready ({size}).",
      update: "Update",
      later: "Later",
      updating: "Updating Varu… {done} of {total}{speed}",
      done: "Varu is updated.",
      reload: "Reload",
      failed: "The update stopped. It carries on the next time Varu opens with internet.",
      live: "This is Varu online. Install it to keep it working without the internet.",
      install: "Install",
      close: "Close",
    },
    et: {
      available: "Varu uus versioon on valmis ({size}).",
      update: "Uuenda",
      later: "Hiljem",
      updating: "Uuendan Varut… {done} / {total}{speed}",
      done: "Varu on uuendatud.",
      reload: "Laadi uuesti",
      failed: "Uuendamine katkes. See jätkub, kui avad Varu järgmine kord internetiga.",
      live: "See on Varu veebis. Paigalda see, et see töötaks ka ilma internetita.",
      install: "Paigalda",
      close: "Sulge",
    },
  };

  const lang = () => (STRINGS[document.documentElement.lang] ? document.documentElement.lang : "en");
  const t = (key, vars = {}) => STRINGS[lang()][key].replace(/\{(\w+)\}/g, (_, k) => vars[k] ?? "");
  const size = (bytes) => `${new Intl.NumberFormat(lang(), { maximumFractionDigits: bytes < 1e9 ? 0 : 1 })
    .format(bytes < 1e9 ? Math.max(1, bytes / 1e6) : bytes / 1e9)} ${bytes < 1e9 ? "MB" : "GB"}`;
  const stored = (key) => { try { return localStorage.getItem(key); } catch { return null; } };
  const store = (key, value) => { try { localStorage.setItem(key, value); } catch { /* private mode */ } };

  // The banner: one line, buttons, and a bar while downloading.
  let banner = null;
  function show(text, buttons = [], progress = null) {
    if (!banner) {
      banner = document.createElement("div");
      banner.setAttribute("role", "status");
      banner.style.cssText = "position:fixed;z-index:1000;left:50%;transform:translateX(-50%);top:calc(env(safe-area-inset-top) + 10px);"
        + "width:min(100% - 24px, 480px);display:flex;flex-wrap:wrap;align-items:center;gap:8px 12px;padding:12px 14px;"
        + "background:var(--surface,#fffdf8);color:var(--ink,#1b2420);border:1px solid var(--line,#d9d4c7);border-radius:12px;"
        + "box-shadow:0 6px 24px rgb(0 0 0 / 0.18);font:14px/1.4 system-ui,-apple-system,sans-serif";
      document.body.append(banner);
    }
    const line = Object.assign(document.createElement("span"), { textContent: text });
    line.style.flex = "1 1 12rem";
    const parts = [line, ...buttons.map(([label, primary, onClick]) => {
      const b = Object.assign(document.createElement("button"), { type: "button", textContent: label });
      b.style.cssText = `padding:6px 12px;border-radius:8px;font:inherit;font-weight:650;cursor:pointer;border:1px solid ${primary ? "var(--accent,#0b5d4b)" : "var(--line,#d9d4c7)"};`
        + `background:${primary ? "var(--accent,#0b5d4b)" : "none"};color:${primary ? "var(--accent-ink,#fff)" : "inherit"}`;
      b.addEventListener("click", onClick);
      return b;
    })];
    if (progress !== null) {
      const track = document.createElement("span");
      track.style.cssText = "flex-basis:100%;height:4px;border-radius:2px;overflow:hidden;background:var(--surface-alt,#eae6dc)";
      const fill = document.createElement("span");
      fill.style.cssText = `display:block;height:100%;width:${Math.round(progress * 100)}%;background:var(--accent,#0b5d4b)`;
      track.append(fill);
      parts.push(track);
    }
    banner.replaceChildren(...parts);
  }
  const hide = () => { banner?.remove(); banner = null; };

  async function main() {
    if (!navigator.onLine || !navigator.serviceWorker?.controller) return;
    const worker = new Worker(`${scope}download.js`);
    const ask = (message) => new Promise((resolve) => {
      worker.onmessage = ({ data }) => resolve(data);
      worker.postMessage(message);
    });
    const before = await ask({ status: true });
    if (before.migrated) navigator.serviceWorker.controller.postMessage("reload");
    const last = Number(stored("varu.updateChecked")) || 0;
    if (!before.installed || (!before.pending && Date.now() - last < QUIET_MS)) return worker.terminate();
    const manifest = await fetch("manifest.json").then((r) => r.json()).catch(() => null);
    const list = manifest && await fetch(`${scope}data/${manifest.region}/files.json`, { cache: "no-store" })
      .then((r) => (r.ok ? r.json() : null)).catch(() => null);
    if (!list) return worker.terminate();
    store("varu.updateChecked", String(Date.now()));
    const { update, pending } = await ask({ status: true, list });
    if (!update) return worker.terminate();
    if (update > AUTO && !pending) {
      const go = await new Promise((resolve) => show(t("available", { size: size(update) }),
        [[t("later"), false, () => resolve(false)], [t("update"), true, () => resolve(true)]]));
      if (!go) {
        hide();
        return worker.terminate();
      }
    }
    let drawn = 0;
    const samples = []; // [time, bytes] of the last seconds
    const rate = (fetched) => {
      const now = performance.now();
      samples.push([now, fetched]);
      while (samples.length > 2 && now - samples[0][0] > 5000) samples.shift();
      const perSec = ((fetched - samples[0][1]) * 1000) / (now - samples[0][0] || 1);
      return now - samples[0][0] < 1500 || perSec <= 0 ? ""
        : ` · ${new Intl.NumberFormat(lang(), { maximumFractionDigits: perSec < 1e7 ? 1 : 0 }).format(perSec / 1e6)} MB/s`;
    };
    worker.onmessage = ({ data }) => {
      if ("done" in data) {
        // Only what is fetched counts: files kept from before are counted done at once.
        const fetched = Math.min(update, Math.max(0, data.done - (data.total - update)));
        if (fetched - drawn < 1e6 && fetched < update) return;
        drawn = fetched;
        return show(t("updating", { done: size(fetched), total: size(update), speed: rate(fetched) }), [], fetched / update);
      }
      worker.terminate();
      if (data.error) {
        show(t("failed"));
        setTimeout(hide, 6000);
        return;
      }
      navigator.serviceWorker.controller?.postMessage("reload");
      show(t("done"), [[t("reload"), true, () => location.reload()]]);
      setTimeout(hide, 10000);
    };
    worker.postMessage({ base: new URL(`${scope}data/${manifest.region}/`, location.href).href, list });
  }

  // Varu online (live/): no updates to check, only the way to install it.
  function live() {
    try { if (sessionStorage.getItem("varu.liveClosed")) return; } catch { /* private mode */ }
    show(t("live"), [
      [t("close"), false, () => { hide(); try { sessionStorage.setItem("varu.liveClosed", "1"); } catch { /* private mode */ } }],
      [t("install"), true, () => { location.href = scope; }],
    ]);
  }

  // After the page has drawn, so the check never slows it down.
  if ("live" in document.currentScript.dataset) {
    addEventListener("load", live);
    // The page picks its language after loading: say it again in that one.
    new MutationObserver(() => banner && live()).observe(document.documentElement, { attributeFilter: ["lang"] });
  }
  else addEventListener("load", () => setTimeout(() => main().catch(() => {}), 1500));
})();
