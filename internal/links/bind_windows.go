//go:build windows

package links

import (
	"encoding/binary"
	"net"
	"strings"
	"syscall"
)

const (
	ipUnicastIF   = 31 // IP_UNICAST_IF
	ipv6UnicastIF = 31 // IPV6_UNICAST_IF
)

// Windows: IP_UNICAST_IF picks the outgoing interface. For IPv4 the index is
// passed in network byte order; for IPv6 in host order.
func bind(fd uintptr, network string, l Link) error {
	ifc, err := net.InterfaceByName(l.Name)
	if err != nil {
		return err
	}
	h := syscall.Handle(fd)
	if strings.HasSuffix(network, "6") {
		return syscall.SetsockoptInt(h, syscall.IPPROTO_IPV6, ipv6UnicastIF, ifc.Index)
	}
	var be [4]byte
	binary.BigEndian.PutUint32(be[:], uint32(ifc.Index))
	return syscall.SetsockoptInt(h, syscall.IPPROTO_IP, ipUnicastIF, int(binary.LittleEndian.Uint32(be[:])))
}
