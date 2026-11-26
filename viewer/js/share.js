// The "share with other devices" panel on the start page.
import { t } from "./common.js";

const $ = (id) => document.getElementById(id);

// Dark modules on white with a 4-module quiet zone, so it scans in dark mode too.
function qrSvg(qrcode, text) {
  const qr = qrcode(0, "M");
  qr.addData(text);
  qr.make();
  const count = qr.getModuleCount();
  const size = count + 8;
  let path = "";
  for (let row = 0; row < count; row++) {
    for (let col = 0; col < count; col++) {
      if (qr.isDark(row, col)) path += `M${col + 4} ${row + 4}h1v1h-1z`;
    }
  }
  const ns = "http://www.w3.org/2000/svg";
  const svg = document.createElementNS(ns, "svg");
  svg.setAttribute("viewBox", `0 0 ${size} ${size}`);
  svg.setAttribute("shape-rendering", "crispEdges");
  svg.setAttribute("role", "img");
  const bg = document.createElementNS(ns, "rect");
  bg.setAttribute("width", size);
  bg.setAttribute("height", size);
  bg.setAttribute("fill", "#fff");
  const modules = document.createElementNS(ns, "path");
  modules.setAttribute("d", path);
  modules.setAttribute("fill", "#000");
  svg.append(bg, modules);
  return svg;
}

let qrLib = null;
async function loadQr() {
  qrLib ??= import("../vendor/qrcode.mjs").then((m) => m.default);
  try {
    return await qrLib;
  } catch (err) {
    qrLib = null;
    throw err;
  }
}

/** Wire the panel up. `status` is the first /api/status answer; returns a function that redraws it. */
export function initShare(status, getLang) {
  const panel = $("share");
  let error = "";
  let drawn = 0;
  let busy = false; // one request at a time, so replies can't arrive out of order

  async function request(on) {
    if (busy) return;
    busy = true;
    error = "";
    try {
      const res = await fetch("/api/share", {
        method: "POST",
        headers: { "X-Varu": "1", "Content-Type": "application/json" },
        body: JSON.stringify({ on }),
      });
      if (!res.ok) {
        error = (await res.text()).trim() || t(getLang(), "shareFailed");
      } else {
        status = await res.json();
      }
    } catch {
      error = t(getLang(), "shareFailed");
    }
    busy = false;
    render();
  }

  async function reload() {
    if (busy) return;
    busy = true;
    error = "";
    try {
      const res = await fetch("/api/status");
      if (res.ok) status = await res.json();
    } catch {
      error = t(getLang(), "shareFailed");
    }
    busy = false;
    render();
  }

  async function drawCode(address, label) {
    const box = $("share-qr");
    const turn = ++drawn;
    try {
      const qrcode = await loadQr();
      if (turn !== drawn) return;
      const svg = qrSvg(qrcode, address);
      svg.setAttribute("aria-label", label);
      box.replaceChildren(svg);
      box.hidden = false;
    } catch {
      box.replaceChildren();
      box.hidden = true;
    }
  }

  function render() {
    // Another device cannot turn sharing on or off, and a launcher older
    // than the switch doesn't say "remote".
    panel.hidden = status.remote !== false;
    if (panel.hidden) return;
    const lang = getLang();
    const addresses = status.lan ? (status.addresses ?? []) : [];
    $("share-off").hidden = status.lan;
    $("share-live").hidden = !(status.lan && addresses.length > 0);
    $("share-none").hidden = !(status.lan && addresses.length === 0);
    $("share-pill").hidden = $("share-live").hidden;

    const list = $("share-addresses");
    list.replaceChildren(...addresses.map((address) => {
      const item = document.createElement("li");
      item.textContent = address;
      return item;
    }));

    if (addresses.length > 0) {
      drawCode(addresses[0], t(lang, "shareCodeFor").replace("{address}", addresses[0]));
    } else {
      drawn++;
      $("share-qr").replaceChildren();
      $("share-qr").hidden = true;
    }

    const message = $("share-error");
    message.textContent = error;
    message.hidden = !error;
  }

  $("share-on").addEventListener("click", () => request(true));
  $("share-off-btn").addEventListener("click", () => request(false));
  $("share-stop-none").addEventListener("click", () => request(false));
  $("share-retry").addEventListener("click", reload);
  return render;
}
