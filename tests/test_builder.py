"""Builder tests. Standard library only: python3 -m unittest discover -s tests"""

from __future__ import annotations

import base64
import contextlib
import gzip
import hashlib
import io
import json
import sys
import tarfile
import tempfile
import threading
import unittest
import zipfile
from http.server import BaseHTTPRequestHandler, ThreadingHTTPServer
from pathlib import Path

REPO = Path(__file__).resolve().parent.parent
sys.path.insert(0, str(REPO))

from builder import cli  # noqa: E402
from builder.app import extract_members  # noqa: E402
from builder.config import ConfigError, ZimSpec, load_region, load_sources, parse_region  # noqa: E402
from builder.context import Context, StepError  # noqa: E402
from builder.data import (  # noqa: E402
    choose_zim, official_to_geojson, older_editions, overpass_query, overpass_to_geojson, parse_catalog,
    remove_older_editions,
)
from builder.fetch import FetchError, download, sri_matches  # noqa: E402
from builder.manifest import build_manifest  # noqa: E402
from builder.places import build_places, lest97_to_wgs84, write_places  # noqa: E402
from builder.release import package  # noqa: E402


def sri(data: bytes) -> str:
    return "sha512-" + base64.b64encode(hashlib.sha512(data).digest()).decode()


class ConfigTests(unittest.TestCase):
    def test_shipped_files_load(self):
        region = load_region("ee", REPO)
        self.assertEqual(region.id, "ee")
        self.assertEqual(region.country_code, "EE")
        self.assertEqual(region.languages[0], "et")
        self.assertGreater(len(region.pois), 3)
        west, south, east, north = region.map.bbox
        # Tallinn and Tartu must be inside the map.
        for lon, lat in ((24.75, 59.44), (26.72, 58.38)):
            self.assertTrue(west < lon < east and south < lat < north)
        sources = load_sources(REPO)
        self.assertEqual({p["name"] for p in sources["npm"]}, {"maplibre-gl", "pmtiles", "@protomaps/basemaps"})
        for package in sources["npm"]:
            self.assertTrue(package["integrity"].startswith("sha512-"))
            self.assertIn(package["version"], package["url"])

    def test_every_label_has_every_interface_language(self):
        region = load_region("ee", REPO)
        for poi in region.pois:
            for code in region.languages:
                self.assertIn(code, poi.label, f"{poi.id} has no '{code}' label")

    def test_rejects_bad_input(self):
        good = {"id": "xx", "country_code": "XX", "map": {"bbox": [1, 2, 3, 4]}}
        parse_region(good)
        bad_cases = [
            {**good, "id": "Bad Id"},
            {**good, "country_code": "est"},
            {**good, "map": {"bbox": [3, 2, 1, 4]}},
            {**good, "map": {"bbox": [1, 2, 3]}},
            {**good, "map": {"bbox": [1, 2, 3, 4], "maxzoom": 22}},
            {**good, "pois": [{"id": "a", "tags": ['amenity"](evil']}]},
            {**good, "pois": [{"id": "a", "tags": ["amenity=x"]}, {"id": "a", "tags": ["amenity=y"]}]},
            {**good, "documents": [{"url": "https://example.org/a.pdf", "file": "../a.pdf"}]},
            {**good, "documents": [{"url": "file:///etc/passwd", "file": "a.pdf"}]},
            {**good, "routing": {"osm_extract": "http://example.org/xx.osm.pbf"}},
            {**good, "routing": {}},
        ]
        for case in bad_cases:
            with self.assertRaises(ConfigError, msg=str(case)):
                parse_region(case)

    def test_unknown_region_lists_the_known_ones(self):
        with self.assertRaises(ConfigError) as caught:
            load_region("zz", REPO)
        self.assertIn("ee", str(caught.exception))

    def test_attribution_under_a_table_is_refused(self):
        data = {"id": "x", "country_code": "EE", "pois": [{"id": "a", "tags": ["a=b"], "attribution": ["c"]}]}
        with self.assertRaises(ConfigError):
            parse_region(data)

    def test_shipped_regions_have_attribution(self):
        for name in ("ee", "ee-sample"):
            self.assertTrue(load_region(name, REPO).attribution, name)


class OverpassTests(unittest.TestCase):
    def test_query(self):
        query = overpass_query("EE", ("amenity=hospital", "healthcare=hospital"))
        self.assertIn('area["ISO3166-1"="EE"][admin_level=2]->.region;', query)
        self.assertIn('nwr["amenity"="hospital"](area.region);', query)
        self.assertIn('nwr["healthcare"="hospital"](area.region);', query)
        self.assertIn("out center tags;", query)

    def test_official_list(self):
        poi = parse_region({"id": "x", "country_code": "EE", "pois": [{
            "id": "shelter", "geojson": "https://example.org/x.json",
            "properties": {"name": "nimi", "addr:street": "aadress"}}]}).pois[0]
        payload = {"features": [
            {"id": "v.1", "geometry": {"type": "Point", "coordinates": [24.7536123, 59.4369876]},
             "properties": {"nimi": "Varjend", "aadress": "Tallinn", "x": 1}},
            {"id": "v.2", "geometry": {"type": "Point", "coordinates": [10.0, 10.0]}, "properties": {"nimi": "far"}},
            {"id": "v.3", "geometry": None, "properties": {}},
        ]}
        out = official_to_geojson(payload, poi, (21.3, 57.45, 28.3, 59.9))
        self.assertEqual(out["features"], [{
            "type": "Feature", "id": "shelter/v.1",
            "geometry": {"type": "Point", "coordinates": [24.753612, 59.436988]},
            "properties": {"name": "Varjend", "addr:street": "Tallinn"}}])

    def test_query_clipped_to_map(self):
        query = overpass_query("EE", ("amenity=pharmacy",), (22.42, 58.22, 22.56, 58.29))
        self.assertIn('nwr["amenity"="pharmacy"](area.region)(58.22,22.42,58.29,22.56);', query)

    def test_conversion(self):
        payload = {"elements": [
            {"type": "way", "id": 7, "center": {"lat": 58.3712345678, "lon": 26.7151234567},
             "tags": {"name": "Tartu Ülikooli Kliinikum", "name:en": "Tartu University Hospital",
                      "amenity": "hospital", "website": "https://example.org", "phone": "+372 731 8111"}},
            {"type": "node", "id": 3, "lat": 59.4, "lon": 24.7, "tags": {"amenity": "hospital"}},
            {"type": "relation", "id": 9, "tags": {"name": "no geometry"}},
        ]}
        collection = overpass_to_geojson(payload, ("et", "en"))
        self.assertEqual([f["id"] for f in collection["features"]], ["node/3", "way/7"])
        hospital = collection["features"][1]
        self.assertEqual(hospital["geometry"], {"type": "Point", "coordinates": [26.715123, 58.371235]})
        self.assertEqual(hospital["properties"], {
            "name": "Tartu Ülikooli Kliinikum", "phone": "+372 731 8111", "name:en": "Tartu University Hospital"})
        json.dumps(collection)


CATALOG = b"""<?xml version="1.0" encoding="UTF-8"?>
<feed xmlns="http://www.w3.org/2005/Atom" xmlns:dc="http://purl.org/dc/terms/" xmlns:opds="https://specs.opds.io/opds-1.2">
  <title>Filtered Entries</title>
  <entry>
    <id>urn:uuid:1</id><title>Vikipeedia</title><updated>2026-03-02T00:00:00Z</updated>
    <language>est</language><name>wikipedia_et_all</name><flavour>maxi</flavour>
    <link rel="http://opds-spec.org/image/thumbnail" href="/catalog/v2/illustration/1/" type="image/png;width=48;height=48;scale=1"/>
    <link type="text/html" href="/content/wikipedia_et_all_maxi_2026-03"/>
    <dc:issued>2026-03-02T00:00:00Z</dc:issued>
    <link rel="http://opds-spec.org/acquisition/open-access" type="application/x-zim"
          href="https://download.kiwix.org/zim/wikipedia/wikipedia_et_all_maxi_2026-03.zim.meta4" length="2147483648"/>
  </entry>
  <entry>
    <id>urn:uuid:2</id><title>Vikipeedia</title><updated>2026-09-05T00:00:00Z</updated>
    <language>est</language><name>wikipedia_et_all</name><flavour>maxi</flavour>
    <dc:issued>2026-09-05T00:00:00Z</dc:issued>
    <link rel="http://opds-spec.org/acquisition/open-access" type="application/x-zim"
          href="https://download.kiwix.org/zim/wikipedia/wikipedia_et_all_maxi_2026-09.zim.meta4" length="2254857830"/>
  </entry>
  <entry>
    <id>urn:uuid:3</id><title>Vikipeedia</title><updated>2026-10-01T00:00:00Z</updated>
    <language>est</language><name>wikipedia_et_all</name><flavour>nopic</flavour>
    <dc:issued>2026-10-01T00:00:00Z</dc:issued>
    <link rel="http://opds-spec.org/acquisition/open-access" type="application/x-zim"
          href="https://download.kiwix.org/zim/wikipedia/wikipedia_et_all_nopic_2026-10.zim.meta4" length="536870912"/>
  </entry>
</feed>
"""


class CatalogTests(unittest.TestCase):
    """The fixture mirrors the catalogue's documented shape; it has not been
    compared with a live response, so treat a parsing surprise as likely."""

    def test_parse_and_choose(self):
        entries = parse_catalog(CATALOG)
        self.assertEqual(len(entries), 3)
        chosen = choose_zim(entries, ZimSpec("wikipedia_et_all", "maxi"))
        self.assertEqual(chosen["url"], "https://download.kiwix.org/zim/wikipedia/wikipedia_et_all_maxi_2026-09.zim")
        self.assertEqual(chosen["size"], 2254857830)
        self.assertEqual(chosen["title"], "Vikipeedia")
        self.assertEqual(chosen["language"], "est")

    def test_flavour_optional_and_missing(self):
        entries = parse_catalog(CATALOG)
        newest = choose_zim(entries, ZimSpec("wikipedia_et_all"))
        self.assertTrue(newest["url"].endswith("nopic_2026-10.zim"))
        self.assertIsNone(choose_zim(entries, ZimSpec("wikipedia_et_all", "mini")))
        self.assertIsNone(choose_zim(entries, ZimSpec("wikipedia_xx_all")))

    def test_older_editions(self):
        with tempfile.TemporaryDirectory() as tmp:
            library = Path(tmp)
            for name in ("wikipedia_et_all_maxi_2026-03.zim", "wikipedia_et_all_maxi_2026-09.zim",
                         "wikipedia_et_all_nopic_2026-03.zim", "wikipedia_et_all_maxi_extra_2026-03.zim"):
                (library / name).touch()
            stale = older_editions(library, "wikipedia_et_all_maxi_2026-09.zim")
            self.assertEqual([p.name for p in stale], ["wikipedia_et_all_maxi_2026-03.zim"])
            self.assertEqual(older_editions(library, "undated.zim"), [])

    def test_remove_older_editions(self):
        with tempfile.TemporaryDirectory() as tmp:
            library = Path(tmp)
            keep = ("wikipedia_et_all_maxi_2026-09.zim", "wikipedia_et_all_maxi_2026-09.search",
                    "wikipedia_et_all_maxi_extra_2026-03.zim")
            keep += ("wikipedia_et_all_maxi_2026-09.zim.part", "wikipedia_et_all_maxi_2026-09.search.part")
            for name in ("wikipedia_et_all_maxi_2026-03.zim", "wikipedia_et_all_maxi_2026-03.search",
                         "wikipedia_et_all_maxi_2026-05.zim.part", "wikipedia_et_all_maxi_2026-03.search.part",
                         *keep):
                (library / name).touch()
            removed = remove_older_editions(library, "wikipedia_et_all_maxi_2026-09.zim")
            self.assertEqual(removed, ["wikipedia_et_all_maxi_2026-03.zim"])
            self.assertEqual(sorted(p.name for p in library.iterdir()), sorted(keep))


class RangeHandler(BaseHTTPRequestHandler):
    """Serves one payload, honours Range, and can cut the first answer short."""

    payload = b""
    cut_first_at: int | None = None
    requests: list[str | None] = []

    def do_GET(self):
        cls = type(self)
        header = self.headers.get("Range")
        cls.requests.append(header)
        start = int(header.split("=")[1].rstrip("-")) if header else 0
        body = cls.payload[start:]
        self.send_response(206 if header else 200)
        if header:
            self.send_header("Content-Range", f"bytes {start}-{len(cls.payload) - 1}/{len(cls.payload)}")
        self.send_header("Content-Length", str(len(body)))
        self.end_headers()
        if cls.cut_first_at is not None:
            body, cls.cut_first_at = body[:cls.cut_first_at], None
        self.wfile.write(body)

    def log_message(self, *args):
        pass


class DownloadTests(unittest.TestCase):
    def setUp(self):
        RangeHandler.payload = bytes(range(256)) * 400
        RangeHandler.cut_first_at = None
        RangeHandler.requests = []
        self.server = ThreadingHTTPServer(("127.0.0.1", 0), RangeHandler)
        threading.Thread(target=self.server.serve_forever, daemon=True).start()
        self.url = f"http://127.0.0.1:{self.server.server_address[1]}/file.zim"
        self.tmp = tempfile.TemporaryDirectory()
        self.dest = Path(self.tmp.name) / "file.zim"

    def tearDown(self):
        self.server.shutdown()
        self.server.server_close()
        self.tmp.cleanup()

    def test_download_and_skip(self):
        download(self.url, self.dest, integrity=sri(RangeHandler.payload), log=lambda m: None)
        self.assertEqual(self.dest.read_bytes(), RangeHandler.payload)
        download(self.url, self.dest, integrity=sri(RangeHandler.payload), log=lambda m: None)
        self.assertEqual(len(RangeHandler.requests), 1, "a finished file must not be fetched again")

    def test_resume_after_interruption(self):
        RangeHandler.cut_first_at = 30_000
        with self.assertRaises(FetchError):
            download(self.url, self.dest, log=lambda m: None)
        self.assertFalse(self.dest.exists(), "an incomplete file must never get the final name")
        self.assertEqual(self.dest.with_name("file.zim.part").stat().st_size, 30_000)

        download(self.url, self.dest, integrity=sri(RangeHandler.payload), log=lambda m: None)
        self.assertEqual(RangeHandler.requests, [None, "bytes=30000-"])
        self.assertEqual(self.dest.read_bytes(), RangeHandler.payload)

    def test_checksum_mismatch_is_refused(self):
        with self.assertRaises(FetchError):
            download(self.url, self.dest, integrity=sri(b"something else"), log=lambda m: None)
        self.assertFalse(self.dest.exists())
        self.assertFalse(self.dest.with_name("file.zim.part").exists())

    def test_sri(self):
        self.dest.write_bytes(b"abc")
        self.assertTrue(sri_matches(self.dest, sri(b"abc")))
        self.assertFalse(sri_matches(self.dest, sri(b"abd")))
        with self.assertRaises(FetchError):
            sri_matches(self.dest, "md5-xxxx")


def make_tar(path: Path, files: dict[str, bytes]) -> None:
    with tarfile.open(path, "w:gz") as tar:
        for name, data in files.items():
            info = tarfile.TarInfo(name)
            info.size = len(data)
            tar.addfile(info, io.BytesIO(data))


class ArchiveTests(unittest.TestCase):
    def test_extract_members_only_writes_where_told(self):
        with tempfile.TemporaryDirectory() as tmp:
            tmp = Path(tmp)
            make_tar(tmp / "pkg.tgz", {
                "package/dist/lib.mjs": b"export {}",
                "package/../../escape.txt": b"nope",
                "package/README.md": b"readme",
            })
            extract_members(tmp / "pkg.tgz", {"package/dist/lib.mjs": tmp / "vendor" / "lib.mjs"})
            self.assertEqual((tmp / "vendor" / "lib.mjs").read_bytes(), b"export {}")
            self.assertEqual(sorted(p.name for p in tmp.iterdir()), ["pkg.tgz", "vendor"])
            with self.assertRaises(StepError):
                extract_members(tmp / "pkg.tgz", {"package/dist/missing.js": tmp / "x.js"})

    def test_lists_only_what_exists(self):
        region = load_region("ee", REPO)
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp)
            ctx = Context(repo=REPO, out=out, cache=out / "cache", region=region, sources={})
            empty = build_manifest(ctx)
            self.assertNotIn("map", empty)
            self.assertEqual((empty["pois"], empty["library"], empty["documents"]), ([], [], []))

            (out / "maps").mkdir()
            (out / "maps" / "ee.pmtiles").write_bytes(b"x" * 10)
            (out / "maps" / "ee.graph").write_bytes(b"g" * 7)
            (out / "maps" / "ee.places").write_bytes(b"p" * 3)
            (out / "pois").mkdir()
            (out / "pois" / "hospital.geojson").write_text(
                json.dumps({"type": "FeatureCollection", "features": [{}, {}]}), encoding="utf-8")
            (out / "pois" / "pharmacy.geojson").write_text("not json", encoding="utf-8")
            (out / "library").mkdir()
            (out / "library" / "wikipedia_et_all_maxi_2026-09.zim").write_bytes(b"z" * 5)
            (out / "library" / "._wikipedia_et_all_maxi_2026-09.zim").write_bytes(b"junk")
            ctx.update_info(map_build="20261120", routing_source="https://example.org/ee-261122.osm.pbf", library={
                "wikipedia_et_all_maxi_2026-09.zim": {"name": "wikipedia_et_all", "title": "Vikipeedia", "language": "est"}})

            with contextlib.redirect_stdout(io.StringIO()):
                manifest = build_manifest(ctx)
            self.assertEqual(manifest["map"]["file"], "maps/ee.pmtiles")
            self.assertEqual(manifest["map"]["build"], "20261120")
            self.assertEqual(manifest["routing"], {"file": "maps/ee.graph", "size": 7,
                                                   "source": "https://example.org/ee-261122.osm.pbf"})
            self.assertEqual(manifest["places"], {"file": "maps/ee.places", "size": 3})
            self.assertEqual([p["id"] for p in manifest["pois"]], ["hospital"])
            self.assertEqual(manifest["pois"][0]["count"], 2)
            self.assertEqual(manifest["pois"][0]["label"]["et"], "Haiglad")
            self.assertEqual(len(manifest["library"]), 1)
            self.assertEqual(manifest["library"][0]["title"], "Vikipeedia")
            json.dumps(manifest)


class PlacesTests(unittest.TestCase):
    def test_lest97(self):
        # A public shelter as the Land Board's map service gives it in both systems.
        lon, lat = lest97_to_wgs84(503206.64, 6579397.51)
        self.assertAlmostEqual(lon, 24.05636882, places=7)
        self.assertAlmostEqual(lat, 59.35262298, places=7)

    def test_build(self):
        def row(short, areas, east, north):
            # Extract 17 gives the northing as x, as geodesists do.
            return {"lahiaadress": short, "taisaadress": ", ".join(areas + [short]),
                    "viitepunkt_x": f"{north}", "viitepunkt_y": f"{east}"}

        tartu = ["Tartu maakond", "Tartu linn", "Tartu linn"]
        rows = [
            row("Rüütli tn 1", tartu, 659000, 6474000),
            row("Rüütli tn 3", tartu, 659100, 6474000),
            row("Rüütli tn 5", tartu, 659900, 6474000),
            row("Rüütli tn 3-7", tartu, 659100, 6474000),  # a flat
            row("Mesika", ["Tartu maakond", "Luunja vald", "Kabina küla"], 670000, 6475000),
            # A short address with commas of its own must not become an area.
            row("Tartu maakond, Elva linn, Tartu-Valga 449,8-451,8 km", ["Tartu maakond", "Elva vald", "Elva linn"],
                643658, 6457734),
        ]
        contexts, records = build_places(rows)
        names = [(kind, name, contexts[ctx], count) for kind, ctx, _, _, count, name in records]
        self.assertEqual(names, [
            ("p", "Tartu linn", "Tartu maakond", 3),
            ("p", "Luunja vald", "Tartu maakond", 1), ("p", "Kabina küla", "Luunja vald, Tartu maakond", 1),
            ("p", "Elva vald", "Tartu maakond", 1), ("p", "Elva linn", "Elva vald, Tartu maakond", 1),
            ("s", "Rüütli tn", "Tartu linn, Tartu maakond", 3),
            ("a", "Rüütli tn 1", "Tartu linn, Tartu maakond", 1), ("a", "Rüütli tn 3", "Tartu linn, Tartu maakond", 1),
            ("a", "Rüütli tn 5", "Tartu linn, Tartu maakond", 1),
            ("a", "Mesika", "Kabina küla, Luunja vald, Tartu maakond", 1),
            ("a", "Tartu maakond, Elva linn, Tartu-Valga 449,8-451,8 km", "Elva linn, Elva vald, Tartu maakond", 1),
        ])
        street = records[5]
        self.assertEqual(street[2:4], lest97_to_wgs84(659100, 6474000))  # the address nearest its middle

        with tempfile.TemporaryDirectory() as tmp:
            path = Path(tmp, "ee.places")
            write_places(contexts, records, path)
            lines = gzip.decompress(path.read_bytes()).decode().splitlines()
            self.assertEqual(lines[0], "varu-places\t1")
            self.assertEqual(lines[1 + len(contexts)].split("\t")[0::4], ["p", "3"])
            self.assertEqual(lines[-1].split("\t")[-1], "Tartu maakond, Elva linn, Tartu-Valga 449,8-451,8 km")


class CliTests(unittest.TestCase):
    def run_cli(self, *argv: str) -> tuple[int, str]:
        buffer = io.StringIO()
        with contextlib.redirect_stdout(buffer), contextlib.redirect_stderr(buffer):
            code = cli.main(list(argv))
        return code, buffer.getvalue()

    def test_dry_run_touches_nothing(self):
        with tempfile.TemporaryDirectory() as tmp:
            out = Path(tmp) / "drive"
            code, text = self.run_cli("--region", "ee", "--out", str(out), "--dry-run",
                                      "--cache", str(Path(tmp) / "cache"), "--planet-build", "20261120")
            self.assertEqual(code, 0, text)
            self.assertFalse(out.exists())
            self.assertIn("pmtiles extract https://build.protomaps.com/20261120.pmtiles", text)
            self.assertIn("--bbox=21.3,57.45,28.3,59.9 --maxzoom=15", text)
            self.assertIn("wikipedia_et_all", text)
            self.assertIn("would download https://download.geofabrik.de/europe/estonia-latest.osm.pbf", text)

    def test_only_and_skip(self):
        with tempfile.TemporaryDirectory() as tmp:
            code, text = self.run_cli("--region", "ee", "--out", tmp, "--dry-run", "--only", "maps,pois", "--skip", "pois")
            self.assertEqual(code, 0, text)
            self.assertIn("[maps]", text)
            self.assertNotIn("[pois]", text)
            self.assertNotIn("[app]", text)

    def test_unknown_region_is_a_clean_error(self):
        with tempfile.TemporaryDirectory() as tmp:
            code, text = self.run_cli("--region", "zz", "--out", tmp)
            self.assertEqual(code, 2)
            self.assertIn("region file not found", text)


class ReleaseTests(unittest.TestCase):
    def test_package(self):
        region = load_region("ee-sample", REPO)
        with tempfile.TemporaryDirectory() as tmp:
            drive, out = Path(tmp, "drive"), Path(tmp, "dist")
            for rel, body in {
                "manifest.json": "{}", "app/index.html": "x", "bin/darwin-arm64/varu": "#!",
                "maps/ee-sample.pmtiles": "tiles", "maps/ee-sample.graph": "roads",
                "maps/ee-sample.places": "addresses", "library/a.zim": "z", "build-info.json": "{}", "._junk": "",
            }.items():
                Path(drive, rel).parent.mkdir(parents=True, exist_ok=True)
                Path(drive, rel).write_text(body)
            Path(drive, "bin/darwin-arm64/varu").chmod(0o755)
            out.mkdir()
            Path(out, "Varu-macOS.dmg").write_text("app")  # from desktop/build.sh

            package(region, drive, out, catalog="unused", log=lambda _: None)
            package(region, drive, out, catalog="unused", log=lambda _: None)  # re-run replaces, not appends

            with zipfile.ZipFile(out / "varu-ee-sample.zip") as bundle:
                self.assertEqual(sorted(bundle.namelist()),
                                 ["app/index.html", "bin/darwin-arm64/varu", "manifest.json"])
                self.assertEqual(bundle.getinfo("bin/darwin-arm64/varu").external_attr >> 16 & 0o777, 0o755)
            index = json.loads((out / "regions.json").read_text())
            self.assertEqual([r["id"] for r in index["regions"]], ["ee-sample"])
            entry = index["regions"][0]
            self.assertEqual(entry["map"]["path"], "maps/ee-sample.pmtiles")
            self.assertEqual(entry["map"]["sha256"], hashlib.sha256(b"tiles").hexdigest())
            self.assertEqual(entry["routing"]["path"], "maps/ee-sample.graph")
            self.assertEqual(entry["places"]["path"], "maps/ee-sample.places")
            self.assertEqual(entry["library"], [])
            self.assertEqual([(d["asset"], d["path"]) for d in index["desktop"]], [("Varu-macOS.dmg", "Varu-macOS.dmg")])


if __name__ == "__main__":
    unittest.main()
