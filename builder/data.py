"""The content steps: `maps`, `pois`, `routing`, `library` and `documents`."""

from __future__ import annotations

import json
import re
import shutil
import subprocess
import time
import urllib.parse
import xml.etree.ElementTree as ET
from datetime import datetime, timedelta, timezone
from pathlib import Path

from .config import PoiSpec, ZimSpec
from .context import Context, StepError
from .binaries import go_command
from .fetch import FetchError, download, exists, fetch_bytes, final_url

# ---------------------------------------------------------------- maps ------


def step_maps(ctx: Context, *, refresh: bool = False) -> None:
    spec = ctx.region.map
    if spec is None:
        ctx.log("region has no [map]; skipping")
        return

    target = ctx.out / "maps" / f"{ctx.region.id}.pmtiles"
    if target.exists() and not refresh and not ctx.dry_run:
        ctx.log(f"have {target.name}; use --refresh to cut a newer one")
        return

    bbox = ",".join(f"{value:g}" for value in spec.bbox)
    if ctx.dry_run:
        url = ctx.sources["protomaps"]["build_url"].format(build=ctx.planet_build or "<newest>")
        ctx.log(f"would run: pmtiles extract {url} {target} --bbox={bbox} --maxzoom={spec.maxzoom}")
        return

    pmtiles = shutil.which("pmtiles")
    if pmtiles is None:
        raise StepError("the pmtiles command is not installed. Download it from "
                        "https://github.com/protomaps/go-pmtiles/releases and put it on your PATH.")

    build = resolve_planet_build(ctx)
    url = ctx.sources["protomaps"]["build_url"].format(build=build)
    partial = target.with_name(f"{ctx.region.id}.partial.pmtiles")
    target.parent.mkdir(parents=True, exist_ok=True)
    partial.unlink(missing_ok=True)

    ctx.log(f"extracting {bbox} up to zoom {spec.maxzoom} from build {build} (this downloads only the region)")
    result = subprocess.run([pmtiles, "extract", url, str(partial), f"--bbox={bbox}", f"--maxzoom={spec.maxzoom}"])
    if result.returncode != 0 or not partial.exists():
        partial.unlink(missing_ok=True)
        raise StepError(f"pmtiles extract failed (exit code {result.returncode})")
    partial.replace(target)
    ctx.update_info(map_build=build)
    ctx.log(f"wrote {target.name} ({target.stat().st_size / 1e6:.0f} MB)")


def resolve_planet_build(ctx: Context) -> str:
    """Newest daily build that exists, as YYYYMMDD."""
    if ctx.planet_build:
        return ctx.planet_build
    settings = ctx.sources["protomaps"]
    today = datetime.now(timezone.utc).date()
    for age in range(int(settings.get("lookback_days", 10))):
        build = (today - timedelta(days=age)).strftime("%Y%m%d")
        if exists(settings["build_url"].format(build=build)):
            return build
    raise StepError("no recent Protomaps build found; pick one at https://maps.protomaps.com/builds "
                    "and pass it with --planet-build YYYYMMDD")


# ---------------------------------------------------------------- pois ------

# Tags worth carrying to the map popup. Everything else is dropped to keep
# the files small.
KEEP_TAGS = (
    "name", "opening_hours", "phone", "contact:phone", "operator", "brand",
    "addr:street", "addr:housenumber", "addr:city",
)


def overpass_query(country_code: str, tags: tuple[str, ...], bbox: tuple[float, float, float, float] | None = None,
                   timeout: int = 300) -> str:
    """Places in the country and, when a bbox is given, inside the map area too."""
    # Overpass wants (south, west, north, east); region files use west, south, east, north.
    box = f"({bbox[1]:g},{bbox[0]:g},{bbox[3]:g},{bbox[2]:g})" if bbox else ""
    clauses = []
    for tag in tags:
        key, value = tag.split("=", 1)
        clauses.append(f'  nwr["{key}"="{value}"](area.region){box};')
    return (
        f"[out:json][timeout:{timeout}];\n"
        f'area["ISO3166-1"="{country_code}"][admin_level=2]->.region;\n'
        "(\n" + "\n".join(clauses) + "\n);\n"
        "out center tags;\n"
    )


def overpass_to_geojson(payload: dict, languages: tuple[str, ...] = ()) -> dict:
    """Turn an Overpass JSON answer into a compact GeoJSON point collection.

    Ways and relations (a hospital drawn as a building outline) are reduced
    to their centre point, which is what `out center` provides.
    """
    keep = KEEP_TAGS + tuple(f"name:{code}" for code in languages)
    features = {}
    for element in payload.get("elements", []):
        position = element if element.get("type") == "node" else element.get("center", {})
        lon, lat = position.get("lon"), position.get("lat")
        if lon is None or lat is None:
            continue
        tags = element.get("tags", {})
        osm_id = f"{element.get('type')}/{element.get('id')}"
        features[osm_id] = {
            "type": "Feature",
            "id": osm_id,
            "geometry": {"type": "Point", "coordinates": [round(lon, 6), round(lat, 6)]},
            "properties": {key: tags[key] for key in keep if key in tags},
        }
    ordered = [features[key] for key in sorted(features)]
    return {"type": "FeatureCollection", "features": ordered}


def step_pois(ctx: Context, *, refresh: bool = False) -> None:
    settings = ctx.sources["overpass"]
    failures = []
    asked = False
    for poi in ctx.region.pois:
        target = ctx.out / "pois" / f"{poi.id}.geojson"
        if ctx.dry_run:
            ctx.log(f"would query OpenStreetMap for {poi.id}: {', '.join(poi.tags)}")
            continue
        if target.exists() and not refresh:
            ctx.log(f"have {target.name}")
            continue
        if asked and not poi.geojson:
            time.sleep(float(settings.get("pause_seconds", 5)))  # be a polite guest on a shared server
        asked = asked or not poi.geojson
        try:
            bbox = ctx.region.map.bbox if ctx.region.map else None
            collection = fetch_pois(settings["url"], ctx.region.country_code, poi, ctx.region.languages, bbox)
        except (FetchError, ValueError) as err:
            failures.append(f"{poi.id}: {err}")
            continue
        target.parent.mkdir(parents=True, exist_ok=True)
        target.write_text(json.dumps(collection, ensure_ascii=False, separators=(",", ":")), encoding="utf-8")
        ctx.log(f"wrote {target.name} ({len(collection['features'])} places)")
    if failures:
        raise StepError("some place layers could not be fetched:\n  " + "\n  ".join(failures))


def official_to_geojson(payload: dict, poi: PoiSpec, bbox: tuple[float, float, float, float] | None) -> dict:
    """Points from an official list, with the fields the region file maps."""
    features = []
    for i, feature in enumerate(payload.get("features", [])):
        geometry = feature.get("geometry") or {}
        if geometry.get("type") != "Point":
            continue
        lon, lat = geometry["coordinates"][:2]
        if bbox and not (bbox[0] <= lon <= bbox[2] and bbox[1] <= lat <= bbox[3]):
            continue
        source = feature.get("properties") or {}
        props = {key: str(source[field]) for key, field in poi.properties.items() if source.get(field) not in (None, "")}
        features.append({
            "type": "Feature", "id": f"{poi.id}/{feature.get('id', i)}",
            "geometry": {"type": "Point", "coordinates": [round(lon, 6), round(lat, 6)]},
            "properties": props,
        })
    return {"type": "FeatureCollection", "features": features}


def fetch_pois(endpoint: str, country_code: str, poi: PoiSpec, languages: tuple[str, ...],
               bbox: tuple[float, float, float, float] | None = None) -> dict:
    if poi.geojson:
        return official_to_geojson(json.loads(fetch_bytes(poi.geojson)), poi, bbox)
    body = urllib.parse.urlencode({"data": overpass_query(country_code, poi.tags, bbox)}).encode()
    payload = json.loads(fetch_bytes(endpoint, data=body, timeout=360))
    return overpass_to_geojson(payload, languages)


# ------------------------------------------------------------- routing ------

_EXTRACT = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*\.osm\.pbf$")


def step_routing(ctx: Context, *, refresh: bool = False) -> None:
    """maps/<id>.graph: the road network Varu's own router finds routes on."""
    if ctx.region.osm_extract is None:
        ctx.log("region has no [routing]; skipping")
        return
    target = ctx.out / "maps" / f"{ctx.region.id}.graph"
    if target.exists() and not refresh and not ctx.dry_run:
        ctx.log(f"have {target.name}; use --refresh to build a newer one")
        return
    if ctx.dry_run:
        ctx.log(f"would download {ctx.region.osm_extract} and build {target.name} from it")
        return

    go = go_command()
    try:
        url = final_url(ctx.region.osm_extract)  # the dated file, so the source is exact
        name = urllib.parse.unquote(url.rsplit("/", 1)[-1])
        if not _EXTRACT.match(name):
            raise StepError(f"unexpected download link {url}")
        cache = ctx.cache / "osm" / ctx.region.id
        extract = download(url, cache / name, log=ctx.log)
    except FetchError as err:
        raise StepError(str(err)) from err
    for old in cache.glob("*.osm.pbf"):
        if old != extract:
            old.unlink()

    partial = target.with_name(f"{ctx.region.id}.partial.graph")
    target.parent.mkdir(parents=True, exist_ok=True)
    result = subprocess.run([go, "run", "./cmd/varu-graph", "-in", str(extract), "-out", str(partial),
                             "-source", url], cwd=ctx.repo / "router")
    if result.returncode != 0 or not partial.exists():
        partial.unlink(missing_ok=True)
        raise StepError(f"building the road network failed (exit code {result.returncode})")
    partial.replace(target)
    ctx.update_info(routing_source=url)
    ctx.log(f"wrote {target.name} ({target.stat().st_size / 1e6:.0f} MB)")


# ------------------------------------------------------------- library ------


def _local(tag: str) -> str:
    return tag.rsplit("}", 1)[-1]


def parse_catalog(xml: bytes) -> list[dict]:
    """Read the entries of a Kiwix OPDS catalogue feed."""
    entries = []
    for entry in ET.fromstring(xml).iter():
        if _local(entry.tag) != "entry":
            continue
        item: dict = {}
        for child in entry:
            tag = _local(child.tag)
            if tag in ("name", "flavour", "title", "language", "updated", "issued"):
                item[tag] = (child.text or "").strip()
            elif tag == "link":
                kind, rel = child.get("type", ""), child.get("rel", "")
                if kind == "application/x-zim" or rel.endswith("/acquisition/open-access"):
                    href = child.get("href", "")
                    # The catalogue links to a metalink; the file itself is the same URL without it.
                    item["url"] = href[:-len(".meta4")] if href.endswith(".meta4") else href
                    if child.get("length", "").isdigit():
                        item["size"] = int(child.get("length"))
        if item.get("name") and item.get("url"):
            entries.append(item)
    return entries


def choose_zim(entries: list[dict], spec: ZimSpec) -> dict | None:
    """Newest edition of the named book, in the requested flavour if one is given."""
    matches = [entry for entry in entries if entry["name"] == spec.name]
    if spec.flavour is not None:
        matches = [entry for entry in matches if entry.get("flavour", "") == spec.flavour]
    if not matches:
        return None
    return max(matches, key=lambda entry: (entry.get("issued") or entry.get("updated") or "", entry["url"]))


_DATED = re.compile(r"^(?P<stem>.+)_\d{4}-\d{2}\.zim$")


def older_editions(library: Path, filename: str) -> list[Path]:
    """Other dated copies of the same book, so a refresh does not keep two."""
    match = _DATED.match(filename)
    if not match:
        return []
    stem = match.group("stem")
    return [path for path in library.glob(f"{stem}_*.zim")
            if path.name != filename and (m := _DATED.match(path.name)) and m.group("stem") == stem]


def remove_older_editions(library: Path, filename: str) -> list[str]:
    """Delete other dated copies of the book and their search indexes; return the removed .zim names."""
    removed = []
    for old in older_editions(library, filename):
        old.unlink()
        old.with_suffix(".search").unlink(missing_ok=True)
        removed.append(old.name)
    return removed


def step_library(ctx: Context) -> None:
    catalog = ctx.sources["kiwix_catalog"]["url"]
    library = ctx.out / "library"
    titles = ctx.read_info().get("library", {})
    problems = []

    for spec in ctx.region.library:
        if ctx.dry_run:
            ctx.log(f"would download {spec.name}" + (f" ({spec.flavour})" if spec.flavour else ""))
            continue
        try:
            if spec.url:
                entry = {"url": spec.url, "name": spec.name, "title": spec.name}
            else:
                query = urllib.parse.urlencode({"name": spec.name, "count": -1})
                entry = choose_zim(parse_catalog(fetch_bytes(f"{catalog}?{query}")), spec)
                if entry is None:
                    flavour = f" in flavour '{spec.flavour}'" if spec.flavour else ""
                    problems.append(f"{spec.name}: not found in the Kiwix catalogue{flavour}")
                    continue
            filename = urllib.parse.unquote(entry["url"].rsplit("/", 1)[-1])
            if not filename.endswith(".zim") or "/" in filename or filename.startswith("."):
                problems.append(f"{spec.name}: unexpected download link {entry['url']}")
                continue
            download(entry["url"], library / filename, log=ctx.log)
        except (FetchError, ET.ParseError) as err:
            problems.append(f"{spec.name}: {err}")
            continue

        for name in remove_older_editions(library, filename):
            ctx.log(f"removing older edition {name}")
            titles.pop(name, None)
        titles[filename] = {
            "name": entry.get("name", spec.name),
            "title": entry.get("title") or spec.name,
            "language": entry.get("language", ""),
        }
        ctx.update_info(library=titles)

    if problems:
        raise StepError("some library files are missing:\n  " + "\n  ".join(problems))


# ----------------------------------------------------------- documents ------


def step_documents(ctx: Context) -> None:
    problems = []
    for doc in ctx.region.documents:
        if ctx.dry_run:
            ctx.log(f"would download {doc.url}")
            continue
        try:
            download(doc.url, ctx.out / "docs" / doc.file, log=ctx.log)
        except FetchError as err:
            problems.append(str(err))
    if problems:
        raise StepError("some documents could not be downloaded:\n  " + "\n  ".join(problems))
