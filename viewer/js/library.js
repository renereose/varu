// The library page: search across the books on the drive, titles first,
// then articles containing the words (launcher /api/library), with a filter
// for one book. /library/<book>/ is the same page with that book chosen.
// While the search box is empty it shows hand-picked survival and DIY
// articles by topic (topics.json), and lists matching ones first among the
// results.

import { applyStrings, currentLanguage, formatBytes, loadManifest, plural, t } from "./common.js";
import { icon } from "./icons.js";
import { initShell } from "./shell.js";

const [manifest, books, topics] = await Promise.all([
  loadManifest(),
  fetch("/api/library").then((res) => (res.ok ? res.json() : [])).catch(() => []),
  fetch("/topics.json").then((res) => (res.ok ? res.json() : {})).catch(() => ({})),
]);
const lang = currentLanguage(manifest);
applyStrings(lang);
initShell(manifest, lang);
document.title = t(lang, "library");

const $ = (id) => document.getElementById(id);
const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
};
const titleOf = Object.fromEntries(books.map((b) => [b.id, b.title]));
const bookPath = (id) => `/library/${id ? `${encodeURIComponent(id)}/` : ""}`;
let filter = books.find((b) => b.id === decodeURIComponent(location.pathname.split("/")[2] ?? ""))?.id ?? "";

$("empty").hidden = books.length > 0;
$("search").hidden = books.length === 0;

// ---------- Books on this device ----------

// Sizes come from the manifest, which names a book by its file.
const sizeOf = Object.fromEntries((manifest.library ?? []).map((zim) =>
  [(zim.file ?? "").split("/").pop().replace(/\.zim$/, ""), zim.size]));
$("books-box").hidden = books.length === 0;
$("books").replaceChildren(...books.map((b) => el("li", {}, el("a", { href: b.home ?? bookPath(b.id) },
  el("span", { className: "row-icon" }, icon("book-open")),
  el("span", { className: "row-text" }, el("strong", { textContent: b.title }),
    el("span", { textContent: [b.description, formatBytes(sizeOf[b.id])].filter(Boolean).join(" · ") })),
  icon("chevron-right", "chev")))));

// ---------- The book filter: a menu on a wide screen, chips on a phone ----------

const select = $("book-select");
const chips = $("book-chips");
const choices = [["", t(lang, "allBooks")], ...books.map((b) => [b.id, b.title])];
select.replaceChildren(...choices.map(([id, title]) => el("option", { value: id, textContent: title })));
chips.replaceChildren(...choices.map(([id, title]) => {
  const chip = el("button", { type: "button", className: "chip", textContent: title });
  chip.dataset.book = id;
  chip.addEventListener("click", () => choose(id));
  return chip;
}));
$("book-select-box").hidden = chips.hidden = books.length < 2;
select.addEventListener("change", () => choose(select.value));
function showFilter() {
  select.value = filter;
  for (const chip of chips.children) chip.setAttribute("aria-pressed", String(chip.dataset.book === filter));
}
function choose(id) {
  filter = id;
  showFilter();
  search();
  watchPreparing();
}
showFilter();

// ---------- Survival and DIY ----------

// topics.json names a book without its edition month, so a newer edition
// keeps the links. Articles of books not on this drive are left out.
const edition = (id) => id.replace(/_\d{4}-\d{2}$/, "");
const topicName = (topic) => topic[lang] ?? topic.en;
const picks = (topics.topics ?? []).flatMap((topic) => topic.articles.map((a) => ({
  ...a, topic, in: books.find((b) => edition(b.id) === a.book),
}))).filter((a) => a.in);
const linkTo = (a) => `/library/${encodeURIComponent(a.in.id)}/${a.path.split("/").map(encodeURIComponent).join("/")}`;
const topicsOnDrive = (topics.topics ?? []).filter((topic) => picks.some((a) => a.topic === topic));
$("topic-list").replaceChildren(...topicsOnDrive.map((topic) => {
  const list = picks.filter((a) => a.topic === topic);
  return el("details", { id: `topic-${topic.id}` },
    el("summary", {}, el("strong", { textContent: topicName(topic) }), el("span", { className: "size", textContent: `${list.length}` }),
      icon("chevron-down", "chev")),
    el("ul", {}, ...list.map((a) => el("li", {}, el("a", { href: linkTo(a), textContent: a.title }),
      el("span", { className: "size", textContent: a.in.title })))));
}));
$("topic-index-box").hidden = topicsOnDrive.length === 0;
$("topic-index").replaceChildren(...topicsOnDrive.map((topic) => el("li", {},
  el("a", { href: `#topic-${topic.id}` }, el("span", { textContent: topicName(topic) }), icon("chevron-right", "chev")))));

// /library/#topic-<id> (from the emergency guide or the list beside) opens that topic.
function openTopic() {
  const target = location.hash.startsWith("#topic-") ? document.getElementById(location.hash.slice(1)) : null;
  if (!target || target.tagName !== "DETAILS") return;
  if (input.value) {
    input.value = "";
    search();
  }
  target.open = true;
  target.scrollIntoView();
}
window.addEventListener("hashchange", openTopic);

// Hand-picked articles whose title has every word typed (as a word start,
// accents ignored: "korge" finds "Kõrge").
const words = (s) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? [];
function picked(q) {
  const typed = words(q);
  const seen = new Set();
  return picks.filter((a) => {
    const title = words(a.title);
    if ((filter && a.in.id !== filter) || !typed.length || !typed.every((w) => title.some((t) => t.startsWith(w)))
      || seen.has(linkTo(a))) return false;
    seen.add(linkTo(a));
    return true;
  }).slice(0, 5);
}

// ---------- Search ----------

const input = $("q");
const hits = $("hits");
const none = $("no-hits");
const count = $("hit-count");
input.placeholder = t(lang, "searchLibrary");
input.setAttribute("aria-label", t(lang, "searchLibrary"));

// Shows which books cannot be searched inside yet, and checks again until
// they can.
const preparing = $("preparing");
function showPreparing(list) {
  preparing.hidden = list.length === 0;
  preparing.textContent = list.map((p) => t(lang, "searchPreparing")
    .replace("{book}", p.title).replace("{n}", p.progress)).join(" ");
}
async function watchPreparing() {
  const list = (await fetch("/api/library").then((res) => res.json()).catch(() => []))
    .filter((b) => b.search === "preparing" && (!filter || b.id === filter)).map((b) => ({ title: b.title, progress: b.progress ?? 0 }));
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

const hit = (href, title, tag, snippet) => el("li", {},
  el("a", { href, textContent: title }),
  el("span", { className: "tag", textContent: tag }),
  ...(snippet ? [el("p", {}, ...snippet)] : []));

let request = null;
async function search() {
  const q = input.value.trim();
  history.replaceState(null, "", `${bookPath(filter)}${q ? `?q=${encodeURIComponent(q)}` : location.hash}`);
  request?.abort();
  $("q-clear").hidden = !q;
  $("topics").hidden = picks.length === 0 || !!q;
  if (!q) {
    hits.replaceChildren();
    hits.hidden = count.hidden = none.hidden = true;
    return;
  }
  request = new AbortController();
  const only = filter ? `&book=${encodeURIComponent(filter)}` : "";
  try {
    const found = await (await fetch(`/api/library/search?q=${encodeURIComponent(q)}${only}`, { signal: request.signal })).json();
    const first = picked(q);
    const shown = new Set(first.map((a) => decodeURIComponent(linkTo(a))));
    const rest = found.hits.filter((h) => !shown.has(decodeURIComponent(h.url)));
    hits.replaceChildren(
      ...first.map((a) => hit(linkTo(a), a.title, `★ ${topicName(a.topic)}`)),
      ...rest.map((h) => hit(h.url, h.title, titleOf[h.book] ?? h.book, h.snippet && highlight(h.snippet, q))));
    const n = first.length + rest.length;
    const [before, after] = plural(lang, "results", n).split("{q}");
    count.replaceChildren(before, el("strong", { textContent: q }), after ?? "");
    hits.hidden = count.hidden = n === 0;
    none.hidden = n > 0;
    showPreparing(found.preparing);
  } catch (err) {
    if (err.name !== "AbortError") none.hidden = false;
  }
}

let timer = 0;
input.addEventListener("input", () => { clearTimeout(timer); timer = setTimeout(search, 150); });
$("q-clear").addEventListener("click", () => {
  input.value = "";
  search();
  input.focus();
});
$("search").addEventListener("submit", (event) => {
  event.preventDefault();
  const first = hits.querySelector("a");
  if (first) location.href = first.href;
});
input.value = new URLSearchParams(location.search).get("q") ?? "";
openTopic();
search();
input.focus({ preventScroll: true });
