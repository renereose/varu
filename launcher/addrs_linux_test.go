package main

import (
	"net"
	"slices"
	"testing"
)

func TestNetlinkAddrs(t *testing.T) {
	got := netlinkAddrs()
	addrs, err := net.InterfaceAddrs()
	if err != nil {
		t.Skip(err)
	}
	for _, a := range addrs {
		ip := a.(*net.IPNet).IP.To4()
		if ip != nil && !slices.ContainsFunc(got, ip.Equal) {
			t.Errorf("%s missing from %v", ip, got)
		}
	}
}
