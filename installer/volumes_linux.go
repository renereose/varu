package main

import "syscall"

// ponytail: no USB listing on Linux (no Linux installer is released); the
// page still accepts any folder, such as /media/<user>/<stick>.
func volumes() []Volume { return nil }

func diskInfo(path string) (free int64, fs string, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(existingParent(path), &st); err != nil {
		return 0, "", err
	}
	switch st.Type {
	case 0x4d44:
		fs = "vfat"
	case 0x2011bab0:
		fs = "exfat"
	}
	return int64(st.Bavail) * st.Bsize, fs, nil
}

func openPath(target string) error { return run("xdg-open", target) }
func startVaru(root string) error  { return openPath(root) }
