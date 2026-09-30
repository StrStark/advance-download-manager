// Package links finds the network connections a device has (Ethernet, Wi-Fi,
// a phone's hotspot, mobile data…) and pins sockets to one of them, so a
// download can use several at once.
package links

import (
	"context"
	"errors"
	"net"
	"sort"
	"strings"
	"sync"
	"syscall"
	"time"
)

// Kind is a coarse link type used for icons and defaults.
type Kind string

const (
	Ethernet Kind = "ethernet"
	WiFi     Kind = "wifi"
	Cellular Kind = "cellular"
	USB      Kind = "usb" // USB tethering
	VPN      Kind = "vpn"
	Other    Kind = "other"
)

// Link is one network connection.
type Link struct {
	ID    string   `json:"id"`    // stable key: the interface name (or "net-<handle>" on Android)
	Name  string   `json:"name"`  // interface name used for binding
	Label string   `json:"label"` // human label, e.g. "Wi-Fi (wlan0)"
	Kind  Kind     `json:"kind"`
	Addrs []string `json:"addrs"`
	// Handle is Android's network handle (Network.getNetworkHandle()).
	Handle int64 `json:"handle,omitempty"`
}

// Provider supplies the current links. Desktop platforms read the OS
// interfaces; the Android app pushes its list in with SetProvided.
var (
	mu       sync.RWMutex
	provided []Link
	usePush  bool
)

// SetProvided replaces the link list with one supplied by the host app
// (Android, where Go cannot enumerate networks itself).
func SetProvided(ls []Link) {
	mu.Lock()
	provided = append([]Link(nil), ls...)
	usePush = true
	mu.Unlock()
}

// List returns the usable links: up, not loopback, with a global unicast
// address. Order is stable (wired first, then Wi-Fi, USB, cellular, others).
func List() []Link {
	mu.RLock()
	if usePush {
		ls := append([]Link(nil), provided...)
		mu.RUnlock()
		sortLinks(ls)
		return ls
	}
	mu.RUnlock()

	ifs, err := net.Interfaces()
	if err != nil {
		return nil
	}
	var out []Link
	for _, ifc := range ifs {
		if ifc.Flags&net.FlagUp == 0 || ifc.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, _ := ifc.Addrs()
		var global []string
		for _, a := range addrs {
			ipn, ok := a.(*net.IPNet)
			if ok && ipn.IP.IsGlobalUnicast() {
				global = append(global, ipn.IP.String())
			}
		}
		if len(global) == 0 {
			continue
		}
		k := classify(ifc.Name, ifc.Flags)
		if k == Other && isVirtual(ifc.Name) {
			continue // docker0, virbr0, veth… never reach the internet directly
		}
		out = append(out, Link{ID: ifc.Name, Name: ifc.Name, Label: label(k, ifc.Name), Kind: k, Addrs: global})
	}
	sortLinks(out)
	return out
}

// Find returns the link with the given ID from the current list.
func Find(id string) (Link, bool) {
	for _, l := range List() {
		if l.ID == id {
			return l, true
		}
	}
	return Link{}, false
}

func sortLinks(ls []Link) {
	rank := map[Kind]int{Ethernet: 0, WiFi: 1, USB: 2, Cellular: 3, VPN: 4, Other: 5}
	sort.SliceStable(ls, func(i, j int) bool {
		if rank[ls[i].Kind] != rank[ls[j].Kind] {
			return rank[ls[i].Kind] < rank[ls[j].Kind]
		}
		return ls[i].ID < ls[j].ID
	})
}

func classify(name string, flags net.Flags) Kind {
	n := strings.ToLower(name)
	switch {
	case strings.HasPrefix(n, "wl"), strings.HasPrefix(n, "wi-fi"), strings.HasPrefix(n, "wifi"), strings.Contains(n, "wireless"), strings.HasPrefix(n, "wlan"):
		return WiFi
	case strings.HasPrefix(n, "usb"), strings.HasPrefix(n, "rndis"), strings.HasPrefix(n, "enx"):
		return USB
	case strings.HasPrefix(n, "wwan"), strings.HasPrefix(n, "ww"), strings.HasPrefix(n, "ppp"), strings.HasPrefix(n, "rmnet"), strings.HasPrefix(n, "ccmni"), strings.Contains(n, "cellular"), strings.Contains(n, "mobile"):
		return Cellular
	case strings.HasPrefix(n, "tun"), strings.HasPrefix(n, "tap"), strings.HasPrefix(n, "utun"), strings.HasPrefix(n, "wg"), strings.HasPrefix(n, "ipsec"), flags&net.FlagPointToPoint != 0:
		return VPN
	case strings.HasPrefix(n, "eth"), strings.HasPrefix(n, "en"), strings.HasPrefix(n, "em"), strings.HasPrefix(n, "ethernet"), strings.HasPrefix(n, "lan"):
		return Ethernet
	}
	return Other
}

func isVirtual(name string) bool {
	for _, p := range []string{"docker", "br-", "veth", "virbr", "vmnet", "vboxnet", "lxc", "lxd", "cni", "flannel", "tailscale", "zt", "awdl", "llw", "anpi", "bridge", "ap"} {
		if strings.HasPrefix(strings.ToLower(name), p) {
			return true
		}
	}
	return false
}

func label(k Kind, name string) string {
	switch k {
	case Ethernet:
		return "Ethernet (" + name + ")"
	case WiFi:
		return "Wi-Fi (" + name + ")"
	case Cellular:
		return "Mobile data (" + name + ")"
	case USB:
		return "USB tethering (" + name + ")"
	case VPN:
		return "VPN (" + name + ")"
	}
	return name
}

// ErrUnsupported is returned when this platform can't pin sockets to a link.
var ErrUnsupported = errors.New("binding to a specific network is not supported on this platform")

// Control returns a net.Dialer Control function that pins the socket to l.
func Control(l Link) func(network, address string, c syscall.RawConn) error {
	return func(network, address string, c syscall.RawConn) error {
		var bindErr error
		err := c.Control(func(fd uintptr) { bindErr = bind(fd, network, l) })
		if err != nil {
			return err
		}
		return bindErr
	}
}

// Dialer returns a dialer whose connections go out through l.
func Dialer(l Link) *net.Dialer {
	return &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second, Control: Control(l)}
}

// Check measures how quickly a TCP connection can be opened through l to
// target (host:port). It is a cheap "does this link reach the internet" test.
func Check(ctx context.Context, l Link, target string) (time.Duration, error) {
	start := time.Now()
	c, err := Dialer(l).DialContext(ctx, "tcp", target)
	if err != nil {
		return 0, err
	}
	c.Close()
	return time.Since(start), nil
}
