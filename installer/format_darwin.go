package main

import (
	"errors"
	"fmt"
	"html"
	"os/exec"
	"regexp"
	"strings"
)

// formatVolume erases the whole external disk the volume is on and makes one
// exFAT partition named label, with an MBR partition map, which every system
// reads. Internal disks are refused.
func formatVolume(path, label string) (string, error) {
	info, err := exec.Command("diskutil", "info", "-plist", path).Output()
	if err != nil {
		return "", fmt.Errorf("diskutil info: %w", err)
	}
	// An APFS volume sits on a container disk; erase the disk under it.
	if store := plistString(info, "APFSPhysicalStore"); store != "" {
		if info, err = exec.Command("diskutil", "info", "-plist", store).Output(); err != nil {
			return "", fmt.Errorf("diskutil info: %w", err)
		}
	}
	disk := plistString(info, "ParentWholeDisk")
	if disk == "" {
		return "", errors.New("could not tell which disk this is")
	}
	if whole, err := exec.Command("diskutil", "info", "-plist", disk).Output(); err != nil || plistBool(whole, "Internal") {
		return "", errors.New("only external disks are formatted")
	}
	if out, err := exec.Command("diskutil", "eraseDisk", "ExFAT", label, "MBR", disk).CombinedOutput(); err != nil {
		return "", fmt.Errorf("diskutil eraseDisk: %s", lastLine(out))
	}
	info, err = exec.Command("diskutil", "info", "-plist", disk+"s1").Output()
	if mount := plistString(info, "MountPoint"); err == nil && mount != "" {
		return mount, nil
	}
	return "", errors.New("formatted, but the stick did not come back; unplug it and plug it in again")
}

func plistString(plist []byte, key string) string {
	m := regexp.MustCompile(`<key>` + regexp.QuoteMeta(key) + `</key>\s*<string>([^<]*)</string>`).FindSubmatch(plist)
	if m == nil {
		return ""
	}
	return html.UnescapeString(string(m[1]))
}

func plistBool(plist []byte, key string) bool {
	return regexp.MustCompile(`<key>` + regexp.QuoteMeta(key) + `</key>\s*<true/>`).Match(plist)
}

func lastLine(out []byte) string {
	lines := strings.Split(strings.TrimSpace(string(out)), "\n")
	return lines[len(lines)-1]
}
