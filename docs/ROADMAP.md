# Roadmap

## 1. First real build (do this before anything else)

The scaffold was written and tested without access to the content services.
A first build on a normal internet connection will settle these:

- [x] `make sample` end to end (2026-11-23).
- [x] Map styling against a current Protomaps build: roads, labels and
      buildings draw in light and dark.
- [ ] **Kiwix catalogue parsing.** `parse_catalog` was written from the
      documented feed shape and tested against a hand-made fixture. Compare
      with a live response and replace the fixture with a real one.
- [x] ZIM names in `regions/ee.toml` resolve in the Kiwix catalogue, with
      checksums (2026-11-23).
- [ ] **Real hardware.** An exFAT stick on Windows 10/11, macOS (Intel and
      Apple Silicon) and at least two Linux desktops. Things to watch:
  - whether macOS Gatekeeper or Windows SmartScreen object to the unsigned
    launcher on a locally built stick;
  - whether Linux desktops mount the stick with permission to run programs;
  - how quickly library pages open from a slow stick;
  - map responsiveness from USB 2.0 ports.

## 2. Content

- [x] Public shelters: the Rescue Board's 303 shelters from the Land Board's
      open map service, on by default and first in the layer list.
- [ ] Official guidance for Estonia in `[[documents]]`, once reuse terms are
      confirmed with the publisher.
- [ ] Crisis radio frequencies page.
- [x] A curated practical library: 213 hand-picked articles from Vikipeedia,
      WikiMed and the post-disaster book in 12 topics (viewer/topics.json),
      checked against the books in every release.
- [x] "In an emergency": steps for 13 situations, a home-supplies checklist
      and a family plan, written for Varu from official Estonian guidance with
      the sources named (viewer/guides.json). Needs a native speaker's review.
- [ ] Gaps in the library: Vikipeedia has little practical how-to on treating
      water at home, keeping warm in a blackout, evacuation and fire safety,
      and many first-aid articles are stubs. An Estonian first-aid manual or
      the Rescue Board's crisis leaflet would help, with reuse permission.
- [ ] Regions for Latvia, Lithuania and Finland.
- [ ] Several regions on one drive, plus a low-zoom map of the rest of Europe.
      Needs a decision on how the viewer combines map files.

## 3. Viewer

- [x] Show my location (GPS on phones works offline; most laptops need the
      internet to locate themselves, and browsers refuse location on a drive
      opened over `-lan`).
- [x] Place and address search that works offline: every town, street and
      house address from the Land Board's address register (ADS, open data
      with no licence agreement), built at release time, plus the names in
      the drive's place layers.
- [ ] Search follow-ups: timing on a phone and a Pi; place names that are
      not addresses (lakes, hills, nature areas) from the place-name
      register; other countries' address sources.
- [x] Varu's own offline router: a road network built from OpenStreetMap at
      release time and a Go router in the launcher (`router/`), so routes work
      on computers and in the Android app with no other app and no internet.
      Walk or drive; Route here / Start here by right-click, long-press or the
      keyboard menu key.
- [x] Live directions: Start follows your position with the next turn
      (street names from the road data, roundabout exits) and what is left,
      and finds a new route when you leave it. Nearest place of any kind by
      road from each map layer.
- [ ] Router follow-ups: turn restrictions and barriers, spoken directions,
      "keep left/right" at forks, cycling, timing on a Pi Zero 2 W and a
      phone; live directions tried on a real phone outdoors.
- [x] Search box on the start page that goes straight into the library.
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

- [x] Installers for Windows, macOS and Android, published from a Mac
      with `release.sh`, with country, home, radius and place selection.
- [x] Varu as a desktop app for macOS, Windows and Linux (Electron): one
      window for the wizard, the map and the library. USB sticks carry it
      for all three systems.
- [ ] Desktop app: run on Windows and on a real Linux desktop (so far: macOS,
      and Ubuntu 24.04 in a container).
- [ ] Formatting a stick from the wizard: tried on macOS (an APFS stick to
      exFAT); not yet on Windows (Format-Volume behind the permission
      prompt) or Linux (UDisks).
- [ ] Stick name, icon and hidden clutter: tried on macOS (a stick made there
      shows only the three Varu apps and the README); check Windows Explorer
      (autorun.inf icon, hidden attribute) and GNOME's start prompt
      (autorun.sh). A stick made on Linux hides its clutter from Linux
      file managers only.
- [ ] First tagged release, then a test of every installer on real hardware.
      The Android app has been built but not yet run on a phone.
- [ ] Code signing: Windows (Authenticode) and macOS (Developer ID and
      notarisation) to remove the first-start warnings.
- [x] Varu's own library reader (`zim/`) in place of kiwix-serve, on every
      device including Android, where kiwix-serve cannot run in an app.
- [x] Library: search inside the text, with Varu's own index of each book
      (Kiwix's Xapian index is not used). The release builds each index and
      the installer downloads it with the book, so no phone or Pi has to;
      the launcher builds one itself only if it is missing.
- [ ] Library: delete indexes of removed books; random article;
      xz-compressed archives (editions from before about 2021).
- [x] Android: downloads carry on with the screen locked, in other apps or
      with Varu swiped away (a foreground service with a progress
      notification, a wake lock and a Wi-Fi lock). Tried on the Android 15
      emulator with Doze forced; not yet on a real phone, where some makers
      add battery savers of their own.
- [x] Android: an app icon, and dark mode that follows the system (tried on
      the emulator, not yet on a phone).
- [ ] Android: an adaptive icon (API 26+), so launchers that shape icons don't
      put it on a white plate.
- [x] Installer: remove older ZIM editions when a newer one is installed,
      with their search indexes; the old one stays until the new one is
      complete.
- [ ] A start script for network sharing (`-lan`), with a QR code for phones.
- [ ] A guide for groups that want to produce drives in bulk.

## 6. Pocket device

A Raspberry Pi Zero 2 W with an e-ink panel, GPS, buttons and a battery:
the map, your position and the nearest water or shelter, with no phone.
Plan: [plans/pi-eink-phase1.md](plans/pi-eink-phase1.md).

- [x] Phase 1: a renderer (`pocket/`, `varu-pocket`) that draws 1-bit and
      4-grey screens from a drive, with the drive's own font, and PNG
      mock-ups (`make mockups`) to choose the panel and the style. The
      PMTiles code is shared with the installer (`pmtiles/`).
- [ ] Choose the panel (4.2" 400×300 or 5.83" 648×480) and 1-bit or
      4-grey from the mock-ups; tune `pocket/style.go`.
- [ ] Phase 1b: the launcher serves rendered map images to a light page,
      for Pis whose browser cannot run the normal map (no WebGL 2).
- [ ] Panel driver (SPI), buttons, GPS (u-blox NMEA over the serial port),
      a nearest-places screen, menus and panning.
- [ ] Battery life, boot time, an SD card image and an enclosure.

## Decisions to make

- **Name.** "Varu" is a placeholder.
- **Licence.** MIT was chosen as a default. Apache-2.0 (patent grant) and
  GPL-3.0 (keeps derivatives open) are the usual alternatives.
- **Phones.** Settled: the Varu Android app carries the same content, run by
  Varu's own code. Features never depend on other apps being installed.
