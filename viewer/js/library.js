// The library page: the books on the drive, and search across all of them:
// titles first, then articles containing the words (launcher /api/library).

import { applyStrings, currentLanguage, loadManifest, t } from "./common.js";

const [manifest, books] = await Promise.all([
  loadManifest(),
  fetch("/api/library").then((res) => (res.ok ? res.json() : [])).catch(() => []),
]);
const lang = currentLanguage(manifest);
applyStrings(lang);
document.title = t(lang, "library");

const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
};
const titleOf = Object.fromEntries(books.map((b) => [b.id, b.title]));

document.getElementById("empty").hidden = books.length > 0;
document.getElementById("search").hidden = books.length === 0;
document.getElementById("books").replaceChildren(...books.map((b) => {
  const icon = el("img", { src: `/api/library/icon?book=${encodeURIComponent(b.id)}`, alt: "", width: 48, height: 48 });
  icon.addEventListener("error", () => icon.remove());
  return el("a", { className: "card", href: `/library/${encodeURIComponent(b.id)}/` },
    icon, el("h2", { textContent: b.title }), el("p", { textContent: b.description }));
}));

const input = document.getElementById("q");
const hits = document.getElementById("hits");
const none = document.getElementById("no-hits");
input.placeholder = t(lang, "searchLibrary");
input.setAttribute("aria-label", t(lang, "searchLibrary"));

// Shows which books cannot be searched inside yet, and checks again until
// they can.
const preparing = document.getElementById("preparing");
function showPreparing(list) {
  preparing.hidden = list.length === 0;
  preparing.textContent = list.map((p) => t(lang, "searchPreparing")
    .replace("{book}", p.title).replace("{n}", p.progress)).join(" ");
}
async function watchPreparing() {
  const list = (await fetch("/api/library").then((res) => res.json()).catch(() => []))
    .filter((b) => b.search === "preparing").map((b) => ({ title: b.title, progress: b.progress ?? 0 }));
  showPreparing(list);
  if (list.length) setTimeout(watchPreparing, 3000);
}
watchPreparing();

// The query's words, marked in an excerpt (with their longer forms).
function highlight(text, q) {
  const words = q.toLowerCase().match(/[\p{L}\p{N}]{2,}/gu) ?? [];
  if (!words.length) return [text];
  const escaped = words.map((w) => w.replace(/[.*+?^${}()|[\]\\]/g, "\\$&"));
  const re = new RegExp(`(?:${escaped.join("|")})[\\p{L}\\p{N}]*`, "giu");
  const out = [];
  let last = 0;
  for (const m of text.matchAll(re)) {
    out.push(text.slice(last, m.index), el("mark", { textContent: m[0] }));
    last = m.index + m[0].length;
  }
  out.push(text.slice(last));
  return out;
}

let request = null;
async function search() {
  const q = input.value.trim();
  history.replaceState(null, "", q ? `?q=${encodeURIComponent(q)}` : location.pathname);
  request?.abort();
  if (!q) {
    hits.replaceChildren();
    none.hidden = true;
    return;
  }
  request = new AbortController();
  try {
    const found = await (await fetch(`/api/library/search?q=${encodeURIComponent(q)}`, { signal: request.signal })).json();
    hits.replaceChildren(...found.hits.map((hit) => el("li", {},
      el("div", {}, el("a", { href: hit.url, textContent: hit.title }),
        ...(hit.snippet ? [el("p", { className: "snippet" }, ...highlight(hit.snippet, q))] : [])),
      el("span", { className: "size", textContent: titleOf[hit.book] ?? hit.book }))));
    none.hidden = found.hits.length > 0;
    showPreparing(found.preparing);
  } catch (err) {
    if (err.name !== "AbortError") none.hidden = false;
  }
}

let timer = 0;
input.addEventListener("input", () => { clearTimeout(timer); timer = setTimeout(search, 150); });
document.getElementById("search").addEventListener("submit", (event) => {
  event.preventDefault();
  const first = hits.querySelector("a");
  if (first) location.href = first.href;
});
input.value = new URLSearchParams(location.search).get("q") ?? "";
if (input.value) search();
input.focus();
