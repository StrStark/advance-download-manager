package links

import (
	"context"
	"net"
	"runtime"
	"testing"
	"time"
)

func TestClassify(t *testing.T) {
	cases := map[string]Kind{
		"eth0": Ethernet, "enp3s0": Ethernet, "Ethernet 2": Ethernet,
		"wlan0": WiFi, "wlp2s0": WiFi, "Wi-Fi": WiFi,
		"usb0": USB, "enx00e04c680001": USB,
		"wwan0": Cellular, "rmnet_data0": Cellular,
		"tun0": VPN, "wg0": VPN, "utun3": VPN,
	}
	for name, want := range cases {
		if got := classify(name, net.FlagUp); got != want {
			t.Errorf("classify(%q) = %s, want %s", name, got, want)
		}
	}
}

func TestProvidedList(t *testing.T) {
	defer func() { mu.Lock(); usePush = false; provided = nil; mu.Unlock() }()
	SetProvided([]Link{{ID: "net-2", Kind: Cellular}, {ID: "net-1", Kind: WiFi}})
	ls := List()
	if len(ls) != 2 || ls[0].ID != "net-1" {
		t.Fatalf("got %+v", ls)
	}
}

// Binding to the loopback device must work for loopback targets and a
// socket bound to another interface must not reach 127.0.0.1. This proves
// the pinning really applies at the socket level.
func TestBindingIsEnforced(t *testing.T) {
	if runtime.GOOS != "linux" {
		t.Skip("socket binding test runs on Linux")
	}
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	go func() {
		for {
			c, err := ln.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	ctx, cancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer cancel()
	if _, err := Check(ctx, Link{Name: "lo"}, ln.Addr().String()); err != nil {
		t.Fatalf("bound to lo: %v", err)
	}
	others := List()
	if len(others) == 0 {
		t.Skip("no non-loopback interface to test against")
	}
	short, cancel2 := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel2()
	if _, err := Check(short, others[0], ln.Addr().String()); err == nil {
		t.Fatalf("socket bound to %s reached loopback; binding not enforced", others[0].Name)
	}
}
