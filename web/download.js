// Keeps a region in the browser's private storage (OPFS, folder "drive").
// The only code that knows how it is laid out; the pages and sw.js ask it.
//
// Each file is saved under its path and hash ("app%2Findex.html~1a2b…"), so
// a new version is written beside the old one and nothing in use changes
// under the app's feet. ".state" names the files of the version in use,
// and is replaced in one write when a new version is complete: that is
// the switch-over. ".next" records an install or update under way, so an
// interrupted one carries on at the part it was on. Files no version
// names are removed at the next "status".
//
// Messages in:
//   { check }          can this browser do it? -> { supported }
//   { status, list }   -> { installed, built, update (bytes to fetch), pending,
//                         migrated (files were renamed: sw.js must read them again) }
//   { base, list }     install or update from base (the region's folder URL)
//                      -> { done, total, path } while going, then { finished } or { error }
// list is the region's files.json.

const name = (path, hash) => `${encodeURIComponent(path)}~${hash}`;
let migrated = false;

onmessage = async ({ data }) => {
  if (data.check) {
    postMessage({ supported: typeof FileSystemFileHandle !== "undefined" && "createSyncAccessHandle" in FileSystemFileHandle.prototype });
    return;
  }
  try {
    const dir = await (await navigator.storage.getDirectory()).getDirectoryHandle("drive", { create: true });
    if (data.status) postMessage(await status(dir, data.list));
    else await download(dir, data.base, data.list);
  } catch (err) {
    postMessage({ error: String(err?.message ?? err) });
  }
};

async function status(dir, list) {
  const state = await readState(dir);
  const next = await readJSON(dir, ".next");
  // Clear out what no version names any more (the last update's leftovers).
  const keep = new Set([".state", ".next"]);
  for (const files of [state.files, next?.files ?? {}]) for (const [p, h] of Object.entries(files)) keep.add(name(p, h));
  if (next?.partial) keep.add(name(next.partial.path, next.partial.hash));
  for await (const entry of dir.keys()) if (!keep.has(entry)) await dir.removeEntry(entry).catch(() => {});
  const update = (list?.files ?? []).filter((f) => state.files[f.path] !== f.hash).reduce((n, f) => n + f.size, 0);
  return { installed: !!state.files["manifest.json"], built: state.built ?? null, update, pending: !!next, migrated };
}

async function download(dir, base, list) {
  const state = await readState(dir);
  const next = (await readJSON(dir, ".next")) ?? { files: {}, partial: null };
  const last = (f) => f.path === "manifest.json";
  const files = [...list.files.filter((f) => !last(f)), ...list.files.filter(last)];
  const total = files.reduce((n, f) => n + f.size, 0);
  let done = 0;
  let path = "";
  const tick = () => postMessage({ done, total, path });

  for (const file of files) {
    path = file.path;
    const fileName = name(file.path, file.hash);
    if ((state.files[file.path] === file.hash || next.files[file.path] === file.hash) && await sizeOf(dir, fileName) === file.size) {
      done += file.size;
      tick();
      continue;
    }
    const access = await (await dir.getFileHandle(fileName, { create: true })).createSyncAccessHandle();
    try {
      const parts = file.parts || 1;
      // Carry on at the first part not yet complete, if it is this same file.
      const same = next.partial?.path === file.path && next.partial.hash === file.hash;
      const first = same ? Math.min(Math.floor(access.getSize() / list.partSize), parts - 1) : 0;
      access.truncate(first * list.partSize);
      done += first * list.partSize;
      tick();
      next.partial = { path: file.path, hash: file.hash };
      await writeJSON(dir, ".next", next);
      for (let i = first; i < parts; i++) {
        const url = `${base}${file.path.split("/").map(encodeURIComponent).join("/")}${file.parts ? `.${i}` : ""}`;
        const res = await fetch(url, { cache: "no-store" });
        if (!res.ok) throw new Error(`${res.status} for ${file.path}`);
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
      }
      access.flush();
      if (access.getSize() !== file.size) throw new Error(`${file.path} came out the wrong size`);
    } finally {
      access.close();
    }
    next.files[file.path] = file.hash;
    next.partial = null;
    await writeJSON(dir, ".next", next);
  }

  // Every file is there: switch over in one write.
  const manifest = JSON.parse(await (await (await dir.getFileHandle(name("manifest.json", files.at(-1).hash))).getFile()).text());
  await writeJSON(dir, ".state", { files: Object.fromEntries(files.map((f) => [f.path, f.hash])), built: manifest.built ?? null });
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
