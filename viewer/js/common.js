// Shared helpers: manifest loading and the small translation layer.
//
// Interface strings live here. Content strings (layer names, document
// titles) come from the region file via manifest.json as {en: "...", et: "..."}.

const STRINGS = {
  en: {
    navHome: "Home",
    navMap: "Map",
    guideTitle: "Emergency guide",
    guideShort: "Guide",
    offlineReady: "Offline ready",
    theme: "Theme",
    themeSystem: "Same as the device",
    themeLight: "Light",
    themeDark: "Dark",
    toDark: "Switch to dark",
    toLight: "Switch to light",
    heroTitle: "Maps, shelters and guidance, ready when the network isn't.",
    heroTitleShort: "Ready when the network isn't.",
    heroStored: "Everything is stored on this device.",
    heroMapDate: "Map data from {date}.",
    heroStoredShort: "stored on this device",
    call112: "Call 112",
    whatNow: "What to do now",
    whatNowHint: "Steps, radio, supplies and your plan",
    onDevice: "On this device",
    heroStoredLive: "This is Varu online: nothing is saved on this device.",
    heroStoredShortLive: "online",
    onDeviceLive: "What's in Varu",
    mapsRoute: "Directions on foot or by car.",
    shelters: "{n} shelters",
    sheltersOne: "{n} shelter",
    layerCount: "{n} layers",
    layerCountOne: "{n} layer",
    books: "{n} books",
    booksOne: "{n} book",
    situations: "{n} situations",
    situationsOne: "{n} situation",
    guideText: "What to do in {n} situations, a home supplies checklist and your family plan.",
    printablePlan: "Printable plan",
    shareLive: "Sharing on",
    offline: "Works without internet",
    mapsText: "Street-level map with shelters, hospitals, pharmacies, water and fuel.",
    library: "Library",
    libraryText: "Wikipedia and reference works, fully searchable offline.",
    documents: "Documents",
    lanNote: "This drive is being shared with other devices on the local network.",
    downloads: "Update maps and data",
    shareTitle: "Share with other devices",
    shareHintOff: "Let phones and computers on the same Wi-Fi open this drive.",
    shareOn: "Turn sharing on",
    shareOff: "Stop sharing",
    shareHint: "Open this address on another phone or computer on the same Wi-Fi, or scan the code with the phone camera. No internet is needed. Sharing stops when Varu is closed.",
    shareNoNetwork: "No Wi-Fi network was found. Connect this computer to Wi-Fi or start a hotspot, then try again.",
    shareRetry: "Try again",
    shareFailed: "Sharing could not be changed.",
    shareCodeFor: "QR code for {address}",
    built: "Built",
    back: "Back",
    findNearest: "Find nearest",
    mapLayers: "Map layers",
    hideAll: "Hide all",
    showAllN: "Show all {n}",
    showFewer: "Show fewer",
    routeTitle: "Route",
    onFoot: "{d} on foot",
    byCar: "{d} by car",
    noLayers: "No place layers on this drive.",
    noMap: "There is no map on this drive.",
    mapError: "The map could not be loaded.",
    openingHours: "Open",
    phone: "Phone",
    operator: "Operator",
    findMyLocation: "Show my location",
    locationUnavailable: "Location not available",
    routeHere: "Route here",
    nearest: "Nearest",
    navStart: "Start",
    navEnd: "End",
    navWaiting: "Waiting for your location…",
    navNoGps: "Live directions need your location. Allow location access and try again.",
    navRerouting: "Off the route: finding a new one…",
    navArrive: "Arrive",
    navArrived: "You have arrived",
    navRoundabout: "At the roundabout, take exit {n}",
    nav_straight: "Continue",
    nav_slight_left: "Bear left",
    nav_slight_right: "Bear right",
    nav_left: "Turn left",
    nav_right: "Turn right",
    nav_sharp_left: "Sharp left",
    nav_sharp_right: "Sharp right",
    nav_uturn: "Make a U-turn",
    startHere: "Start here",
    walk: "Walk",
    drive: "Drive",
    clearRoute: "Clear",
    fromGps: "From your location",
    fromHome: "From home",
    fromPoint: "From the start you chose",
    routing: "Finding a route…",
    needStart: "Choose a start: right-click or long-press the map and pick Start here, or show your location.",
    needDestination: "Now choose a destination: right-click or long-press the map and pick Route here.",
    noRoute: "No route found.",
    notConnected: "No road or path connects these places.",
    startTooFar: "The start is too far from any road or path.",
    endTooFar: "The destination is too far from any road or path.",
    noGraph: "There is no road data on this drive.",
    routeFerry: "Includes a ferry. Check whether ferries are running.",
    snapNote: "Starts or ends {n} from the nearest road or path.",
    routeSafety: "Estimate from OpenStreetMap. Follow road signs.",
    searchLibrary: "Search the library",
    librarySub: "Search Wikipedia and reference works stored on this device.",
    bookFilter: "Book",
    allBooks: "All books",
    booksHere: "Books on this device",
    results: "{n} results for “{q}”",
    resultsOne: "{n} result for “{q}”",
    search: "Search",
    topicsTitle: "Survival and DIY",
    noHits: "Nothing found.",
    searchPreparing: "Preparing search inside {book}: {n}%. Titles can be searched already.",
    noBooks: "There are no books on this drive.",
    searchMap: "Search the map",
    searchMapHint: "Search addresses and places",
    searching: "Searching…",
    emergencyTitle: "In an emergency",
    emergencyText: "What to do, supplies at home, and your own plan.",
    sos112Hint: "Rescue, ambulance, police",
    nearestShelter: "Nearest shelter",
    nearestShelterHint: "Show the closest one on the map",
    guideSub: "Clear steps for common crises, a home supplies checklist and your family's plan.",
    printPlan: "Print my plan",
    lifeDanger: "Life in danger? Call 112 first.",
    suppliesShort: "Supplies",
    steps: "{n} steps",
    stepsOne: "{n} step",
    readyWhen: "When it happens",
    readySupplies: "Home supplies",
    readyPlan: "My plan",
    readyNearest: "Nearest: {name}",
    readyMore: "Read more: {topic}",
    readyRadio: "Radio",
    readyRadioLink: "Radio frequencies",
    radioCounty: "County",
    radioSite: "Transmitter",
    radioArea: "Area",
    radioAsOf: "ERR's list as of {date}.",
    radioSub: "{stations} frequencies, printed with your plan",
    source: "Source",
    sources: "Sources",
    noGuides: "There are no emergency guides on this drive.",
    clear: "Clear",
    clearConfirm: "Clear all ticks?",
    suppliesCount: "{n}/{total}",
    planNote: "Saved only on this device. Print it or write it down on paper too.",
    planContacts: "Family and close contacts",
    planName: "Name",
    planPhone: "Phone",
    planAddContact: "Add a contact",
    planRemove: "Remove",
    planMeetNear: "Meeting place near home",
    planMeetFar: "Meeting place further away",
    planDocs: "Where documents and medicines are kept",
    planNotes: "Notes",
    planEmpty: "Nothing filled in yet.",
  },
  et: {
    navHome: "Avaleht",
    navMap: "Kaart",
    guideTitle: "Hädaolukorra juhend",
    guideShort: "Juhend",
    offlineReady: "Töötab võrguta",
    theme: "Teema",
    themeSystem: "Nagu seadmes",
    themeLight: "Hele",
    themeDark: "Tume",
    toDark: "Lülita tumedaks",
    toLight: "Lülita heledaks",
    heroTitle: "Kaardid, varjumiskohad ja juhised – valmis ka siis, kui võrku pole.",
    heroTitleShort: "Valmis ka siis, kui võrku pole.",
    heroStored: "Kõik on salvestatud selles seadmes.",
    heroMapDate: "Kaardiandmed seisuga {date}.",
    heroStoredShort: "salvestatud selles seadmes",
    call112: "Helista 112",
    whatNow: "Mida teha kohe",
    whatNowHint: "Sammud, raadio, varud ja sinu plaan",
    onDevice: "Selles seadmes",
    heroStoredLive: "See on Varu veebis: selles seadmes pole midagi salvestatud.",
    heroStoredShortLive: "veebis",
    onDeviceLive: "Mis Varus on",
    mapsRoute: "Teejuhised jalgsi või autoga.",
    shelters: "{n} varjumiskohta",
    sheltersOne: "{n} varjumiskoht",
    layerCount: "{n} kihti",
    layerCountOne: "{n} kiht",
    books: "{n} raamatut",
    booksOne: "{n} raamat",
    situations: "{n} olukorda",
    situationsOne: "{n} olukord",
    guideText: "Mida teha {n} olukorras, koduse varu nimekiri ja sinu pere plaan.",
    printablePlan: "Prinditav plaan",
    shareLive: "Jagamine sees",
    offline: "Töötab ilma internetita",
    mapsText: "Tänavatasemel kaart varjumiskohtade, haiglate, apteekide, joogivee ja tanklatega.",
    library: "Raamatukogu",
    libraryText: "Vikipeedia ja teatmeteosed, otsing töötab ka ilma internetita.",
    documents: "Dokumendid",
    lanNote: "Seda mälupulka jagatakse kohtvõrgu teiste seadmetega.",
    downloads: "Uuenda kaarte ja andmeid",
    shareTitle: "Jaga teiste seadmetega",
    shareHintOff: "Lase samas WiFi-võrgus olevatel telefonidel ja arvutitel see mälupulk avada.",
    shareOn: "Lülita jagamine sisse",
    shareOff: "Lõpeta jagamine",
    shareHint: "Ava see aadress teises telefonis või arvutis, mis on samas WiFi-võrgus, või skanni kood telefoni kaameraga. Internetti pole vaja. Jagamine lõpeb, kui Varu sulged.",
    shareNoNetwork: "WiFi-võrku ei leitud. Ühenda arvuti WiFi-võrku või lülita sisse kuumpunkt ja proovi siis uuesti.",
    shareRetry: "Proovi uuesti",
    shareFailed: "Jagamist ei õnnestunud muuta.",
    shareCodeFor: "QR-kood aadressile {address}",
    built: "Koostatud",
    back: "Tagasi",
    findNearest: "Leia lähim",
    mapLayers: "Kaardikihid",
    hideAll: "Peida kõik",
    showAllN: "Näita kõiki ({n})",
    showFewer: "Näita vähem",
    routeTitle: "Teekond",
    onFoot: "{d} jalgsi",
    byCar: "{d} autoga",
    noLayers: "Sellel mälupulgal pole kohakihte.",
    noMap: "Sellel mälupulgal pole kaarti.",
    mapError: "Kaarti ei õnnestunud laadida.",
    openingHours: "Avatud",
    phone: "Telefon",
    operator: "Haldaja",
    findMyLocation: "Näita minu asukohta",
    locationUnavailable: "Asukoht pole kättesaadav",
    routeHere: "Tee siia",
    nearest: "Lähim",
    navStart: "Alusta",
    navEnd: "Lõpeta",
    navWaiting: "Ootan su asukohta…",
    navNoGps: "Juhised teel vajavad su asukohta. Luba asukoht ja proovi uuesti.",
    navRerouting: "Teelt kõrval: otsin uut teed…",
    navArrive: "Kohale jõudmine",
    navArrived: "Oled kohal",
    navRoundabout: "Ringristmikul vali {n}. väljasõit",
    nav_straight: "Jätka otse",
    nav_slight_left: "Hoia vasakule",
    nav_slight_right: "Hoia paremale",
    nav_left: "Pööra vasakule",
    nav_right: "Pööra paremale",
    nav_sharp_left: "Pööra järsult vasakule",
    nav_sharp_right: "Pööra järsult paremale",
    nav_uturn: "Pööra tagasi",
    startHere: "Alusta siit",
    walk: "Jalgsi",
    drive: "Autoga",
    clearRoute: "Tühista",
    fromGps: "Sinu asukohast",
    fromHome: "Kodust",
    fromPoint: "Valitud alguspunktist",
    routing: "Otsin teekonda…",
    needStart: "Vali alguspunkt: paremklõpsa või vajuta pikalt kaardil ja vali „Alusta siit“ või näita oma asukohta.",
    needDestination: "Nüüd vali sihtkoht: paremklõpsa või vajuta pikalt kaardil ja vali „Tee siia“.",
    noRoute: "Teekonda ei leitud.",
    notConnected: "Neid kohti ei ühenda ükski tee ega rada.",
    startTooFar: "Alguspunkt on teedest ja radadest liiga kaugel.",
    endTooFar: "Sihtkoht on teedest ja radadest liiga kaugel.",
    noGraph: "Sellel mälupulgal pole teekonnaandmeid.",
    routeFerry: "Teekonnal on praam. Kontrolli, kas praamid sõidavad.",
    snapNote: "Algab või lõpeb lähimast teest või rajast {n} kaugusel.",
    routeSafety: "Hinnang OpenStreetMapi põhjal. Järgi liiklusmärke.",
    searchLibrary: "Otsi raamatukogust",
    librarySub: "Otsi selles seadmes olevast Vikipeediast ja teatmeteostest.",
    bookFilter: "Raamat",
    allBooks: "Kõik raamatud",
    booksHere: "Raamatud selles seadmes",
    results: "{n} tulemust päringule „{q}“",
    resultsOne: "{n} tulemus päringule „{q}“",
    search: "Otsi",
    topicsTitle: "Ellujäämine ja tee-ise",
    noHits: "Midagi ei leitud.",
    searchPreparing: "Valmistan ette otsingut raamatus {book}: {n}%. Pealkirju saab juba otsida.",
    noBooks: "Sellel mälupulgal pole raamatuid.",
    searchMap: "Otsi kaardilt",
    searchMapHint: "Otsi aadresse ja kohti",
    searching: "Otsin…",
    emergencyTitle: "Hädaolukorras",
    emergencyText: "Mida teha, kodune varu ja sinu enda plaan.",
    sos112Hint: "Pääste, kiirabi, politsei",
    nearestShelter: "Lähim varjumiskoht",
    nearestShelterHint: "Näita lähimat kaardil",
    guideSub: "Selged sammud levinumateks kriisideks, koduse varu nimekiri ja sinu pere plaan.",
    printPlan: "Prindi minu plaan",
    lifeDanger: "Elu on ohus? Helista kõigepealt 112.",
    suppliesShort: "Varud",
    steps: "{n} sammu",
    stepsOne: "{n} samm",
    readyWhen: "Kui juhtub",
    readySupplies: "Kodune varu",
    readyPlan: "Minu plaan",
    readyNearest: "Lähim: {name}",
    readyMore: "Loe lähemalt: {topic}",
    readyRadio: "Raadio",
    readyRadioLink: "Raadiosagedused",
    radioCounty: "Maakond",
    radioSite: "Saatja",
    radioArea: "Piirkond",
    radioAsOf: "ERR-i andmed seisuga {date}.",
    radioSub: "{stations} sagedused, prinditakse koos sinu plaaniga",
    source: "Allikas",
    sources: "Allikad",
    noGuides: "Sellel mälupulgal pole hädaolukorra juhiseid.",
    clear: "Tühjenda",
    clearConfirm: "Kas eemaldan kõik märked?",
    suppliesCount: "{n}/{total}",
    planNote: "Salvestatud ainult sellesse seadmesse. Prindi see välja või kirjuta ka paberile.",
    planContacts: "Pere ja lähikondsed",
    planName: "Nimi",
    planPhone: "Telefon",
    planAddContact: "Lisa kontakt",
    planRemove: "Eemalda",
    planMeetNear: "Kokkusaamiskoht kodu lähedal",
    planMeetFar: "Kokkusaamiskoht kaugemal",
    planDocs: "Kus on dokumendid ja ravimid",
    planNotes: "Märkmed",
    planEmpty: "Midagi pole veel täidetud.",
  },
};

function stored(key) {
  try { return localStorage.getItem(key); } catch { return null; }
}

function store(key, value) {
  try { localStorage.setItem(key, value); } catch { /* private mode: fine */ }
}

// The language and theme chosen on this device. Kept in a cookie as well as
// localStorage: a cookie belongs to the address without its port, so the
// choice outlives the launcher starting on another port.
const TEN_YEARS = 10 * 365 * 24 * 60 * 60;

export function pref(name) {
  const cookie = document.cookie.match(new RegExp(`(?:^|; )varu_${name}=([^;]*)`));
  return cookie ? decodeURIComponent(cookie[1]) : stored(`varu.${name}`);
}

export function setPref(name, value) {
  document.cookie = `varu_${name}=${encodeURIComponent(value)}; max-age=${TEN_YEARS}; path=/; SameSite=Lax`;
  store(`varu.${name}`, value);
}

/** Whether the page is dark: the theme chosen here, else the device's. */
export function isDark() {
  const theme = document.documentElement.dataset.theme;
  return theme ? theme === "dark" : window.matchMedia("(prefers-color-scheme: dark)").matches;
}

/** Interface languages offered on this drive, in the region's order. */
export function availableLanguages(manifest) {
  const wanted = manifest.languages?.length ? manifest.languages : ["en"];
  const usable = wanted.filter((code) => STRINGS[code]);
  return usable.length ? usable : ["en"];
}

export function currentLanguage(manifest) {
  const offered = availableLanguages(manifest);
  const saved = pref("lang");
  if (saved && offered.includes(saved)) return saved;
  const browser = (navigator.language || "").slice(0, 2).toLowerCase();
  return offered.includes(browser) ? browser : offered[0];
}

export function setLanguage(code) {
  setPref("lang", code);
}

// Varu online (the web app's live/, sw.js marks it): nothing is stored here.
const LIVE = globalThis.document?.documentElement.hasAttribute("data-live");

/** Translate an interface key. */
export function t(lang, key) {
  if (LIVE && `${key}Live` in STRINGS.en) key = `${key}Live`;
  return STRINGS[lang]?.[key] ?? STRINGS.en[key] ?? key;
}

/** A count in words: plural(lang, "books", 1) is "1 book", from the key "booksOne". */
export function plural(lang, key, n) {
  return t(lang, n === 1 ? `${key}One` : key).replace("{n}", n.toLocaleString(lang));
}

/** Pick a content string from a {lang: text} map (or pass plain strings through). */
export function pick(lang, value) {
  if (value == null) return "";
  if (typeof value === "string") return value;
  return value[lang] ?? value.en ?? Object.values(value)[0] ?? "";
}

/** Fill every [data-i18n] element in the page. */
export function applyStrings(lang) {
  document.documentElement.lang = lang;
  for (const el of document.querySelectorAll("[data-i18n]")) {
    el.textContent = t(lang, el.dataset.i18n);
  }
  for (const el of document.querySelectorAll("[data-i18n-label]")) {
    el.setAttribute("aria-label", t(lang, el.dataset.i18nLabel));
  }
}

export async function loadManifest() {
  try {
    const res = await fetch("manifest.json");
    return res.ok ? await res.json() : {};
  } catch {
    return {};
  }
}

export async function loadStatus() {
  try {
    const res = await fetch("api/status");
    return res.ok ? await res.json() : {};
  } catch {
    return {};
  }
}

export function formatBytes(bytes) {
  if (!Number.isFinite(bytes) || bytes <= 0) return "";
  const units = ["B", "kB", "MB", "GB", "TB"];
  const i = Math.min(units.length - 1, Math.floor(Math.log10(bytes) / 3));
  const value = bytes / 1000 ** i;
  return `${value >= 100 || i === 0 ? value.toFixed(0) : value.toFixed(1)} ${units[i]}`;
}

export { stored, store };
