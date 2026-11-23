"""Package a built drive as release assets for the installer.

    python3 -m builder --region ee --out drives/ee --skip library
    python3 -m builder.release --region ee --drive drives/ee --out dist

Writes to dist/:
  varu-<id>.zip    everything on the drive except the map and the library
  <id>.pmtiles     the map, downloaded on its own so it can resume
  regions.json     what the installer offers; one entry per packaged region

Library files are not re-hosted: regions.json points at the Kiwix mirrors,
pinned to the edition that was newest when the release was made.
"""

from __future__ import annotations

import argparse
import hashlib
import json
import shutil
import sys
import urllib.parse
import urllib.request
import zipfile
from pathlib import Path

from .config import ConfigError, Region, ZimSpec, load_region, load_sources
from .data import choose_zim, parse_catalog
from .fetch import CHUNK, USER_AGENT, FetchError, fetch_bytes

INDEX_SCHEMA = 1
# Not part of the bundle: shipped separately (maps) or fetched from Kiwix (library).
LEFT_OUT = {"maps", "library", "build-info.json"}


def sha256(path: Path) -> str:
    digest = hashlib.sha256()
    with path.open("rb") as handle:
        for block in iter(lambda: handle.read(CHUNK), b""):
            digest.update(block)
    return digest.hexdigest()


def asset(file: Path, **extra) -> dict:
    return {"asset": file.name, "size": file.stat().st_size, "sha256": sha256(file), **extra}


def write_bundle(drive: Path, target: Path) -> None:
    """Zip the drive, keeping file modes so the launchers stay executable."""
    with zipfile.ZipFile(target, "w", zipfile.ZIP_DEFLATED) as bundle:
        for path in sorted(drive.rglob("*")):
            rel = path.relative_to(drive)
            if rel.parts[0] in LEFT_OUT or any(part.startswith(".") for part in rel.parts):
                continue
            if path.is_file() and not path.name.endswith(".part"):
                bundle.write(path, rel.as_posix())


def _head_size(url: str) -> int | None:
    request = urllib.request.Request(url, method="HEAD", headers={"User-Agent": USER_AGENT})
    with urllib.request.urlopen(request, timeout=60) as response:
        length = response.headers.get("Content-Length")
        return int(length) if length else None


def resolve_zim(spec: ZimSpec, catalog: str) -> dict:
    """Pin a library entry to one downloadable file."""
    if spec.url:
        entry = {"url": spec.url, "name": spec.name, "title": spec.name, "size": _head_size(spec.url)}
    else:
        query = urllib.parse.urlencode({"name": spec.name, "count": -1})
        entry = choose_zim(parse_catalog(fetch_bytes(f"{catalog}?{query}")), spec)
        if entry is None:
            raise FetchError(f"{spec.name}: not found in the Kiwix catalogue")
    file = urllib.parse.unquote(entry["url"].rsplit("/", 1)[-1])
    if not file.endswith(".zim") or "/" in file or file.startswith("."):
        raise FetchError(f"{spec.name}: unexpected download link {entry['url']}")
    try:
        # Kiwix publishes "<hash>  <file>" next to every archive.
        checksum = fetch_bytes(entry["url"] + ".sha256").decode().split()[0].lower()
    except (FetchError, IndexError, UnicodeDecodeError):
        checksum = None  # the installer then checks the size only
    return {
        "name": spec.name,
        "title": entry.get("title") or spec.name,
        "language": entry.get("language", ""),
        "file": file,
        "url": entry["url"],
        "size": entry.get("size"),
        "sha256": checksum,
    }


def package(region: Region, drive: Path, out: Path, catalog: str, log=print) -> dict:
    out.mkdir(parents=True, exist_ok=True)
    if not (drive / "manifest.json").is_file():
        raise FileNotFoundError(f"{drive} has no manifest.json; build the drive first")

    bundle = out / f"varu-{region.id}.zip"
    write_bundle(drive, bundle)
    with zipfile.ZipFile(bundle) as packed:
        unpacked = sum(info.file_size for info in packed.infolist())
    manifest = json.loads((drive / "manifest.json").read_text(encoding="utf-8"))
    entry = {
        "id": region.id, "name": region.name, "country_code": region.country_code,
        "languages": list(region.languages),
        "bounds": manifest.get("map", {}).get("bounds"), "center": manifest.get("map", {}).get("center"),
        # The installer offers these as categories and trims them to the family's radius.
        "pois": [{key: layer[key] for key in ("id", "label", "color", "default", "count")}
                 for layer in manifest.get("pois", [])],
        "bundle": asset(bundle, unpacked=unpacked),
    }
    log(f"wrote {bundle.name} ({bundle.stat().st_size / 1e6:.0f} MB)")

    map_file = drive / "maps" / f"{region.id}.pmtiles"
    if map_file.is_file():
        copy = out / map_file.name
        shutil.copyfile(map_file, copy)
        entry["map"] = asset(copy, path=f"maps/{map_file.name}")
        log(f"wrote {copy.name} ({copy.stat().st_size / 1e6:.0f} MB)")

    entry["library"] = []
    for spec in region.library:
        zim = resolve_zim(spec, catalog)
        entry["library"].append(zim)
        log(f"pinned {zim['file']}")

    index_path = out / "regions.json"
    index = json.loads(index_path.read_text(encoding="utf-8")) if index_path.is_file() else {"schema": INDEX_SCHEMA}
    index["regions"] = [r for r in index.get("regions", []) if r["id"] != region.id] + [entry]
    index_path.write_text(json.dumps(index, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    return entry


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(prog="python3 -m builder.release", description=__doc__.split("\n")[0])
    parser.add_argument("--region", required=True)
    parser.add_argument("--drive", required=True, type=Path, help="a drive built with --skip library")
    parser.add_argument("--out", required=True, type=Path)
    args = parser.parse_args(argv)
    repo = Path(__file__).resolve().parent.parent
    try:
        region = load_region(args.region, repo)
        catalog = load_sources(repo)["kiwix_catalog"]["url"]
        package(region, args.drive.resolve(), args.out, catalog)
    except (ConfigError, FetchError, OSError) as err:
        print(f"error: {err}", file=sys.stderr)
        return 1
    return 0


if __name__ == "__main__":
    sys.exit(main())
