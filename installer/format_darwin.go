package main

import (
	"errors"
	"fmt"
	"html"
	"os/exec"
	"regexp"
	"strconv"
	"strings"
	"sync"
)

// formatVolume erases the whole external disk the volume is on and makes one
// exFAT partition named label, with an MBR partition map, which every system
// reads. Internal disks are refused.
func formatVolume(path, label string) (string, error) {
	disk, whole, err := wholeDisk(path)
	if err != nil {
		return "", err
	}
	if plistBool(whole, "Internal") {
		return "", errors.New("only external disks are formatted")
	}
	if out, err := exec.Command("diskutil", "eraseDisk", "ExFAT", label, "MBR", disk).CombinedOutput(); err != nil {
		return "", fmt.Errorf("diskutil eraseDisk: %s", lastLine(out))
	}
	info, err := exec.Command("diskutil", "info", "-plist", disk+"s1").Output()
	if mount := plistString(info, "MountPoint"); err == nil && mount != "" {
		return mount, nil
	}
	return "", errors.New("formatted, but the stick did not come back; unplug it and plug it in again")
}

// wholeDisk finds the disk a volume is on (for an APFS volume, the disk
// under its container) and that disk's diskutil information.
func wholeDisk(path string) (string, []byte, error) {
	info, err := exec.Command("diskutil", "info", "-plist", path).Output()
	if err != nil {
		return "", nil, fmt.Errorf("diskutil info: %w", err)
	}
	if store := plistString(info, "APFSPhysicalStore"); store != "" {
		if info, err = exec.Command("diskutil", "info", "-plist", store).Output(); err != nil {
			return "", nil, fmt.Errorf("diskutil info: %w", err)
		}
	}
	disk := plistString(info, "ParentWholeDisk")
	if disk == "" {
		return "", nil, errors.New("could not tell which disk this is")
	}
	whole, err := exec.Command("diskutil", "info", "-plist", disk).Output()
	if err != nil {
		return "", nil, fmt.Errorf("diskutil info: %w", err)
	}
	return disk, whole, nil
}

// diskSizes remembers each stick's size: diskutil is slow, and the page asks
// for the list every few seconds.
var diskSizes sync.Map

func diskSize(path string, total int64) int64 {
	key := fmt.Sprint(path, total)
	if size, ok := diskSizes.Load(key); ok {
		return size.(int64)
	}
	var size int64
	if _, whole, err := wholeDisk(path); err == nil {
		size, _ = strconv.ParseInt(plistValue(whole, "Size", "integer"), 10, 64)
	}
	diskSizes.Store(key, size)
	return size
}

func plistValue(plist []byte, key, kind string) string {
	m := regexp.MustCompile(`<key>` + regexp.QuoteMeta(key) + `</key>\s*<` + kind + `>([^<]*)</` + kind + `>`).FindSubmatch(plist)
	if m == nil {
		return ""
	}
	return string(m[1])
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
