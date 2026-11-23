# Roadmap

## 1. First real build (do this before anything else)

The scaffold was written and tested without access to the content services.
A first build on a normal internet connection will settle these:

- [ ] `make sample` end to end. Confirms the `pmtiles extract` call, the
      Protomaps build lookup and the Overpass queries.
- [ ] **Map styling against a current Protomaps build.** The viewer was only
      seen with an older-format sample archive, where just land and water
      drew. Roads, labels and buildings need a look.
- [ ] **kiwix-tools file names** in `sources.toml`. They follow the pattern of
      earlier releases and were not checked for the pinned version.
- [ ] **Kiwix catalogue parsing.** `parse_catalog` was written from the
      documented feed shape and tested against a hand-made fixture. Compare
      with a live response and replace the fixture with a real one.
- [ ] **ZIM names** in `regions/ee.toml` against https://library.kiwix.org.
- [ ] **Real hardware.** An exFAT stick on Windows 10/11, macOS (Intel and
      Apple Silicon) and at least two Linux desktops. Things to watch:
  - whether macOS Gatekeeper or Windows SmartScreen object to the unsigned
    launcher on a locally built stick;
  - whether Linux desktops mount the stick with permission to run programs;
  - how long kiwix-serve takes to open large archives from a slow stick;
  - map responsiveness from USB 2.0 ports.

## 2. Content

- [ ] Public shelters from the Rescue Board's dataset (not OpenStreetMap).
- [ ] Official guidance for Estonia in `[[documents]]`, once reuse terms are
      confirmed with the publisher.
- [ ] Crisis radio frequencies page.
- [ ] A curated practical library: first aid, water, food, heating, repair.
- [ ] Regions for Latvia, Lithuania and Finland.
- [ ] Several regions on one drive, plus a low-zoom map of the rest of Europe.
      Needs a decision on how the viewer combines map files.

## 3. Viewer

- [ ] Place search (towns, streets, place layers) that works offline.
- [ ] "Nearest hospital / pharmacy / water" from a point on the map.
- [ ] Search box on the start page that goes straight into the library.
- [ ] Review of the Estonian interface text by a native speaker; Russian,
      Latvian, Lithuanian and Finnish translations.
- [ ] Print-friendly map view.

## 4. Builder

- [ ] Size budget: `--size 128` that totals the planned downloads and
      refuses, or suggests smaller library flavours, before starting.
- [ ] Checksums for ZIM downloads (the Kiwix mirrors publish them).
- [ ] Check that the target is exFAT and warn otherwise.
- [ ] Download the `pmtiles` tool automatically instead of requiring it.
- [ ] Reproducible builds: record every source URL, version and hash in a
      lock file on the drive.

## 5. Distribution

- [ ] Prebuilt launchers attached to GitHub releases, so building a drive
      needs Python only (`--launcher-dir` already accepts them).
- [ ] Code signing and notarisation for the launcher, if testing shows the
      warnings are a real obstacle.
- [ ] A start script for network sharing (`-lan`), with a QR code for phones.
- [ ] A guide for groups that want to produce drives in bulk.

## Decisions to make

- **Name.** "Varu" is a placeholder.
- **Licence.** MIT was chosen as a default. Apache-2.0 (patent grant) and
  GPL-3.0 (keeps derivatives open) are the usual alternatives.
- **Phones.** The drive needs a computer. Options are the `-lan` mode, or
  pointing people to the Kiwix and Organic Maps apps with the same content.
