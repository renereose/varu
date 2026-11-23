package main

import (
	"os"
	"os/exec"
	"path/filepath"
)

// Volume is a drive the page offers as a target: in practice a USB stick.
type Volume struct {
	Path  string `json:"path"`
	Label string `json:"label"`
	FS    string `json:"fs"`
	Free  int64  `json:"free"`
	Total int64  `json:"total"`
}

// existingParent walks up to the nearest folder that exists, so free space
// can be measured for a target that has not been created yet.
func existingParent(path string) string {
	for {
		if _, err := os.Stat(path); err == nil {
			return path
		}
		parent := filepath.Dir(path)
		if parent == path {
			return path
		}
		path = parent
	}
}

// run starts a helper program without waiting for it.
func run(name string, args ...string) error {
	cmd := exec.Command(name, args...)
	if err := cmd.Start(); err != nil {
		return err
	}
	go cmd.Wait() //nolint:errcheck // reap it; the result does not matter
	return nil
}
