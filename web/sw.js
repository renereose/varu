// The web app's service worker. Under app/ it answers every request with
// the launcher itself (varu.wasm, launcher/wasm.go), reading the drive the
// install page saved in the browser's private storage (OPFS, folder
// "drive"). Everything else here is the install page, kept for offline.

importScripts("wasm_exec.js");

const SHELL = "varu-shell-v1";
const SHELL_FILES = ["./", "install.js", "download.js", "manifest.webmanifest", "icon-192.png", "icon-512.png", "icon-maskable.png", "apple-touch-icon.png"];
const scope = new URL(self.registration.scope).pathname; // "/varu/"
const APP = `${scope}app/`;

self.addEventListener("install", (event) => {
  event.waitUntil(caches.open(SHELL).then((cache) => cache.addAll(SHELL_FILES)).then(() => self.skipWaiting()));
});
self.addEventListener("activate", (event) => {
  event.waitUntil(caches.keys()
    .then((keys) => Promise.all(keys.filter((k) => k !== SHELL).map((k) => caches.delete(k))))
    .then(() => self.clients.claim()));
});

self.addEventListener("fetch", (event) => {
  const url = new URL(event.request.url);
  if (url.origin !== location.origin || !url.pathname.startsWith(scope)) return;
  if (url.pathname.startsWith(APP)) event.respondWith(app(event.request, url));
  else if (event.request.method === "GET" && !url.pathname.startsWith(`${scope}data/`)) event.respondWith(shell(event.request));
});

// The install page: the network when there is one, else the saved copy.
async function shell(request) {
  const cache = await caches.open(SHELL);
  try {
    const res = await fetch(request);
    if (res.ok && new URL(request.url).search === "") cache.put(request, res.clone());
    return res;
  } catch {
    return (await cache.match(request, { ignoreSearch: true })) ?? Response.error();
  }
}

// ---------- the drive ----------

let started = null;

// The install page changed the saved files: read them again on the next request.
self.addEventListener("message", (event) => {
  if (event.data !== "reload") return;
  started = null;
  delete globalThis.varuServe;
});

async function app(request, url) {
  let go;
  try {
    go = await (started ??= start());
  } catch (err) {
    started = null;
    return Response.redirect(scope, 302); // nothing installed (yet): the install page
  }
  const path = `/${url.pathname.slice(APP.length)}${url.search}`;
  let res;
  try {
    res = await go(request.method, path, request.headers.get("Range") ?? "");
  } catch (err) {
    return new Response(`Varu could not read ${path}: ${err?.message ?? err}`, { status: 500 });
  }
  const type = res.headers["Content-Type"] ?? "";
  let body = [101, 204, 205, 304].includes(res.status) ? null : res.body;
  if (body && type.startsWith("text/html")) {
    // The pages are written for a drive's root; here they live under app/.
    body = new TextDecoder().decode(body).replace('<base href="/">',
      `<base href="${APP}"><link rel="manifest" href="${scope}manifest.webmanifest">` +
      `<link rel="apple-touch-icon" href="${scope}apple-touch-icon.png"><meta name="apple-mobile-web-app-capable" content="yes">`);
  }
  const headers = new Headers(res.headers);
  const to = headers.get("Location");
  if (to) headers.set("Location", to.startsWith("/") ? `${APP}${to.slice(1)}` : new URL(to, url).href);
  return new Response(body, { status: res.status, headers });
}

// Loads the drive's file list and starts the launcher. Fails when nothing
// is installed.
async function start() {
  const root = await navigator.storage.getDirectory();
  const dir = await root.getDirectoryHandle("drive");
  const files = new Map();
  for await (const [name, handle] of dir.entries()) {
    if (handle.kind === "file") files.set(`/drive/${decodeURIComponent(name)}`, await handle.getFile());
  }
  if (!files.has("/drive/manifest.json") || !files.has("/drive/varu.wasm")) throw new Error("not installed");
  driveFS(files);
  const go = new Go();
  const { instance } = await WebAssembly.instantiate(await files.get("/drive/varu.wasm").arrayBuffer(), go.importObject);
  go.run(instance);
  while (typeof globalThis.varuServe !== "function") await new Promise((r) => setTimeout(r, 0));
  return globalThis.varuServe;
}

// The little of Node's fs that Go's os package uses, read-only, over the
// saved files. Reads slice the file, so a large map is never loaded whole.
function driveFS(files) {
  const dirs = new Map(); // folder -> names in it
  for (const path of files.keys()) {
    for (let p = path; p !== "/";) {
      const parent = p.slice(0, p.lastIndexOf("/")) || "/";
      if (!dirs.has(parent)) dirs.set(parent, new Set());
      dirs.get(parent).add(p.slice(parent.length + (parent === "/" ? 0 : 1)));
      p = parent;
    }
  }
  const fail = (code) => Object.assign(new Error(code), { code });
  const stat = (path) => {
    path = path.replace(/\/+$/, "") || "/";
    const file = files.get(path);
    if (!file && !dirs.has(path)) return null;
    const dir = !file;
    const ms = file?.lastModified ?? 0;
    return {
      dev: 0, ino: 0, mode: dir ? 0o40555 : 0o100444, nlink: 1, uid: 0, gid: 0, rdev: 0,
      size: file?.size ?? 0, blksize: 4096, blocks: Math.ceil((file?.size ?? 0) / 512),
      atimeMs: ms, mtimeMs: ms, ctimeMs: ms, isDirectory: () => dir,
    };
  };
  const open = new Map();
  let next = 10;
  Object.assign(globalThis.fs, {
    constants: { O_WRONLY: 1, O_RDWR: 2, O_CREAT: 64, O_EXCL: 128, O_TRUNC: 512, O_APPEND: 1024, O_DIRECTORY: 65536 },
    open(path, flags, mode, cb) {
      if (flags & 3) return cb(fail("EACCES"));
      if (!stat(path)) return cb(fail("ENOENT"));
      open.set(next, { path: path.replace(/\/+$/, "") || "/", pos: 0 });
      cb(null, next++);
    },
    close(fd, cb) { open.delete(fd); cb(null); },
    stat(path, cb) { const st = stat(path); st ? cb(null, st) : cb(fail("ENOENT")); },
    lstat(path, cb) { this.stat(path, cb); },
    fstat(fd, cb) { const f = open.get(fd); f ? cb(null, stat(f.path)) : cb(fail("EBADF")); },
    readdir(path, cb) { const names = dirs.get(path.replace(/\/+$/, "") || "/"); names ? cb(null, [...names]) : cb(fail("ENOTDIR")); },
    read(fd, buffer, offset, length, position, cb) {
      const f = open.get(fd);
      const file = f && files.get(f.path);
      if (!file) return cb(fail("EBADF"));
      const at = position ?? f.pos;
      file.slice(at, at + length).arrayBuffer().then((data) => {
        buffer.set(new Uint8Array(data), offset);
        if (position === null || position === undefined) f.pos += data.byteLength;
        cb(null, data.byteLength);
      }, () => cb(fail("EIO")));
    },
  });
}
