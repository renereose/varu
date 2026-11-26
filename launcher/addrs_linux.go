package main

import (
	"encoding/binary"
	"net"
	"syscall"
)

// netlinkAddrs asks the kernel for the IPv4 addresses directly, so a phone's
// own hotspot is found too. Android 11+ refuses apps the interface list that
// net.InterfaceAddrs reads first, and bind() on these sockets
// (golang/go#40569), but not an address dump on an unbound socket.
func netlinkAddrs() []net.IP {
	fd, err := syscall.Socket(syscall.AF_NETLINK, syscall.SOCK_RAW|syscall.SOCK_CLOEXEC, syscall.NETLINK_ROUTE)
	if err != nil {
		return nil
	}
	defer syscall.Close(fd)
	_ = syscall.SetsockoptTimeval(fd, syscall.SOL_SOCKET, syscall.SO_RCVTIMEO, &syscall.Timeval{Sec: 2})

	// A netlink header and an ifaddrmsg asking for every IPv4 address.
	req := make([]byte, syscall.NLMSG_HDRLEN+syscall.SizeofIfAddrmsg)
	binary.NativeEndian.PutUint32(req[0:], uint32(len(req)))
	binary.NativeEndian.PutUint16(req[4:], syscall.RTM_GETADDR)
	binary.NativeEndian.PutUint16(req[6:], syscall.NLM_F_REQUEST|syscall.NLM_F_DUMP)
	binary.NativeEndian.PutUint32(req[8:], 1)
	req[syscall.NLMSG_HDRLEN] = syscall.AF_INET
	if err := syscall.Sendto(fd, req, 0, &syscall.SockaddrNetlink{Family: syscall.AF_NETLINK}); err != nil {
		return nil
	}

	var out []net.IP
	buf := make([]byte, 1<<16)
	for {
		n, _, err := syscall.Recvfrom(fd, buf, 0)
		if err != nil {
			return out
		}
		msgs, err := syscall.ParseNetlinkMessage(buf[:n])
		if err != nil {
			return out
		}
		for i := range msgs {
			switch msgs[i].Header.Type {
			case syscall.NLMSG_DONE, syscall.NLMSG_ERROR:
				return out
			case syscall.RTM_NEWADDR:
				if ip := addrOf(&msgs[i]); ip != nil {
					out = append(out, ip)
				}
			}
		}
	}
}

// addrOf is this end's address in an RTM_NEWADDR message: IFA_LOCAL, which
// differs from IFA_ADDRESS only on a point-to-point link.
func addrOf(m *syscall.NetlinkMessage) net.IP {
	attrs, err := syscall.ParseNetlinkRouteAttr(m)
	if err != nil {
		return nil
	}
	var ip net.IP
	for _, a := range attrs {
		if len(a.Value) == 4 && (a.Attr.Type == syscall.IFA_LOCAL || a.Attr.Type == syscall.IFA_ADDRESS && ip == nil) {
			ip = net.IP(append([]byte(nil), a.Value...))
		}
	}
	return ip
}
