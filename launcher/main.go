// Varu launcher: a single static binary that turns the drive it lives on
// into a small local website (maps, library, documents) and opens it in
// the default browser. It needs no installation and no network.
package main

import (
	"errors"
	"flag"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"
	"time"
)

// version is overridden at build time with -ldflags "-X main.version=...".
var version = "dev"

const defaultPort = 8480

func main() {
	var (
		rootFlag  = flag.String("root", "", "drive root (the folder that contains app/); auto-detected by default")
		port      = flag.Int("port", defaultPort, "preferred port; the next free one is used if it is taken")
		lan       = flag.Bool("lan", false, "also accept connections from other devices on the local network")
		noBrowser = flag.Bool("no-browser", false, "do not open the browser automatically")
		showVer   = flag.Bool("version", false, "print the version and exit")
	)
	flag.Parse()

	if *showVer {
		fmt.Println("varu", version)
		return
	}
	log.SetFlags(0)

	root, err := resolveRoot(*rootFlag)
	if err != nil {
		fatal(err)
	}

	host := "127.0.0.1"
	if *lan {
		host = "0.0.0.0"
	}
	ln, err := listen(host, *port)
	if err != nil {
		fatal(err)
	}
	actualPort := ln.Addr().(*net.TCPAddr).Port

	cfg := Config{Root: root, LAN: *lan, Version: version}

	srv := &http.Server{
		Handler:           NewHandler(cfg),
		ReadHeaderTimeout: 10 * time.Second,
	}
	go func() {
		if err := srv.Serve(ln); err != nil && !errors.Is(err, http.ErrServerClosed) {
			fatal(err)
		}
	}()

	url := fmt.Sprintf("http://localhost:%d/", actualPort)
	fmt.Println()
	fmt.Println("  Varu is running.")
	fmt.Println("  Open:   ", url)
	if *lan {
		for _, ip := range lanAddresses() {
			fmt.Printf("  Network: http://%s:%d/\n", ip, actualPort)
		}
	}
	fmt.Println("  Keep this window open. Close it (or press Ctrl+C) to stop.")
	fmt.Println()

	if !*noBrowser {
		if err := openBrowser(url); err != nil {
			log.Printf("Could not open the browser automatically (%v). Open the address above by hand.", err)
		}
	}

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	_ = srv.Close()
}

// resolveRoot returns the drive root: the nearest folder at or above the
// executable that contains an app/ directory.
func resolveRoot(explicit string) (string, error) {
	if explicit != "" {
		abs, err := filepath.Abs(explicit)
		if err != nil {
			return "", err
		}
		if !isDir(filepath.Join(abs, "app")) {
			return "", fmt.Errorf("%s does not look like a Varu drive (no app/ folder)", abs)
		}
		return abs, nil
	}

	var starts []string
	if exe, err := os.Executable(); err == nil {
		if real, err := filepath.EvalSymlinks(exe); err == nil {
			exe = real
		}
		starts = append(starts, filepath.Dir(exe))
	}
	if wd, err := os.Getwd(); err == nil {
		starts = append(starts, wd)
	}
	for _, dir := range starts {
		for i := 0; i < 4; i++ {
			if isDir(filepath.Join(dir, "app")) {
				return dir, nil
			}
			parent := filepath.Dir(dir)
			if parent == dir {
				break
			}
			dir = parent
		}
	}
	return "", errors.New("could not find the drive contents (no app/ folder next to the launcher); use -root")
}

// listen binds the preferred port, falling back to the next few and then
// to any free port, so a second copy or another program never blocks startup.
func listen(host string, preferred int) (net.Listener, error) {
	for p := preferred; p < preferred+20; p++ {
		ln, err := net.Listen("tcp", net.JoinHostPort(host, fmt.Sprint(p)))
		if err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp", net.JoinHostPort(host, "0"))
}

func lanAddresses() []string {
	var out []string
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		return out
	}
	for _, a := range addrs {
		ipnet, ok := a.(*net.IPNet)
		if !ok || ipnet.IP.IsLoopback() || ipnet.IP.IsLinkLocalUnicast() {
			continue
		}
		if ip4 := ipnet.IP.To4(); ip4 != nil {
			out = append(out, ip4.String())
		}
	}
	return out
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}

// fatal reports an error and waits briefly so that a double-clicked console
// window does not vanish before the message can be read.
func fatal(err error) {
	fmt.Fprintln(os.Stderr, "Varu could not start:", err)
	time.Sleep(8 * time.Second)
	os.Exit(1)
}
