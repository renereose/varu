"""Loading and validating region files and the pinned sources file."""

from __future__ import annotations

import re
import tomllib
from dataclasses import dataclass, field
from pathlib import Path


class ConfigError(ValueError):
    """A region or sources file is missing something or makes no sense."""


_ID = re.compile(r"^[a-z0-9][a-z0-9_-]*$")
_TAG = re.compile(r"^[A-Za-z0-9_:.-]+=[A-Za-z0-9_:.;-]+$")
_COLOR = re.compile(r"^#[0-9a-fA-F]{6}$")
_SAFE_FILE = re.compile(r"^[A-Za-z0-9][A-Za-z0-9._-]*$")


@dataclass(frozen=True)
class MapSpec:
    bbox: tuple[float, float, float, float]
    maxzoom: int
    center: tuple[float, float]
    zoom: float


@dataclass(frozen=True)
class PoiSpec:
    id: str
    tags: tuple[str, ...]
    color: str
    label: dict[str, str]
    default: bool = False


@dataclass(frozen=True)
class ZimSpec:
    name: str
    flavour: str | None = None
    url: str | None = None  # bypass the catalogue with a direct link


@dataclass(frozen=True)
class DocumentSpec:
    url: str
    file: str
    title: dict[str, str]
    lang: str | None = None


@dataclass(frozen=True)
class Region:
    id: str
    country_code: str
    languages: tuple[str, ...]
    name: dict[str, str]
    map: MapSpec | None
    pois: tuple[PoiSpec, ...] = ()
    library: tuple[ZimSpec, ...] = ()
    documents: tuple[DocumentSpec, ...] = ()
    attribution: tuple[str, ...] = field(default_factory=tuple)


def _require(table: dict, key: str, where: str):
    if key not in table:
        raise ConfigError(f"{where}: missing '{key}'")
    return table[key]


def _labels(value, where: str) -> dict[str, str]:
    if isinstance(value, str):
        return {"en": value}
    if not isinstance(value, dict) or not value or not all(isinstance(v, str) for v in value.values()):
        raise ConfigError(f"{where}: expected text or a table like {{ en = \"…\", et = \"…\" }}")
    return dict(value)


def _parse_map(table: dict) -> MapSpec:
    bbox = _require(table, "bbox", "[map]")
    if not (isinstance(bbox, list) and len(bbox) == 4 and all(isinstance(n, (int, float)) for n in bbox)):
        raise ConfigError("[map] bbox must be [west, south, east, north]")
    west, south, east, north = (float(n) for n in bbox)
    if not (-180 <= west < east <= 180 and -90 <= south < north <= 90):
        raise ConfigError("[map] bbox must be [west, south, east, north] with west < east and south < north")
    maxzoom = int(table.get("maxzoom", 15))
    if not 0 <= maxzoom <= 15:
        raise ConfigError("[map] maxzoom must be between 0 and 15")
    center = table.get("center", [(west + east) / 2, (south + north) / 2])
    if not (isinstance(center, list) and len(center) == 2):
        raise ConfigError("[map] center must be [longitude, latitude]")
    return MapSpec(
        bbox=(west, south, east, north),
        maxzoom=maxzoom,
        center=(float(center[0]), float(center[1])),
        zoom=float(table.get("zoom", 6)),
    )


def _parse_pois(items: list) -> tuple[PoiSpec, ...]:
    seen: set[str] = set()
    out = []
    for i, item in enumerate(items):
        where = f"[[pois]] #{i + 1}"
        poi_id = _require(item, "id", where)
        if not _ID.match(poi_id):
            raise ConfigError(f"{where}: id '{poi_id}' must be lowercase letters, digits, - or _")
        if poi_id in seen:
            raise ConfigError(f"{where}: duplicate id '{poi_id}'")
        seen.add(poi_id)
        tags = _require(item, "tags", where)
        if not tags or not all(isinstance(tag, str) and _TAG.match(tag) for tag in tags):
            raise ConfigError(f"{where}: tags must be a list of OpenStreetMap \"key=value\" strings")
        color = item.get("color", "#455a64")
        if not _COLOR.match(color):
            raise ConfigError(f"{where}: color must look like #1a2b3c")
        out.append(PoiSpec(
            id=poi_id,
            tags=tuple(tags),
            color=color,
            label=_labels(item.get("label", poi_id), where),
            default=bool(item.get("default", False)),
        ))
    return tuple(out)


def _parse_documents(items: list) -> tuple[DocumentSpec, ...]:
    out = []
    for i, item in enumerate(items):
        where = f"[[documents]] #{i + 1}"
        url = _require(item, "url", where)
        if not url.startswith(("https://", "http://")):
            raise ConfigError(f"{where}: url must be http(s)")
        file = _require(item, "file", where)
        if not _SAFE_FILE.match(file):
            raise ConfigError(f"{where}: file '{file}' must be a plain file name")
        out.append(DocumentSpec(
            url=url, file=file,
            title=_labels(item.get("title", file), where),
            lang=item.get("lang"),
        ))
    return tuple(out)


def parse_region(data: dict) -> Region:
    region_id = _require(data, "id", "region")
    if not _ID.match(region_id):
        raise ConfigError(f"region id '{region_id}' must be lowercase letters, digits, - or _")
    country = _require(data, "country_code", "region")
    if not re.fullmatch(r"[A-Z]{2}", country):
        raise ConfigError("country_code must be a two-letter ISO 3166-1 code such as EE")
    languages = data.get("languages", ["en"])
    if not languages or not all(re.fullmatch(r"[a-z]{2}", code) for code in languages):
        raise ConfigError("languages must be a list of two-letter codes such as [\"et\", \"en\"]")

    library = []
    for i, item in enumerate(data.get("library", [])):
        name = _require(item, "name", f"[[library]] #{i + 1}")
        library.append(ZimSpec(name=name, flavour=item.get("flavour"), url=item.get("url")))

    return Region(
        id=region_id,
        country_code=country,
        languages=tuple(languages),
        name=_labels(data.get("name", f"Varu – {country}"), "name"),
        map=_parse_map(data["map"]) if "map" in data else None,
        pois=_parse_pois(data.get("pois", [])),
        library=tuple(library),
        documents=_parse_documents(data.get("documents", [])),
        attribution=tuple(data.get("attribution", [])),
    )


def load_region(ref: str, repo: Path) -> Region:
    """Accepts a region id ("ee") or a path to a .toml file."""
    path = Path(ref)
    if not path.suffix:
        path = repo / "regions" / f"{ref}.toml"
    if not path.is_file():
        known = ", ".join(sorted(p.stem for p in (repo / "regions").glob("*.toml"))) or "none"
        raise ConfigError(f"region file not found: {path} (available: {known})")
    with path.open("rb") as handle:
        try:
            data = tomllib.load(handle)
        except tomllib.TOMLDecodeError as err:
            raise ConfigError(f"{path}: {err}") from err
    return parse_region(data)


def load_sources(repo: Path) -> dict:
    path = repo / "sources.toml"
    with path.open("rb") as handle:
        sources = tomllib.load(handle)
    for section in ("npm", "assets", "protomaps", "overpass", "kiwix_catalog", "kiwix_tools", "launcher"):
        if section not in sources:
            raise ConfigError(f"sources.toml: missing [{section}]")
    return sources
