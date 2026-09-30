//go:build darwin

package links

import (
	"net"
	"strings"
	"syscall"
)

const (
	ipBoundIF   = 25  // IP_BOUND_IF
	ipv6BoundIF = 125 // IPV6_BOUND_IF
)

// macOS: IP_BOUND_IF scopes the socket to one interface (scoped routing).
func bind(fd uintptr, network string, l Link) error {
	ifc, err := net.InterfaceByName(l.Name)
	if err != nil {
		return err
	}
	if strings.HasSuffix(network, "6") {
		return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IPV6, ipv6BoundIF, ifc.Index)
	}
	return syscall.SetsockoptInt(int(fd), syscall.IPPROTO_IP, ipBoundIF, ifc.Index)
}
