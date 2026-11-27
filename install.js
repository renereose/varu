// The web app's install wizard, in the installer's steps and look
// (installer/wizard.html): country, what is included, then install.
// download.js saves the region in the browser's private storage, and
// sw.js serves it under app/.
"use strict";

const STRINGS = {
  en: {
    installer: "Installer",
    lead: "Maps, shelters, directions and emergency guidance, saved on this device so they work without the internet. The internet is needed only once, to download.",
    loading: "Loading the list of downloads…",
    retry: "Try again",
    offlineTitle: "Could not load the list of downloads",
    offline: "Check the internet connection. Varu needs the internet only once, to download everything.",
    unsupportedTitle: "This browser cannot keep Varu",
    unsupported: "It cannot save files for use without the internet. Use a recent Safari, Chrome, Edge or Firefox.",
    iosFirstTitle: "First, add Varu to the Home Screen",
    iosStep1: "Tap Share {icon}, then Add to Home Screen.",
    iosStep2: "Open Varu from the Home Screen.",
    iosStep3: "The installer opens there. Safari and the Home Screen app keep separate storage, so a download made here would not reach the app.",
    stepCountry: "Country",
    tryLive: "Or try Varu online first, without installing →",
    stepInclude: "What to include",
    stepInstall: "Install",
    stepOf: "Step {n} of {total} ·",
    theme: "Theme",
    themeSystem: "Same as the device",
    themeLight: "Light",
    themeDark: "Dark",
    toDark: "Switch to dark",
    toLight: "Switch to light",
    countryTitle: "Your country",
    countryHint: "The whole country is saved: the street map, places and directions everywhere in it.",
    includeTitle: "What to include",
    includeHint: "Choose what to save in this browser. All of it works without the internet.",
    partApp: "The app and the emergency guide",
    partAppHint: "What to do, radio, supplies and your own plan",
    partLayers: "Map layers",
    partLayersHint: "Shelters, hospitals, pharmacies, water, fuel and more",
    partPlaces: "Address search",
    partPlacesHint: "Towns, streets and addresses",
    partRoads: "Directions",
    partRoadsHint: "On foot or by car, with live turn-by-turn",
    partMap: "Street map",
    partMapHint: "The whole country down to street level, with shelters and the other map layers",
    needsMap: "Needs the street map",
    speed: "{speed}/s",
    left: "about {n} min left",
    leftSoon: "under a minute left",
    change: "Change what is included",
    partLibrary: "Library",
    partLibraryHint: "Wikipedia and reference books: only in the apps and on USB sticks, too large for a browser for now",
    reviewTitle: "Ready to install",
    reviewHint: "Check the details, then install.",
    rowCountry: "Country",
    rowWhere: "Saved in",
    rowSize: "Download",
    rowFree: "Space this browser offers",
    where: "This browser, on this device",
    unknown: "Not known",
    yourVaru: "Your Varu",
    works: "Works",
    offlineWorks: "Without the internet",
    ofFree: "{free} available",
    noSpace: "There may not be room: this browser offers about {free}, and Varu needs {size}.",
    keepNote: "Keep this page open and the screen on while it downloads. If it stops, open this page again and it carries on.",
    updateNote: "A newer version is ready. Only what changed is downloaded.",
    back: "Back",
    next: "Next",
    install: "Install",
    installing: "Installing…",
    installingHint: "Saving in this browser, on this device.",
    of: "{done} of {total}",
    keepOpen: "Keep this page open and the screen on until it finishes.",
    failed: "The download stopped: {error}. Install again to carry on where it stopped.",
    doneTitle: "Varu is ready",
    doneText: "{region} is saved on this device. Open Varu any time, also without the internet.",
    doneMade: "{region}, made on {date}. Open Varu any time, also without the internet.",
    newer: "A newer version is available.",
    notPersisted: "This browser may delete saved sites when the device runs low on space. Adding Varu to the home screen makes that less likely. Check now and then that it still opens.",
    open: "Open Varu",
    addApp: "Install as an app",
    update: "Update",
    remove: "Remove from this device",
    removeConfirm: "Remove the saved maps and data from this device?",
  },
  et: {
    installer: "Paigaldaja",
    lead: "Kaardid, varjumiskohad, teejuhised ja hädaolukorra juhend, salvestatud sellesse seadmesse, et need töötaksid ilma internetita. Internetti on vaja üks kord, allalaadimiseks.",
    loading: "Laadin allalaadimiste nimekirja…",
    retry: "Proovi uuesti",
    offlineTitle: "Allalaadimiste nimekirja ei õnnestunud laadida",
    offline: "Kontrolli internetiühendust. Varu vajab internetti ainult üks kord, et kõik alla laadida.",
    unsupportedTitle: "See brauser ei saa Varut hoida",
    unsupported: "See ei saa faile võrguühenduseta kasutamiseks salvestada. Kasuta uut Safarit, Chrome'i, Edge'i või Firefoxi.",
    iosFirstTitle: "Lisa Varu esmalt avakuvale",
    iosStep1: "Puuduta nuppu Jaga {icon} ja lisa Varu avakuvale (Home Screen).",
    iosStep2: "Ava Varu avakuvalt.",
    iosStep3: "Paigaldaja avaneb seal. Safari ja avakuva rakendus hoiavad andmeid eraldi, nii et siin alla laaditu rakenduseni ei jõuaks.",
    stepCountry: "Riik",
    tryLive: "Või proovi Varut enne veebis, ilma paigaldamata →",
    stepInclude: "Mida kaasa võtta",
    stepInstall: "Paigalda",
    stepOf: "Samm {n}/{total} ·",
    theme: "Teema",
    themeSystem: "Nagu seadmes",
    themeLight: "Hele",
    themeDark: "Tume",
    toDark: "Lülita tumedaks",
    toLight: "Lülita heledaks",
    countryTitle: "Sinu riik",
    countryHint: "Salvestatakse kogu riik: tänavakaart, kohad ja teejuhised kõikjal selles.",
    includeTitle: "Mida kaasa võtta",
    includeHint: "Vali, mida sellesse brauserisse salvestada. Kõik see töötab ilma internetita.",
    partApp: "Rakendus ja hädaolukorra juhend",
    partAppHint: "Mida teha, raadio, varud ja sinu oma plaan",
    partLayers: "Kaardikihid",
    partLayersHint: "Varjumiskohad, haiglad, apteegid, vesi, kütus ja muu",
    partPlaces: "Aadressiotsing",
    partPlacesHint: "Linnad, tänavad ja aadressid",
    partRoads: "Teejuhised",
    partRoadsHint: "Jala või autoga, juhistega pöörete kaupa",
    partMap: "Tänavakaart",
    partMapHint: "Kogu riik tänavate tasemeni, koos varjumiskohtade ja teiste kaardikihtidega",
    needsMap: "Vajab tänavakaarti",
    speed: "{speed}/s",
    left: "umbes {n} min jäänud",
    leftSoon: "alla minuti jäänud",
    change: "Muuda, mida kaasa võtta",
    partLibrary: "Raamatukogu",
    partLibraryHint: "Vikipeedia ja teatmeteosed: ainult rakendustes ja mälupulgal, brauseri jaoks praegu liiga suur",
    reviewTitle: "Valmis paigaldama",
    reviewHint: "Kontrolli andmeid ja paigalda.",
    rowCountry: "Riik",
    rowWhere: "Salvestatakse",
    rowSize: "Allalaadimine",
    rowFree: "Brauseri pakutav ruum",
    where: "Sellesse brauserisse, selles seadmes",
    unknown: "Pole teada",
    yourVaru: "Sinu Varu",
    works: "Töötab",
    offlineWorks: "Ilma internetita",
    ofFree: "{free} vaba",
    noSpace: "Ruumi ei pruugi jätkuda: brauser pakub umbes {free}, Varu vajab {size}.",
    keepNote: "Hoia see leht allalaadimise ajal avatuna ja ekraan sees. Kui see katkeb, ava leht uuesti ja see jätkab.",
    updateNote: "Uuem versioon on valmis. Alla laaditakse ainult see, mis muutus.",
    back: "Tagasi",
    next: "Edasi",
    install: "Paigalda",
    installing: "Paigaldan…",
    installingHint: "Salvestan sellesse brauserisse, selles seadmes.",
    of: "{done} / {total}",
    keepOpen: "Hoia see leht avatuna ja ekraan sees, kuni see lõpeb.",
    failed: "Allalaadimine katkes: {error}. Paigalda uuesti, et jätkata sealt, kus see pooleli jäi.",
    doneTitle: "Varu on valmis",
    doneText: "{region} on selles seadmes. Ava Varu igal ajal, ka ilma internetita.",
    doneMade: "{region}, koostatud {date}. Ava Varu igal ajal, ka ilma internetita.",
    newer: "Saadaval on uuem versioon.",
    notPersisted: "See brauser võib salvestatud saidid kustutada, kui seadmes saab ruum otsa. Varu lisamine avakuvale muudab selle vähem tõenäoliseks. Kontrolli aeg-ajalt, et see ikka avaneb.",
    open: "Ava Varu",
    addApp: "Paigalda rakendusena",
    update: "Uuenda",
    remove: "Eemalda sellest seadmest",
    removeConfirm: "Kas eemaldada salvestatud kaardid ja andmed sellest seadmest?",
  },
};

// Icons drawn inline: Lucide (https://lucide.dev), ISC License, Copyright (c)
// 2026 Lucide Icons and Contributors; the arrows, check, download, info and
// search come from Feather, MIT License, Copyright (c) 2013-present Cole
// Bemis. Permission to use, copy, modify, and/or distribute this software for
// any purpose with or without fee is hereby granted, provided that the above
// copyright notice and this permission notice appear in all copies. THE
// SOFTWARE IS PROVIDED "AS IS" AND THE AUTHOR DISCLAIMS ALL WARRANTIES WITH
// REGARD TO THIS SOFTWARE. "pin" is Varu's own map pin.
const ICONS = {
  "pin": '<path fill="currentColor" stroke="none" d="M12 2a7 7 0 0 0-7 7c0 5 7 13 7 13s7-8 7-13a7 7 0 0 0-7-7zm0 9.5A2.5 2.5 0 1 1 12 6.5a2.5 2.5 0 0 1 0 5z"/>',
  "arrow-left": '<path d="m12 19-7-7 7-7"/><path d="M19 12H5"/>',
  "arrow-right": '<path d="M5 12h14"/><path d="m12 5 7 7-7 7"/>',
  "check": '<path d="M20 6 9 17l-5-5"/>',
  "circle-alert": '<circle cx="12" cy="12" r="10"/><line x1="12" x2="12" y1="8" y2="12"/><line x1="12" x2="12.01" y1="16" y2="16"/>',
  "download": '<path d="M12 15V3"/><path d="M21 15v4a2 2 0 0 1-2 2H5a2 2 0 0 1-2-2v-4"/><path d="m7 10 5 5 5-5"/>',
  "history": '<path d="M3 12a9 9 0 1 0 9-9 9.75 9.75 0 0 0-6.74 2.74L3 8"/><path d="M3 3v5h5"/><path d="M12 7v5l4 2"/>',
  "info": '<circle cx="12" cy="12" r="10"/><path d="M12 16v-4"/><path d="M12 8h.01"/>',
  "monitor": '<rect width="20" height="14" x="2" y="3" rx="2"/><line x1="8" x2="16" y1="21" y2="21"/><line x1="12" x2="12" y1="17" y2="21"/>',
  "moon": '<path d="M20.985 12.486a9 9 0 1 1-9.473-9.472c.405-.022.617.46.402.803a6 6 0 0 0 8.268 8.268c.344-.215.825-.004.803.401"/>',
  "refresh-cw": '<path d="M3 12a9 9 0 0 1 9-9 9.75 9.75 0 0 1 6.74 2.74L21 8"/><path d="M21 3v5h-5"/><path d="M21 12a9 9 0 0 1-9 9 9.75 9.75 0 0 1-6.74-2.74L3 16"/><path d="M8 16H3v5"/>',
  "smartphone": '<rect x="6" y="2" width="12" height="20" rx="2" ry="2"/><path d="M12 18h.01"/>',
  "sun": '<circle cx="12" cy="12" r="4"/><path d="M12 2v2"/><path d="M12 20v2"/><path d="m4.93 4.93 1.41 1.41"/><path d="m17.66 17.66 1.41 1.41"/><path d="M2 12h2"/><path d="M20 12h2"/><path d="m6.34 17.66-1.41 1.41"/><path d="m19.07 4.93-1.41 1.41"/>',
  "triangle-alert": '<path d="m21.73 18-8-14a2 2 0 0 0-3.48 0l-8 14A2 2 0 0 0 4 21h16a2 2 0 0 0 1.73-3"/><path d="M12 9v4"/><path d="M12 17h.01"/>',
  "wifi-off": '<path d="M12 20h.01"/><path d="M8.5 16.429a5 5 0 0 1 7 0"/><path d="M5 12.859a10 10 0 0 1 5.17-2.69"/><path d="M19 12.859a10 10 0 0 0-2.007-1.523"/><path d="M2 8.82a15 15 0 0 1 4.177-2.643"/><path d="M22 8.82a15 15 0 0 0-11.288-3.764"/><path d="m2 2 20 20"/>',
  "map": '<path d="M14.106 5.553a2 2 0 0 0 1.788 0l3.659-1.83A1 1 0 0 1 21 4.619v12.764a1 1 0 0 1-.553.894l-4.553 2.277a2 2 0 0 1-1.788 0l-4.212-2.106a2 2 0 0 0-1.788 0l-3.659 1.83A1 1 0 0 1 3 19.381V6.618a1 1 0 0 1 .553-.894l4.553-2.277a2 2 0 0 1 1.788 0z"/><path d="M15 5.764v15"/><path d="M9 3.236v15"/>',
  "shield": '<path d="M20 13c0 5-3.5 7.5-7.66 8.95a1 1 0 0 1-.67-.01C7.5 20.5 4 18 4 13V6a1 1 0 0 1 1-1c2 0 4.5-1.2 6.24-2.72a1.17 1.17 0 0 1 1.52 0C14.51 3.81 17 5 19 5a1 1 0 0 1 1 1z"/>',
  "navigation": '<polygon points="3 11 22 2 13 21 11 13 3 11"/>',
  "search": '<path d="m21 21-4.34-4.34"/><circle cx="11" cy="11" r="8"/>',
  "life-buoy": '<circle cx="12" cy="12" r="10"/><path d="m4.93 4.93 4.24 4.24"/><path d="m14.83 9.17 4.24-4.24"/><path d="m14.83 14.83 4.24 4.24"/><path d="m9.17 14.83-4.24 4.24"/><circle cx="12" cy="12" r="4"/>',
  "book-open": '<path d="M12 5v16"/><path d="M20.001 19A2 2 0 0022 17V5a2 2 0 00-1.999-2L16 3.002A5 5 0 0012 5a5 5 0 00-4-2H4a2 2 0 00-2 2v12a2 2 0 001.999 2H8a5 5 0 014 2 5 5 0 014-2z"/>',
  "list-checks": '<path d="M13 5h8"/><path d="M13 12h8"/><path d="M13 19h8"/><path d="m3 17 2 2 4-4"/><path d="m3 7 2 2 4-4"/>',
  "trash": '<path d="M3 6h18"/><path d="M19 6v14c0 1-1 2-2 2H7c-1 0-2-1-2-2V6"/><path d="M8 6V4c0-1 1-2 2-2h4c1 0 2 1 2 2v2"/>',
  "share": '<path d="M12 2v13"/><path d="m16 6-4-4-4 4"/><path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8"/>',
};
// The parts of a region, in the order they download. choice: what leaves
// it out in step 2 (download.js knows the same names); none for the app.
const PARTS = [
  { key: "partApp", has: (p) => !/^pois\/|\.(pmtiles|places|graph)$/.test(p) },
  { key: "partLayers", choice: "map", has: (p) => p.startsWith("pois/") },
  { key: "partPlaces", choice: "places", has: (p) => p.endsWith(".places") },
  { key: "partRoads", choice: "roads", has: (p) => p.endsWith(".graph") },
  { key: "partMap", choice: "map", has: (p) => p.endsWith(".pmtiles") },
];
// Step 2. Search and directions are used on the map, so they need it.
const CHOICES = [
  { key: "partApp", icon: "life-buoy" },
  { id: "map", key: "partMap", icon: "map" },
  { id: "places", key: "partPlaces", icon: "search", needs: "map" },
  { id: "roads", key: "partRoads", icon: "navigation", needs: "map" },
];
const STEPS = ["stepCountry", "stepInclude", "stepInstall"];
const REGION = "ee";
const BASE = `data/${REGION}/`;

const $ = (id) => document.getElementById(id);
const root = document.documentElement;
const standalone = matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
const ios = /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);

// ---------- settings kept on this device, shared with the app ----------

function pref(name) {
  const cookie = document.cookie.match(new RegExp(`(?:^|; )varu_${name}=([^;]*)`));
  if (cookie) return decodeURIComponent(cookie[1]);
  try { return localStorage.getItem(`varu.${name}`); } catch { return null; }
}
function setPref(name, value) {
  document.cookie = `varu_${name}=${encodeURIComponent(value)}; max-age=315360000; path=/; SameSite=Lax`;
  try { localStorage.setItem(`varu.${name}`, value); } catch { /* private mode */ }
}

let lang = STRINGS[pref("lang")] ? pref("lang") : navigator.language?.startsWith("et") ? "et" : "en";
const t = (key, vars = {}) => STRINGS[lang][key].replace(/\{(\w+)\}/g, (_, k) => vars[k] ?? "");
const size = (bytes) => bytes < 1e9
  ? `${new Intl.NumberFormat(lang).format(Math.max(1, Math.round(bytes / 1e6)))} MB`
  : `${new Intl.NumberFormat(lang, { maximumFractionDigits: 1 }).format(bytes / 1e9)} GB`;
const date = (iso) => new Date(iso).toLocaleDateString(lang === "en" ? "en-GB" : lang, { dateStyle: "long" });
const svg = (name) => `<svg class="i" viewBox="0 0 24 24" aria-hidden="true">${ICONS[name] ?? ""}</svg>`;
function el(tag, props = {}, ...children) {
  const node = Object.assign(document.createElement(tag), props);
  node.append(...children.filter((c) => c !== null && c !== undefined));
  return node;
}
const icon = (name) => el("span", { innerHTML: svg(name) }).firstChild;
function drawIcons() {
  for (const node of document.querySelectorAll("svg[data-icon]")) node.replaceWith(icon(node.dataset.icon));
}

// ---------- state ----------

const s = {
  step: 1,
  list: null, // data/<region>/files.json
  installed: null, // the saved drive's state, when an install finished
  free: null, // bytes this browser offers, if it says
  prompt: null, // the browser's own "install app" prompt
  without: new Set(), // the parts left out
  samples: [], // [time, bytes] of the last seconds, for the speed
};
const name = () => (s.list ? s.list.name[lang] ?? s.list.name.en : "");
// What is left out, with what needs a part that is left out.
const without = () => new Set([...s.without, ...CHOICES.filter((c) => c.needs && s.without.has(c.needs)).map((c) => c.id)]);
const picked = () => PARTS.filter((part) => !without().has(part.choice));
const bytes = (parts) => (s.list ? s.list.files.filter((f) => parts.some((part) => part.has(f.path))).reduce((n, f) => n + f.size, 0) : 0);
const total = () => bytes(picked());
const partSize = (part) => bytes([part]);
const current = () => s.installed && s.list && s.installed.update === 0;

// ---------- drawing ----------

function draw() {
  root.lang = lang;
  for (const node of document.querySelectorAll("[data-t]")) node.textContent = t(node.dataset.t);
  for (const node of document.querySelectorAll("[data-tl]")) {
    node.setAttribute("aria-label", t(node.dataset.tl));
    node.title = t(node.dataset.tl);
  }
  $("ios-step1").innerHTML = t("iosStep1", { icon: svg("share").replace('class="i"', 'class="i share"') });
  $("langs").replaceChildren(...["et", "en"].map((code) => {
    const b = el("button", { type: "button", textContent: code.toUpperCase() });
    b.setAttribute("aria-pressed", String(code === lang));
    b.addEventListener("click", () => { lang = code; setPref("lang", code); draw(); });
    return b;
  }));
  showTheme();
  if (!s.list) return;

  // Step 1: the country.
  $("regions").replaceChildren(el("li", { className: "card on" },
    el("label", { className: "choice" },
      el("input", { type: "radio", name: "region", checked: true }),
      el("span", { className: "grow" }, el("strong", { textContent: name() })),
      el("span", { className: "size", textContent: size(total()) }))));

  // Step 2: what comes with it.
  $("contents").replaceChildren(
    ...CHOICES.map((choice) => {
      const blocked = choice.needs && s.without.has(choice.needs);
      const on = !s.without.has(choice.id) && !blocked;
      const box = el("input", { type: "checkbox", checked: on, disabled: !choice.id || blocked });
      box.addEventListener("change", () => {
        if (box.checked) s.without.delete(choice.id);
        else s.without.add(choice.id);
        draw();
      });
      return el("li", { className: `card${on ? " on" : ""}${blocked ? " off" : ""}` },
        el("label", { className: "choice" }, box, icon(choice.icon),
          el("span", { className: "grow" }, el("strong", { textContent: t(choice.key) }),
            el("small", { textContent: t(blocked ? "needsMap" : `${choice.key}Hint`) })),
          el("span", { className: "size", textContent: size(bytes(PARTS.filter((part) => part.choice === choice.id))) })));
    }),
    el("li", { className: "card off" },
      el("div", { className: "choice" }, icon("book-open"),
        el("span", { className: "grow" }, el("strong", { textContent: t("partLibrary") }), el("small", { textContent: t("partLibraryHint") })))));

  // Step 3: the review.
  const row = (iconName, label, value) => el("div", {}, icon(iconName), el("dt", { textContent: t(label) }), el("dd", { textContent: value }));
  $("review").replaceChildren(
    row("map", "rowCountry", name()),
    row(ios ? "smartphone" : "monitor", "rowWhere", t("where")),
    row("download", "rowSize", size(total())),
    row("info", "rowFree", s.free === null ? t("unknown") : size(s.free)));
  const short = s.free !== null && s.free < total() * 1.05;
  $("space-box").hidden = !short;
  $("space").textContent = t("noSpace", { free: size(s.free ?? 0), size: size(total()) });
  $("update-note").hidden = !s.installed;

  // "Your Varu", beside the steps on a wide screen.
  const fact = (iconName, label, value) => el("div", {}, icon(iconName), el("span", {}, el("dt", { textContent: t(label) }), el("dd", { textContent: value })));
  const track = el("div", { className: `track${short ? " over" : ""}` }, el("span"));
  track.firstChild.style.width = `${s.free ? Math.min(100, (100 * total()) / s.free) : 100}%`;
  $("summary").replaceChildren(
    el("h2", { textContent: t("yourVaru") }),
    el("dl", { className: "facts" },
      fact("map", "rowCountry", name()),
      fact(ios ? "smartphone" : "monitor", "rowWhere", t("where")),
      fact("wifi-off", "works", t("offlineWorks"))),
    el("hr"),
    el("div", { className: "size-part" },
      el("div", { className: "size-top" }, el("span", { textContent: t("rowSize") }), el("strong", { textContent: size(total()) })),
      track,
      s.free === null ? null : el("span", { textContent: t("ofFree", { free: size(s.free) }) })));

  drawSteps();
  drawDone();
}

function drawSteps() {
  $("steps").replaceChildren(...STEPS.map((key, i) => {
    const n = i + 1;
    const li = el("li", {}, el("span", { className: "dot" }, n < s.step ? icon("check") : String(n)), t(key));
    li.dataset.state = n < s.step ? "done" : n === s.step ? "now" : "next";
    return li;
  }));
  $("bars").replaceChildren(...STEPS.map((_, i) => el("span", { className: i < s.step ? "on" : "" })));
  $("step-count").textContent = t("stepOf", { n: s.step, total: STEPS.length });
  $("step-name").textContent = t(STEPS[s.step - 1]);
  for (const node of document.querySelectorAll("[data-step]")) node.hidden = Number(node.dataset.step) !== s.step;
  $("back").style.visibility = s.step === 1 ? "hidden" : "";
  $("next").hidden = s.step === STEPS.length;
  $("install").hidden = s.step !== STEPS.length;
}

function drawDone() {
  if (!s.installed) return;
  $("done-title").textContent = t("doneTitle");
  $("done-text").textContent = s.installed.built
    ? t("doneMade", { region: name() || "Varu", date: date(s.installed.built) })
    : t("doneText", { region: name() || "Varu" });
  $("newer").hidden = $("update").hidden = !s.list || current();
}

// Shows one screen: "setup" (the steps), "progress", "done", "notice", "ios-first".
function screen(which) {
  for (const id of ["loading", "setup", "progress", "done", "notice", "ios-first"]) $(id).hidden = id !== which;
  $("stepper").hidden = $("foot").hidden = which !== "setup";
  window.scrollTo(0, 0);
}

function notice(title, text) {
  $("notice-title").textContent = t(title);
  $("notice-text").textContent = t(text);
  screen("notice");
}

// ---------- theme: as in the app (viewer/js/shell.js) ----------

const isDark = () => (root.dataset.theme ? root.dataset.theme === "dark" : matchMedia("(prefers-color-scheme: dark)").matches);
function showTheme() {
  const chosen = root.dataset.theme ?? "system";
  for (const b of document.querySelectorAll("[data-theme-choice]")) b.setAttribute("aria-pressed", String(b.dataset.themeChoice === chosen));
  $("theme-toggle").replaceChildren(icon(isDark() ? "sun" : "moon"));
  $("theme-toggle").setAttribute("aria-label", t(isDark() ? "toLight" : "toDark"));
}
function chooseTheme(theme) {
  setPref("theme", theme);
  if (theme === "light" || theme === "dark") root.dataset.theme = theme; else delete root.dataset.theme;
  showTheme();
}
for (const b of document.querySelectorAll("[data-theme-choice]")) b.addEventListener("click", () => chooseTheme(b.dataset.themeChoice));
$("theme-toggle").addEventListener("click", () => chooseTheme(isDark() ? "light" : "dark"));
matchMedia("(prefers-color-scheme: dark)").addEventListener("change", showTheme);

// ---------- storage ----------

// What download.js, which keeps the saved files, says about them:
// { installed, built, update (bytes a newer list would fetch) }.
function readInstalled(list = null) {
  return new Promise((resolve) => {
    const worker = new Worker("download.js");
    const done = (data) => {
      worker.terminate();
      if (data?.migrated) navigator.serviceWorker.controller?.postMessage("reload");
      resolve(data?.installed ? data : null);
    };
    worker.onmessage = ({ data }) => done(data);
    worker.onerror = () => done(null);
    worker.postMessage({ status: true, list });
  });
}

async function readFree() {
  const { quota, usage = 0 } = (await navigator.storage.estimate?.().catch(() => null)) ?? {};
  // What an update replaces is already counted in usage; leave it be.
  return quota ? quota - usage : null;
}

// ---------- installing ----------

// "12 MB/s · about 3 min left", from the last few seconds.
function speed(done, sum) {
  const now = performance.now();
  s.samples.push([now, done]);
  while (s.samples.length > 2 && now - s.samples[0][0] > 5000) s.samples.shift();
  const [then, before] = s.samples[0];
  if (now - then < 1500) return "";
  const rate = ((done - before) * 1000) / (now - then);
  if (rate <= 0) return "";
  const secs = (sum - done) / rate;
  const fast = `${new Intl.NumberFormat(lang, { maximumFractionDigits: rate < 1e7 ? 1 : 0 }).format(rate / 1e6)} MB`;
  return ` · ${t("speed", { speed: fast })} · ${secs < 60 ? t("leftSoon") : t("left", { n: Math.ceil(secs / 60) })}`;
}

function drawProgress(done, path) {
  const sum = total();
  const parts = picked();
  $("bar").setAttribute("aria-valuenow", String(done / sum));
  $("bar").firstElementChild.style.width = `${(100 * done) / sum}%`;
  $("amount").textContent = t("of", { done: size(done), total: size(sum) }) + speed(done, sum);
  // manifest.json comes last and closes the install: keep the last part going.
  const now = path === "manifest.json" ? parts.length - 1 : Math.max(0, parts.findIndex((p) => p.has(path ?? "")));
  $("current").textContent = t(parts[now].key);
  $("parts").replaceChildren(...parts.map((part, i) => {
    const li = el("li", {}, el("span", { className: "mark" }, i < now ? icon("check") : null), t(part.key),
      el("span", { className: "size", textContent: size(partSize(part)) }));
    li.dataset.state = i < now ? "done" : i === now ? "now" : "next";
    return li;
  }));
}

async function install() {
  $("problem-box").hidden = true;
  screen("progress");
  drawProgress(0, "");
  const persisted = await navigator.storage.persist?.().catch(() => false);
  let lock = await navigator.wakeLock?.request("screen").catch(() => null);
  const relock = async () => {
    if (document.visibilityState === "visible" && lock?.released) lock = await navigator.wakeLock.request("screen").catch(() => null);
  };
  document.addEventListener("visibilitychange", relock);
  // In the order of PARTS, so the list fills from the top.
  const order = (f) => PARTS.findIndex((p) => p.has(f.path));
  const list = { ...s.list, files: [...s.list.files].sort((a, b) => order(a) - order(b)) };
  s.samples = [];
  const worker = new Worker("download.js");
  worker.postMessage({ base: new URL(BASE, location.href).href, list, without: [...without()] });
  let latest = null; // progress comes in per chunk; draw it once a frame
  worker.onmessage = async ({ data }) => {
    if ("done" in data) {
      if (!latest) requestAnimationFrame(() => { drawProgress(latest.done, latest.path); latest = null; });
      latest = data;
      return;
    }
    worker.terminate();
    document.removeEventListener("visibilitychange", relock);
    lock?.release().catch(() => {});
    if (data.error) {
      $("problem").textContent = t("failed", { error: data.error });
      $("problem-box").hidden = false;
      s.step = STEPS.length;
      draw();
      screen("setup");
      return;
    }
    (await navigator.serviceWorker.ready).active?.postMessage("reload");
    s.installed = await readInstalled(s.list);
    $("not-persisted").hidden = persisted !== false;
    draw();
    screen("done");
  };
}

// ---------- start ----------

async function main() {
  drawIcons();
  draw();
  const supported = "serviceWorker" in navigator && navigator.storage?.getDirectory && await new Promise((resolve) => {
    const worker = new Worker("download.js");
    worker.onmessage = ({ data }) => { worker.terminate(); resolve(data.supported); };
    worker.onerror = () => resolve(false);
    worker.postMessage({ check: true });
  });
  if (!supported) {
    $("retry").hidden = true;
    notice("unsupportedTitle", "unsupported");
    return;
  }
  navigator.serviceWorker.register("sw.js").catch(() => {});
  s.installed = await readInstalled();
  // Opened from the home screen with everything saved: straight to the app.
  if (s.installed && standalone) {
    location.replace("app/");
    return;
  }
  s.list = await fetch(`${BASE}files.json`, { cache: "no-store" }).then((r) => (r.ok ? r.json() : null)).catch(() => null);
  if (s.installed && s.list) s.installed = await readInstalled(s.list);
  s.without = new Set(s.installed?.without ?? []);
  s.free = await readFree();
  draw();
  if (s.installed) {
    $("not-persisted").hidden = (await navigator.storage.persisted?.()) !== false;
    screen("done");
    return;
  }
  if (ios && !standalone) {
    screen("ios-first");
    return;
  }
  if (!s.list) {
    $("retry").hidden = false;
    notice("offlineTitle", "offline");
    return;
  }
  screen("setup");
}

$("next").addEventListener("click", () => { s.step++; drawSteps(); window.scrollTo(0, 0); });
$("back").addEventListener("click", () => { s.step--; drawSteps(); window.scrollTo(0, 0); });
$("install").addEventListener("click", install);
$("update").addEventListener("click", () => { s.step = STEPS.length; draw(); screen("setup"); });
$("change").addEventListener("click", () => { s.step = 2; draw(); screen("setup"); });
$("retry").addEventListener("click", () => location.reload());
$("remove").addEventListener("click", async () => {
  if (!confirm(t("removeConfirm"))) return;
  await (await navigator.storage.getDirectory()).removeEntry("drive", { recursive: true }).catch(() => {});
  (await navigator.serviceWorker.ready).active?.postMessage("reload");
  location.reload();
});
window.addEventListener("beforeinstallprompt", (event) => {
  event.preventDefault();
  s.prompt = event;
  $("add-app").hidden = standalone;
});
$("add-app").addEventListener("click", async () => {
  await s.prompt?.prompt();
  $("add-app").hidden = true;
});
main();
