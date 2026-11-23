package main

import "syscall"

// hide sets the hidden attribute, which Windows and macOS both honour.
func hide(path string) error {
	p, err := syscall.UTF16PtrFromString(path)
	if err != nil {
		return err
	}
	attrs, err := syscall.GetFileAttributes(p)
	if err != nil {
		return err
	}
	return syscall.SetFileAttributes(p, attrs|syscall.FILE_ATTRIBUTE_HIDDEN)
}

func setVolumeIcon(string) {} // a Mac shows .VolumeIcon.icns only when a Mac sets this
