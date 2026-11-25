"""The `places` step: maps/<id>.places, what the map's search box finds.

Built from the Estonian address register (ADS) of the Land and Spatial
Development Board: open data with no licence agreement, credited to the
Board. Its monthly extract 17 holds every valid address with a postcode
and a point in L-EST97. Streets, settlements and municipalities are
derived from the addresses.

The file is gzip-compressed UTF-8 text, one record per line, fields split
by tabs (the launcher reads it, launcher/places.go):

  varu-places  1
  c  <context>                         areas a name lies in; ids by order
  <kind>  <context id>  <lon>  <lat>  <addresses>  <name>
      kind: p area, s street, a address; addresses: how many it has (1 for
      an address), so a city outranks a far smaller village of its name
"""

from __future__ import annotations

import csv
import gzip
import io
import json
import math
import re
import urllib.parse
import zipfile
from pathlib import Path

from .context import Context, StepError
from .fetch import FetchError, download, fetch_bytes

FORMAT = 1
EXTRACT = 17  # "Kehtivad aadressid koos sihtnumbriga": valid addresses, with points
# "Kooli tn 2a-5" is flat 5 of Kooli tn 2a: the building is enough to find it.
_FLAT = re.compile(r"\d[^\s-]*-[^\s-]+$")
# "Rüütli tn 12" is number 12 on the street "Rüütli tn".
_NUMBERED = re.compile(r"^(.+?) (\d[^\s]*)$")


def lest97_to_wgs84(east: float, north: float) -> tuple[float, float]:
    """L-EST97 (EPSG:3301, Lambert conformal conic on GRS80) to lon, lat."""
    a, f = 6378137.0, 1 / 298.257222101
    e = math.sqrt(f * (2 - f))
    lat1, lat2, lat0, lon0 = (math.radians(d) for d in (59 + 1 / 3, 58.0, 57.51755393055556, 24.0))

    def m(phi):
        return math.cos(phi) / math.sqrt(1 - (e * math.sin(phi)) ** 2)

    def t(phi):
        s = e * math.sin(phi)
        return math.tan(math.pi / 4 - phi / 2) / ((1 - s) / (1 + s)) ** (e / 2)

    n = (math.log(m(lat1)) - math.log(m(lat2))) / (math.log(t(lat1)) - math.log(t(lat2)))
    big_f = m(lat1) / (n * t(lat1) ** n)
    r0 = a * big_f * t(lat0) ** n
    x, y = east - 500000, r0 - (north - 6375000)
    r = math.copysign(math.hypot(x, y), n)
    tt = (r / (a * big_f)) ** (1 / n)
    phi = math.pi / 2 - 2 * math.atan(tt)
    for _ in range(8):
        s = e * math.sin(phi)
        phi = math.pi / 2 - 2 * math.atan(tt * ((1 - s) / (1 + s)) ** (e / 2))
    return math.degrees(math.atan2(x, y) / n) + 24.0, math.degrees(phi)


def build_places(rows) -> tuple[list[str], list[tuple]]:
    """Rows of extract 17 (dicts) to (contexts, records).

    A record is (kind, context id, lon, lat, addresses, name). An area or a
    street is placed on its address nearest the middle of all its
    addresses, so it never lands in a lake the way a plain average can.
    """
    contexts: dict[str, int] = {}
    addresses = []  # (lon, lat, name, ctx, groups)
    groups: dict[tuple, list] = {}  # (kind, name, ctx) -> [sum lon, sum lat, count, best, best distance]

    def ctx_id(parts):
        return contexts.setdefault(", ".join(reversed(parts)), len(contexts))

    for row in rows:
        name = row["lahiaadress"].strip()
        if not name or _FLAT.search(name):
            continue
        try:
            x, y = float(row["viitepunkt_x"].replace(",", ".")), float(row["viitepunkt_y"].replace(",", "."))
        except ValueError:
            continue
        lon, lat = lest97_to_wgs84(min(x, y), max(x, y))  # the 7-digit one is the northing
        # The full address ends with the short one, which may hold commas itself.
        full = row["taisaadress"].strip()
        within = full[:-len(name) - 2] if full.endswith(", " + name) else full.rpartition(", ")[0]
        areas = []
        for part in within.split(", "):
            if part and (not areas or areas[-1] != part):  # "Tartu linn, Tartu linn"
                areas.append(part)
        ctx = ctx_id(areas)
        # Municipalities and settlements; a county is too big to fly to.
        keys = [("p", areas[i], ctx_id(areas[:i])) for i in range(1, len(areas))]
        numbered = _NUMBERED.match(name)
        if numbered:
            keys.append(("s", numbered.group(1), ctx))
        for key in keys:
            g = groups.setdefault(key, [0.0, 0.0, 0, None, math.inf])
            g[0] += lon
            g[1] += lat
            g[2] += 1
        addresses.append((lon, lat, name, ctx, keys))

    for lon, lat, _, _, keys in addresses:
        for key in keys:
            g = groups[key]
            d = (lon - g[0] / g[2]) ** 2 + (lat - g[1] / g[2]) ** 2
            if d < g[4]:
                g[3], g[4] = (lon, lat), d

    records = [(kind, ctx, *g[3], g[2], name) for (kind, name, ctx), g in groups.items()]
    records.sort(key=lambda r: r[0] != "p")  # areas, then streets
    records += [("a", ctx, lon, lat, 1, name) for lon, lat, name, ctx, _ in addresses]
    return list(contexts), records


def write_places(contexts: list[str], records: list[tuple], path: Path) -> None:
    with gzip.open(path, "wt", encoding="utf-8", newline="\n") as out:
        out.write(f"varu-places\t{FORMAT}\n")
        for c in contexts:
            out.write(f"c\t{c}\n")
        for kind, ctx, lon, lat, count, name in records:
            out.write(f"{kind}\t{ctx}\t{lon:.5f}\t{lat:.5f}\t{count}\t{name}\n")


def read_extract(path: Path):
    with zipfile.ZipFile(path) as z:
        member = next(n for n in z.namelist() if n.lower().endswith(".csv"))
        with z.open(member) as raw:
            yield from csv.DictReader(io.TextIOWrapper(raw, encoding="utf-8-sig"), delimiter=";")


def step_places(ctx: Context, *, refresh: bool = False) -> None:
    listing = ctx.region.places_ads
    if listing is None:
        ctx.log("region has no [places]; skipping")
        return
    target = ctx.out / "maps" / f"{ctx.region.id}.places"
    if target.exists() and not refresh and not ctx.dry_run:
        ctx.log(f"have {target.name}; use --refresh to build a newer one")
        return
    if ctx.dry_run:
        ctx.log(f"would download address extract {EXTRACT} from {listing} and build {target.name}")
        return
    try:
        # The list answers HTTP 500 to a request that names no language.
        extracts = json.loads(fetch_bytes(listing, headers={"Accept-Language": "et"}))
        latest = next(x for x in extracts if x.get("vvnr") == EXTRACT and not x.get("kov"))
        name = latest["fail"]
        if not re.fullmatch(r"[A-Za-z0-9._-]+\.zip", name):
            raise StepError(f"unexpected extract name {name!r}")
        cache = ctx.cache / "ads"
        extract = download(f"{listing}/{urllib.parse.quote(name)}", cache / name, log=ctx.log)
    except (FetchError, ValueError, StopIteration, KeyError) as err:
        raise StepError(f"the address register's extract list at {listing}: {err!r}") from err
    for old in cache.glob(f"vv{EXTRACT}_*.zip"):
        if old != extract:
            old.unlink()

    contexts, records = build_places(read_extract(extract))
    partial = target.with_name(f"{ctx.region.id}.partial.places")
    target.parent.mkdir(parents=True, exist_ok=True)
    write_places(contexts, records, partial)
    partial.replace(target)
    ctx.update_info(places_source=name)
    ctx.log(f"wrote {target.name}: {len(records):,} places and addresses ({target.stat().st_size / 1e6:.0f} MB)")
