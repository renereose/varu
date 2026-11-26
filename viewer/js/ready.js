// "In an emergency": what to do (guides.json), a tickable list of home
// supplies, and the family's own plan. The ticks and the plan live in this
// browser's localStorage and nowhere else.
//
// Known limit: localStorage belongs to one origin (address and port). If the
// launcher ever starts on another port, the saved plan is not seen there, so
// printing it is the backup.

import {
  applyStrings, currentLanguage, loadManifest, pick, store, stored, t,
} from "./common.js";

const SUPPLIES_KEY = "varu.supplies";
const PLAN_KEY = "varu.plan";

const [manifest, guides, topics] = await Promise.all([
  loadManifest(),
  fetch("/guides.json").then((res) => (res.ok ? res.json() : {})).catch(() => ({})),
  fetch("/topics.json").then((res) => (res.ok ? res.json() : {})).catch(() => ({})),
]);
const lang = currentLanguage(manifest);
applyStrings(lang);
document.title = t(lang, "emergencyTitle");

const el = (tag, props = {}, ...children) => {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children);
  return node;
};
const readJson = (key, fallback) => {
  try { return JSON.parse(stored(key) ?? "null") ?? fallback; } catch { return fallback; }
};
const sourceLinks = (sources) => (sources ?? []).flatMap((s, i) => [
  ...(i ? [" · "] : []),
  el("a", { href: s.url, textContent: s.title || s.url, target: "_blank", rel: "noopener" }),
]);

// ---------- When it happens ----------

const situations = guides.situations ?? [];
document.getElementById("no-guides").hidden = situations.length > 0;
const pois = manifest.pois ?? [];
const topicById = Object.fromEntries((topics.topics ?? []).map((topic) => [topic.id, topic]));

document.getElementById("situation-list").replaceChildren(...situations.map((s) => {
  const text = s[lang] ?? s.en ?? Object.values(s).find((v) => v?.title) ?? {};
  const links = [];
  const layer = s.map && manifest.map ? pois.find((p) => p.id === s.map) : null;
  if (layer) {
    links.push(el("a", {
      className: "btn btn-primary",
      href: `map.html?nearest=${encodeURIComponent(layer.id)}`,
      textContent: t(lang, "readyNearest").replace("{name}", pick(lang, layer.label) || layer.id),
    }));
  }
  for (const id of s.topics ?? []) {
    const topic = topicById[id];
    if (!topic) continue;
    links.push(el("a", {
      className: "btn",
      href: `/library/#topic-${encodeURIComponent(id)}`,
      textContent: t(lang, "readyMore").replace("{topic}", topic[lang] ?? topic.en),
    }));
  }
  const item = el("details", { id: `situation-${s.id}`, className: "situation" },
    el("summary", { textContent: text.title }),
    el("ol", {}, ...(text.steps ?? []).map((step) => el("li", { textContent: step }))));
  if (links.length) item.append(el("p", { className: "actions no-print" }, ...links));
  if (s.sources?.length) item.append(el("p", { className: "sources" }, `${t(lang, "source")}: `, ...sourceLinks(s.sources)));
  return item;
}));
if (location.hash.startsWith("#situation-")) document.getElementById(location.hash.slice(1))?.setAttribute("open", "");

// ---------- Home supplies ----------

const supplies = guides.supplies;
if (supplies) {
  document.getElementById("supplies").hidden = false;
  const text = supplies[lang] ?? supplies.en ?? {};
  document.getElementById("supplies-title").textContent = text.title ?? t(lang, "readySupplies");
  document.getElementById("supplies-intro").textContent = (text.intro ?? "").replace("{days}", supplies.days ?? "");

  const saved = readJson(SUPPLIES_KEY, []);
  const ticked = new Set(Array.isArray(saved) ? saved : []);
  const boxes = [];
  const count = document.getElementById("supplies-count");
  const refresh = () => {
    count.textContent = t(lang, "suppliesCount")
      .replace("{n}", boxes.filter((b) => b.checked).length).replace("{total}", boxes.length);
  };

  document.getElementById("supply-groups").replaceChildren(...(supplies.groups ?? []).map((group) =>
    el("fieldset", { className: "supply-group" },
      el("legend", { textContent: group[lang] ?? group.en }),
      ...group.items.map((item) => {
        const box = el("input", { type: "checkbox", checked: ticked.has(item.id) });
        boxes.push(box);
        box.addEventListener("change", () => {
          if (box.checked) ticked.add(item.id); else ticked.delete(item.id);
          store(SUPPLIES_KEY, JSON.stringify([...ticked]));
          refresh();
        });
        return el("label", {}, box, el("span", { textContent: item[lang] ?? item.en }));
      }))));
  refresh();

  document.getElementById("supplies-clear").addEventListener("click", () => {
    if (!confirm(t(lang, "clearConfirm"))) return;
    ticked.clear();
    store(SUPPLIES_KEY, "[]");
    for (const box of boxes) box.checked = false;
    refresh();
  });
  if (supplies.sources?.length) {
    document.getElementById("supplies-sources").append(`${t(lang, "sources")}: `, ...sourceLinks(supplies.sources));
  }
}

// ---------- My plan ----------

const plan = readJson(PLAN_KEY, {});
if (!Array.isArray(plan.contacts)) plan.contacts = [];
const save = () => store(PLAN_KEY, JSON.stringify(plan));

for (const field of document.querySelectorAll("[data-key]")) {
  field.value = plan[field.dataset.key] ?? "";
  field.addEventListener("input", () => { plan[field.dataset.key] = field.value; save(); });
}

const contactList = document.getElementById("contacts");
function contactRow(contact) {
  const name = el("input", { value: contact.name ?? "", placeholder: t(lang, "planName"), autocomplete: "off" });
  const phone = el("input", { value: contact.phone ?? "", placeholder: t(lang, "planPhone"), type: "tel", autocomplete: "off" });
  name.setAttribute("aria-label", t(lang, "planName"));
  phone.setAttribute("aria-label", t(lang, "planPhone"));
  const remove = el("button", { type: "button", className: "btn no-print", textContent: "×" });
  remove.setAttribute("aria-label", t(lang, "planRemove"));
  const row = el("div", { className: "contact" }, name, phone, remove);
  name.addEventListener("input", () => { contact.name = name.value; save(); });
  phone.addEventListener("input", () => { contact.phone = phone.value; save(); });
  remove.addEventListener("click", () => {
    plan.contacts.splice(plan.contacts.indexOf(contact), 1);
    row.remove();
    save();
  });
  return row;
}
function addContact(focus) {
  const contact = { name: "", phone: "" };
  plan.contacts.push(contact);
  const row = contactRow(contact);
  contactList.append(row);
  if (focus) row.querySelector("input").focus();
}
contactList.replaceChildren(...plan.contacts.map(contactRow));
// One empty row to start from; it is saved once something is typed.
if (!plan.contacts.length) addContact(false);
document.getElementById("add-contact").addEventListener("click", () => addContact(true));

document.getElementById("print").addEventListener("click", () => window.print());

// The sections are on one page, so a late #plan or #supplies still needs a
// scroll once the content above it has been built.
if (location.hash) document.getElementById(location.hash.slice(1))?.scrollIntoView();
