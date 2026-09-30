package netpath

import (
	"testing"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/links"
)

func TestRoutes(t *testing.T) {
	links.SetProvided([]links.Link{
		{ID: "eth0", Name: "eth0", Label: "Ethernet (eth0)", Kind: links.Ethernet},
		{ID: "wlan0", Name: "wlan0", Label: "Wi-Fi (wlan0)", Kind: links.WiFi},
		{ID: "tun0", Name: "tun0", Label: "VPN (tun0)", Kind: links.VPN},
	})
	s := core.Settings{
		Proxies:     []core.ProxyProfile{{ID: "p1", Name: "Office", Type: "http", URL: "http://10.0.0.1:3128"}},
		ProxyBypass: []string{"<local>"},
	}
	r := New(func() core.Settings { return s })
	defer r.Close()

	rs, err := r.Routes("https://example.com/f", "")
	if err != nil || len(rs) != 1 || rs[0].ID != "default" || rs[0].Label != "" {
		t.Fatalf("single direct route expected: %+v %v", rs, err)
	}

	s.MultiLink = true
	rs, _ = r.Routes("https://example.com/f", "")
	if len(rs) != 2 || rs[0].ID != "eth0" || rs[1].ID != "wlan0" {
		t.Fatalf("expected eth0+wlan0 (VPN excluded by default): %+v", rs)
	}

	s.Links = []string{"wlan0", "tun0"}
	rs, _ = r.Routes("https://example.com/f", "p1")
	if len(rs) != 2 || rs[0].Label != "Wi-Fi (wlan0) · Office" {
		t.Fatalf("explicit link selection with proxy label: %+v", rs)
	}

	// Local hosts bypass the proxy.
	s.MultiLink, s.DefaultProxy = false, "p1"
	rs, _ = r.Routes("http://192.168.1.10/f", "")
	if rs[0].Label != "" {
		t.Fatalf("LAN host should go direct: %+v", rs)
	}
	rs, _ = r.Routes("https://example.com/f", "")
	if rs[0].Label != "Office" {
		t.Fatalf("default proxy should apply: %+v", rs)
	}
	if _, err := r.Routes("https://example.com/f", "gone"); err == nil {
		t.Fatal("missing proxy should be an error")
	}
	// Clients are cached per (proxy, link).
	a, _ := r.Routes("https://example.com/a", "")
	b, _ := r.Routes("https://example.com/b", "")
	if a[0].Client != b[0].Client {
		t.Fatal("client should be reused")
	}
}
