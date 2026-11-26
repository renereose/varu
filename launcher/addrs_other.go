//go:build !linux

package main

import "net"

func netlinkAddrs() []net.IP { return nil }
