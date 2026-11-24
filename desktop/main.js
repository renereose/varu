// Varu as a desktop app: a window around the same Go programs the drive
// uses. With Varu data found (next to the app, the folder it installed to,
// ~/Varu, or any plugged-in drive) it starts the drive's launcher and shows
// the map; otherwise it starts the bundled installer and shows the wizard.

const { app, BrowserWindow, Menu, dialog, ipcMain, session, shell } = require("electron");
const { spawn } = require("node:child_process");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");

const GOOS = { darwin: "darwin", win32: "windows", linux: "linux" }[process.platform];
const GOARCH = { x64: "amd64", arm64: "arm64" }[process.arch];
const EXE = process.platform === "win32" ? ".exe" : "";

// ponytail: AppImages cannot carry Chromium's setuid sandbox helper, and Ubuntu
// 24.04 blocks the other kind. Pages here are only Varu's own, from the drive.
if (process.env.APPIMAGE) app.commandLine.appendSwitch("no-sandbox");

if (!app.requestSingleInstanceLock()) app.quit();

let win = null;
let child = null;
let mode = ""; // "viewer" or "installer"
let viewing = null; // the drive the viewer shows
let quitting = false;

const isLocal = (url) => /^http:\/\/(localhost|127\.0\.0\.1):\d+\//.test(url);
const isDrive = (dir) => !!dir && fs.existsSync(path.join(dir, "manifest.json")) && fs.existsSync(path.join(dir, "app"));
const settingsFile = () => path.join(app.getPath("userData"), "settings.json");

function remembered() {
  try { return JSON.parse(fs.readFileSync(settingsFile(), "utf8")).drive; } catch { return null; }
}

function remember(drive) {
  try { fs.writeFileSync(settingsFile(), JSON.stringify({ drive })); } catch { /* next start searches again */ }
}

/** Plugged-in drives and memory cards. */
function volumes() {
  const list = (dir) => { try { return fs.readdirSync(dir).map((name) => path.join(dir, name)); } catch { return []; } };
  if (process.platform === "win32") return [..."DEFGHIJKLMNOPQRSTUVWXYZ"].map((letter) => `${letter}:\\`);
  if (process.platform === "darwin") return list("/Volumes");
  const user = os.userInfo().username;
  return [`/media/${user}`, `/run/media/${user}`, "/media", "/mnt"].flatMap(list);
}

function findDrive() {
  // The app itself on a Varu stick: Windows portable .exe, AppImage, or an unpacked folder.
  const near = [process.env.PORTABLE_EXECUTABLE_DIR, process.env.APPIMAGE && path.dirname(process.env.APPIMAGE)];
  for (let dir = path.dirname(app.getPath("exe")); dir !== path.dirname(dir); dir = path.dirname(dir)) near.push(dir);
  return [...near, remembered(), path.join(os.homedir(), "Varu"), ...volumes()].find(isDrive) ?? null;
}

function startViewer(drive) {
  remember(drive);
  mode = "viewer";
  viewing = drive;
  run(path.join(drive, "bin", `${GOOS}-${GOARCH}`, `varu${EXE}`), ["-root", drive, "-no-browser"]);
}

/** update: a drive whose earlier choices the wizard starts from. */
function startInstaller(update = null) {
  mode = "installer";
  // VARU_SOURCE: try release files before they are published (a folder or URL).
  const source = process.env.VARU_SOURCE ? ["-source", process.env.VARU_SOURCE] : [];
  const drive = update ? ["-update", update] : [];
  run(path.join(process.resourcesPath, "bin", `varu-installer${EXE}`), ["-no-browser", ...source, ...drive]);
}

// A new app does not change the data a drive was installed with (its map
// pages, road data, library). Offer to update a drive older than the app;
// "Later" keeps the map, which matters offline.
const parseVersion = (v) => /^v?(\d+)\.(\d+)\.(\d+)$/.exec(v ?? "")?.slice(1).map(Number) ?? null;
function olderThanApp(drive) {
  const mine = parseVersion(app.getVersion());
  if (!mine) return false;
  let release = "";
  try { release = JSON.parse(fs.readFileSync(path.join(drive, "manifest.json"), "utf8")).release; } catch { /* treat as old */ }
  const data = parseVersion(release);
  if (!data) return true; // installed before releases were recorded
  const i = data.findIndex((n, k) => n !== mine[k]);
  return i >= 0 && data[i] < mine[i];
}
async function offerUpdate(drive) {
  const l = LABELS[app.getLocale().slice(0, 2)] ?? LABELS.en;
  const { response } = await dialog.showMessageBox(win, {
    type: "question", buttons: [l.update, l.later], defaultId: 0, cancelId: 1,
    message: l.updateTitle, detail: l.updateText.replace("{version}", app.getVersion()),
  });
  if (response === 0) startInstaller(drive);
}

/** Starts a Go program and shows the address it prints. */
function run(program, args) {
  stop();
  let output = "";
  let opened = false;
  const proc = spawn(program, args, { windowsHide: true });
  child = proc;
  // The address comes on stdout; stderr (the library reader's too) is kept for error messages.
  proc.stdout.on("data", (chunk) => {
    output = (output + chunk).slice(-4000);
    const url = !opened && /(http:\/\/\S+)\s/.exec(output)?.[1];
    if (url) { opened = true; win.loadURL(url); }
  });
  proc.stderr.on("data", (chunk) => { output = (output + chunk).slice(-4000); });
  const failed = (detail) => {
    if (child === proc && !quitting) showMessage("Varu could not start", `${program}\n\n${detail}`);
  };
  proc.on("error", (err) => failed(err.message));
  // Exit code 0 is the installer's own "Close the installer"; the page then tells the app.
  proc.on("exit", (code, signal) => { if (code !== 0) failed(`${output}\nIt stopped (${signal ?? `exit code ${code}`}).`); });
}

function stop() {
  const proc = child;
  child = null;
  proc?.kill();
}

function showMessage(title, text) {
  const esc = (s) => s.replace(/[&<>]/g, (c) => ({ "&": "&amp;", "<": "&lt;", ">": "&gt;" })[c]);
  win.loadURL("data:text/html;charset=utf-8," + encodeURIComponent(
    `<title>Varu</title><body style="font:16px system-ui,sans-serif;padding:2rem;max-width:40rem">` +
    `<h1 style="font-size:1.4rem">${esc(title)}</h1><pre style="white-space:pre-wrap">${esc(text)}</pre>`));
}

function go(where) {
  if (mode === "installer") {
    // Leave a running install alone; once there is data, switch to it.
    const drive = where !== "installer" && findDrive();
    if (drive) startViewer(drive);
    return;
  }
  if (where === "installer") return startInstaller(viewing);
  const url = win.webContents.getURL();
  if (isLocal(url)) win.loadURL(new URL(where, url).href);
}

const LABELS = {
  en: {
    go: "Go", home: "Home", map: "Map", library: "Library", back: "Back", forward: "Forward", install: "Install or update…",
    update: "Update", later: "Later", updateTitle: "Update the maps and data?",
    updateText: "This is Varu {version}, but the maps, library and other data here come from an earlier version, so some new features are missing. Updating needs the internet.",
  },
  et: {
    go: "Mine", home: "Avaleht", map: "Kaart", library: "Raamatukogu", back: "Tagasi", forward: "Edasi", install: "Paigalda või uuenda…",
    update: "Uuenda", later: "Hiljem", updateTitle: "Uuenda kaarte ja andmeid?",
    updateText: "See on Varu {version}, kuid siinsed kaardid, raamatukogu ja muud andmed on varasemast versioonist, nii et osa uusi võimalusi puudub. Uuendamiseks on vaja internetti.",
  },
};

const focused = () => (BrowserWindow.getFocusedWindow() ?? win).webContents;

function menu() {
  const mac = process.platform === "darwin";
  const l = LABELS[app.getLocale().slice(0, 2)] ?? LABELS.en;
  const nav = (label, accelerator, where) => ({ label, accelerator, click: () => go(where) });
  Menu.setApplicationMenu(Menu.buildFromTemplate([
    ...(mac ? [{ role: "appMenu" }] : []),
    {
      label: l.go,
      submenu: [
        nav(l.home, "CmdOrCtrl+Shift+H", "/"),
        nav(l.map, "CmdOrCtrl+M", "/map.html"),
        nav(l.library, "CmdOrCtrl+L", "/library/"),
        { type: "separator" },
        { label: l.back, accelerator: mac ? "Cmd+[" : "Alt+Left", click: () => focused().navigationHistory.goBack() },
        { label: l.forward, accelerator: mac ? "Cmd+]" : "Alt+Right", click: () => focused().navigationHistory.goForward() },
        { type: "separator" },
        nav(l.install, undefined, "installer"),
        ...(mac ? [] : [{ type: "separator" }, { role: "quit" }]),
      ],
    },
    { role: "editMenu" },
    { label: "View", submenu: [{ role: "reload" }, { role: "zoomIn" }, { role: "zoomOut" }, { role: "resetZoom" }, { type: "separator" }, { role: "togglefullscreen" }] },
    { role: "windowMenu" },
  ]));
}

app.on("second-instance", () => { if (win) { if (win.isMinimized()) win.restore(); win.focus(); } });

app.whenReady().then(() => {
  // Location for "Show my location"; nothing else is asked for.
  session.defaultSession.setPermissionRequestHandler((_wc, permission, done, details) =>
    done(isLocal(details.requestingUrl) && ["geolocation", "fullscreen", "clipboard-sanitized-write"].includes(permission)));

  win = new BrowserWindow({
    width: 1280, height: 860, minWidth: 360, minHeight: 480, title: "Varu",
    backgroundColor: "#f3f0e8",
    webPreferences: { preload: path.join(__dirname, "preload.js") },
  });
  win.on("closed", () => app.quit()); // a document window alone is no use

  // The installer page, when it runs in this app (installer/wizard.html).
  const fromInstaller = (event) => mode === "installer" && isLocal(event.senderFrame?.url ?? "");
  ipcMain.on("open-varu", (event, target) => {
    if (fromInstaller(event) && isDrive(target)) startViewer(target);
  });
  ipcMain.on("close-installer", (event) => {
    if (!fromInstaller(event)) return;
    const drive = findDrive();
    if (drive) startViewer(drive);
    else app.quit();
  });

  menu();
  const drive = findDrive();
  if (drive) {
    startViewer(drive);
    if (olderThanApp(drive)) offerUpdate(drive);
  } else {
    startInstaller();
  }
});

// Every window: Varu's own pages stay in the app, and a link that asks for a
// new window (documents, PDFs) gets one, so closing it returns to where you
// were. Credits and sources open in the browser.
const outside = (url) => { if (/^https?:\/\//.test(url)) shell.openExternal(url); };
app.on("web-contents-created", (_event, contents) => {
  contents.setWindowOpenHandler(({ url }) => {
    if (isLocal(url)) return { action: "allow", overrideBrowserWindowOptions: { width: 1000, height: 860, autoHideMenuBar: true } };
    outside(url);
    return { action: "deny" };
  });
  contents.on("will-navigate", (event, url) => {
    if (!isLocal(url)) { event.preventDefault(); outside(url); }
  });
});

app.on("window-all-closed", () => app.quit());
app.on("before-quit", () => { quitting = true; stop(); });
