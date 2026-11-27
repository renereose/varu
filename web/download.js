// Saves a region into the browser's private storage (OPFS, folder
// "drive"), one file per drive file, named by its path. Large files come
// in parts (GitHub Pages takes files up to 100 MB). ".state" remembers
// which files are complete (by their hash), so an update fetches only
// what changed and an interrupted install carries on at the part it was
// on. manifest.json is written last: the app counts as installed once it is.
//
// Message in:  { base, list } (base: the region's folder URL, list: files.json),
// or { check } to ask whether this browser can do it.
// Messages out: { done, total, path } while going (path: the file it is on), then { finished } or { error }.

onmessage = async ({ data: { base, list, check } }) => {
  // The page asks first: writing files is only offered to workers.
  if (check) return postMessage({ supported: typeof FileSystemFileHandle !== "undefined" && "createSyncAccessHandle" in FileSystemFileHandle.prototype });
  try {
    const root = await navigator.storage.getDirectory();
    const dir = await root.getDirectoryHandle("drive", { create: true });
    const state = await readState(dir);
    const last = (f) => f.path === "manifest.json";
    const files = [...list.files.filter((f) => !last(f)), ...list.files.filter(last)];
    const total = files.reduce((n, f) => n + f.size, 0);
    let done = 0;
    let path = "";
    const tick = () => postMessage({ done, total, path });

    // Files of an older install that this one no longer has.
    const wanted = new Set([".state", ...files.map((f) => encodeURIComponent(f.path))]);
    for await (const name of dir.keys()) if (!wanted.has(name)) await dir.removeEntry(name);

    for (const file of files) {
      path = file.path;
      if (state.done[file.path] === file.hash) {
        done += file.size;
        tick();
        continue;
      }
      const handle = await dir.getFileHandle(encodeURIComponent(file.path), { create: true });
      const access = await handle.createSyncAccessHandle();
      try {
        const parts = file.parts || 1;
        // Carry on at the first part not yet complete, if it is this same file.
        const same = state.partial?.path === file.path && state.partial.hash === file.hash;
        const first = same ? Math.min(Math.floor(access.getSize() / list.partSize), parts - 1) : 0;
        access.truncate(first * list.partSize);
        done += first * list.partSize;
        tick();
        delete state.done[file.path];
        state.partial = { path: file.path, hash: file.hash };
        await writeState(dir, state);
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
      state.done[file.path] = file.hash;
      state.partial = null;
      await writeState(dir, state);
    }
    postMessage({ finished: true });
  } catch (err) {
    postMessage({ error: String(err?.message ?? err) });
  }
};

async function readState(dir) {
  try {
    const file = await (await dir.getFileHandle(".state")).getFile();
    return { done: {}, ...JSON.parse(await file.text()) };
  } catch {
    return { done: {}, partial: null };
  }
}

async function writeState(dir, state) {
  const access = await (await dir.getFileHandle(".state", { create: true })).createSyncAccessHandle();
  try {
    const data = new TextEncoder().encode(JSON.stringify(state));
    access.truncate(0);
    access.write(data, { at: 0 });
    access.flush();
  } finally {
    access.close();
  }
}
