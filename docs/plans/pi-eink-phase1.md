# Plan: Pi Zero e-ink pocket device, phase 1 (map renderer on the Mac, no hardware)

Status: built 2026-11-26 with option A for all seven decisions, in `pocket/` with
the command `varu-pocket` (read those for `device/` and `varu-device` below). Where
this plan and the code differ, the code is right. Found while building:

- The tiles are Protomaps v4: railways are `roads` with kind `rail`, pedestrian
  streets are `path` with kind_detail `pedestrian` (drawn as streets), forests are
  `landuse` forest/wood (there is no `landcover` layer), and places carry `min_zoom`.
- Drives also have a `shelter` layer: drawn with the ring-and-dot symbol, second in
  priority after hospitals, from zoom 11.
- Tuned from the mock-ups: villages show one zoom before the tiles' `min_zoom` (else
  the countryside has no names at zoom 10, though small villages are still missing
  there because the zoom 10 tiles leave them out); town and city names are placed
  before the symbols; 1-bit buildings are a diagonal hatch (the 50% checker buried
  the streets), 1-bit forest is one dot in 16, and 1-bit rivers are dashed so they
  differ from roads.
- The drive's books are read by Varu's own `zim/` reader now, not kiwix-serve.
- Not made: the Tartu-home and Otepää-beyond-the-radius mock-ups. The parent-tile
  fallback is covered by `TestParentTile` and was checked on a 1 km cut map.

Goal of the device: Raspberry Pi Zero 2 W + ~4.2" e-ink (400×300, 1-bit or 4-grey) +
5–6 buttons + GPS + battery: an offline map, your position and "nearest water /
hospital", no browser. Phase 1 = a Go renderer that draws e-ink screens from a Varu
drive, plus mock-ups (Tallinn, Tartu, a rural area, several zooms) to choose the panel
size and style before buying hardware.

Hard rules: Go standard library only; no third-party apps or services at runtime; no
GitHub Actions triggers.

## Amendment (user, 2026-11-23): Pi as a desktop

"The raspberry one must also support a scenario where you have it installed on a
raspberry pi or similar and you attach a keyboard, mouse and hdmi screen to it."

- **Pi 4/5 with 64-bit Raspberry Pi OS** should already work: the drive carries the
  linux-arm64 launcher and kiwix-serve, and Chromium shows the normal viewer. Untested
  on a real Pi.
- **Gap 1, 32-bit Pi OS and older Pis:** add linux/arm GOARM=6 launcher builds
  (`sources.toml` targets), kiwix-tools `linux-armv6`, and `armv6l|armv7l) arch=armv6`
  in `START_LINUX` (builder/binaries.py).
- **Gap 2, installing on the Pi itself:** release Linux arm64 (+ armv6) installer
  builds and list USB sticks under `/media/<user>` and `/run/media/<user>` in
  `installer/volumes_linux.go`.
- **Gap 3, weak Pis (Zero, Zero 2 W, Pi 3):** the viewer's MapLibre 6 needs WebGL 2,
  which their VideoCore IV GPU lacks (verify on hardware), and 512 MB RAM is tight for
  a browser. Use this phase's Go renderer a second way: the launcher serves rendered
  map images to a light page (pan/zoom by mouse and keyboard, no WebGL), so one
  renderer feeds both the e-ink panel and an HDMI screen. Plan this as phase 1b after
  the mock-ups.
- The router (separate plan) runs inside the launcher, so directions work on the Pi
  desktop too; recommend a Pi 4/5 for desktop use (router + Chromium RAM).

## 1. Decisions (recommended option first)

1. **Reading PMTiles without breaking the installer.** **A:** move the format code
   unchanged into a new stdlib-only repo-root module `pmtiles/`; the installer imports
   it via `require` + `replace ../pmtiles`; the local-file tile reader lives there too.
   B: copy ~90 lines into the device module.
2. **Fonts and labels.** **A:** read the drive's MapLibre SDF glyph files
   (`app/assets/fonts/Noto Sans Medium/{start}-{end}.pbf`, same protobuf format as the
   tiles); ink pixels ≥ 192/255 for sharp 1-bit text at any size and rotation; halo by
   growing the ink; ranges loaded on demand (õäöü in 0-255, šž in 256-511); no new
   dependency. B: golang.org/x/image + a committed TTF (> 500 KB, against the policy).
   C: a hand-made bitmap font.
3. **Zoom meaning.** **A:** MapLibre scale (world = 512·2^z px), so numbers match the
   viewer; mock up z10, z12, z14 (~16 km, 4 km, 1 km across at 400 px).
   B: 256 px scale z11/13/15. C: MapLibre scale z11/13/15.
4. **Street names.** **A:** straight rotated labels along the longest nearly straight
   stretch, one per name per screen, never upside down. B: horizontal only. C: none.
5. **POI clutter.** **A:** per-category minimum zoom, fixed priority (hospital first),
   overlapping symbols dropped (upgrade = clusters). B: draw all.
6. **Judging aids.** **A:** `-legend` sheet + Tallinn at 648×480 (5.83" panel).
   B: legend only. C: neither.
7. **Country border.** **A:** dashed 2 px black line for `boundaries` with
   `kind_detail <= 2`. B: leave out.

Fixed: hand-written scanline fill and lines without anti-aliasing; everything drawn
into an in-memory `*image.Gray` (levels 0, 85, 170, 255; only area fills differ
between modes via 4×4 screen-aligned patterns); a pixel occupancy grid for label and
symbol collision; per-tile clipping with layer-by-layer drawing; missing tiles fall
back to the parent tile scaled up.

## 2. Build plan

**Layer A — shared PMTiles module.** A1 `pmtiles/go.mod`. A2 move header/directory/
tile-ID code from `installer/pmtiles.go` (exported: `HeaderLen`, `RootMax`, `Gzip`,
`Header`, `ParseHeader`, `Bytes`, `Entry`, `DecodeDir`, `EncodeDir`, `BuildDirs`,
`ZxyToID`, `IDToZxy`; new `MercatorX/Y`). A3 local reader `File` (`Open`, `Tile(z,x,y)`
with leaf directories and gunzip, `Close`, `findEntry`). A4 tests (moved
`TestTileIDs`/`TestLeafDirectories`, new `TestFileTile` with 60,000 entries and a run
of 3). A5–A7 installer `require`/`replace`, rename uses, keep `TestCut`/
`TestRealArchive` green, cross-build windows/darwin/android. **Checkpoint A.**

**Layer B — device module, tiles and fonts.** B1 `device/go.mod` (same replace). B2
`mvt.go` (~140 lines: `pbf` wire reader, `decodeTile` → layers/features with
MoveTo/LineTo/ClosePath, bounds-checked). B3 `mvt_test.go` (test protobuf writer;
roads line + polygon with hole; truncated input errors). B4 `text.go` (~130 lines:
`openFont`, `text(s, size, angle)`, `inkThreshold = 192`, missing glyphs leave a
space). B5 `text_test.go` (synthetic õ/š range files; rotation swaps w/h; missing range
no error).

**Layer C — drawing and style.** C1 `raster.go` (~150 lines: `fill`, `stroke` with
round joins, `dashes`, `stamp`/`art`/`halo`/`drawStamp`; clipping via `SubImage`). C2
`raster_test.go`. C3 `style.go` (~280 lines: rules, `roadRules`, `placeSize`,
`streetSize`, symbols in priority order, `homeArt`, `fallbackArt`) — the only file to
edit when tuning. C4 `style_test.go` (well-formed drawings and patterns).
**Checkpoint B/C.**

**Layer D — labels, render, CLI.** D1 `labels.go` (~110 lines: occupancy grid,
`longestRun`, `placeLabels`). D2 `labels_test.go`. D3 `render.go` (~220 lines:
`drive`, `view{lat, lon, zoom, w, h, grey}`, `viewTiles`, `render`, `scaleBar`; order:
white, tiles (parent fallback), rules, then scale bar, home, POI symbols, place labels,
street labels). D4 `render_test.go` (`TestViewTiles`, `TestScaleBar`, env-gated
`TestRealRender` with `VARU_DRIVE`). D5 `main.go` (~110 lines:
`varu-device -drive DIR [-center LAT,LON] [-zoom 12] [-size 400x300] [-grey] -o OUT.png`,
`-legend`; `openDrive` reads manifest, map, font, POIs, home). **Checkpoint D: first
unlabelled mock-ups.**

**Layer E — wiring, docs, mock-ups.** E1 Makefile: `pmtiles` and device tests, Pi
cross-builds (linux/arm64, linux/arm GOARM=6), `mockups` target
(tallinn 59.4370,24.7450 · tartu 58.3801,26.7225 · rouge 57.7290,26.9130 × z10/12/14 ×
1-bit/4-grey → `build/mockups/`). E2 `.gitignore` `/device/device`. E3 README pieces
table, status, contributing. E4 ROADMAP section "6. Pocket device". E5 generate
mock-ups (plus Tartu home drive and Otepää beyond the radius); tune `style.go`; view
400×300 at 85 mm wide for true size.

## 3. Style (starting values)

Areas: water — 1-bit horizontal hatch 1 row in 4 / 4-grey light grey; forest (z10+) —
sparse dots; buildings (z14+) — 50% checker / dark grey. Land is white.

Lines (px at z10/11/12/13/14/15+; c = cased): highway 2/3/4c/5c/7c/9c; major_road
1/1.5/3c/4c/6c/8c; minor_road, other –/–/1/1.5/4c/6c; path z13+ 1 dashed 3/2; rail
1/1/1.5/2/3 with white dashes; rivers 1/1/2/2/3/3; streams z13+ 1; country border 2
dashed 8/4.

Labels (Noto Sans Medium, black, 2 px white halo): city 18, town 15, village 12, hamlet
and neighbourhood 11 (z13+); streets 11 px (major z13+, minor z14+).

Symbols (15×15, 1 px halo, priority order): hospital (black square, white +, z8);
water (black drop, z10); border (black diamond, z8); rescue (black flame, z10); police
(black shield, z10); spring (white drop, z11); clinic (white square, black +, z12);
pharmacy (white circle, black +, z12); fuel (black pump, z12); food (black basket,
z13); unknown/shelter (ring with dot, z12); home (19×19 house, always).

Scale bar bottom-left, ≤ 100 px, 1-2-5 steps.

## 4. Order, sizes, risks, scope

Order: day 1 layer A with B2–B3 and C1–C2 in parallel; day 2 areas/roads, unlabelled
mock-ups (verify tile schema); day 3 text, check 11–12 px legibility, labels; day 4
streets, symbols, home, scale bar, tests, tuning; day 5 wiring, docs, final mock-ups.
Sizes: `pmtiles/` ~85 new + ~200 moved; `device/` ~1,150 + ~350 test lines; installer
net −190.

Risks: small 1-bit text (check day 3; fall back to option 2B if needed); tile schema
assumptions (unlabelled render on day 2); installer regression from the move (tests,
real-file test, cross-builds).

Out of scope: SPI panel driver, GPIO buttons, GPS, nearest-places screen, menus and
panning UI, auto-off, boot time, SD image, enclosure, battery, Pi binaries in releases,
curved text, POI/water/park names, other label languages, clustering, fractional zoom,
tile cache, anti-aliased text, shelter symbol, on-device attribution screen.
