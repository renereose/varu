// A small bar the launcher adds to every page of a library book: back,
// search, and to the library. In a shadow root, so the book's own
// styles cannot touch it.

import { stored, t } from "/js/common.js";

const saved = stored("varu.lang");
const lang = saved || (navigator.language || "en").slice(0, 2);

const host = document.createElement("varu-bar");
const root = host.attachShadow({ mode: "closed" });
root.innerHTML = `<style>
  :host { all: initial; position: fixed; right: 12px; bottom: 12px; z-index: 2147483647; }
  nav { display: flex; align-items: center; gap: 2px; padding: 3px; border-radius: 999px; background: #0b5d4b;
    box-shadow: 0 4px 16px rgb(0 0 0 / 0.25); font: 600 14px/1 system-ui, -apple-system, "Segoe UI", sans-serif; }
  a, button { all: unset; cursor: pointer; color: #fff; padding: 9px 13px; border-radius: 999px; white-space: nowrap; }
  a:hover, button:hover, a:focus-visible, button:focus-visible { background: rgb(255 255 255 / 0.2); }
  form { display: flex; align-items: center; margin: 0; }
  form button { display: flex; padding: 9px 11px; }
  /* 16px keeps phones from zooming in on the field. */
  input { all: unset; width: min(14em, 45vw); padding: 7px 12px; border-radius: 999px; background: #fff;
    color: #1d1d1b; font: 400 16px/1.2 system-ui, -apple-system, "Segoe UI", sans-serif; }
  input[hidden] { display: none; }
  /* On a phone the open field takes the whole bar. */
  @media (max-width: 600px) { nav.searching > button, nav.searching > a { display: none; } }
  @media print { nav { display: none; } }
</style>
<nav><button type="button"></button><form role="search"><input type="search" enterkeyhint="search" autocomplete="off" hidden><button type="submit"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5"/><path d="m15.5 15.5 5 5"/></svg></button></form><a href="/library/"></a></nav>`;

const back = root.querySelector("nav > button");
const library = root.querySelector("a");
back.textContent = `← ${t(lang, "back")}`;
library.textContent = t(lang, "library");

// Search: the button opens the field, Enter shows the library's results
// (titles first, then articles, across every book).
const find = root.querySelector("form");
const field = find.querySelector("input");
const findButton = find.querySelector("button");
field.placeholder = t(lang, "searchLibrary");
field.setAttribute("aria-label", t(lang, "searchLibrary"));
findButton.setAttribute("aria-label", t(lang, "searchLibrary"));
const showField = (open) => {
  field.hidden = !open;
  root.querySelector("nav").classList.toggle("searching", open);
  if (open) field.focus();
  place();
};
find.addEventListener("submit", (event) => {
  event.preventDefault();
  const q = field.value.trim();
  if (field.hidden || !q) return showField(field.hidden);
  location.assign(`/library/?q=${encodeURIComponent(q)}`);
});
field.addEventListener("keydown", (event) => { if (event.key === "Escape") showField(false); });
// A page opened in its own window has nowhere to go back to.
back.addEventListener("click", () => (history.length > 1 ? history.back() : location.assign("/library/")));
document.body.append(host);

// Keep it in the corner of what is on screen. On a phone, a page wider than
// the screen widens the layout, and plain position: fixed would follow that.
function place() {
  const view = window.visualViewport;
  if (!view) return;
  host.style.inset = "0 auto auto 0";
  host.style.transform = `translate(${view.offsetLeft + view.width - host.offsetWidth - 12}px, ${view.offsetTop + view.height - host.offsetHeight - 12}px)`;
}
place();
window.visualViewport?.addEventListener("resize", place);
window.visualViewport?.addEventListener("scroll", place);

// Vikipeedia's front page lists its portals as plain text: the offline
// edition has no portal pages. Link each to the article of the same name,
// when the book has one.
for (const name of document.querySelectorAll(".main-page-portals b")) {
  if (name.closest("a")) continue;
  const href = `./${encodeURIComponent(name.textContent.trim().replaceAll(" ", "_"))}`;
  fetch(href, { method: "HEAD" }).then((res) => {
    if (!res.ok) return;
    const link = Object.assign(document.createElement("a"), { href });
    name.replaceWith(link);
    link.append(name);
  }).catch(() => {});
}
