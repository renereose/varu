# Varu for developers

How Varu is built, released and tested. The user guides are in the [README](../README.md) (English) and [README.et.md](../README.et.md) (eesti keeles).

This repository does not contain the content. It contains the pieces that
turn freely available content into a drive:

| Piece | What it is | Folder |
|---|---|---|
| **Desktop app** | Varu for macOS, Windows and Linux: one window that installs Varu and then shows it | `desktop/` |
| **Installer** | The setup wizard inside the apps: puts Varu on a USB stick, a computer or a phone | `installer/`, `android/` |
| **Builder** | A Python tool that downloads everything for a region and assembles the drive | `builder/`, `regions/`, `sources.toml` |
| **Launcher** | A small Go program on the drive that serves it to the browser | `launcher/` |
| **Router** | Varu's own offline directions: builds a road network from OpenStreetMap and finds routes on it | `router/` |
| **Library reader** | Varu's own reader for the ZIM books (Wikipedia and others): pages, images, files, and search by title and inside the text | `zim/` |
| **Map files** | The PMTiles format, shared by the installer (which cuts maps) and the pocket renderer (which reads them) | `pmtiles/` |
| **Pocket renderer** | Draws e-ink map screens from a drive, for a planned Raspberry Pi pocket device; for now it writes PNG mock-ups | `pocket/` |
| **Viewer** | The web pages you see: start page, map and library | `viewer/` |

## Status

Early. What has been exercised so far:

- Launcher: unit-tested, cross-compiles for Windows, macOS, Linux and
  Android. Run for real on Linux and macOS.
- Viewer: start page and map load in a headless browser with zero network
  requests leaving the machine, against a current Protomaps build.
- Viewer: "show my location" checked in a browser posing as a phone; not yet
  on a real phone.
- Directions: built from the full Estonia extract (36 MB with street names,
  0.15 s to load, Tallinn → Tartu found in under 50 ms on a Mac) and driven
  in a headless browser with mouse, keyboard and long-press. Live directions
  (Start) walked in a browser posing as a phone, position by position,
  including leaving the route. Not yet timed on a Pi or tried on a phone
  outdoors.
- Map search: the address register's whole-country extract (1.38 million
  addresses, 30 MB) becomes 775,000 towns, streets and house addresses in a
  10 MB file in about 15 s. The launcher loads it in under a second at the
  first search and answers in 20–60 ms on a Mac (a full scan). Searched,
  picked and routed in a headless browser. Not yet timed on a phone.
- Builder: the `app`, `launcher`, `maps`, `pois` and `manifest` steps have
  run end to end for the Kuressaare sample. `library` and `documents` have
  not run against the real services yet.
- Library reader: reads the real books (Vikipeedia, 761,588 entries; WikiMed;
  the post-disaster library) in about a millisecond to open and 5–15 ms per
  page or title search on a Mac; pages, images, PDFs and title search checked
  in a browser and in the Android app (Android 15 emulator).
- Library search inside the text: each book's index is built with the
  release (Vikipeedia: 264,000 articles in under 2 minutes on a Mac,
  183 MB, about 150 MB of memory while building) and downloaded with the
  book, so devices only build one if it is missing; searches take 15–100 ms.
- Desktop app: built for all three systems on a Mac. On macOS it has run end
  to end: wizard, install, map, library, and opening from a stick's
  `Varu-macOS.dmg` on a computer without Varu. The Linux AppImage has started
  from a stick in Ubuntu 24.04 (in a container). Not yet run on Windows.
- Installer: unit-tested and run end to end on macOS against a locally
  built release, including the radius map cut and the Android flow in
  phone mode. Not yet run on Windows or on a phone.
- Pocket renderer: draws 1-bit and 4-grey screens from the release drive,
  under 0.1 s each on a Mac (the busiest: central Tallinn at 648×480);
  mock-ups of Tallinn, Tartu and Rõuge at zooms 10, 12 and 14 with
  `make mockups DRIVE=…`. Cross-compiles for the
  Pi (64-bit, and 32-bit ARMv6 for a Pi Zero W) but has not run on a Pi or
  drawn on a real e-ink panel; there is no panel driver, GPS or buttons yet.

Expect the first real build to need small fixes. The likeliest places are
listed in [ROADMAP.md](ROADMAP.md).

## Building it yourself

You need Python 3.11+, Go 1.22+, and the
[`pmtiles` command](https://github.com/protomaps/go-pmtiles/releases) on your
PATH. Node 20+ only for the desktop app (`make desktop`). No Docker.

```sh
make test      # run all tests
make sample    # small trial drive in ./drive (one town, a few MB)
make run       # start it from source and open the browser
```

A full drive, written straight to a mounted USB stick:

```sh
python3 -m builder --region ee --out /Volumes/VARU      # macOS
python3 -m builder --region ee --out /media/$USER/VARU  # Linux
python3 -m builder --region ee --out E:\                # Windows
```

To try the installer against a locally built release:

```sh
make sample
python3 -m builder.release --region ee-sample --drive ./drive --out ./dist
cd installer && go run . -source ../dist
```

or the desktop app around it (`make desktop` first):

```sh
VARU_SOURCE=$PWD/dist build/desktop/mac-universal/Varu.app/Contents/MacOS/Varu
```

Useful builder options:

```sh
--dry-run               # show what would be downloaded, change nothing
--only maps,pois        # run some steps
--skip library          # leave some out
--refresh               # fetch maps and place layers again
--planet-build 20261120 # pin the map to one Protomaps daily build
```

The build can be interrupted and re-run. Finished downloads are kept and
partial ones resume. If a step fails, the others still run and the drive
works with whatever was built.

**Format the stick as exFAT.** It is the only file system that Windows, macOS
and Linux all read and write and that holds files larger than 4 GB. The
wizard does this for you (macOS: `diskutil`; Windows: `Format-Volume`, after
the usual permission prompt; Linux: UDisks, which needs `exfatprogs`).

## How it works

```
 Varu app (desktop or Android), or Start-* in a browser
        │
        ▼
 bin/<os>-<arch>/varu  ── serves http://localhost:8480 ──▶  the app's window
        │                                                      │
        ├── app/         viewer + MapLibre + fonts             ├── start page
        ├── maps/*.pmtiles   read with HTTP Range requests     ├── map
        ├── maps/*.graph     road network ─▶ /api/route        ├── directions
        ├── maps/*.places    addresses ─▶ /api/places          ├── map search
        ├── pois/*.geojson   hospitals, pharmacies, water …    │
        ├── docs/            PDFs                              │
        └── library/*.zim   read by the launcher itself ──▶ /library/
            library/*.search  Varu's search index of each book, built with the release
```

### Releases

Releases are built on a Mac and published with one command, without
GitHub Actions:

```sh
sh release.sh v0.1.2 --dry-run   # build everything into dist/ and stop
sh release.sh v0.1.2             # build, tag and publish the release
```

It runs the tests, builds each country's drive with a fresh map and places
(without the library), packages it with `builder/release.py`, builds each
library book's search index (the books are downloaded once into
`~/.cache/varu/zim`, about 5.5 GB for Estonia, and kept for the next
release), builds Varu
for macOS, Windows, Linux and Android and publishes them all as one GitHub
release. Each app downloads from the release with its own tag, so content
and apps always match. It publishes only a clean, up-to-date `main`.

- `regions.json` lists the countries, place layers and library books.
- `Varu-macOS.dmg`, `Varu-Windows.exe` (portable) and
  `Varu-Linux-x86_64.AppImage` are also what the wizard puts on USB sticks.
- `varu-<id>.zip` is the drive without its map and library.
- `<id>.pmtiles` is the country's map. For a radius install the installer
  cuts a smaller map out of it with HTTP range requests
  (`installer/pmtiles.go`, with the file format in `pmtiles/`), so only
  that part is downloaded.
- `<id>.graph` is the road network for directions (about 36 MB), always
  for the whole country.
- `<id>.places` holds the towns, streets and addresses the map's search box
  finds (about 10 MB), also for the whole country.
- Library books come straight from the Kiwix mirrors, pinned to the edition
  that was newest at release time, and are checked against Kiwix's SHA-256.

The Android APK must be signed with the same key in every release, or
phones refuse the update. The release key lives in
`~/.android/varu-release.keystore` with its password in the macOS Keychain
under "Varu Android release keystore". **Back up both**: without them no
future version can update the app on people's phones.

A browser cannot read a multi-gigabyte map straight off a disk, which is why
there is a launcher at all. It binds to `127.0.0.1` only, rejects requests
for other host names, hides dotfiles and never lists directories. Started
with `-lan`, or once **Share with other devices** is switched on from the
start page (`POST /api/share`), it also serves phones and other computers on
the same Wi-Fi from a second listener on `0.0.0.0`. That listener exists only
while sharing is on, it asks for the same port (Linux and some systems give
it the next one), and only the person at the computer can switch it: the
request must come from `127.0.0.1` and carry an `X-Varu: 1` header. Pages
inside the library's books share the launcher's origin, so a book's own
script could still switch it; Varu only ships books from the Kiwix catalogue.

`manifest.json` on the drive lists what is actually there. The viewer shows
only what the manifest lists, so a drive without a library simply has a
greyed-out Library card.

### Where the content comes from

| Content | Source | Build step |
|---|---|---|
| Map tiles | [Protomaps](https://protomaps.com) daily build of OpenStreetMap, cut to the region with `pmtiles extract` | `maps` |
| Road network | The [Geofabrik](https://download.geofabrik.de) OpenStreetMap extract (`[routing]` in the region file, about 125 MB to download), turned into `maps/<id>.graph` by `router/cmd/varu-graph` | `routing` |
| Places | OpenStreetMap via the Overpass API, one query per layer, or an official list published as GeoJSON (`geojson =` in the region file) | `pois` |
| Addresses for search | The Estonian address register's (ADS) monthly extract of valid addresses (`[places]` in the region file, about 30 MB to download), turned into `maps/<id>.places` by `builder/places.py`; streets, settlements and municipalities are derived from the addresses | `places` |
| Wikipedia and reference works | ZIM archives from the [Kiwix catalogue](https://library.kiwix.org) | `library` |
| Documents | URLs listed in the region file | `documents` |
| Map renderer, fonts, icons | MapLibre GL JS, PMTiles, Protomaps basemaps, pinned and checksummed in `sources.toml` | `app` |

## Adding a region

Copy `regions/ee.toml`, then change the id, country code, bounding box, place
layers, library list and documents. Run it with `--dry-run` first, then
`--only maps,pois` to check the map before the long downloads.

Interface text lives in `viewer/js/common.js`. To add a language, add a block
there and list its code under `languages` in the region file.

Two content files sit next to the pages and ship with the app:

- `viewer/topics.json`: the library's hand-picked survival and DIY articles by
  topic. A book is named without its edition month (`wikipedia_et_all_maxi`),
  and a path is the article's address inside the book, not URL-encoded.
  `go test -run TestTopics ./launcher` checks the file; with
  `VARU_ZIM_DIR=~/.cache/varu/zim` it also checks every article exists in the
  books there, which `release.sh` does after indexing them.
- `viewer/guides.json`: the "In an emergency" steps, radio frequencies and
  home-supplies list, written for Varu from the official sources each entry
  names. Keep steps in plain words and re-check the sources when they change.

## Licensing

- **This repository's code** is MIT licensed. See [`LICENSE`](../LICENSE).
- **klauspost/compress** (BSD-3-Clause) unpacks the library's zstd
  compression inside the launcher.
- **Map data** is © OpenStreetMap contributors under the ODbL. Attribution
  is shown on the map and start page and must stay. The road network is a
  derived database under the same licence.
- **Library content** keeps the licence of each source, mostly CC BY-SA for
  Wikimedia projects.
- **Documents** are only as redistributable as their publishers allow. Check
  before adding one to a region file, and prefer the publisher's own URL.
- Third-party notices are written to `licenses/` on every drive.

## A note on crisis information

Directions are an estimate from OpenStreetMap: no traffic, no closures, no
turn restrictions or barriers yet, and ferries without timetables. The route
panel says so every time. Follow road signs.

A drive is a snapshot. Maps, opening hours and official guidance go out of
date, and place data from OpenStreetMap is volunteer-maintained and can be
wrong or incomplete. The start page shows the build date. Rebuild drives
regularly, and treat official channels as the authority whenever they are
reachable.

## Contributing

Run `make test` before sending a change. The router, launcher, installer,
map files and pocket renderer use only the Go standard library, except for one zstd decoder
(klauspost/compress) in the library reader; the builder uses only the Python
standard library
and the Android app only the Android framework (it builds without Gradle:
`make android`). The desktop app is Electron with one `main.js` and no other
runtime dependencies; electron-builder packages it. Please keep it that way
unless there is a strong reason.
