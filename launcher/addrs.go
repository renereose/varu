package main

import (
	"fmt"
	"net"
	"os"
)

// listen binds the preferred port, falling back to the next few and then
// to any free port, so a second copy or another program never blocks startup.
func listen(host string, preferred int) (net.Listener, error) {
	for p := preferred; p < preferred+20; p++ {
		// IPv4 only: a "tcp" wildcard would also take [::1], where browsers
		// try localhost first, away from the guarded local server.
		ln, err := net.Listen("tcp4", net.JoinHostPort(host, fmt.Sprint(p)))
		if err == nil {
			return ln, nil
		}
	}
	return net.Listen("tcp4", net.JoinHostPort(host, "0"))
}

// lanAddresses lists the IPv4 addresses other devices may reach this one
// on, the outgoing route's first: a VM or container bridge is no use to a phone.
func lanAddresses() []string {
	var ips []net.IP
	if addrs, err := net.InterfaceAddrs(); err == nil {
		for _, a := range addrs {
			if ipnet, ok := a.(*net.IPNet); ok {
				ips = append(ips, ipnet.IP)
			}
		}
	} else {
		ips = netlinkAddrs() // Android 11+
	}
	return pickAddrs(ips, routeAddr())
}

func pickAddrs(ips []net.IP, route string) []string {
	var out []string
	for _, ip := range ips {
		ip4 := ip.To4()
		if ip4 == nil || ip4.IsLoopback() || ip4.IsLinkLocalUnicast() {
			continue
		}
		if s := ip4.String(); s == route {
			out = append([]string{s}, out...)
		} else {
			out = append(out, s)
		}
	}
	if len(out) == 0 && route != "" {
		out = []string{route}
	}
	return out
}

// routeAddr is this device's address on the outgoing route, or "" without
// one (an offline phone hotspot). Connecting a UDP socket sends nothing.
func routeAddr() string {
	conn, err := net.Dial("udp4", "192.168.0.1:9")
	if err != nil {
		return ""
	}
	defer conn.Close()
	ip := conn.LocalAddr().(*net.UDPAddr).IP.To4()
	if ip == nil || ip.IsLoopback() || ip.IsUnspecified() {
		return ""
	}
	return ip.String()
}

func isDir(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.IsDir()
}

func isFile(path string) bool {
	info, err := os.Stat(path)
	return err == nil && info.Mode().IsRegular()
}
