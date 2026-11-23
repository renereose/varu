import {
  applyStrings, availableLanguages, currentLanguage, formatBytes,
  loadManifest, loadStatus, pick, setLanguage, t,
} from "./common.js";

const [manifest, status] = await Promise.all([loadManifest(), loadStatus()]);
let lang = currentLanguage(manifest);

function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
}

function renderLanguages() {
  const nav = document.getElementById("langs");
  nav.replaceChildren();
  const offered = availableLanguages(manifest);
  if (offered.length < 2) return;
  for (const code of offered) {
    const button = el("button", { type: "button", textContent: code });
    button.setAttribute("aria-pressed", String(code === lang));
    button.addEventListener("click", () => {
      lang = code;
      setLanguage(code);
      render();
    });
    nav.append(button);
  }
}

function setCard(id, available, emptyTextId) {
  const card = document.getElementById(id);
  card.classList.toggle("is-empty", !available);
  if (!available) {
    card.setAttribute("aria-disabled", "true");
    card.tabIndex = -1;
    document.getElementById(emptyTextId).textContent = t(lang, "notIncluded");
  }
}

function renderList(listId, panelId, items, describe) {
  const list = document.getElementById(listId);
  list.replaceChildren();
  document.getElementById(panelId).hidden = items.length === 0;
  for (const item of items) {
    const { href, title } = describe(item);
    const link = el("a", { href, textContent: title });
    if (href.endsWith(".pdf")) link.target = "_blank";
    list.append(el("li", {}, link, el("span", { className: "size", textContent: formatBytes(item.size) })));
  }
}

function render() {
  applyStrings(lang);
  renderLanguages();

  const name = pick(lang, manifest.name) || "Varu";
  document.title = name;
  document.getElementById("drive-name").textContent = name;

  const meta = [];
  if (manifest.built) meta.push(`${t(lang, "built")} ${manifest.built.slice(0, 10)}`);
  if (manifest.map?.build) meta.push(`${t(lang, "mapData")} ${manifest.map.build}`);
  document.getElementById("drive-meta").textContent = meta.join(" · ");

  const docs = manifest.documents ?? [];
  const zims = manifest.library ?? [];

  setCard("card-map", Boolean(manifest.map), "card-map-text");
  setCard("card-library", Boolean(status.library), "card-library-text");
  setCard("card-docs", docs.length > 0, "card-docs-text");

  renderList("doc-list", "documents", docs, (doc) => ({
    href: doc.file,
    title: pick(lang, doc.title) || doc.file,
  }));
  renderList("library-list", "library-list-panel", status.library ? zims : [], (zim) => ({
    href: `library/${encodeURIComponent((zim.file ?? "").replace(/\.zim$/, ""))}/`,
    title: zim.title || zim.name || zim.file,
  }));

  document.getElementById("lan-note").hidden = !status.lan;

  const attribution = document.getElementById("attribution");
  attribution.replaceChildren(...(manifest.attribution ?? []).map((line) => el("li", { textContent: line })));
  document.getElementById("version").textContent = status.version ? `Varu ${status.version}` : "";
}

render();
