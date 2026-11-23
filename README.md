# Varu

An open-source offline drive: maps, Wikipedia, reference works and official
guidance for one region on a USB stick that works with no internet and
nothing to install. Plug it in, double-click, and it opens in your browser.

*Varu* is Estonian for "reserve" or "supply". The name is a working title.

This repository does not contain the content. It contains the three pieces
that turn freely available content into a drive:

| Piece | What it is | Folder |
|---|---|---|
| **Builder** | A Python tool that downloads everything for a region and assembles the drive | `builder/`, `regions/`, `sources.toml` |
| **Launcher** | A small Go program on the drive that serves it to the browser | `launcher/` |
| **Viewer** | The web pages you see: start page and map | `viewer/` |

Estonia is the first region. Adding a country means adding one file in
`regions/`.

## Status

Early scaffold. What has been exercised so far:

- Launcher: unit-tested, cross-compiles for Windows, macOS (Intel and Apple
  Silicon) and Linux (x86-64 and ARM64). Run for real only on Linux.
- Viewer: start page and map load in a headless browser with zero network
  requests leaving the machine. The map was tested with a small sample
  archive in an older basemap format, so full street-level styling against a
  current Protomaps build has not been seen yet.
- Builder: unit-tested. The `app`, `launcher` and `manifest` steps have run
  end to end. The `maps`, `pois`, `library`, `kiwix` and `documents` steps
  have **not yet run against the real services**.

Expect the first real build to need small fixes. The likeliest places are
listed in [docs/ROADMAP.md](docs/ROADMAP.md).

## Quick start

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

Useful options:

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
        ├── pois/*.geojson   hospitals, pharmacies, water …    │
        ├── docs/            PDFs                              │
        └── /library/ ──▶ bin/<os>-<arch>/kiwix-serve ──▶ library/*.zim
```

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
| Places | OpenStreetMap via the Overpass API, one query per layer | `pois` |
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
  is shown on the map and start page and must stay.
- **Library content** keeps the licence of each source, mostly CC BY-SA for
  Wikimedia projects.
- **Documents** are only as redistributable as their publishers allow. Check
  before adding one to a region file, and prefer the publisher's own URL.
- Third-party notices are written to `licenses/` on every drive.

## A note on crisis information

A drive is a snapshot. Maps, opening hours and official guidance go out of
date, and place data from OpenStreetMap is volunteer-maintained and can be
wrong or incomplete. The start page shows the build date. Rebuild drives
regularly, and treat official channels as the authority whenever they are
reachable.

## Contributing

Run `make test` before sending a change. The launcher uses only the Go
standard library and the builder only the Python standard library; please
keep it that way unless there is a strong reason.
