import * as maplibregl from "../vendor/maplibre-gl.mjs";
import { layers as basemapLayers, namedFlavor } from "../vendor/basemaps.mjs";
import {
  applyStrings, currentLanguage, isDark, loadManifest, pick, store, stored, t,
} from "./common.js";
import { initShell } from "./shell.js";
import { icon } from "./icons.js";
import { setUpRouting } from "./route.js";
import { setUpSearch } from "./search.js";

const SOURCE = "basemap";
const POI_STATE_KEY = "varu.pois";
const LABEL_FONT = ["Noto Sans Medium"];
const OVERVIEW_ZOOM = 12; // matches overviewZoom in installer/pmtiles.go
// Each layer's icon, by its id in the region file.
const POI_ICONS = {
  shelter: "shield", hospital: "hospital", pharmacy: "pill", water: "droplet", spring: "droplets",
  clinic: "stethoscope", fuel: "fuel", food: "shopping-basket", police: "badge", rescue: "flame", border: "flag",
};
// "Find nearest" always offers these, and any other layer that is switched on.
const NEAREST_ALWAYS = ["shelter", "hospital", "water", "pharmacy"];

const manifest = await loadManifest();
const lang = currentLanguage(manifest);
applyStrings(lang);
initShell(manifest, lang);

const message = document.getElementById("map-message");
function showMessage(key) {
  message.textContent = t(lang, key);
  message.hidden = false;
}

// Absolute URLs: MapLibre resolves sprites and glyphs in a worker, where
// relative paths have no page to be relative to.
const base = new URL("./", document.baseURI).href;
const dark = isDark();
// The map's colours are fixed when it loads; a new theme needs a fresh start.
document.addEventListener("varu-theme", () => location.reload());
let routing = null; // Route here / Start here, when the drive has road data

if (!manifest.map?.file) {
  showMessage("noMap");
} else {
  startMap(manifest.map);
}

function startMap(mapInfo) {
  // `pmtiles` is a global from vendor/pmtiles.js, loaded by map.html.
  const protocol = new globalThis.pmtiles.Protocol();
  maplibregl.addProtocol("pmtiles", protocol.tile);

  const flavor = dark ? "dark" : "light";

  const sources = {
    [SOURCE]: {
      type: "vector",
      url: `pmtiles://${base}${mapInfo.file}`,
      attribution: "© OpenStreetMap contributors · Protomaps",
    },
  };
  let layers = basemapLayers(SOURCE, namedFlavor(flavor), { lang });

  // A map cut around home has street detail only near home; elsewhere it
  // stops at OVERVIEW_ZOOM. Draw that overview, stretched, underneath, so
  // zooming in outside the radius shows roads instead of a blank map.
  if (Number.isFinite(manifest.home?.radius_km)) {
    sources.overview = { type: "vector", tiles: [`pmtiles://${base}${mapInfo.file}/{z}/{x}/{y}`], maxzoom: OVERVIEW_ZOOM };
    const overview = basemapLayers("overview", namedFlavor(flavor), { lang })
      .filter((layer) => layer.type !== "symbol" && layer.type !== "background")
      .map((layer) => ({ ...layer, id: `overview-${layer.id}`, minzoom: OVERVIEW_ZOOM + 1 }));
    const background = layers.filter((layer) => layer.type === "background");
    layers = [...background, ...overview, ...layers.filter((layer) => layer.type !== "background")];
  }

  const map = new maplibregl.Map({
    container: "map",
    style: {
      version: 8,
      glyphs: `${base}assets/fonts/{fontstack}/{range}.pbf`,
      sprite: `${base}assets/sprites/v4/${flavor}`,
      sources,
      layers,
    },
    center: mapInfo.center ?? [0, 0],
    zoom: mapInfo.zoom ?? 6,
    maxZoom: 19,
    maxBounds: mapInfo.bounds ? padBounds(mapInfo.bounds, 1.5) : undefined,
    hash: true,
    attributionControl: { compact: true },
    locale: {
      "GeolocateControl.FindMyLocation": t(lang, "findMyLocation"),
      "GeolocateControl.LocationNotAvailable": t(lang, "locationUnavailable"),
    },
  });

  map.addControl(new maplibregl.NavigationControl({ showCompass: false }), "top-right");
  // Your position. A phone's GPS works offline; a laptop usually locates itself
  // online. Browsers allow it only on localhost, not on a drive shared with -lan.
  const geolocate = new maplibregl.GeolocateControl({
    positionOptions: { enableHighAccuracy: true },
    trackUserLocation: true,
  });
  map.addControl(geolocate, "top-right");
  // Remembered so a delayed ?nearest does not switch tracking off by pressing the button again.
  geolocate.on("trackuserlocationstart", () => { tracking = true; });
  geolocate.on("trackuserlocationend", () => { tracking = false; });
  map.addControl(new maplibregl.ScaleControl({ unit: "metric" }), "bottom-left");

  let failed = false;
  map.on("error", (event) => {
    console.error(event.error ?? event);
    // Only a basemap that never loads deserves a full-screen message;
    // a missing glyph range or a single bad tile does not.
    if (!failed && !map.isStyleLoaded() && event.sourceId === SOURCE) {
      failed = true;
      showMessage("mapError");
    }
  });

  // The installer records the family's home; mark it so "nearest" is obvious.
  if (Number.isFinite(manifest.home?.lon) && Number.isFinite(manifest.home?.lat)) {
    new maplibregl.Marker({ color: "#0b5d4b" }).setLngLat([manifest.home.lon, manifest.home.lat]).addTo(map);
  }
  map.on("load", () => {
    routing = setUpRouting(map, { manifest, lang, geolocate, dark });
    setUpSearch(map, { manifest, lang, base, routing });
    setUpPois(map, geolocate);
  });
  setUpSheet();

  // Handy when poking at the map from the browser console.
  window.varuMap = map;
}

function padBounds([west, south, east, north], degrees) {
  return [
    [Math.max(-180, west - degrees), Math.max(-85, south - degrees)],
    [Math.min(180, east + degrees), Math.min(85, north + degrees)],
  ];
}

// ---------- Places (hospitals, pharmacies, ...) ----------

let tracking = false;

function setUpPois(map, geolocate) {
  const list = document.getElementById("layer-list");
  const pois = manifest.pois ?? [];
  if (pois.length === 0) {
    list.append(Object.assign(document.createElement("li"), {
      className: "empty", textContent: t(lang, "noLayers"),
    }));
    return;
  }

  let saved = null;
  try { saved = JSON.parse(stored(POI_STATE_KEY) ?? "null"); } catch { /* ignore */ }
  const enabled = new Set(Array.isArray(saved) ? saved : pois.filter((p) => p.default).map((p) => p.id));

  const showNearest = async (row) => {
    if (!row.toggle.checked) {
      row.toggle.checked = true;
      row.toggle.dispatchEvent(new Event("change"));
    }
    const data = await fetch(`${base}${row.poi.file}`).then((res) => res.json()).catch(() => null);
    routing.nearest((data?.features ?? []).map((f) => ({
      lngLat: f.geometry.coordinates,
      name: f.properties?.[`name:${lang}`] || f.properties?.name || row.label,
    })), { label: row.label, color: row.poi.color, icon: POI_ICONS[row.poi.id] });
  };
  const rows = new Map();
  const hideAll = document.getElementById("layers-hide");
  const chips = document.getElementById("nearest-chips");
  document.getElementById("nearest-box").hidden = !routing;
  const refresh = () => {
    hideAll.hidden = enabled.size === 0;
    if (!routing) return;
    chips.replaceChildren(...[...rows.values()]
      .filter((row) => NEAREST_ALWAYS.includes(row.poi.id) || enabled.has(row.poi.id))
      .map((row) => {
        const chip = Object.assign(document.createElement("button"), { type: "button", className: "chip" });
        chip.style.setProperty("--poi", row.poi.color);
        chip.setAttribute("aria-label", `${t(lang, "nearest")}: ${row.label}`);
        chip.append(icon(POI_ICONS[row.poi.id] ?? "pin"), row.label);
        chip.addEventListener("click", () => {
          map.fire("userroute");
          showNearest(row);
        });
        return chip;
      }));
  };

  for (const poi of pois) {
    const label = pick(lang, poi.label) || poi.id;
    const toggle = Object.assign(document.createElement("input"), {
      type: "checkbox", checked: enabled.has(poi.id), className: "switch",
    });
    toggle.setAttribute("role", "switch");
    const badge = Object.assign(document.createElement("span"), { className: "poi-badge" });
    badge.style.background = poi.color;
    if (POI_ICONS[poi.id]) badge.append(icon(POI_ICONS[poi.id]));
    const text = Object.assign(document.createElement("span"), { className: "layer-text" });
    text.append(Object.assign(document.createElement("strong"), { textContent: label }),
      Object.assign(document.createElement("span"), { textContent: poi.count?.toLocaleString(lang) ?? "" }));
    const row = document.createElement("label");
    row.append(badge, text, toggle);
    const item = document.createElement("li");
    item.append(row);
    list.append(item);

    rows.set(poi.id, { poi, toggle, label });
    toggle.addEventListener("change", () => {
      setPoiVisible(map, poi, toggle.checked);
      if (toggle.checked) enabled.add(poi.id); else enabled.delete(poi.id);
      store(POI_STATE_KEY, JSON.stringify([...enabled]));
      refresh();
    });
    if (toggle.checked) setPoiVisible(map, poi, true);
  }
  hideAll.addEventListener("click", () => {
    for (const { toggle } of rows.values()) {
      if (!toggle.checked) continue;
      toggle.checked = false;
      toggle.dispatchEvent(new Event("change"));
    }
  });
  refresh();
  setUpMore(pois.length);

  // map.html?nearest=<layer id>, from the start page and the emergency guide:
  // show that layer and find the closest one. Without road data, just show
  // the layer; without a location the route starts from home, or says so.
  const wanted = rows.get(new URLSearchParams(location.search).get("nearest"));
  if (wanted) openNearest(map, geolocate, wanted, showNearest);
}

// On a phone the layers are a sheet at the bottom showing the first few;
// "Show all" opens the rest. A route card takes its place. The map's own
// buttons keep above whichever is showing (--below).
function setUpSheet() {
  const area = document.getElementById("map-area");
  const sheet = document.getElementById("layers");
  const route = document.getElementById("route-panel");
  const below = new ResizeObserver(() => area.style.setProperty("--below",
    `${Math.max(sheet.offsetHeight, route.hidden ? 0 : route.offsetHeight + 8)}px`));
  below.observe(sheet);
  below.observe(route);
}

function setUpMore(count) {
  const sheet = document.getElementById("layers");
  const more = document.getElementById("layers-more");
  const SHOWN = 4;
  if (count <= SHOWN) return;
  more.hidden = false;
  const show = (all) => {
    sheet.classList.toggle("all", all);
    more.setAttribute("aria-expanded", String(all));
    more.textContent = all ? t(lang, "showFewer") : t(lang, "showAllN").replace("{n}", count);
  };
  show(false);
  more.addEventListener("click", () => show(!sheet.classList.contains("all")));
}

// Waits for the first position fix (up to 8 s), then asks for the nearest.
async function openNearest(map, geolocate, row, showNearest) {
  const { poi, toggle } = row;
  if (!toggle.checked) {
    toggle.checked = true;
    toggle.dispatchEvent(new Event("change"));
  }
  if (!routing) {
    const data = await fetch(`${base}${poi.file}`).then((res) => res.json()).catch(() => null);
    const bounds = new maplibregl.LngLatBounds();
    for (const f of data?.features ?? []) bounds.extend(f.geometry.coordinates);
    if (!bounds.isEmpty()) map.fitBounds(bounds, { padding: 60, maxZoom: 15, duration: 0 });
    return;
  }
  // Any route the person starts meanwhile (Nearest, Route here, Start here) wins.
  let cancelled = false;
  const cancel = () => { cancelled = true; };
  map.once("userroute", cancel);
  const located = new Promise((resolve) => {
    const done = () => {
      clearTimeout(timer);
      for (const type of ["geolocate", "error", "outofmaxbounds"]) geolocate.off(type, done);
      map.off("userroute", done);
      resolve();
    };
    const timer = setTimeout(done, 8000);
    for (const type of ["geolocate", "error", "outofmaxbounds"]) geolocate.on(type, done);
    map.on("userroute", done);
  });
  if (!tracking) geolocate.trigger();
  await located;
  map.off("userroute", cancel);
  if (!cancelled) showNearest(row);
}


function setPoiVisible(map, poi, visible) {
  const source = `poi-${poi.id}`;
  const circle = `${source}-circle`;
  const label = `${source}-label`;

  if (!map.getSource(source)) {
    if (!visible) return;
    // Loaded on first use so an unused layer costs nothing.
    map.addSource(source, { type: "geojson", data: `${base}${poi.file}` });
    map.addLayer({
      id: circle,
      type: "circle",
      source,
      paint: {
        "circle-color": poi.color,
        "circle-radius": ["interpolate", ["linear"], ["zoom"], 5, 2.5, 10, 5, 15, 8],
        "circle-stroke-color": "#ffffff",
        "circle-stroke-width": ["interpolate", ["linear"], ["zoom"], 5, 0.5, 12, 2],
      },
    });
    map.addLayer({
      id: label,
      type: "symbol",
      source,
      minzoom: 12,
      layout: {
        "text-field": ["coalesce", ["get", `name:${lang}`], ["get", "name"], ""],
        "text-font": LABEL_FONT,
        "text-size": 12,
        "text-offset": [0, 1.1],
        "text-anchor": "top",
        "text-optional": true,
      },
      paint: {
        // Same ink and background as app.css, so labels read on either map flavour.
        "text-color": dark ? "#e9e6dd" : "#1b2420",
        "text-halo-color": dark ? "#121715" : "#ffffff",
        "text-halo-width": 1.5,
      },
    });
    map.on("click", circle, (event) => showPopup(map, poi, event.features[0]));
    map.on("mouseenter", circle, () => { map.getCanvas().style.cursor = "pointer"; });
    map.on("mouseleave", circle, () => { map.getCanvas().style.cursor = ""; });
    return;
  }

  const visibility = visible ? "visible" : "none";
  map.setLayoutProperty(circle, "visibility", visibility);
  map.setLayoutProperty(label, "visibility", visibility);
}

function showPopup(map, poi, feature) {
  const props = feature.properties ?? {};
  const root = Object.assign(document.createElement("div"), { className: "poi-popup" });
  const line = (tag, text, className) => {
    if (!text) return;
    root.append(Object.assign(document.createElement(tag), { textContent: text, className: className ?? "" }));
  };

  const kind = pick(lang, poi.label);
  line("p", kind, "kind");
  const name = props[`name:${lang}`] || props.name;
  line("h3", name || kind);

  const street = [props["addr:street"], props["addr:housenumber"]].filter(Boolean).join(" ");
  line("p", [street, props["addr:city"]].filter(Boolean).join(", "));
  if (props.opening_hours) line("p", `${t(lang, "openingHours")}: ${props.opening_hours}`);
  const phone = props.phone || props["contact:phone"];
  if (phone) line("p", `${t(lang, "phone")}: ${phone}`);
  if (props.operator) line("p", `${t(lang, "operator")}: ${props.operator}`);

  const popup = new maplibregl.Popup({ offset: 10 }).setLngLat(feature.geometry.coordinates);
  const buttons = routing?.buttons(feature.geometry.coordinates, () => popup.remove(), name);
  if (buttons) root.append(buttons);
  popup.setDOMContent(root).addTo(map);
}
