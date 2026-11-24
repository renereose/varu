package main

import (
	"os"
	"path/filepath"
	"syscall"
)

// From <sys/mount.h>; the syscall package does not export them for darwin.
const (
	mntRdonly     = 0x00000001
	mntLocal      = 0x00001000
	mntDontBrowse = 0x00100000 // hidden from Finder: Recovery and other system volumes
)

// volumes lists writable local disks mounted under /Volumes, which is
// where macOS mounts USB sticks. The startup disk's alias there is skipped.
func volumes() []Volume {
	entries, _ := os.ReadDir("/Volumes")
	var out []Volume
	for _, e := range entries {
		path := filepath.Join("/Volumes", e.Name())
		var st syscall.Statfs_t
		if syscall.Statfs(path, &st) != nil || cstr(st.Mntonname[:]) != path {
			continue
		}
		if st.Flags&(mntRdonly|mntDontBrowse) != 0 || st.Flags&mntLocal == 0 {
			continue
		}
		total := int64(st.Blocks) * int64(st.Bsize)
		out = append(out, Volume{
			Path: path, Label: e.Name(), FS: cstr(st.Fstypename[:]),
			Free: int64(st.Bavail) * int64(st.Bsize), Total: total, Disk: diskSize(path, total),
		})
	}
	return out
}

func diskInfo(path string) (free int64, fs string, err error) {
	var st syscall.Statfs_t
	if err := syscall.Statfs(existingParent(path), &st); err != nil {
		return 0, "", err
	}
	return int64(st.Bavail) * int64(st.Bsize), cstr(st.Fstypename[:]), nil
}

func cstr(b []int8) string {
	out := make([]byte, 0, len(b))
	for _, c := range b {
		if c == 0 {
			break
		}
		out = append(out, byte(c))
	}
	return string(out)
}

func openPath(target string) error { return run("open", target) }

// startVaru opens the drive's start script in Terminal, exactly as a
// double-click in Finder would.
func startVaru(root string) error { return run("open", filepath.Join(root, "Start-macOS.command")) }
