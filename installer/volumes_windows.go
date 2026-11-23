package main

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"syscall"
	"unsafe"
)

var (
	kernel32             = syscall.NewLazyDLL("kernel32.dll")
	getLogicalDrives     = kernel32.NewProc("GetLogicalDrives")
	getDriveType         = kernel32.NewProc("GetDriveTypeW")
	getVolumeInformation = kernel32.NewProc("GetVolumeInformationW")
	getDiskFreeSpaceEx   = kernel32.NewProc("GetDiskFreeSpaceExW")
)

const (
	driveRemovable     = 2
	driveFixed         = 3 // USB hard drives and some fast sticks report as fixed
	fileReadOnlyVolume = 0x00080000
	createNewConsole   = 0x00000010
)

// volumes lists drive letters other than the system drive that are
// writable and have media in them.
func volumes() []Volume {
	mask, _, _ := getLogicalDrives.Call()
	system := strings.ToUpper(os.Getenv("SystemDrive"))
	var out []Volume
	for i := 0; i < 26; i++ {
		if mask&(1<<i) == 0 {
			continue
		}
		root := string(rune('A'+i)) + `:\`
		if root[:2] == system {
			continue
		}
		kind, _, _ := getDriveType.Call(uintptr(unsafe.Pointer(utf16(root))))
		if kind != driveRemovable && kind != driveFixed {
			continue
		}
		label, fs, readOnly := volumeInfo(root)
		if fs == "" || readOnly {
			continue // empty card reader, or a write-protected stick
		}
		free, total := diskFree(root)
		if label == "" {
			label = "USB"
		}
		out = append(out, Volume{Path: root, Label: root[:2] + " " + label, FS: fs, Free: free, Total: total})
	}
	return out
}

func diskInfo(path string) (free int64, fs string, err error) {
	dir := existingParent(path)
	free, _ = diskFree(dir)
	_, fs, _ = volumeInfo(filepath.VolumeName(dir) + `\`)
	return free, fs, nil
}

func volumeInfo(root string) (label, fs string, readOnly bool) {
	var name, fsName [261]uint16
	var flags uint32
	ok, _, _ := getVolumeInformation.Call(
		uintptr(unsafe.Pointer(utf16(root))),
		uintptr(unsafe.Pointer(&name[0])), uintptr(len(name)),
		0, 0, uintptr(unsafe.Pointer(&flags)),
		uintptr(unsafe.Pointer(&fsName[0])), uintptr(len(fsName)),
	)
	if ok == 0 {
		return "", "", false
	}
	return syscall.UTF16ToString(name[:]), syscall.UTF16ToString(fsName[:]), flags&fileReadOnlyVolume != 0
}

func diskFree(dir string) (free, total int64) {
	var avail, size, totalFree uint64
	getDiskFreeSpaceEx.Call(uintptr(unsafe.Pointer(utf16(dir))), //nolint:errcheck
		uintptr(unsafe.Pointer(&avail)), uintptr(unsafe.Pointer(&size)), uintptr(unsafe.Pointer(&totalFree)))
	return int64(avail), int64(size)
}

func utf16(s string) *uint16 {
	p, _ := syscall.UTF16PtrFromString(s)
	return p
}

// openPath hands a URL or folder to Windows, as a double-click would.
func openPath(target string) error {
	if strings.Contains(target, "://") {
		return run("rundll32", "url.dll,FileProtocolHandler", target)
	}
	return run("explorer.exe", target)
}

// startVaru runs the launcher in a console window of its own, which is
// what Start-Windows.bat does.
func startVaru(root string) error {
	cmd := exec.Command(filepath.Join(root, "bin", "windows-amd64", "varu.exe"), "-root", root)
	cmd.SysProcAttr = &syscall.SysProcAttr{CreationFlags: createNewConsole}
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck
	return nil
}
