// A small bar the launcher adds to every page of a library book: back,
// search in this book, and to the library. In a shadow root, so the book's
// own styles cannot touch it.

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
  a.find { display: flex; align-items: center; gap: 6px; }
  @media print { nav { display: none; } }
</style>
<nav><button type="button"></button><a class="find"><svg width="16" height="16" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2.5" stroke-linecap="round" aria-hidden="true"><circle cx="10.5" cy="10.5" r="6.5"/><path d="m15.5 15.5 5 5"/></svg><span></span></a><a href="/library/"></a></nav>`;

const back = root.querySelector("nav > button");
const library = root.querySelector('a[href="/library/"]');
back.textContent = `← ${t(lang, "back")}`;
library.textContent = t(lang, "library");

// Search opens this book's own search page (/library/<book>/).
const find = root.querySelector("a.find");
find.href = `/library/${location.pathname.split("/")[2]}/`;
find.querySelector("span").textContent = t(lang, "search");

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
