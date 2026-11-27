// Keeps a region in the browser's private storage (OPFS, folder "drive").
// The only code that knows how it is laid out; the pages and sw.js ask it.
//
// Each file is saved under its path and hash ("app%2Findex.html~1a2b…"), so
// a new version is written beside the old one and nothing in use changes
// under the app's feet. ".state" names the files of the version in use and
// the parts left out, and is replaced in one write when a new version is
// complete: that is the switch-over. ".next" records an install or update
// under way, so an interrupted one carries on where it was. Files no
// version names are removed at the next "status".
//
// Messages in:
//   { check }          can this browser do it? -> { supported }
//   { status, list }   -> { installed, built, without, update (bytes to fetch),
//                         pending, migrated (files were renamed: sw.js must read them again) }
//   { base, list, without }  install or update from base (the region's folder
//                      URL), leaving out the parts in without (as before if not given)
//                      -> { done, total, path } while going, then { finished } or { error }
// list is the region's files.json.

const name = (path, hash) => `${encodeURIComponent(path)}~${hash}`;
// The parts a person may leave out (install.js offers them).
const PARTS = {
  map: (path) => path.endsWith(".pmtiles") || path.startsWith("pois/"),
  places: (path) => path.endsWith(".places"),
  roads: (path) => path.endsWith(".graph"),
};
const wanted = (list, without) => (list?.files ?? []).filter((f) => !without.some((part) => PARTS[part]?.(f.path)));
// Small files are fetched this many at a time: an app has hundreds, and
// each costs a round trip.
const AT_ONCE = 8;
let migrated = false;

onmessage = async ({ data }) => {
  if (data.check) {
    postMessage({ supported: typeof FileSystemFileHandle !== "undefined" && "createSyncAccessHandle" in FileSystemFileHandle.prototype });
    return;
  }
  try {
    const dir = await (await navigator.storage.getDirectory()).getDirectoryHandle("drive", { create: true });
    if (data.status) postMessage(await status(dir, data.list));
    else await download(dir, data.base, data.list, data.without);
  } catch (err) {
    postMessage({ error: String(err?.message ?? err) });
  }
};

async function status(dir, list) {
  const state = await readState(dir);
  const next = await readJSON(dir, ".next");
  // Clear out what no version names any more (the last update's leftovers).
  const keep = new Set([".state", ".next"]);
  for (const files of [state.files, next?.files ?? {}, next?.partials ?? {}]) {
    for (const [p, h] of Object.entries(files)) keep.add(name(p, h));
  }
  for await (const entry of dir.keys()) if (!keep.has(entry)) await dir.removeEntry(entry).catch(() => {});
  const without = state.without ?? [];
  const update = wanted(list, without).filter((f) => state.files[f.path] !== f.hash).reduce((n, f) => n + f.size, 0);
  return { installed: !!state.files["manifest.json"], built: state.built ?? null, without, update, pending: !!next, migrated };
}

async function download(dir, base, list, without) {
  const state = await readState(dir);
  without ??= state.without ?? [];
  const next = (await readJSON(dir, ".next")) ?? {};
  next.files ??= {};
  next.partials ??= {}; // large files part-way done: path -> hash
  let saving = Promise.resolve();
  const save = () => (saving = saving.then(() => writeJSON(dir, ".next", next)));
  const last = (f) => f.path === "manifest.json";
  const files = wanted(list, without);
  const order = [...files.filter((f) => !last(f)), ...files.filter(last)];
  const total = files.reduce((n, f) => n + f.size, 0);
  let done = 0;
  // Progress names the first file in order not yet finished.
  const finished = new Set();
  let first = 0;
  const tick = () => {
    while (first < order.length && finished.has(order[first].path)) first++;
    postMessage({ done, total, path: order[Math.min(first, order.length - 1)].path });
  };

  async function fetchFile(file) {
    const fileName = name(file.path, file.hash);
    if ((state.files[file.path] === file.hash || next.files[file.path] === file.hash) && await sizeOf(dir, fileName) === file.size) {
      done += file.size;
      return;
    }
    const access = await (await dir.getFileHandle(fileName, { create: true })).createSyncAccessHandle();
    try {
      const parts = file.parts || 1;
      // A large file carries on at its first part not yet complete.
      const from = file.parts && next.partials[file.path] === file.hash
        ? Math.min(Math.floor(access.getSize() / list.partSize), parts - 1) : 0;
      access.truncate(from * list.partSize);
      done += from * list.partSize;
      if (file.parts) {
        next.partials[file.path] = file.hash;
        await save();
      }
      const part = async (i) => {
        const url = `${base}${file.path.split("/").map(encodeURIComponent).join("/")}${file.parts ? `.${i}` : ""}`;
        const res = await fetch(url, { cache: "no-store" });
        if (!res.ok) {
          await res.body?.cancel(); // else it holds the connection
          throw Object.assign(new Error(`${res.status} for ${file.path}`), { final: res.status < 500 && res.status !== 429 });
        }
        let at = i * list.partSize;
        access.truncate(at);
        const reader = res.body.getReader();
        for (;;) {
          const { done: end, value } = await reader.read();
          if (end) break;
          access.write(value, { at });
          at += value.byteLength;
          done += value.byteLength;
          tick();
        }
      };
      // GitHub Pages now and then answers 503, or a connection drops: wait
      // and try again (2, 4, 8, 16 s) before giving up.
      for (let i = from; i < parts; i++) {
        for (let tries = 1; ; tries++) {
          const before = done;
          try {
            await part(i);
            break;
          } catch (err) {
            done = before;
            if (tries === 5 || err.final) throw err;
            await new Promise((resolve) => setTimeout(resolve, 1000 * 2 ** tries));
          }
        }
      }
      access.flush();
      if (access.getSize() !== file.size) throw new Error(`${file.path} came out the wrong size`);
    } finally {
      access.close();
    }
    next.files[file.path] = file.hash;
    delete next.partials[file.path];
    await save();
  }

  // Everything but the manifest, a few at a time in order; then the manifest.
  const queue = order.filter((f) => !last(f));
  await Promise.all(Array.from({ length: AT_ONCE }, async () => {
    for (let file; (file = queue.shift());) {
      await fetchFile(file);
      finished.add(file.path);
      tick();
    }
  }));
  for (const file of order.filter(last)) {
    await fetchFile(file);
    finished.add(file.path);
    tick();
  }
  await saving;

  // Every file is there: switch over in one write.
  const manifestFile = files.find(last);
  const manifest = JSON.parse(await (await (await dir.getFileHandle(name("manifest.json", manifestFile.hash))).getFile()).text());
  await writeJSON(dir, ".state", { files: Object.fromEntries(files.map((f) => [f.path, f.hash])), without, built: manifest.built ?? null });
  await dir.removeEntry(".next").catch(() => {});
  postMessage({ finished: true });
}

// The version in use. An install from before files carried their hash
// (".state" with "done") is renamed to the new names where the browser can.
async function readState(dir) {
  const state = (await readJSON(dir, ".state")) ?? {};
  if (state.files) return state;
  const files = {};
  if (!state.partial) {
    for (const [path, hash] of Object.entries(state.done ?? {})) {
      const handle = await dir.getFileHandle(encodeURIComponent(path)).catch(() => null);
      if (handle?.move && await handle.move(name(path, hash)).then(() => true, () => false)) files[path] = hash;
    }
  }
  const renamed = { files, built: null };
  if (files["manifest.json"]) {
    renamed.built = JSON.parse(await (await (await dir.getFileHandle(name("manifest.json", files["manifest.json"]))).getFile()).text()).built ?? null;
  }
  if (state.done) {
    await writeJSON(dir, ".state", renamed);
    migrated = true;
  }
  return renamed;
}

async function sizeOf(dir, fileName) {
  try {
    return (await (await dir.getFileHandle(fileName)).getFile()).size;
  } catch {
    return -1;
  }
}

async function readJSON(dir, fileName) {
  try {
    return JSON.parse(await (await (await dir.getFileHandle(fileName)).getFile()).text());
  } catch {
    return null;
  }
}

async function writeJSON(dir, fileName, value) {
  const access = await (await dir.getFileHandle(fileName, { create: true })).createSyncAccessHandle();
  try {
    const data = new TextEncoder().encode(JSON.stringify(value));
    access.truncate(0);
    access.write(data, { at: 0 });
    access.flush();
  } finally {
    access.close();
  }
}
