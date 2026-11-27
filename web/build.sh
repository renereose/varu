#!/bin/sh
# Builds the web app site from a built drive:
#
#   sh web/build.sh build/ee site/ [version]
#
# site/ then holds the install page, its service worker, and data/<region>/:
# the drive's app, manifest, map layers, map, road network and places
# (not the library), plus varu.wasm (the launcher compiled for the
# browser). Files over 48 MB are split into parts (.0, .1, …) because
# GitHub Pages refuses files over 100 MB; files.json lists them with their
# sizes and hashes. Serve site/ at any path, over HTTPS (or localhost).
set -eu
drive=${1:?usage: sh web/build.sh <drive> <site> [version]}
site=${2:?usage: sh web/build.sh <drive> <site> [version]}
version=${3:-dev}
here=$(cd "$(dirname "$0")" && pwd)
[ -f "$drive/manifest.json" ] && [ -d "$drive/app" ] || { echo "web: $drive is not a built drive" >&2; exit 1; }

rm -rf "$site"
mkdir -p "$site"
touch "$site/.nojekyll" # GitHub Pages: serve files as they are
cp "$here"/index.html "$here"/install.js "$here"/download.js "$here"/sw.js \
  "$here"/manifest.webmanifest "$here"/*.png "$site"/
cp "$(go env GOROOT)/lib/wasm/wasm_exec.js" "$site"/
wasm="$site/varu.wasm.tmp"
(cd "$here/../launcher" && GOOS=js GOARCH=wasm go build -trimpath -ldflags "-s -w -X main.version=$version" -o "$wasm" .)

python3 - "$drive" "$site" "$wasm" <<'EOF'
import hashlib, json, os, shutil, sys

drive, site, wasm = sys.argv[1:]
PART = 48 * 1024 * 1024
manifest = json.load(open(os.path.join(drive, "manifest.json")))
region = manifest["region"]
out = os.path.join(site, "data", region)

wanted = ["manifest.json"]
for top in ("app", "pois"):
    for base, _, names in os.walk(os.path.join(drive, top)):
        wanted += [os.path.relpath(os.path.join(base, n), drive) for n in names if not n.startswith(".")]
for key in ("map", "routing", "places"):
    if manifest.get(key, {}).get("file"):
        wanted.append(manifest[key]["file"])

files = []
for rel, src in [(r, os.path.join(drive, r)) for r in sorted(wanted)] + [("varu.wasm", wasm)]:
    rel = rel.replace(os.sep, "/")
    size = os.path.getsize(src)
    digest = hashlib.sha256()
    dest = os.path.join(out, rel)
    os.makedirs(os.path.dirname(dest), exist_ok=True)
    with open(src, "rb") as f:
        if size <= PART:
            data = f.read()
            digest.update(data)
            open(dest, "wb").write(data)
            parts = 0
        else:
            parts = 0
            while chunk := f.read(PART):
                digest.update(chunk)
                open(f"{dest}.{parts}", "wb").write(chunk)
                parts += 1
    files.append({"path": rel, "size": size, "hash": digest.hexdigest()[:16], "parts": parts})
os.remove(wasm)

name = manifest.get("name", {})
name = {k: v.split("–")[-1].strip() for k, v in (name.items() if isinstance(name, dict) else [("en", name)])}
json.dump({"region": region, "name": name, "partSize": PART, "files": files},
          open(os.path.join(out, "files.json"), "w"), ensure_ascii=False, indent=1)
print(f"web: {len(files)} files, {sum(f['size'] for f in files) / 1e6:.0f} MB, in {out}")
EOF
