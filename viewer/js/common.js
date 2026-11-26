// Shared helpers: manifest loading and the small translation layer.
//
// Interface strings live here. Content strings (layer names, document
// titles) come from the region file via manifest.json as {en: "...", et: "..."}.

const STRINGS = {
  en: {
    offline: "Works without internet",
    maps: "Maps",
    mapsText: "Street-level map with hospitals, pharmacies, water and fuel.",
    library: "Library",
    libraryText: "Wikipedia and reference works, searchable.",
    documents: "Documents",
    documentsText: "Official guidance and practical guides.",
    inLibrary: "In the library",
    notIncluded: "Not on this drive.",
    lanNote: "This drive is being shared with other devices on the local network.",
    built: "Built",
    mapData: "map data",
    back: "Back",
    layers: "Layers",
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
    search: "Search",
    searchBook: "Search for an article or a word",
    frontPage: "The book's front page",
    searchAll: "Search all books",
    topicsTitle: "Survival and DIY",
    noHits: "Nothing found.",
    searchPreparing: "Preparing search inside {book}: {n}%. Titles can be searched already.",
    noBooks: "There are no books on this drive.",
    searchMap: "Search the map",
    searchMapHint: "Town, street, address or place",
    searching: "Searching…",
    emergencyTitle: "In an emergency",
    emergencyText: "What to do, supplies at home, and your own plan.",
    sos112: "112",
    sos112Hint: "Emergency: rescue, ambulance, police",
    nearestShelter: "Nearest shelter",
    nearestShelterHint: "Show the closest one on the map",
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
    source: "Source",
    sources: "Sources",
    noGuides: "There are no emergency guides on this drive.",
    clear: "Clear",
    clearConfirm: "Clear all ticks?",
    suppliesCount: "{n}/{total}",
    planNote: "Saved only on this device. Print it or write it down on paper too.",
    print: "Print",
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
    offline: "Töötab ilma internetita",
    maps: "Kaardid",
    mapsText: "Tänavatasemel kaart haiglate, apteekide, joogivee ja tanklatega.",
    library: "Raamatukogu",
    libraryText: "Vikipeedia ja teatmeteosed, otsinguga.",
    documents: "Dokumendid",
    documentsText: "Ametlikud juhised ja praktilised käsiraamatud.",
    inLibrary: "Raamatukogus",
    notIncluded: "Sellel mälupulgal puudub.",
    lanNote: "Seda mälupulka jagatakse kohtvõrgu teiste seadmetega.",
    built: "Koostatud",
    mapData: "kaardiandmed",
    back: "Tagasi",
    layers: "Kihid",
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
    search: "Otsi",
    searchBook: "Otsi artiklit või sõna",
    frontPage: "Raamatu esileht",
    searchAll: "Otsi kõigist raamatutest",
    topicsTitle: "Ellujäämine ja tee-ise",
    noHits: "Midagi ei leitud.",
    searchPreparing: "Valmistan ette otsingut raamatus {book}: {n}%. Pealkirju saab juba otsida.",
    noBooks: "Sellel mälupulgal pole raamatuid.",
    searchMap: "Otsi kaardilt",
    searchMapHint: "Asula, tänav, aadress või koht",
    searching: "Otsin…",
    emergencyTitle: "Hädaolukorras",
    emergencyText: "Mida teha, kodune varu ja sinu enda plaan.",
    sos112: "112",
    sos112Hint: "Hädaabi: pääste, kiirabi, politsei",
    nearestShelter: "Lähim varjumiskoht",
    nearestShelterHint: "Näita lähimat kaardil",
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
    source: "Allikas",
    sources: "Allikad",
    noGuides: "Sellel mälupulgal pole hädaolukorra juhiseid.",
    clear: "Tühjenda",
    clearConfirm: "Kas eemaldan kõik märked?",
    suppliesCount: "{n}/{total}",
    planNote: "Salvestatud ainult sellesse seadmesse. Prindi see välja või kirjuta ka paberile.",
    print: "Prindi",
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

const LANG_KEY = "varu.lang";

function stored(key) {
  try { return localStorage.getItem(key); } catch { return null; }
}

function store(key, value) {
  try { localStorage.setItem(key, value); } catch { /* private mode: fine */ }
}

/** Interface languages offered on this drive, in the region's order. */
export function availableLanguages(manifest) {
  const wanted = manifest.languages?.length ? manifest.languages : ["en"];
  const usable = wanted.filter((code) => STRINGS[code]);
  return usable.length ? usable : ["en"];
}

export function currentLanguage(manifest) {
  const offered = availableLanguages(manifest);
  const saved = stored(LANG_KEY);
  if (saved && offered.includes(saved)) return saved;
  const browser = (navigator.language || "").slice(0, 2).toLowerCase();
  return offered.includes(browser) ? browser : offered[0];
}

export function setLanguage(code) {
  store(LANG_KEY, code);
}

/** Translate an interface key. */
export function t(lang, key) {
  return STRINGS[lang]?.[key] ?? STRINGS.en[key] ?? key;
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
}

export async function loadManifest() {
  try {
    const res = await fetch("/manifest.json");
    return res.ok ? await res.json() : {};
  } catch {
    return {};
  }
}

export async function loadStatus() {
  try {
    const res = await fetch("/api/status");
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
