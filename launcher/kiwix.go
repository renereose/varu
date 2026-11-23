package main

import (
	"errors"
	"fmt"
	"log"
	"net"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"sort"
)

// Kiwix is a running kiwix-serve child process that serves the ZIM
// archives in library/.
type Kiwix struct {
	URL  *url.URL
	cmd  *exec.Cmd
	done chan struct{} // closed once kiwix-serve has exited
}

// startKiwix returns (nil, nil) when the drive simply has no library.
func startKiwix(root string) (*Kiwix, error) {
	zims, _ := filepath.Glob(filepath.Join(root, "library", "*.zim"))
	if len(zims) == 0 {
		return nil, nil
	}
	sort.Strings(zims)

	bin, err := findKiwixServe(root)
	if err != nil {
		return nil, err
	}
	port, err := freePort()
	if err != nil {
		return nil, err
	}

	args := []string{
		"--address=127.0.0.1",
		fmt.Sprintf("--port=%d", port),
		"--urlRootLocation=/library",
		// Nothing on the drive should send the reader to the internet.
		"--blockexternal",
		// Make kiwix-serve exit with us even if we are killed abruptly.
		fmt.Sprintf("--attachToProcess=%d", os.Getpid()),
	}
	args = append(args, zims...)

	cmd := exec.Command(bin, args...)
	cmd.Stderr = os.Stderr
	if err := cmd.Start(); err != nil {
		return nil, fmt.Errorf("starting %s: %w", filepath.Base(bin), err)
	}
	k := &Kiwix{
		URL:  &url.URL{Scheme: "http", Host: fmt.Sprintf("127.0.0.1:%d", port)},
		cmd:  cmd,
		done: make(chan struct{}),
	}
	go func() {
		// Otherwise a reader that dies leaves the library "starting" forever.
		_ = cmd.Wait()
		log.Printf("The library reader stopped (%v)", cmd.ProcessState)
		close(k.done)
	}()
	return k, nil
}

// Stop is safe to call on a nil receiver.
func (k *Kiwix) Stop() {
	if k == nil || k.cmd == nil || k.cmd.Process == nil {
		return
	}
	_ = k.cmd.Process.Kill()
	<-k.done
}

// findKiwixServe looks in bin/<os>-<arch>/ on the drive, then falls back to
// an Intel build on ARM machines that can emulate it, then to PATH (handy
// during development).
func findKiwixServe(root string) (string, error) {
	name := "kiwix-serve"
	if runtime.GOOS == "windows" {
		name += ".exe"
	}
	arches := []string{runtime.GOARCH}
	if runtime.GOARCH == "arm64" && (runtime.GOOS == "darwin" || runtime.GOOS == "windows") {
		arches = append(arches, "amd64")
	}
	for _, arch := range arches {
		candidate := filepath.Join(root, "bin", runtime.GOOS+"-"+arch, name)
		if isFile(candidate) {
			return candidate, nil
		}
	}
	if path, err := exec.LookPath(name); err == nil {
		return path, nil
	}
	return "", errors.New("the library reader (kiwix-serve) for this computer is not on the drive")
}

func freePort() (int, error) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return 0, err
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port, nil
}
