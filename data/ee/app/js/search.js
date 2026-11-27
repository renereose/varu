// The map's search box: towns, streets and addresses from the launcher
// (/api/places), and the drive's own place layers (hospitals, water ...)
// matched here. Picking a result flies there and offers directions.

import * as maplibregl from "../vendor/maplibre-gl.mjs";
import { pick, t } from "./common.js";

const LIMIT = 10;
const FROM_LAYERS = 3; // at most this many results from the place layers
const ZOOM = { area: 13, street: 16, address: 17, layer: 17 };

/** Words without accents or punctuation: "Pärnu mnt." -> "parnu mnt". */
const fold = (s) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase().replace(/[^\p{L}\p{N}]+/gu, " ").trim();

export function setUpSearch(map, { manifest, lang, base, routing }) {
  if (!manifest.places?.file) return; // a drive from before search
  const form = document.getElementById("search");
  const input = document.getElementById("search-input");
  const list = document.getElementById("search-results");
  form.hidden = false;
  input.placeholder = t(lang, "searchMapHint");
  input.setAttribute("aria-label", t(lang, "searchMap"));

  const marker = new maplibregl.Marker({ color: "#6a1b9a" });
  const layers = []; // the place layers' named points, loaded on first use
  let loading = null;
  let request = null;
  let timer = 0;
  let runs = 0;
  let shown = [];

  const ready = () => (loading ??= Promise.all((manifest.pois ?? []).map(async (poi) => {
    const data = await fetch(`${base}${poi.file}`).then((res) => res.json()).catch(() => null);
    for (const f of data?.features ?? []) {
      const name = f.properties?.[`name:${lang}`] || f.properties?.name;
      if (name && f.geometry?.type === "Point") {
        layers.push({ name, context: pick(lang, poi.label), kind: "layer", lngLat: f.geometry.coordinates, words: ` ${fold(name)}` });
      }
    }
  })));

  // The first look at the box loads the layers and the launcher's index,
  // so the first search does not wait for them.
  input.addEventListener("focus", () => {
    ready();
    fetch("api/places?q=").catch(() => {});
  }, { once: true });

  input.addEventListener("input", () => {
    clearTimeout(timer);
    timer = setTimeout(run, 150);
  });

  async function run() {
    const seq = ++runs;
    const q = input.value.trim();
    request?.abort();
    if (!q) {
      marker.remove();
      return show([]);
    }
    await ready();
    if (seq !== runs) return; // typing went on meanwhile
    const words = fold(q).split(" ").filter(Boolean);
    const c = map.getCenter();
    const away = ({ lngLat: [lon, lat] }) => ((lon - c.lng) * Math.cos((c.lat * Math.PI) / 180)) ** 2 + (lat - c.lat) ** 2;
    // Names that start with the first word, then shorter ones, then nearer ones.
    const rank = (p) => [p.words.startsWith(` ${words[0]}`) ? 0 : 1, p.name.length, away(p)];
    const local = layers.filter((p) => words.every((w) => p.words.includes(` ${w}`)))
      .map((p) => [rank(p), p]).sort(([a], [b]) => a[0] - b[0] || a[1] - b[1] || a[2] - b[2])
      .map(([, p]) => p).slice(0, FROM_LAYERS);
    show(local, t(lang, "searching"));
    request = new AbortController();
    try {
      const res = await fetch(`api/places?q=${encodeURIComponent(q)}&near=${c.lat.toFixed(5)},${c.lng.toFixed(5)}`,
        { signal: request.signal });
      const found = (res.ok ? await res.json() : []).map((r) => ({ ...r, lngLat: [r.lon, r.lat] }));
      // Towns first, then the drive's own places, then streets and addresses.
      const areas = found.filter((r) => r.kind === "area");
      show([...areas, ...local, ...found.filter((r) => r.kind !== "area")].slice(0, LIMIT), t(lang, "noHits"));
    } catch (err) {
      if (err.name !== "AbortError") show(local, t(lang, "noHits"));
    }
  }

  function show(hits, empty = "") {
    shown = hits;
    list.replaceChildren(...hits.map((hit) => {
      const button = Object.assign(document.createElement("button"), { type: "button" });
      button.append(
        Object.assign(document.createElement("span"), { className: "name", textContent: hit.name }),
        Object.assign(document.createElement("span"), { className: "context", textContent: hit.context }),
      );
      button.addEventListener("click", () => choose(hit));
      const item = document.createElement("li");
      item.append(button);
      return item;
    }));
    if (!hits.length && empty) list.append(Object.assign(document.createElement("li"), { className: "empty", textContent: empty }));
    list.hidden = !list.children.length;
    input.setAttribute("aria-expanded", String(!list.hidden));
  }

  function choose(hit) {
    show([]);
    input.value = hit.name;
    input.blur(); // puts a phone's keyboard away
    map.flyTo({ center: hit.lngLat, zoom: ZOOM[hit.kind] ?? 16 });
    const root = Object.assign(document.createElement("div"), { className: "poi-popup" });
    root.append(
      Object.assign(document.createElement("h3"), { textContent: hit.name }),
      Object.assign(document.createElement("p"), { textContent: hit.context }),
    );
    const popup = new maplibregl.Popup({ offset: 30 });
    const buttons = routing?.buttons(hit.lngLat, () => popup.remove(), hit.name);
    if (buttons) root.append(buttons);
    marker.setLngLat(hit.lngLat).setPopup(popup.setDOMContent(root)).addTo(map);
    marker.togglePopup();
  }

  // Enter takes the first result for what is typed now, not for what the
  // list showed a keystroke ago.
  form.addEventListener("submit", async (event) => {
    event.preventDefault();
    clearTimeout(timer);
    const seq = runs + 1;
    await run();
    if (seq === runs && shown.length) choose(shown[0]);
  });
  // Up and down move through the results; Escape closes them.
  form.addEventListener("keydown", (event) => {
    const buttons = [...list.querySelectorAll("button")];
    const at = buttons.indexOf(document.activeElement);
    if (event.key === "Escape") {
      show([]);
      input.focus();
    } else if (event.key === "ArrowDown" || event.key === "ArrowUp") {
      event.preventDefault();
      const next = at + (event.key === "ArrowDown" ? 1 : -1);
      (next < 0 ? input : buttons[Math.min(next, buttons.length - 1)])?.focus();
    }
  });
  map.on("click", () => show([]));
}
