// The web app's install page: downloads a region into the browser's
// private storage (download.js) and opens it (sw.js serves it under app/).

const STRINGS = {
  en: {
    title: "Varu on your phone or computer",
    intro: "Maps, shelters, directions and emergency guidance, saved on this device so they work without the internet. Download once while you are online.",
    iosFirstTitle: "On iPhone and iPad, add Varu to the Home Screen first",
    iosStep1: "Tap Share {icon}, then Add to Home Screen.",
    iosStep2: "Open Varu from the Home Screen.",
    iosStep3: "Download there. Safari and the Home Screen app keep separate storage, so a download made here would not reach the app.",
    gotMap: "Street map with shelters, hospitals, pharmacies, water and more",
    gotRoute: "Address search, and walking or driving directions",
    gotGuide: "What to do in an emergency, radio, supplies and your own plan",
    install: "Download",
    downloading: "Downloading…",
    keepOpen: "Keep this page open and the screen on until it finishes. If it stops, open this page again and it carries on.",
    readyTitle: "{region} is saved on this device",
    readyText: "Made on {date}. Works without the internet.",
    updateAvailable: "Made on {date}. A newer version is available.",
    notPersisted: "This browser may delete saved sites when the device runs low on space. Adding Varu to the home screen makes that less likely. Check now and then that it still opens.",
    open: "Open Varu",
    addApp: "Add to home screen",
    update: "Update",
    remove: "Remove from this device",
    removeConfirm: "Remove the saved maps and data from this device?",
    unsupported: "This browser cannot save Varu for offline use. Use a recent Safari, Chrome, Edge or Firefox.",
    noSpace: "There may not be room: this browser offers about {free}, and Varu needs {size}.",
    offline: "No internet connection. Varu needs the internet once, to download.",
    failed: "The download stopped ({error}). Open this page again to carry on.",
    footer: "Map data © OpenStreetMap contributors. Everything comes from this site, and nothing about you is sent anywhere.",
  },
  et: {
    title: "Varu sinu telefonis või arvutis",
    intro: "Kaardid, varjumiskohad, teejuhised ja hädaolukorra juhend, salvestatud sellesse seadmesse, et need töötaksid ilma internetita. Laadi need alla üks kord, kui oled võrgus.",
    iosFirstTitle: "iPhone'is ja iPadis lisa Varu esmalt avakuvale",
    iosStep1: "Puuduta nuppu Jaga {icon} ja lisa Varu avakuvale (Home Screen).",
    iosStep2: "Ava Varu avakuvalt.",
    iosStep3: "Laadi alla seal. Safari ja avakuva rakendus hoiavad andmeid eraldi, nii et siin alla laaditu rakenduseni ei jõuaks.",
    gotMap: "Tänavakaart varjumiskohtade, haiglate, apteekide, vee ja muuga",
    gotRoute: "Aadressiotsing ning jala- või autoteekond",
    gotGuide: "Mida teha hädaolukorras, raadio, varud ja sinu oma plaan",
    install: "Laadi alla",
    downloading: "Laadin alla…",
    keepOpen: "Hoia see leht avatuna ja ekraan sees, kuni allalaadimine lõpeb. Kui see katkeb, ava leht uuesti ja see jätkab.",
    readyTitle: "{region} on selles seadmes",
    readyText: "Koostatud {date}. Töötab ilma internetita.",
    updateAvailable: "Koostatud {date}. Saadaval on uuem versioon.",
    notPersisted: "See brauser võib salvestatud saidid kustutada, kui seadmes saab ruum otsa. Varu lisamine avakuvale muudab selle vähem tõenäoliseks. Kontrolli aeg-ajalt, et see ikka avaneb.",
    open: "Ava Varu",
    addApp: "Lisa avakuvale",
    update: "Uuenda",
    remove: "Eemalda sellest seadmest",
    removeConfirm: "Kas eemaldada salvestatud kaardid ja andmed sellest seadmest?",
    unsupported: "See brauser ei saa Varu võrguühenduseta kasutamiseks salvestada. Kasuta uut Safarit, Chrome'i, Edge'i või Firefoxi.",
    noSpace: "Ruumi ei pruugi jätkuda: brauser pakub umbes {free}, Varu vajab {size}.",
    offline: "Internetiühendus puudub. Varu vajab internetti üks kord, allalaadimiseks.",
    failed: "Allalaadimine katkes ({error}). Ava see leht uuesti, et jätkata.",
    footer: "Kaardiandmed © OpenStreetMapi kaastöötajad. Kõik tuleb sellelt saidilt ja sinu kohta ei saadeta kuhugi midagi.",
  },
};
const SHARE_ICON = '<svg class="share" width="18" height="18" viewBox="0 0 24 24" fill="none" stroke="currentColor" stroke-width="2" stroke-linecap="round" stroke-linejoin="round" aria-hidden="true"><path d="M12 2v13"/><path d="m16 6-4-4-4 4"/><path d="M4 12v8a2 2 0 0 0 2 2h12a2 2 0 0 0 2-2v-8"/></svg>';
const REGION = "ee";
const BASE = `data/${REGION}/`;

const $ = (id) => document.getElementById(id);
// The same remembered language as the app's pages (viewer/js/common.js).
const stored = (document.cookie.match(/(?:^|; )varu_lang=([^;]*)/) || [])[1];
let lang = STRINGS[stored] ? stored : navigator.language?.startsWith("et") ? "et" : "en";
const t = (key, vars = {}) => STRINGS[lang][key].replace(/\{(\w+)\}/g, (_, k) => vars[k] ?? "");
const size = (bytes) => `${new Intl.NumberFormat(lang, { maximumFractionDigits: bytes < 1e9 ? 0 : 1 }).format(bytes < 1e9 ? bytes / 1e6 : bytes / 1e9)} ${bytes < 1e9 ? "MB" : "GB"}`;
const date = (iso) => new Date(iso).toLocaleDateString(lang === "en" ? "en-GB" : lang, { dateStyle: "long" });
const standalone = matchMedia("(display-mode: standalone)").matches || navigator.standalone === true;
const ios = /iPad|iPhone|iPod/.test(navigator.userAgent) || (navigator.platform === "MacIntel" && navigator.maxTouchPoints > 1);

let list = null; // data/<region>/files.json
let installed = null; // the saved drive's .state, when complete
let installPrompt = null;

function draw() {
  document.documentElement.lang = lang;
  for (const node of document.querySelectorAll("[data-t]")) node.textContent = t(node.dataset.t);
  $("ios-step1").innerHTML = t("iosStep1", { icon: SHARE_ICON });
  $("langs").replaceChildren(...Object.keys(STRINGS).map((code) => {
    const b = Object.assign(document.createElement("button"), { type: "button", textContent: code.toUpperCase() });
    b.setAttribute("aria-pressed", String(code === lang));
    b.addEventListener("click", () => {
      lang = code;
      document.cookie = `varu_lang=${code}; max-age=315360000; path=/; SameSite=Lax`;
      try { localStorage.setItem("varu.lang", code); } catch {}
      draw();
    });
    return b;
  }));
  const name = list ? list.name[lang] ?? list.name.en : "";
  $("region-name").textContent = name;
  $("region-size").textContent = list ? size(list.total) : "";
  const current = installed && list && list.files.every((f) => installed.done[f.path] === f.hash);
  $("ready-title").textContent = t("readyTitle", { region: name || "Varu" });
  $("ready-text").textContent = installed ? t(current || !list ? "readyText" : "updateAvailable", { date: date(installed.built) }) : "";
  $("update").hidden = !installed || !list || current;
}

function show(which) {
  for (const id of ["choose", "progress-box", "ready"]) $(id).hidden = id !== which;
}

function problem(text) {
  $("problem").textContent = text;
  $("problem").hidden = !text;
}

async function drive(create = false) {
  return (await navigator.storage.getDirectory()).getDirectoryHandle("drive", { create });
}

// The saved drive's state, if an install finished.
async function readInstalled() {
  try {
    const dir = await drive();
    const state = JSON.parse(await (await (await dir.getFileHandle(".state")).getFile()).text());
    if (state.partial || !state.done["manifest.json"]) return null;
    const manifest = JSON.parse(await (await (await dir.getFileHandle("manifest.json")).getFile()).text());
    return { ...state, built: manifest.built };
  } catch {
    return null;
  }
}

async function checkSpace() {
  const { quota = 0, usage = 0 } = (await navigator.storage.estimate?.()) ?? {};
  const free = quota - usage;
  $("space").hidden = !quota || free > list.total * 1.05;
  $("space").textContent = t("noSpace", { free: size(free), size: size(list.total) });
}

async function download() {
  problem("");
  show("progress-box");
  const persisted = await navigator.storage.persist?.().catch(() => false);
  let lock = await navigator.wakeLock?.request("screen").catch(() => null);
  const relock = async () => { if (document.visibilityState === "visible" && lock?.released) lock = await navigator.wakeLock.request("screen").catch(() => null); };
  document.addEventListener("visibilitychange", relock);
  const worker = new Worker("download.js");
  worker.postMessage({ base: new URL(BASE, location.href).href, list });
  worker.onmessage = async ({ data }) => {
    if ("done" in data) {
      $("bar").value = data.done / data.total;
      $("progress-text").textContent = `${size(data.done)} / ${size(data.total)}`;
      return;
    }
    worker.terminate();
    document.removeEventListener("visibilitychange", relock);
    lock?.release().catch(() => {});
    if (data.error) {
      problem(t("failed", { error: data.error }));
      show("choose");
      return;
    }
    (await navigator.serviceWorker.ready).active?.postMessage("reload");
    installed = await readInstalled();
    $("not-persisted").hidden = persisted !== false;
    draw();
    show("ready");
  };
}

async function main() {
  draw();
  const supported = "serviceWorker" in navigator && navigator.storage?.getDirectory && await new Promise((resolve) => {
    const worker = new Worker("download.js");
    worker.onmessage = ({ data }) => { worker.terminate(); resolve(data.supported); };
    worker.onerror = () => resolve(false);
    worker.postMessage({ check: true });
  });
  if (!supported) {
    $("unsupported").hidden = false;
    return;
  }
  navigator.serviceWorker.register("sw.js").catch((err) => problem(String(err)));
  installed = await readInstalled();
  // Opened from the home screen with everything saved: straight to the app.
  if (installed && standalone) {
    location.replace("app/");
    return;
  }
  list = await fetch(`${BASE}files.json`, { cache: "no-store" }).then((r) => (r.ok ? r.json() : null)).catch(() => null);
  if (list) list.total = list.files.reduce((n, f) => n + f.size, 0);
  draw();
  if (installed) {
    $("not-persisted").hidden = (await navigator.storage.persisted?.()) !== false;
    show("ready");
    return;
  }
  if (ios && !standalone) {
    $("ios-first").hidden = false;
    return;
  }
  if (!list) {
    problem(t("offline"));
    return;
  }
  await checkSpace();
  show("choose");
}

$("install").addEventListener("click", download);
$("update").addEventListener("click", download);
$("remove").addEventListener("click", async () => {
  if (!confirm(t("removeConfirm"))) return;
  await (await navigator.storage.getDirectory()).removeEntry("drive", { recursive: true }).catch(() => {});
  (await navigator.serviceWorker.ready).active?.postMessage("reload");
  installed = null;
  location.reload();
});
window.addEventListener("beforeinstallprompt", (event) => {
  event.preventDefault();
  installPrompt = event;
  $("add-app").hidden = standalone;
});
$("add-app").addEventListener("click", async () => {
  await installPrompt?.prompt();
  $("add-app").hidden = true;
});
main();
