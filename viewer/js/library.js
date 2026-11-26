// The library page: the books on the drive, and search across all of them:
// titles first, then articles containing the words (launcher /api/library).
// At /library/<book>/ the same page searches that one book, with a link to
// the book's own front page. Both show hand-picked survival and DIY articles
// by topic (topics.json) while the search box is empty, and list matching
// ones first among the results.

import { applyStrings, currentLanguage, loadManifest, t } from "./common.js";

const [manifest, books, topics] = await Promise.all([
  loadManifest(),
  fetch("/api/library").then((res) => (res.ok ? res.json() : [])).catch(() => []),
  fetch("/topics.json").then((res) => (res.ok ? res.json() : {})).catch(() => ({})),
]);
const lang = currentLanguage(manifest);
applyStrings(lang);
const bookId = decodeURIComponent(location.pathname.split("/")[2] ?? "");
const book = books.find((b) => b.id === bookId);
document.title = book?.title ?? t(lang, "library");

const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
};
const titleOf = Object.fromEntries(books.map((b) => [b.id, b.title]));

function card(b, href, title, text) {
  const icon = el("img", { src: `/api/library/icon?book=${encodeURIComponent(b.id)}`, alt: "", width: 48, height: 48 });
  icon.addEventListener("error", () => icon.remove());
  return el("a", { className: "card", href }, icon, el("h2", { textContent: title }), el("p", { textContent: text }));
}
document.getElementById("empty").hidden = books.length > 0;
document.getElementById("search").hidden = books.length === 0;
document.getElementById("books").replaceChildren(...(book
  ? (book.home ? [card(book, book.home, t(lang, "frontPage"), book.description)] : [])
  : books.map((b) => card(b, `/library/${encodeURIComponent(b.id)}/`, b.title, b.description))));
if (book) {
  document.querySelector("h1").textContent = book.title;
  const up = document.querySelector(".back");
  up.href = "/library/";
  up.textContent = t(lang, "library");
}

// topics.json names a book without its edition month, so a newer edition
// keeps the links. Articles of books not on this drive are left out.
const edition = (id) => id.replace(/_\d{4}-\d{2}$/, "");
const topicName = (topic) => topic[lang] ?? topic.en;
const picks = (topics.topics ?? []).flatMap((topic) => topic.articles.map((a) => ({
  ...a, topic, in: books.find((b) => edition(b.id) === a.book),
}))).filter((a) => a.in && (!book || a.in === book));
const linkTo = (a) => `/library/${encodeURIComponent(a.in.id)}/${a.path.split("/").map(encodeURIComponent).join("/")}`;
const topicSection = document.getElementById("topics");
document.getElementById("topic-list").replaceChildren(...(topics.topics ?? []).map((topic) => {
  const list = picks.filter((a) => a.topic === topic);
  return list.length && el("details", { id: `topic-${topic.id}` },
    el("summary", {}, topicName(topic), " ", el("span", { className: "size", textContent: `${list.length}` })),
    el("ul", { className: "doc-list" }, ...list.map((a) => el("li", {}, el("a", { href: linkTo(a), textContent: a.title }),
      ...(book ? [] : [el("span", { className: "size", textContent: a.in.title })])))));
}).filter(Boolean));
topicSection.hidden = picks.length === 0;

// /library/#topic-<id> (from the emergency guide) opens that topic.
function openTopic() {
  const target = location.hash.startsWith("#topic-") ? document.getElementById(location.hash.slice(1)) : null;
  if (!target || target.tagName !== "DETAILS") return;
  target.open = true;
  target.scrollIntoView();
}
openTopic();
window.addEventListener("hashchange", openTopic);

// Hand-picked articles whose title has every word typed (as a word start,
// accents ignored: "korge" finds "Kõrge").
const words = (s) => s.normalize("NFD").replace(/\p{M}/gu, "").toLowerCase().match(/[\p{L}\p{N}]+/gu) ?? [];
function picked(q) {
  const typed = words(q);
  const seen = new Set();
  return picks.filter((a) => {
    const title = words(a.title);
    if (!typed.length || !typed.every((w) => title.some((t) => t.startsWith(w))) || seen.has(linkTo(a))) return false;
    seen.add(linkTo(a));
    return true;
  }).slice(0, 5);
}

const input = document.getElementById("q");
const hits = document.getElementById("hits");
const none = document.getElementById("no-hits");
const all = document.getElementById("all-books");
const hint = t(lang, book ? "searchBook" : "searchLibrary");
input.placeholder = hint;
input.setAttribute("aria-label", hint);

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
    .filter((b) => b.search === "preparing" && (!book || b.id === book.id)).map((b) => ({ title: b.title, progress: b.progress ?? 0 }));
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
  all.hidden = !book || books.length < 2 || !q;
  topicSection.hidden = picks.length === 0 || !!q;
  all.href = `/library/?q=${encodeURIComponent(q)}`;
  if (!q) {
    hits.replaceChildren();
    none.hidden = true;
    return;
  }
  request = new AbortController();
  const only = book ? `&book=${encodeURIComponent(book.id)}` : "";
  try {
    const found = await (await fetch(`/api/library/search?q=${encodeURIComponent(q)}${only}`, { signal: request.signal })).json();
    const first = picked(q);
    const shown = new Set(first.map((a) => decodeURIComponent(linkTo(a))));
    const rest = found.hits.filter((hit) => !shown.has(decodeURIComponent(hit.url)));
    hits.replaceChildren(...first.map((a) => el("li", {},
      el("div", {}, el("a", { href: linkTo(a), textContent: a.title })),
      el("span", { className: "size", textContent: `★ ${topicName(a.topic)}` }))), ...rest.map((hit) => el("li", {},
      el("div", {}, el("a", { href: hit.url, textContent: hit.title }),
        ...(hit.snippet ? [el("p", { className: "snippet" }, ...highlight(hit.snippet, q))] : [])),
      ...(book ? [] : [el("span", { className: "size", textContent: titleOf[hit.book] ?? hit.book })]))));
    none.hidden = first.length + rest.length > 0;
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
input.focus({ preventScroll: true });
