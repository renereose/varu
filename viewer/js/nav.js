// Live directions: Start follows your position along the route, shows the
// next turn and what is left, and finds a new route when you leave it.
// The steps come from the launcher's router (/api/route, properties.steps).

import * as maplibregl from "../vendor/maplibre-gl.mjs";
import { t } from "./common.js";

const ARRIVED_M = 20;
// Off the route by more than this (plus the fix's own inaccuracy, up to
// 50 m) twice in a row: find a new route.
const OFF_ROUTE_M = { foot: 30, car: 50 };
// A position may not jump back along the route by more than this, so a
// street the route uses twice is matched to the right pass.
const BACKTRACK_M = 50;
// A lost signal is shown only once the last fix is this old.
const STALE_MS = 15000;

// Arrow rotation for each turn; roundabouts and arrival have their own icon.
const ANGLE = {
  straight: 0, "slight right": 45, right: 90, "sharp right": 135, uturn: 180,
  "slight left": -45, left: -90, "sharp left": -135,
};
const ARROW = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M12 3 5 11h4.5v10h5V11H19z" fill="currentColor"/></svg>`;
const ROUNDABOUT = `<svg viewBox="0 0 24 24" aria-hidden="true"><circle cx="12" cy="13" r="5" fill="none" stroke="currentColor" stroke-width="2.5"/><path d="M12 1 8 5.5h2.75V8h2.5V5.5H16z" fill="currentColor"/></svg>`;
const FLAG = `<svg viewBox="0 0 24 24" aria-hidden="true"><path d="M6 21V3h2v1h10l-2.5 4L18 12H8v9z" fill="currentColor"/></svg>`;

/**
 * hooks: lang, mode() ("foot"/"car"), route() (the GeoJSON Feature shown),
 * destName(), here() (a recent position, [lon, lat], or null),
 * reroute(lngLat) (route again from there), stopFollowing() (the location
 * button lets go of the map), stopped(),
 * formatDistance(m), formatDuration(s), say(summary, notes).
 */
export function setUpNavigation(map, hooks) {
  const { lang } = hooks;
  const banner = document.getElementById("nav-banner");
  const arrow = document.getElementById("nav-arrow");
  const distance = document.getElementById("nav-distance");
  const text = document.getElementById("nav-text");
  const road = document.getElementById("nav-name");

  const me = document.createElement("div");
  me.className = "nav-me";
  const marker = new maplibregl.Marker({ element: me, rotationAlignment: "map", pitchAlignment: "map" });

  let watch = null;
  let lock = null;
  let along = 0; // metres along the current route
  let off = 0; // fixes in a row off the route
  let rerouting = false;
  let lastFix = 0;
  let lastReroute = 0;
  let shape = null; // { line, cum, feature } for the route being followed

  function show(icon, rotate, dist, words, name) {
    banner.hidden = false;
    arrow.innerHTML = icon;
    arrow.style.transform = rotate ? `rotate(${rotate}deg)` : "";
    distance.textContent = dist;
    text.textContent = words;
    road.textContent = name ?? "";
  }

  function follow() {
    const feature = hooks.route();
    if (shape?.feature === feature) return shape;
    const line = feature.geometry.coordinates;
    const cum = [0];
    for (let i = 1; i < line.length; i++) cum.push(cum[i - 1] + metres(line[i - 1], line[i]));
    along = 0;
    return (shape = { feature, line, cum });
  }

  function onFix(pos) {
    lastFix = Date.now();
    const here = [pos.coords.longitude, pos.coords.latitude];
    marker.setLngLat(here).addTo(map);
    const feature = hooks.route();
    if (rerouting) return;
    // No route (the last try failed, say far from any road): try again now and then.
    if (!feature) return Date.now() - lastReroute > 10000 && reroute(here);
    const { line, cum } = follow();
    const at = locate(line, cum, here, along - BACKTRACK_M);
    const slack = OFF_ROUTE_M[hooks.mode()] + Math.min(pos.coords.accuracy ?? 0, 50);
    if (at.off > slack) {
      if (++off >= 2) {
        off = 0;
        show(ARROW, 0, "", t(lang, "navRerouting"));
        reroute(here);
      }
      return;
    }
    off = 0;
    along = Math.max(along, at.along);

    const props = feature.properties;
    const total = props.distance_m;
    const heading = pos.coords.speed > 1 && Number.isFinite(pos.coords.heading) ? pos.coords.heading : at.bearing;
    marker.setRotation(heading);
    map.easeTo({
      center: at.point, bearing: heading, pitch: 45, zoom: hooks.mode() === "car" ? 15.5 : 16.5, duration: 900,
      padding: { top: banner.offsetHeight + 16, bottom: document.getElementById("route-panel").offsetHeight + 16 },
    });

    const left = Math.max(0, total - along);
    hooks.say(`${hooks.formatDistance(left)} · ${hooks.formatDuration(props.duration_s * (left / Math.max(total, 1)))}`, []); // the card's head names the place
    if (left < ARRIVED_M) {
      show(FLAG, 0, "", t(lang, "navArrived"), hooks.destName());
      return;
    }
    const next = props.steps.find((s) => s.along_m > along + 3) ?? props.steps.at(-1);
    const dist = hooks.formatDistance(roundNicely(next.along_m - along));
    if (next.type === "arrive") show(FLAG, 0, dist, t(lang, "navArrive"), hooks.destName());
    else if (next.type === "roundabout") show(ROUNDABOUT, 0, dist, t(lang, "navRoundabout").replace("{n}", next.exit), next.name);
    else show(ARROW, ANGLE[next.modifier] ?? 0, dist, t(lang, `nav_${(next.modifier ?? "straight").replace(" ", "_")}`), next.name);
  }

  function reroute(here) {
    rerouting = true;
    lastReroute = Date.now();
    return Promise.resolve(hooks.reroute(here)).finally(() => { rerouting = false; });
  }

  // No permission ends it; a lost signal (indoors, a tunnel) is waited out.
  function onError(err) {
    if (err?.code === 1 || !navigator.geolocation) {
      stop();
      hooks.say(t(lang, "navNoGps"), []);
      return;
    }
    if (Date.now() - lastFix > STALE_MS) show(ARROW, 0, "", t(lang, "navWaiting"));
  }

  function start() {
    if (!navigator.geolocation) return onError();
    hooks.stopFollowing?.();
    document.body.classList.add("navigating");
    show(ARROW, 0, "", t(lang, "navWaiting"));
    shape = null;
    off = 0;
    lastFix = 0;
    // The first fix routes again from where you actually are. A one-off
    // request alongside the watch gets it sooner on some devices.
    let first = true;
    const fix = (pos) => {
      if (watch === null) return;
      if (first) {
        first = false;
        reroute([pos.coords.longitude, pos.coords.latitude]).finally(() => onFix(pos));
        return;
      }
      onFix(pos);
    };
    const options = { enableHighAccuracy: true, maximumAge: 2000, timeout: 30000 };
    const recent = hooks.here();
    if (recent) onFix({ coords: { longitude: recent[0], latitude: recent[1] } });
    watch = navigator.geolocation.watchPosition(fix, onError, options);
    navigator.geolocation.getCurrentPosition(fix, () => {}, options);
    navigator.wakeLock?.request("screen").then((l) => { lock = l; }).catch(() => {});
  }

  function stop() {
    if (watch !== null) navigator.geolocation.clearWatch(watch);
    watch = null;
    lock?.release().catch(() => {});
    lock = null;
    marker.remove();
    banner.hidden = true;
    document.body.classList.remove("navigating");
    map.easeTo({ pitch: 0, bearing: 0, duration: 600 });
    hooks.stopped();
  }

  return { start, stop, active: () => watch !== null };
}

// Flat-earth metres, plenty for following a route.
function metres([lon1, lat1], [lon2, lat2]) {
  const kx = 111320 * Math.cos(((lat1 + lat2) / 2) * Math.PI / 180);
  return Math.hypot((lon2 - lon1) * kx, (lat2 - lat1) * 110540);
}

/** The nearest point of the line to p, at least `from` metres along it. */
function locate(line, cum, [lon, lat], from) {
  const kx = 111320 * Math.cos(lat * Math.PI / 180);
  const ky = 110540;
  let best = { off: Infinity, along: 0, point: line[0], bearing: 0 };
  for (let i = 1; i < line.length; i++) {
    if (cum[i] < from) continue;
    const [ax, ay] = line[i - 1];
    const [bx, by] = line[i];
    const dx = (bx - ax) * kx;
    const dy = (by - ay) * ky;
    const px = (lon - ax) * kx;
    const py = (lat - ay) * ky;
    const len2 = dx * dx + dy * dy;
    const f = len2 ? Math.max(0, Math.min(1, (px * dx + py * dy) / len2)) : 0;
    const off = Math.hypot(px - f * dx, py - f * dy);
    if (off < best.off) {
      best = {
        off,
        along: cum[i - 1] + f * (cum[i] - cum[i - 1]),
        point: [ax + f * (bx - ax), ay + f * (by - ay)],
        bearing: (Math.atan2(dx, dy) * 180) / Math.PI,
      };
    }
  }
  return best;
}

// Distances to a turn in steps a person can act on.
function roundNicely(m) {
  if (m < 100) return Math.max(5, Math.round(m / 5) * 5);
  if (m < 1000) return Math.round(m / 10) * 10;
  return Math.round(m / 100) * 100;
}
