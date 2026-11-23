package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

// A stick cannot start Varu by itself: Windows and macOS refuse to run
// anything from a USB stick when it is plugged in. So a stick shows only
// what to double-click, under Varu's name and icon, and on a GNOME desktop
// (Ubuntu, Fedora) the system offers to run Varu as it is plugged in.

//go:embed stick/varu.ico stick/varu.icns
var stickIcons embed.FS

// stickShown is all that is left in view at the top of a stick.
var stickShown = []string{"Varu-Windows.exe", "Varu-macOS.dmg", "Varu-Linux-x86_64.AppImage", "README.txt"}

// autorunSh is run by GNOME, after asking, when the stick is plugged in.
const autorunSh = `#!/bin/sh
# Starts Varu when this stick is plugged into a Linux desktop that offers to
# run it (GNOME asks first).
cd "$(dirname "$0")" || exit 1
case "$(uname -m)" in
  x86_64 | amd64) [ -f Varu-Linux-x86_64.AppImage ] && exec ./Varu-Linux-x86_64.AppImage ;;
esac
exec sh ./Start-Linux.sh
`

// prepareStick adds the stick's name, icon and start prompt, and hides
// everything at the top but Varu for each system and the README. It is best
// effort: a stick that keeps its clutter in view still works.
func prepareStick(root string) {
	ico, _ := stickIcons.ReadFile("stick/varu.ico")
	icns, _ := stickIcons.ReadFile("stick/varu.icns")
	for name, body := range map[string][]byte{
		// Windows shows this icon and name for the stick; it runs nothing from it.
		"autorun.inf":      []byte("[autorun]\r\nicon=varu.ico\r\nlabel=Varu\r\n"),
		"autorun.sh":       []byte(autorunSh),
		"varu.ico":         ico,
		".VolumeIcon.icns": icns,
	} {
		if err := os.WriteFile(filepath.Join(root, name), body, 0o755); err != nil {
			log.Printf("stick: %v", err)
		}
	}
	entries, err := os.ReadDir(root)
	if err != nil {
		log.Printf("stick: %v", err)
		return
	}
	var hidden []string
	for _, e := range entries {
		name := e.Name()
		if slices.Contains(stickShown, name) || strings.HasPrefix(name, "._") {
			continue
		}
		hidden = append(hidden, name)
		if err := hide(filepath.Join(root, name)); err != nil {
			log.Printf("stick: hiding %s: %v", name, err)
		}
	}
	// Linux file managers hide what this file lists.
	list := filepath.Join(root, ".hidden")
	if err := os.WriteFile(list, []byte(strings.Join(append(hidden, ".hidden"), "\n")+"\n"), 0o644); err == nil {
		_ = hide(list)
	}
	setVolumeIcon(root)
}
