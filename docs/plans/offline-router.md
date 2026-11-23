# Plan: Varu's own offline router

Status: approved 2026-11-23 with the recommended option for every decision, plus the
safety note (see the open question in section 1: include it). Requested by the user:
"in the case of a blackout where there is no wifi or info then we need to use our own map
data for the routing" · "We need our own router, do not use 3rd party ones" · the Pi must
also work as a desktop with keyboard, mouse and HDMI screen.

Hard rules: no third-party apps or online services at runtime (downloading open data at
release time is fine); Go and Python standard library only; releases built locally with
`release.sh` (no GitHub Actions triggers); commits carry no Claude attribution.

This is a big change: 4 Go modules or packages, the Python builder, the installer, the
viewer and the docs; about 1,100 lines of Go, 600 lines of Go tests and 350 lines of
JS/CSS. Stop at **Checkpoint A** (router core measured on real data), **B** (build,
release and install) and **C** (launcher and viewer).

## 1. Decisions (recommended option first)

1. **Where the graph is built.** **A:** Go tool `router/cmd/varu-graph`, run by the
   builder with `go run` (Go is already required; ~15M nodes would take minutes and over
   1 GB of RAM in pure Python; writer and reader share one format definition).
   B: Python stdlib (slow, format in two languages). C: on device or at install
   (impossible on a Pi Zero; phones would need the 125 MB extract).
2. **Module layout.** **A:** new repo-root module `router/`
   (`github.com/renereose/varu/router`, go 1.22, stdlib only); `launcher/go.mod` gets
   `require … v0.0.0-00010101000000-000000000000` + `replace … => ../router`. Same
   pattern as the `pmtiles/` module in the Pi plan; the device program imports it the
   same way; existing `go build` calls keep working. B: `launcher/router` subpackage.
   C: `go.work`.
3. **Sharing the protobuf reader with the device plan.** **A:** one unexported
   wire-format reader in `router/pbf.go` (~50 lines); whichever plan lands second moves
   it to an exported `router/wire` subpackage. B: export `router/wire` now.
   C: separate `protowire/` module. D: two copies (rejected).
4. **Algorithm.** **A:** unidirectional A* with a reused binary heap (lazy deletion),
   admissible straight-line heuristic `0.99 × distM / vmax`, and per-mode connected
   components so "not connected" is instant (~80 lines). B: bidirectional A*
   (contingency). C: contraction hierarchies (not justified at Estonia's size).
   D: plain Dijkstra (~3–6 s on a Pi Zero 2 W for Tallinn→Tartu).
5. **Graph detail and file size.** **A:** junctions and way endpoints are vertices;
   intermediate OSM nodes are shape points; no compression; measure at step 7 and apply
   7a (Douglas-Peucker at 2 m) only if the file exceeds 60 MB. B: simplify now.
   C: gzip/delta. D: every OSM node a vertex.
6. **Ferries.** **A:** include `route=ferry` for both modes; speed from `duration`
   (`HH:MM[:SS]`) or 20 km/h; fixed 20-minute boarding cost; return `ferry: true` and
   show "Includes a ferry. Check whether ferries are running." Without ferries
   Saaremaa, Hiiumaa, Muhu, Vormsi and Kihnu are unreachable. B: exclude. C: no penalty.
7. **`highway=track` for cars.** **A:** allowed at 15 km/h unless access forbids
   (`access`/`motor_vehicle`/`motorcar` = no, private, agricultural, forestry).
   B: exclude (like OSRM). C: only `tracktype=grade1/2`.
8. **Where the Geofabrik URL lives.** **A:** region file:
   `[routing] osm_extract = "https://download.geofabrik.de/europe/estonia-latest.osm.pbf"`
   in `regions/ee.toml` (keeps Estonia-specific facts out of `sources.toml`).
   B: `sources.toml`. C: template in `sources.toml` + path in the region file.
9. **Choosing start and destination.** **A:** right-click or long-press opens a small
   popup with **Route here** and **Start here**; place popups get **Route here**. Start
   = explicit "Start here", else a GPS fix from the last 5 minutes, else manifest
   `home`, else a hint to pick a start. Works with mouse (Pi desktop), touch and
   keyboard. B: auto-start GPS (slow offline cold start, surprise permission prompt).
   C: a "pick on map" mode button.
10. **Sample region.** **A:** no `[routing]` in `ee-sample.toml` (sample stays a few
    MB). B: include (125 MB download). C: `-bbox` clipping (new feature).
11. **Snapping.** **A:** per-mode components at load; snap to the nearest mode-usable,
    non-ferry segment in a component of at least 500 junctions within 1 km, else any
    usable segment within 5 km, else "too far". B: nearest edge only.
    C: components at build time.
12. **32-bit Pi launcher target.** **A:** out of scope here (belongs to the Pi plan);
    `make test` cross-builds `router` for linux/arm GOARM=6. B: add now.
13. **How the launcher finds the graph.** **A:** `routing.file` from `manifest.json`
    with a `filepath.IsLocal` check. B: glob `maps/*.graph`.

Safety note (approved): the route panel always shows one line, "Estimate from
OpenStreetMap. Follow road signs." / "Hinnang OpenStreetMapi põhjal. Järgi liiklusmärke."

## 2. Estimates for Estonia (measured at step 7)

Assumptions: 124.8 MB PBF ≈ 14–16M nodes, 1.6–1.9M ways; ~25–30% of ways routable
(400–550k); junction vertices V ≈ 0.6–1.0M, segments S ≈ 0.8–1.3M, shape points
P ≈ 2.5–3.5M (central case V = 0.8M, S = 1.05M, P = 3M).

- **File size:** 8V + 18S + 4 + 8P ≈ ~50 MB (35–60 MB); ~37 MB with 7a.
- **RAM once loaded:** arrays ~50 MB, adjacency ~12 MB, snapping grid ~6 MB, component
  arrays ~6 MB, search state ~10 MB, heap 5–25 MB → ~85–110 MB steady, ~130 MB peak.
  Fits a Pi Zero 2 W running the device program; not comfortably a Pi Zero 2 W running
  Chromium as a desktop (recommend Pi 4/5 for desktop use).
- **Load time:** laptop 0.3–0.8 s (+~1.5 s from USB 2); Pi Zero 2 W ~3–6 s.
- **Queries:** Tallinn→Tartu by car 50–150 ms (laptop), 0.3–0.6 s (Pi 4), 1–3 s
  (Pi Zero 2 W); a walk inside a town <10 ms (laptop), <100 ms (Pi Zero); different
  component: instant.
- **Build on the Mac:** two passes over the PBF, ~15–30 s, peak ~300–500 MB.

## 3. Graph file format (`maps/<id>.graph`, little-endian, version 1)

```
magic    [8]byte "VARUGRPH"
version  uint32 = 1
V, S, P  uint32 ×3
metaLen  uint32, then meta JSON: {"source": <dated Geofabrik URL>, "osm_timestamp": <PBF replication time>, "built": <UTC>}
vlat, vlon  [V]int32      1e-7 degrees
sfrom, sto  [S]uint32     vertex indexes
slen        [S]float32    metres (from full geometry)
skmh        [S]uint8      vehicle/ferry speed km/h (0 = no cars)
sflags      [S]uint8      bit0 car fwd, bit1 car bwd, bit2 foot, bit3 ferry
sshape      [S+1]uint32   start index into shape arrays (intermediate points only)
plat, plon  [P]int32      1e-7 degrees
```

One method `(*Graph).arrays() []any` lists the arrays in on-disk order; `WriteTo` and
`Load` both iterate it. `Load` validates magic/version ("routing data format N, this
Varu reads 1"), exact file length, `sfrom`/`sto` < V, and monotonic `sshape` ending at
P; it reads with `bufio` + `binary.Read` into preallocated slices.

## 4. Algorithm details (`router/route.go`)

- **Distance:** `distM`, equirectangular with the cosine of the mean latitude; used for
  lengths, snapping and the heuristic (×0.99 keeps it admissible).
- **Snapping:** uniform grid ~0.01° lat × 0.02° lon (~1.1 km); segments registered in
  the cells their bounding box covers; ring search; projection onto polylines; returns
  segment, sub-segment index, projected point, metres along and offset. Never snaps to
  ferries.
- **Seeds:** source seeds `sto` with `(len − along)/v` if forward allowed and `sfrom`
  with `along/v` if backward allowed; target candidates when popping the target
  segment's endpoints; same-segment direct path as initial best; stop when min f ≥ best.
- **Costs:** car `len × inv[kmh]`; foot 5 km/h (ferries at their own speed); ferry
  segments +1,200 s; different components → `ErrNotConnected` before searching.
- **Path:** `parent[v]` = half-edge `(seg<<1 | reverse)`; geometry = snapped start,
  partial start segment, full segments (reversed where needed), partial end, snapped
  end; distance and duration summed along it.
- **Concurrency:** one query at a time behind `sync.Mutex` (`ponytail:` upgrade = pool
  of search states).

## 5. Build steps

### Layer 1: router core (`router/`), then Checkpoint A

1. `router/go.mod` (3 lines).
2. `router/pbf.go` (~200 lines): wire reader (`next`, `uvarint`, `bytes`, `skip`,
   `zigzag`, packed helpers); `pbfHandler{node, way}` with nil = skip; `readPBF(r,
   h) (osmTime, err)`. Blob framing (BlobHeader length, `raw`/`zlib_data`, size caps
   64 KB / 32 MB); HeaderBlock rejects required features outside {OsmSchema-V0.6,
   DenseNodes} and reads the replication timestamp (field 32); PrimitiveBlock string
   table, granularity/offsets, DenseNodes, Ways. Plain `Node` → "not supported" error.
   `ponytail:` skips: relations (no turn restrictions), node tags (no barriers),
   metadata, lzma/zstd blobs.
3. `router/build.go` (~230 lines): `Mode` (Foot, Car) + `ParseMode`; `wayRule(tags)`
   with default car speeds (motorway 100, trunk 85, primary 75, secondary 65, tertiary
   55, unclassified 45, residential 25, living_street 10, service 15, track 15, road 30,
   `*_link` 45); foot-only classes (footway, path, pedestrian, steps, cycleway,
   bridleway); excluded (construction, proposed, abandoned, platform, raceway, corridor,
   elevator, busway, via_ferrata); motorways closed to foot; `maxspeed` (integer ≤ 130,
   `EE:urban` 50, `EE:rural` 90); car oneway (`yes/true/1`, `-1`, `reversible` closed,
   roundabouts and motorways oneway); access hierarchy (car: motorcar > motor_vehicle >
   vehicle > access; foot: foot > access; denied: no, private, agricultural, forestry,
   military; allowed: yes, designated, permissive, destination, customers, delivery);
   ferries. Public `Way`, `Meta`, `Build(ways, nodes, meta)`, `BuildPBF(path, meta)`
   (two passes). `assemble`: keep routable ways, sort refs for unique IDs and use
   counts, junction = used ≥ 2 or endpoint, fill coordinates by binary search, split at
   junctions, skip duplicate refs, a missing node splits the way, ferry km/h from
   duration. `ponytail:` no junction/signal delays, flat 5 km/h foot ignoring oneway,
   `area=yes` routed along the outline.
4. `router/graph.go` (~200 lines): `Graph` (format arrays + `first`, `adj`, `comp[2]`,
   `compSize[2]`, `grid`, `maxKmh`, `mu`, search buffers); `arrays()`, `WriteTo`,
   `Load`, `index()` (CSR, per-mode components by BFS, grid, buffers), `Stats()`.
   `ponytail:` whole graph in RAM (~100 MB), upgrade = mmap on unix.
5. `router/route.go` (~230 lines): `Point`, `Route{Line, DistanceM, DurationS, Ferry,
   SnapFromM, SnapToM}`, errors `ErrStartTooFar`, `ErrEndTooFar`, `ErrNotConnected`,
   `ErrNoRoute`; `(g *Graph) Route(from, to, mode)`; `distM`, `snap`, `usable`, heap.
6. `router/cmd/varu-graph/main.go` (~40 lines): `-in`, `-out`, `-source`; prints
   V/S/P, MB, seconds.
7. **Measurement gate:** download the extract, run `TestRealEstonia`, record V/S/P,
   file size, load time, HeapInuse, Tallinn→Tartu and town-walk timings; cross-compile
   the test for linux/arm64 and run it on a Pi Zero 2 W. Contingencies: 7a
   Douglas-Peucker 2 m if > 60 MB; 7b Morton vertex reordering if a Pi Zero query takes
   > 5 s; bidirectional A* only if 7b is not enough. **Checkpoint A.**

### Layer 2: build, release, install (parallel once the format is frozen)

8. `builder/config.py`: `RoutingSpec(osm_extract)` (https only), `Region.routing`.
9. `regions/ee.toml`: `[routing]` block before the `[[pois]]` arrays.
   `ee-sample.toml` unchanged.
10. `builder/fetch.py`: `final_url(url)` (HEAD, follow redirects).
11. `builder/binaries.py`: extract `go_command(hint)`; used by `_compile` and the new
    step.
12. `builder/data.py`: `step_routing(ctx, refresh)`: skip without `[routing]`; target
    `maps/<id>.graph`; resolve the dated extract URL; download to `cache/osm/<id>/`
    (remove older extracts); `go run ./cmd/varu-graph … -out <id>.partial.graph`
    (cwd `router/`); replace target; record `routing_source`.
13. `builder/cli.py`: `routing` step after `pois`.
14. `builder/manifest.py`: `manifest["routing"] = {"file", "size", "source"}` when the
    graph exists (schema stays 1).
15. `builder/release.py`: extract `_ship(file, out, log)`; ship the map and
    `entry["routing"]`.
16. `installer/install.go`: `Region.Routing *Asset`; `assetItem(kind, a, target)`
    helper; a `routing` item for every install (radius and phone included) with resume
    and checksum; `writeManifest` drops `map`/`routing` if missing.
17. `installer/wizard.html`: `step_routing` — "Downloading road data for directions" /
    "Laadin alla teekonnaandmeid".

### Layer 3: launcher and viewer, then Checkpoint C

18. `launcher/go.mod`: require + replace `../router`.
19. `launcher/route.go` (~90 lines): lazy graph load from manifest `routing.file`;
    `GET /api/route?from=lat,lon&to=lat,lon&mode=foot|car` → GeoJSON Feature
    (LineString, `mode`, `distance_m`, `duration_s`, `ferry`, `snap_from_m`,
    `snap_to_m`); errors 400 `badRequest`, 404 `noGraph`, 422 `startTooFar` /
    `endTooFar` / `notConnected` / `noRoute`, 500 `badGraph`.
20. `launcher/server.go`: `mux.Handle("/api/route", &routes{root: cfg.Root})`.
21. `viewer/js/route.js` (~200 lines): `setUpRouting(map, {manifest, lang,
    geolocate})` → `{buttons(lngLat, name)}` or null without routing; route source +
    casing/line layers under the POI layers; destination marker (start marker only
    for an explicit start); GPS cache from the geolocate control; `contextmenu` +
    550 ms long-press; Route here / Start here popup; Walk/Drive (remembered); abort
    stale requests; fitBounds; `Intl.NumberFormat` distance/time; error messages; snap
    note over 30 m; Clear.
22. `viewer/js/map.js`: keep `geolocate` in a const; set up routing on load; popups
    append `routing?.buttons(...)`.
23. `viewer/map.html`: `#route-panel` (Walk/Drive, summary, from line, note, Clear).
24. `viewer/js/common.js`: routeHere "Tee siia", startHere "Alusta siit", walk
    "Jalgsi", drive "Autoga", clearRoute "Tühista", fromGps, fromHome, fromPoint,
    routing, needStart, noRoute, notConnected, startTooFar, endTooFar, noGraph,
    routeFerry, snapNote (English and Estonian; native-speaker review pending).
25. `viewer/css/app.css`: route panel (bottom-left; full width ≤ 640 px), shared
    pressed-button styles, popup buttons, no long-press callout on `#map`.

### Layer 4: tooling and docs

26. `Makefile` test: `cd router && go vet ./... && go test ./... && GOOS=linux
    GOARCH=arm GOARM=6 go build ./...`.
27. `ci.yml`: router step, still `workflow_dispatch` only (no triggers).
28. `README.md`: status, diagram line, release asset, content table, ODbL note,
    contributing, ~125 MB download.
29. `docs/ROADMAP.md`: tick the router; follow-ups (turn restrictions, barriers,
    turn-by-turn/voice, re-routing, cycling, Pi Zero timing, nearest hospital by road).

## 6. UI flow

1. Destination: **Route here** in a place popup, or right-click/long-press the map →
   **Route here**.
2. Start: explicit "Start here" → GPS fix from the last 5 minutes → manifest home →
   hint to pick a start.
3. "Finding a route…" (first request also loads the graph: 3–6 s on a Pi Zero).
4. Line drawn, map fits it; panel "12.4 km · 2 h 5 min", "From home", ferry note, snap
   note.
5. Walk/Drive recalculates; Clear removes everything.
6. Errors appear in the panel; without routing data no routing buttons are shown.

Works on phones (WebView GPS), laptops, a Pi with mouse/keyboard/HDMI, Pi touchscreens
and `-lan` clients (no GPS: home or picked start).

## 7. Tests

- `router/pbf_test.go` `TestReadPBF` (test encoder; rejects unknown features,
  truncated blobs, plain Nodes).
- `router/build_test.go` `TestWayRules` (table), `TestBuildGraph` (splitting, shapes,
  lengths, flags, missing nodes, WriteTo→Load round trip, Load rejects bad input).
- `router/route_test.go` `TestRoute` (synthetic town: fast vs short, oneway, private,
  island, ferry, same segment, from == to, hand-computed values),
  `TestAStarMatchesDijkstra` (30×30 grid, 50 pairs), `TestSnap`.
- `router/real_test.go` `TestRealEstonia` (env `VARU_GRAPH` or `VARU_OSM_PBF`):
  Tallinn→Tartu by car 175–200 km and 1h45–3h; a Tartu old-town walk 1.2–1.8 km;
  Kuressaare→Tallinn has a ferry; logs sizes and timings; also runs on a Pi.
- `launcher/server_test.go` `TestRouteAPI`; `installer/install_test.go` routing asset
  (+ `TestManifestDropsMissingRouting`); `tests/test_builder.py` additions.
- Manual: Chrome/Firefox right-click, Android long-press, Pi 4 Chromium with a mouse,
  dark mode, 360 px, et/en, Ruhnu, a tap in the sea, zero external requests.

## 8. Out of scope

Turn-by-turn and voice, re-routing while moving, turn restrictions, barriers, traffic
and signal delays, elevation, cycling, alternatives, multiple stops, address search,
nearest hospital by road, cutting the graph to a radius, compression (unless 7a),
linux/arm launcher target (Pi plan), the e-ink device program, printable directions,
ferry timetables.

## 9. Order of work

1. Router core, steps 1–6 (~4–5 days), step 7 gate, **Checkpoint A**.
2. In parallel: builder 8–15, installer 16–17, launcher 18–20 (~2 days),
   **Checkpoint B** (`make release-content REGION=ee`, local installer run).
3. Viewer 21–25 (~2–3 days), **Checkpoint C** (manual checklist).
4. Tooling and docs 26–29, `release.sh --dry-run`.

Total about 1.5–2 weeks.

## 10. Top risks

1. Estimates are inferred, not measured → step 7 gate with contingencies 7a/7b.
2. OSM data quality (fragments, access tags, no turn restrictions) → component-aware
   snapping, `ponytail:` notes, optional "follow road signs" note.
3. Pi Zero 2 W running Chromium as a desktop is tight on RAM → recommend Pi 4/5 for
   desktop use.
4. Long-press conflicts in MapLibre / Android WebView → one replaceable popup, CSS
   callout suppression, real-phone test.
5. PBF reader and its test encoder written by the same hand → `TestRealEstonia` on the
   real Geofabrik file before Checkpoint A.
