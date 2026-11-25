"""The `manifest` step: describe what is actually on the drive.

The viewer reads manifest.json and shows only what it lists, so the
manifest is built from the files that exist, not from what the region file
hoped for. A partial build therefore still gives a working drive.
"""

from __future__ import annotations

import json
from datetime import datetime, timezone

from .context import Context

SCHEMA = 1


def build_manifest(ctx: Context) -> dict:
    region = ctx.region
    info = ctx.read_info()
    manifest: dict = {
        "schema": SCHEMA,
        "region": region.id,
        "name": region.name,
        "languages": list(region.languages),
        "built": datetime.now(timezone.utc).strftime("%Y-%m-%dT%H:%M:%SZ"),
        "attribution": list(region.attribution),
    }

    map_file = ctx.out / "maps" / f"{region.id}.pmtiles"
    if region.map and map_file.is_file():
        manifest["map"] = {
            "file": f"maps/{map_file.name}",
            "bounds": list(region.map.bbox),
            "center": list(region.map.center),
            "zoom": region.map.zoom,
            "maxzoom": region.map.maxzoom,
            "size": map_file.stat().st_size,
        }
        if info.get("map_build"):
            manifest["map"]["build"] = info["map_build"]

    graph = ctx.out / "maps" / f"{region.id}.graph"
    if graph.is_file():
        manifest["routing"] = {"file": f"maps/{graph.name}", "size": graph.stat().st_size}
        if info.get("routing_source"):
            manifest["routing"]["source"] = info["routing_source"]

    places = ctx.out / "maps" / f"{region.id}.places"
    if places.is_file():
        manifest["places"] = {"file": f"maps/{places.name}", "size": places.stat().st_size}
        if info.get("places_source"):
            manifest["places"]["source"] = info["places_source"]

    pois = []
    for poi in region.pois:
        path = ctx.out / "pois" / f"{poi.id}.geojson"
        if not path.is_file():
            continue
        try:
            count = len(json.loads(path.read_text(encoding="utf-8")).get("features", []))
        except (OSError, ValueError):
            ctx.log(f"warning: {path.name} is not valid GeoJSON; leaving it out")
            continue
        pois.append({
            "id": poi.id,
            "file": f"pois/{path.name}",
            "label": poi.label,
            "color": poi.color,
            "count": count,
            "default": poi.default,
        })
    manifest["pois"] = pois

    titles = info.get("library", {})
    library = []
    for path in sorted((ctx.out / "library").glob("*.zim")):
        if path.name.startswith("."):
            continue  # macOS "._" companions on exFAT
        known = titles.get(path.name, {})
        library.append({
            "file": f"library/{path.name}",
            "name": known.get("name", path.stem),
            "title": known.get("title", path.stem),
            "language": known.get("language", ""),
            "size": path.stat().st_size,
        })
    manifest["library"] = library

    documents = []
    for doc in region.documents:
        path = ctx.out / "docs" / doc.file
        if path.is_file():
            entry = {"file": f"docs/{doc.file}", "title": doc.title, "size": path.stat().st_size, "source": doc.url}
            if doc.lang:
                entry["lang"] = doc.lang
            documents.append(entry)
    manifest["documents"] = documents
    return manifest


def step_manifest(ctx: Context) -> None:
    if ctx.dry_run:
        ctx.log("would write manifest.json from the files present on the drive")
        return
    manifest = build_manifest(ctx)
    ctx.out.mkdir(parents=True, exist_ok=True)
    (ctx.out / "manifest.json").write_text(
        json.dumps(manifest, indent=2, ensure_ascii=False) + "\n", encoding="utf-8")
    parts = [
        "map" if "map" in manifest else "no map",
        "road data" if "routing" in manifest else "no road data",
        "addresses" if "places" in manifest else "no addresses",
        f"{len(manifest['pois'])} place layers",
        f"{len(manifest['library'])} library files",
        f"{len(manifest['documents'])} documents",
    ]
    ctx.log("wrote manifest.json: " + ", ".join(parts))
