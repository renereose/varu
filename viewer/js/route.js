// Directions on the drive's own road network (launcher /api/route).
//
// Destination: "Route here" from a right-click or long-press on the map, or
// from a place popup. Start: an explicit "Start here", else a GPS fix from
// the last 5 minutes, else the home the installer recorded.

import * as maplibregl from "../vendor/maplibre-gl.mjs";
import { store, stored, t } from "./common.js";
import { setUpNavigation } from "./nav.js";

const MODE_KEY = "varu.routeMode";
const GPS_MAX_AGE = 5 * 60 * 1000;
const LONG_PRESS_MS = 550;
const SNAP_NOTE_M = 30;
const ERRORS = ["startTooFar", "endTooFar", "notConnected", "noGraph"];
// "Nearest" compares this many places, closest as the crow flies, by road.
const NEAREST_TRIES = 5;

/** Returns { buttons(lngLat, done), nearest(places) }, or null without road data. */
export function setUpRouting(map, { manifest, lang, geolocate, dark }) {
  if (!manifest.routing?.file) return null;

  const panel = document.getElementById("route-panel");
  const summary = document.getElementById("route-summary");
  const notes = document.getElementById("route-notes");
  const modeButtons = panel.querySelectorAll("[data-mode]");
  const home = Number.isFinite(manifest.home?.lat) ? [manifest.home.lon, manifest.home.lat] : null;

  let mode = stored(MODE_KEY) === "car" ? "car" : "foot";
  let start = null; // explicit start, [lon, lat]
  let dest = null;
  let destName = null;
  let gps = null;
  let request = null;
  let current = null; // the route on the map, a GeoJSON Feature
  const startButton = document.getElementById("route-start");
  const startMarker = new maplibregl.Marker({ color: "#5d6963" });
  const destMarker = new maplibregl.Marker({ color: "#1565c0" });

  geolocate.on("geolocate", (pos) => {
    gps = { at: Date.now(), lngLat: [pos.coords.longitude, pos.coords.latitude] };
  });
  // The location button, while it follows you, moves the map itself.
  let tracking = false;
  geolocate.on("trackuserlocationstart", () => { tracking = true; });
  geolocate.on("trackuserlocationend", () => { tracking = false; });

  // Under the basemap's labels, so street names stay readable.
  const beforeId = map.getStyle().layers.find((layer) => layer.type === "symbol")?.id;
  map.addSource("route", { type: "geojson", data: { type: "FeatureCollection", features: [] } });
  map.addLayer({
    id: "route-casing", type: "line", source: "route",
    layout: { "line-join": "round", "line-cap": "round" },
    paint: { "line-color": dark ? "#0d1512" : "#ffffff", "line-width": ["interpolate", ["linear"], ["zoom"], 6, 5, 16, 12] },
  }, beforeId);
  map.addLayer({
    id: "route-line", type: "line", source: "route",
    layout: { "line-join": "round", "line-cap": "round" },
    paint: { "line-color": "#1565c0", "line-width": ["interpolate", ["linear"], ["zoom"], 6, 3, 16, 7] },
  }, beforeId);

  const setLine = (feature) => map.getSource("route").setData(feature ?? { type: "FeatureCollection", features: [] });
  const say = (text, extra = []) => {
    panel.hidden = false;
    summary.textContent = text;
    notes.replaceChildren(...extra.map((line) => Object.assign(document.createElement("p"), { textContent: line })));
  };

  function from() {
    if (start) return { lngLat: start, key: "fromPoint" };
    if (gps && Date.now() - gps.at < GPS_MAX_AGE) return { lngLat: gps.lngLat, key: "fromGps" };
    if (home) return { lngLat: home, key: "fromHome" };
    return null;
  }

  async function update() {
    request?.abort();
    setLine(null);
    current = null;
    startButton.hidden = true;
    const origin = from();
    if (!dest) return say(t(lang, "needDestination"), origin ? [t(lang, origin.key)] : []);
    if (!origin) return say(t(lang, "needStart"));
    say(t(lang, "routing"));
    request = new AbortController();
    const point = ([lon, lat]) => `${lat.toFixed(6)},${lon.toFixed(6)}`;
    try {
      const res = await fetch(`api/route?from=${point(origin.lngLat)}&to=${point(dest)}&mode=${mode}`,
        { signal: request.signal });
      const body = await res.json();
      if (!res.ok) return say(t(lang, ERRORS.includes(body.error) ? body.error : "noRoute"));
      setLine(body);
      current = body;
      if (nav.active()) return; // it says what is left itself
      startButton.hidden = false;
      const p = body.properties;
      const extra = [destName, t(lang, origin.key)].filter(Boolean);
      if (p.ferry) extra.push(t(lang, "routeFerry"));
      const snapped = Math.max(p.snap_from_m, p.snap_to_m);
      if (snapped > SNAP_NOTE_M) extra.push(t(lang, "snapNote").replace("{n}", formatDistance(snapped, lang)));
      say(`${formatDistance(p.distance_m, lang)} · ${formatDuration(p.duration_s)}`, extra);
      const bounds = body.geometry.coordinates.reduce((b, c) => b.extend(c), new maplibregl.LngLatBounds());
      const pad = 48;
      map.fitBounds(bounds, { padding: { top: pad, left: pad, right: pad, bottom: pad + panel.offsetHeight }, maxZoom: 16 });
    } catch (err) {
      if (err.name !== "AbortError") say(t(lang, "noRoute"));
    }
  }

  function routeTo(lngLat, name = null) {
    map.fire("userroute");
    dest = lngLat;
    destName = name;
    destMarker.setLngLat(dest).addTo(map);
    update();
  }

  function startFrom(lngLat) {
    map.fire("userroute");
    start = lngLat;
    startMarker.setLngLat(start).addTo(map);
    update();
  }

  function clear() {
    if (nav.active()) nav.stop();
    request?.abort();
    current = null;
    start = dest = destName = null;
    startMarker.remove();
    destMarker.remove();
    setLine(null);
    panel.hidden = true;
  }

  const showMode = () => modeButtons.forEach((b) => b.setAttribute("aria-pressed", String(b.dataset.mode === mode)));
  showMode();
  modeButtons.forEach((b) => b.addEventListener("click", () => {
    mode = b.dataset.mode;
    store(MODE_KEY, mode);
    showMode();
    if (dest) update();
  }));
  document.getElementById("route-clear").addEventListener("click", clear);

  const nav = setUpNavigation(map, {
    lang,
    mode: () => mode,
    route: () => current,
    destName: () => destName,
    here: () => (gps && Date.now() - gps.at < GPS_MAX_AGE ? gps.lngLat : null),
    // Live directions steer the map; the location button would undo it.
    stopFollowing: () => { if (tracking) geolocate.trigger(); },
    reroute(lngLat) {
      gps = { at: Date.now(), lngLat };
      start = null;
      startMarker.remove();
      return update();
    },
    stopped: () => update(),
    formatDistance: (m) => formatDistance(m, lang),
    formatDuration,
    say,
  });
  startButton.addEventListener("click", () => nav.start());
  document.getElementById("route-end").addEventListener("click", () => nav.stop());

  // name, when given, is shown with a route there.
  function buttons(lngLat, done, name = null) {
    const point = maplibregl.LngLat.convert(lngLat).toArray();
    const box = Object.assign(document.createElement("div"), { className: "route-buttons" });
    for (const [key, act] of [["routeHere", () => routeTo(point, name)], ["startHere", () => startFrom(point)]]) {
      const button = Object.assign(document.createElement("button"), { type: "button", textContent: t(lang, key) });
      button.addEventListener("click", () => { done?.(); act(); });
      box.append(button);
    }
    return box;
  }

  // One small menu at a time: right-click, long-press, or the keyboard menu key at the map centre.
  // Lifting the finger after a long press sends a click, which must not close the menu.
  let menu = null;
  let touchDown = false;
  let keepMenu = false;
  function openMenu(lngLat) {
    menu?.remove();
    keepMenu = touchDown;
    menu = new maplibregl.Popup({ offset: 6, closeOnClick: false }).setLngLat(lngLat);
    menu.setDOMContent(buttons(lngLat, () => menu.remove())).addTo(map);
    menu.getElement().querySelector("button")?.focus();
  }
  map.on("contextmenu", (event) => openMenu(event.lngLat));
  map.on("click", () => {
    if (keepMenu) keepMenu = false;
    else menu?.remove();
  });

  const canvas = map.getCanvasContainer();
  let timer = 0;
  const cancel = () => { touchDown = false; clearTimeout(timer); };
  canvas.addEventListener("touchstart", (event) => {
    cancel();
    keepMenu = false;
    touchDown = event.touches.length === 1;
    if (!touchDown) return;
    const rect = canvas.getBoundingClientRect();
    const touch = event.touches[0];
    timer = setTimeout(() => openMenu(map.unproject([touch.clientX - rect.left, touch.clientY - rect.top])), LONG_PRESS_MS);
  }, { passive: true });
  canvas.addEventListener("touchend", cancel, { passive: true });
  canvas.addEventListener("touchcancel", cancel, { passive: true });
  map.on("movestart", cancel);
  canvas.addEventListener("keydown", (event) => {
    if (event.key === "ContextMenu" || (event.shiftKey && event.key === "F10")) {
      event.preventDefault();
      openMenu(map.getCenter());
    }
  });

  // The closest of places ([{ lngLat, name }]) by road: the few nearest as the
  // crow flies are routed, and the shortest wins (a river or a bay can make
  // the nearest-looking one far).
  async function nearest(places) {
    const origin = from();
    if (!origin) return say(t(lang, "needStart"));
    if (!places.length) return say(t(lang, "noRoute"));
    say(t(lang, "routing"));
    const [x, y] = origin.lngLat;
    const k = Math.cos((y * Math.PI) / 180);
    // Flat-earth distance, fine for ranking within a country.
    const crow = ({ lngLat: [lon, lat] }) => ((lon - x) * k) ** 2 + (lat - y) ** 2;
    const near = [...places].sort((a, b) => crow(a) - crow(b)).slice(0, NEAREST_TRIES);
    const point = ([lon, lat]) => `${lat.toFixed(6)},${lon.toFixed(6)}`;
    const lengths = await Promise.all(near.map((p) =>
      fetch(`api/route?from=${point(origin.lngLat)}&to=${point(p.lngLat)}&mode=${mode}`)
        .then((res) => (res.ok ? res.json() : null))
        .then((body) => body?.properties?.distance_m ?? Infinity, () => Infinity)));
    const best = near[lengths.indexOf(Math.min(...lengths))];
    routeTo(best.lngLat, best.name);
  }

  return { buttons, nearest };
}

function formatDistance(metres, lang) {
  const km = metres >= 1000;
  return new Intl.NumberFormat(lang, {
    style: "unit", unit: km ? "kilometer" : "meter", maximumFractionDigits: km && metres < 10000 ? 1 : 0,
  }).format(km ? metres / 1000 : Math.round(metres));
}

function formatDuration(seconds) {
  const minutes = Math.max(1, Math.round(seconds / 60));
  const h = Math.floor(minutes / 60);
  return h ? `${h} h ${minutes % 60} min` : `${minutes} min`;
}
