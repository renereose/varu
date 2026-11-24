package main

import (
	"errors"
	"fmt"
	"os/exec"
	"strings"
	"syscall"
)

// formatVolume formats the drive as exFAT with PowerShell's Format-Volume.
// That needs an administrator, so Windows asks the user for permission.
func formatVolume(path, label string) (string, error) {
	letter := strings.TrimSuffix(strings.TrimSuffix(path, `\`), ":")
	if len(letter) != 1 {
		return "", errors.New("not a drive letter")
	}
	inner := fmt.Sprintf("Format-Volume -DriveLetter %s -FileSystem exFAT -NewFileSystemLabel %s -Force -Confirm:$false | Out-Null", letter, label)
	outer := "$p = Start-Process powershell -Verb RunAs -Wait -PassThru -WindowStyle Hidden " +
		"-ArgumentList '-NoProfile','-NonInteractive','-Command','" + inner + "'; exit $p.ExitCode"
	cmd := exec.Command("powershell", "-NoProfile", "-NonInteractive", "-Command", outer)
	cmd.SysProcAttr = &syscall.SysProcAttr{HideWindow: true}
	if out, err := cmd.CombinedOutput(); err != nil {
		return "", fmt.Errorf("format: %s", strings.TrimSpace(string(out)+" "+err.Error()))
	}
	if _, fs, _ := volumeInfo(path); !strings.EqualFold(fs, "exFAT") {
		return "", errors.New("the drive was not formatted (was permission refused?)")
	}
	return path, nil
}
