import {
  applyStrings, currentLanguage, formatBytes, loadManifest, loadStatus, pick, plural, t,
} from "./common.js";
import { icon } from "./icons.js";
import { initShare } from "./share.js";
import { initShell } from "./shell.js";

const [manifest, status, books, guides] = await Promise.all([
  loadManifest(),
  loadStatus(),
  fetch("/api/library").then((res) => (res.ok ? res.json() : [])).catch(() => []),
  fetch("/guides.json").then((res) => (res.ok ? res.json() : {})).catch(() => ({})),
]);
const lang = currentLanguage(manifest);
applyStrings(lang);
initShell(manifest, lang, status);
document.title = pick(lang, manifest.name) || "Varu";

const $ = (id) => document.getElementById(id);
function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
}
const date = (value, month) => {
  const d = new Date(value);
  // Day before month in English too, as in Estonia.
  const locale = lang === "en" ? "en-GB" : lang;
  return Number.isNaN(d.getTime()) ? "" : d.toLocaleDateString(locale, { day: "numeric", month, year: "numeric" });
};

// The map's data date, from its build ("20261125").
const mapBuild = /^(\d{4})(\d{2})(\d{2})$/.exec(manifest.map?.build ?? "");
const mapDate = mapBuild ? `${mapBuild[1]}-${mapBuild[2]}-${mapBuild[3]}T12:00` : null;
$("meta-wide").textContent = [t(lang, "heroStored"),
  mapDate && t(lang, "heroMapDate").replace("{date}", date(mapDate, "long"))].filter(Boolean).join(" ");
const region = pick(lang, manifest.name).replace(/^Varu\s*[–-]\s*/, "");
$("meta-narrow").textContent = [region !== pick(lang, manifest.name) && region, t(lang, "heroStoredShort"),
  mapDate && date(mapDate, "short")].filter(Boolean).join(" · ");

const pois = manifest.pois ?? [];
$("quick-shelter").hidden = !(manifest.map && pois.some((poi) => poi.id === "shelter"));

// ---------- Search and "On this device" ----------

const hasLibrary = Boolean(status.library) && books.length > 0;
const search = $("home-search");
search.hidden = !hasLibrary;
search.q.placeholder = t(lang, "searchLibrary");
search.q.setAttribute("aria-label", t(lang, "searchLibrary"));

$("card-map").hidden = !manifest.map;
$("card-map-text").textContent = [t(lang, "mapsText"), manifest.routing && t(lang, "mapsRoute")].filter(Boolean).join(" ");
const shelters = pois.find((poi) => poi.id === "shelter");
$("card-map-meta").textContent = [shelters && plural(lang, "shelters", shelters.count ?? 0),
  pois.length && plural(lang, "layerCount", pois.length)].filter(Boolean).join(" · ");

// Book sizes come from the manifest, which names a book by its file.
const sizeOf = Object.fromEntries((manifest.library ?? []).map((zim) =>
  [(zim.file ?? "").split("/").pop().replace(/\.zim$/, ""), zim.size]));
const total = books.reduce((sum, b) => sum + (sizeOf[b.id] ?? 0), 0);
$("card-library").hidden = !hasLibrary;
$("card-library-meta").textContent = [plural(lang, "books", books.length), formatBytes(total)].filter(Boolean).join(" · ");
$("library-list").hidden = !hasLibrary;
$("library-list").replaceChildren(...books.map((b) => el("li", {}, el("a", { href: `/library/${encodeURIComponent(b.id)}/` },
  el("span", { className: "row-icon" }, icon("book-open")),
  el("span", { className: "row-text" }, el("strong", { textContent: b.title }), el("span", { textContent: b.description ?? "" })),
  el("span", { className: "size", textContent: formatBytes(sizeOf[b.id]) })))));

const situations = (guides.situations ?? []).length;
$("card-guide-text").textContent = situations ? t(lang, "guideText").replace("{n}", situations) : t(lang, "emergencyText");
$("card-guide-meta").textContent = [situations && plural(lang, "situations", situations), t(lang, "printablePlan")]
  .filter(Boolean).join(" · ");

const docs = manifest.documents ?? [];
$("documents").hidden = docs.length === 0;
$("doc-list").replaceChildren(...docs.map((doc) => {
  const link = el("a", { href: doc.file },
    el("span", { className: "row-icon" }, icon("clipboard-list")),
    el("span", { className: "row-text" }, el("strong", { textContent: pick(lang, doc.title) || doc.file })),
    el("span", { className: "size", textContent: formatBytes(doc.size) }));
  if (doc.file.endsWith(".pdf")) link.target = "_blank";
  return el("li", {}, link);
}));

// ---------- Sharing and the footer ----------

$("lan-note").hidden = !(status.lan && status.remote);
// Open on a wide screen; on a phone it is one tap away.
$("share").open = window.matchMedia("(min-width: 720px)").matches;
initShare(status, () => lang)();

$("attribution").replaceChildren(...(manifest.attribution ?? []).map((line) => el("li", { textContent: line })));
$("version").textContent = [status.version && `Varu ${status.version}`,
  manifest.built && `${t(lang, "built")} ${date(manifest.built, "short")}`].filter(Boolean).join(" · ");
