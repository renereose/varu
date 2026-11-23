package main

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
)

// volumes lists writable drives the desktop has mounted, which it does under
// /media/<user> or /run/media/<user>: in practice USB sticks.
func volumes() []Volume {
	var out []Volume
	for _, m := range mounts() {
		if !strings.HasPrefix(m.dir, "/media/") && !strings.HasPrefix(m.dir, "/run/media/") || !m.rw {
			continue
		}
		var st syscall.Statfs_t
		if syscall.Statfs(m.dir, &st) != nil {
			continue
		}
		out = append(out, Volume{Path: m.dir, Label: filepath.Base(m.dir), FS: m.fs,
			Free: int64(st.Bavail) * int64(st.Bsize), Total: int64(st.Blocks) * int64(st.Bsize), Disk: diskOf(m.dev)})
	}
	return out
}

func diskInfo(path string) (free int64, fs string, err error) {
	var st syscall.Statfs_t
	dir := existingParent(path)
	if err := syscall.Statfs(dir, &st); err != nil {
		return 0, "", err
	}
	return int64(st.Bavail) * int64(st.Bsize), mountOf(dir).fs, nil
}

// diskOf is the size of the whole disk a partition such as /dev/sdb1 is on.
func diskOf(dev string) int64 {
	part, err := filepath.EvalSymlinks("/sys/class/block/" + filepath.Base(dev))
	if err != nil {
		return 0
	}
	disk := part
	if _, err := os.Stat(filepath.Join(part, "partition")); err == nil {
		disk = filepath.Dir(part)
	}
	raw, err := os.ReadFile(filepath.Join(disk, "size"))
	sectors, _ := strconv.ParseInt(strings.TrimSpace(string(raw)), 10, 64)
	if err != nil {
		return 0
	}
	return sectors * 512
}

type mount struct {
	dev, dir, fs string
	rw           bool
}

func mounts() []mount {
	raw, _ := os.ReadFile("/proc/self/mounts")
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	var out []mount
	for _, line := range strings.Split(string(raw), "\n") {
		f := strings.Fields(line)
		if len(f) < 4 {
			continue
		}
		fs := f[2]
		if fs == "fuseblk" {
			fs = "ntfs" // ntfs-3g
		}
		rw := false
		for _, o := range strings.Split(f[3], ",") {
			rw = rw || o == "rw"
		}
		out = append(out, mount{unescape.Replace(f[0]), unescape.Replace(f[1]), fs, rw})
	}
	return out
}

// mountOf is the mount a path is on: the longest mount point above it.
func mountOf(path string) mount {
	var best mount
	for _, m := range mounts() {
		if (path == m.dir || strings.HasPrefix(path, strings.TrimSuffix(m.dir, "/")+"/")) && len(m.dir) >= len(best.dir) {
			best = m
		}
	}
	return best
}

// formatVolume formats the stick as exFAT through UDisks, as the desktop's
// own disk tools do; it may ask for a password.
func formatVolume(path, label string) (string, error) {
	m := mountOf(path)
	if m.dir != path || !strings.HasPrefix(m.dev, "/dev/") {
		return "", errors.New("not a mounted USB stick")
	}
	if out, err := exec.Command("udisksctl", "unmount", "-b", m.dev).CombinedOutput(); err != nil {
		return "", fmt.Errorf("unmount: %s", strings.TrimSpace(string(out)))
	}
	object := "/org/freedesktop/UDisks2/block_devices/" + strings.TrimPrefix(m.dev, "/dev/")
	out, err := exec.Command("gdbus", "call", "--system", "--timeout", "600", "--dest", "org.freedesktop.UDisks2",
		"--object-path", object, "--method", "org.freedesktop.UDisks2.Block.Format", "exfat",
		fmt.Sprintf("{'label': <'%s'>, 'update-partition-type': <true>}", label)).CombinedOutput()
	if err != nil {
		_ = exec.Command("udisksctl", "mount", "-b", m.dev).Run()
		return "", fmt.Errorf("format (is exfatprogs installed?): %s", strings.TrimSpace(string(out)))
	}
	out, err = exec.Command("udisksctl", "mount", "-b", m.dev).CombinedOutput()
	// "Mounted /dev/sdb1 at /media/ann/VARU"
	if _, at, ok := strings.Cut(strings.TrimSpace(string(out)), " at "); err == nil && ok {
		return strings.TrimSuffix(at, "."), nil
	}
	return "", errors.New("formatted, but the stick did not come back; unplug it and plug it in again")
}

func openPath(target string) error { return run("xdg-open", target) }
func startVaru(root string) error  { return openPath(root) }
