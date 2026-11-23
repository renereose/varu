package main

import (
	"os/exec"
	"syscall"
)

// hide sets the hidden flag. Finder hides the item, and on a FAT or exFAT
// stick it is kept as the hidden attribute that Windows also honours.
func hide(path string) error {
	var st syscall.Stat_t
	if err := syscall.Lstat(path, &st); err != nil {
		return err
	}
	const ufHidden = 0x8000
	return syscall.Chflags(path, int(st.Flags)|ufHidden)
}

// setVolumeIcon tells Finder that the stick has its own icon (.VolumeIcon.icns).
func setVolumeIcon(root string) {
	_ = exec.Command("xattr", "-wx", "com.apple.FinderInfo",
		"0000000000000000040000000000000000000000000000000000000000000000", root).Run()
}
