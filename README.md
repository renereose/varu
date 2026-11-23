# Varu

An open-source offline drive: maps, Wikipedia, reference works and official
guidance for one region on a USB stick that works with no internet and
nothing to install. Plug it in, double-click, and it opens in your browser.

*Varu* is Estonian for "reserve" or "supply". The name is a working title.

This repository does not contain the content. It contains the pieces that
turn freely available content into a drive:

| Piece | What it is | Folder |
|---|---|---|
| **Installer** | The app people download: a wizard that puts Varu on a USB stick, a computer or a phone | `installer/`, `android/` |
| **Builder** | A Python tool that downloads everything for a region and assembles the drive | `builder/`, `regions/`, `sources.toml` |
| **Launcher** | A small Go program on the drive that serves it to the browser | `launcher/` |
| **Router** | Varu's own offline directions: builds a road network from OpenStreetMap and finds routes on it | `router/` |
| **Viewer** | The web pages you see: start page and map | `viewer/` |

## Installing (for everyone)

Download the installer for your system from the
[latest release](https://github.com/renereose/varu/releases/latest):

| System | File |
|---|---|
| Windows | `Varu-Installer.exe` |
| macOS | `Varu-Installer-macOS.zip`, then open `Varu Installer` |
| Android | `Varu.apk` |

The installer opens a wizard (in your browser on a computer, in the app on
Android) that asks:

1. **Your country.**
2. **Your home** (search for the town, or use your location) and a
   **radius**: 10, 25, 50 or 100 km, or the whole country. Inside the radius
   you get street-level map and places; the rest of the country stays on the
   map at road level, for travel.
3. **Which places** to show: drinking water, hospitals, pharmacies, clinics,
   food stores, fuel, police, rescue stations, border crossings, springs.
4. **Library** books (computers only for now).
5. **Where to put it**: a USB stick (listed as soon as it is plugged in),
   a folder on this computer, or the phone's storage or memory card.

An interrupted install continues where it stopped when you run it again.

**The installers are not signed yet**, so the first start needs one extra step:

- **Windows:** "Windows protected your PC" → *More info* → *Run anyway*.
- **macOS:** open the app once, close the warning, then *System Settings →
  Privacy & Security → Open Anyway*.
- **Android:** allow your browser or file manager to *install unknown apps*
  when asked.

Estonia is the first region. Adding a country means adding one file in
`regions/`.

## Status

Early. What has been exercised so far:

- Launcher: unit-tested, cross-compiles for Windows, macOS, Linux and
  Android. Run for real on Linux and macOS.
- Viewer: start page and map load in a headless browser with zero network
  requests leaving the machine, against a current Protomaps build.
- Viewer: "show my location" checked in a browser posing as a phone; not yet
  on a real phone.
- Directions: built from the full Estonia extract (33 MB, 0.15 s to load,
  Tallinn → Tartu found in under 50 ms on a Mac) and driven in a headless
  browser with mouse, keyboard and long-press. Not yet timed on a Pi or a
  phone.
- Builder: the `app`, `launcher`, `maps`, `pois` and `manifest` steps have
  run end to end for the Kuressaare sample. `library` and `documents` have
  not run against the real services yet.
- Installer: unit-tested and run end to end on macOS against a locally
  built release, including the radius map cut and the Android flow in
  phone mode. Not yet run on Windows or on a phone.

Expect the first real build to need small fixes. The likeliest places are
listed in [docs/ROADMAP.md](docs/ROADMAP.md).

## Building it yourself

You need Python 3.11+, Go 1.22+, and the
[`pmtiles` command](https://github.com/protomaps/go-pmtiles/releases) on your
PATH. No Node, no Docker.

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
and Linux all read and write and that holds files larger than 4 GB.

## How it works

```
 Start-Windows.bat / Start-macOS.command / Start-Linux.sh
        │
        ▼
 bin/<os>-<arch>/varu  ── serves http://localhost:8480 ──▶  your browser
        │                                                      │
        ├── app/         viewer + MapLibre + fonts             ├── start page
        ├── maps/*.pmtiles   read with HTTP Range requests     ├── map
        ├── maps/*.graph     road network ─▶ /api/route        ├── directions
        ├── pois/*.geojson   hospitals, pharmacies, water …    │
        ├── docs/            PDFs                              │
        └── /library/ ──▶ bin/<os>-<arch>/kiwix-serve ──▶ library/*.zim
```

### Releases

Releases are built on a Mac and published with one command, without
GitHub Actions:

```sh
sh release.sh v0.1.2 --dry-run   # build everything into dist/ and stop
sh release.sh v0.1.2             # build, tag and publish the release
```

It runs the tests, builds each country's drive with a fresh map and places
(without the library), packages it with `builder/release.py`, builds the
three installers and publishes them all as one GitHub release. Each
installer downloads from the release with its own tag, so content and
installers always match. It publishes only a clean, up-to-date `main`.

- `regions.json` lists the countries, place layers and library books.
- `varu-<id>.zip` is the drive without its map and library.
- `<id>.pmtiles` is the country's map. For a radius install the installer
  cuts a smaller map out of it with HTTP range requests
  (`installer/pmtiles.go`), so only that part is downloaded.
- `<id>.graph` is the road network for directions (about 35 MB), always
  for the whole country.
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
with `-lan`, it also serves phones and other computers on the same Wi-Fi.

`manifest.json` on the drive lists what is actually there. The viewer shows
only what the manifest lists, so a drive without a library simply has a
greyed-out Library card.

### Where the content comes from

| Content | Source | Build step |
|---|---|---|
| Map tiles | [Protomaps](https://protomaps.com) daily build of OpenStreetMap, cut to the region with `pmtiles extract` | `maps` |
| Road network | The [Geofabrik](https://download.geofabrik.de) OpenStreetMap extract (`[routing]` in the region file, about 125 MB to download), turned into `maps/<id>.graph` by `router/cmd/varu-graph` | `routing` |
| Places | OpenStreetMap via the Overpass API, one query per layer, or an official list published as GeoJSON (`geojson =` in the region file) | `pois` |
| Wikipedia and reference works | ZIM archives from the [Kiwix catalogue](https://library.kiwix.org) | `library` |
| Library reader | `kiwix-serve` from kiwix-tools | `kiwix` |
| Documents | URLs listed in the region file | `documents` |
| Map renderer, fonts, icons | MapLibre GL JS, PMTiles, Protomaps basemaps, pinned and checksummed in `sources.toml` | `app` |

## Adding a region

Copy `regions/ee.toml`, then change the id, country code, bounding box, place
layers, library list and documents. Run it with `--dry-run` first, then
`--only maps,pois` to check the map before the long downloads.

Interface text lives in `viewer/js/common.js`. To add a language, add a block
there and list its code under `languages` in the region file.

## Licensing

- **This repository's code** is MIT licensed. See `LICENSE`.
- **kiwix-serve** is GPL-3.0-or-later. The drive ships it as a separate,
  unmodified program that the launcher starts; it is not linked into Varu.
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

Run `make test` before sending a change. The router, launcher and installer
use only the Go standard library, the builder only the Python standard library
and the Android app only the Android framework (it builds without Gradle:
`make android`). Please keep it that way unless there is a strong reason.
