package proxy

import (
	"bytes"
	"context"
	"crypto/ecdsa"
	"crypto/elliptic"
	"crypto/rand"
	"crypto/sha256"
	"crypto/x509"
	"crypto/x509/pkix"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"encoding/pem"
	"fmt"
	"io"
	"math/big"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/StrStark/advance-download-manager/internal/core"

	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
)

func TestParseShareLinks(t *testing.T) {
	vm, _ := json.Marshal(map[string]any{"v": "2", "ps": "My VMess", "add": "vm.example.com", "port": "443", "id": "b831381d-6324-4d53-ad4f-8cda48b30811", "aid": "0", "net": "ws", "path": "/ray", "host": "cdn.example.com", "tls": "tls", "sni": "vm.example.com"})
	ssUser := base64.RawURLEncoding.EncodeToString([]byte("aes-256-gcm:pa ss"))
	cases := []struct {
		link, typ, name, server string
	}{
		{"vmess://" + base64.StdEncoding.EncodeToString(vm), "vmess", "My VMess", "vm.example.com:443"},
		{"vless://b831381d-6324-4d53-ad4f-8cda48b30811@1.2.3.4:443?security=reality&sni=www.microsoft.com&fp=chrome&pbk=Z84J2IelR9ch3k8VtlVhhs5ycBUlXA7wHBWcBrjqnAw&sid=6ba85179e30d4fc2&type=tcp&flow=xtls-rprx-vision#Reality%20DE", "vless", "Reality DE", "1.2.3.4:443"},
		{"vless://id@h.example.com:8443?type=grpc&serviceName=svc&security=tls#gRPC", "vless", "gRPC", "h.example.com:8443"},
		{"vless://id@h.example.com:80?type=xhttp&path=%2Fx&mode=auto", "vless", "Vless h.example.com:80", "h.example.com:80"},
		{"trojan://secret@t.example.com:443?sni=t.example.com#Trojan", "trojan", "Trojan", "t.example.com:443"},
		{"ss://" + ssUser + "@5.6.7.8:8388#SS%20Node", "shadowsocks", "SS Node", "5.6.7.8:8388"},
		{"ss://" + base64.StdEncoding.EncodeToString([]byte("chacha20-ietf-poly1305:pw@9.9.9.9:1234")) + "#legacy", "shadowsocks", "legacy", "9.9.9.9:1234"},
		{"socks5://user:pw@127.0.0.1:1080", "socks5", "SOCKS5 127.0.0.1:1080", "127.0.0.1:1080"},
		{"http://10.0.0.1:3128#Office", "http", "Office", "10.0.0.1:3128"},
	}
	for _, c := range cases {
		p, err := Parse(c.link)
		if err != nil {
			t.Errorf("%s: %v", c.link[:20], err)
			continue
		}
		if p.Type != c.typ || p.Name != c.name || Server(p) != c.server {
			t.Errorf("%s: got type=%q name=%q server=%q", c.link[:20], p.Type, p.Name, Server(p))
		}
		if p.Type != "http" && p.Type != "socks5" {
			// Every generated outbound must be accepted by Xray.
			ob, _, _ := outbound(c.link)
			delete(ob, "_server")
			raw, _ := json.Marshal(map[string]any{"outbounds": []any{ob}})
			cfg, err := serial.DecodeJSONConfig(bytes.NewReader(raw))
			if err == nil {
				_, err = cfg.Build()
			}
			if err != nil {
				t.Errorf("%s: Xray rejected config: %v", c.typ, err)
			}
		}
	}
	for _, bad := range []string{"vless://@host:1", "ss://garbage", "ftp://x", "vmess://notbase64!", "ss://" + ssUser + "@h:1?plugin=obfs"} {
		if _, err := Parse(bad); err == nil {
			t.Errorf("expected error for %q", bad)
		}
	}
}

func TestParseManySubscription(t *testing.T) {
	list := "vless://id@a.example.com:443?security=tls#A\ntrojan://pw@b.example.com:443#B\nnot a link\nss://broken#C\n"
	for _, body := range []string{list, base64.StdEncoding.EncodeToString([]byte(list))} {
		ps, errs := ParseMany(body)
		if len(ps) != 2 || len(errs) != 1 {
			t.Fatalf("got %d profiles, errs=%v", len(ps), errs)
		}
	}
}

func TestBypass(t *testing.T) {
	rules := []string{"localhost", "<local>", "example.com", "*.corp.net", "10.0.0.0/8"}
	yes := []string{"localhost", "192.168.1.5", "printer", "nas.local", "example.com", "dl.example.com", "a.corp.net", "10.2.3.4", "127.0.0.1"}
	no := []string{"github.com", "notexample.com", "8.8.8.8", "corp.network"}
	for _, h := range yes {
		if !Bypass(h, rules) {
			t.Errorf("%s should bypass", h)
		}
	}
	for _, h := range no {
		if Bypass(h, rules) {
			t.Errorf("%s should not bypass", h)
		}
	}
}

// ---------- end to end through real Xray servers ----------

func freePort(t *testing.T) int {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	return ln.Addr().(*net.TCPAddr).Port
}

func selfSigned(t *testing.T) (certPEM, keyPEM, sha string) {
	key, _ := ecdsa.GenerateKey(elliptic.P256(), rand.Reader)
	tpl := &x509.Certificate{
		SerialNumber: big.NewInt(1), Subject: pkix.Name{CommonName: "adm.test"},
		NotBefore: time.Now().Add(-time.Hour), NotAfter: time.Now().Add(time.Hour),
		DNSNames: []string{"adm.test"}, KeyUsage: x509.KeyUsageDigitalSignature, ExtKeyUsage: []x509.ExtKeyUsage{x509.ExtKeyUsageServerAuth},
	}
	der, err := x509.CreateCertificate(rand.Reader, tpl, tpl, &key.PublicKey, key)
	if err != nil {
		t.Fatal(err)
	}
	kb, _ := x509.MarshalECPrivateKey(key)
	sum := sha256.Sum256(der)
	return string(pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: der})),
		string(pem.EncodeToMemory(&pem.Block{Type: "EC PRIVATE KEY", Bytes: kb})), hex.EncodeToString(sum[:])
}

func TestDownloadThroughEveryProtocol(t *testing.T) {
	payload := bytes.Repeat([]byte("ADM!"), 256<<10) // 1 MiB
	origin := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.ServeContent(w, r, "f.bin", time.Now(), bytes.NewReader(payload))
	}))
	defer origin.Close()

	const uuid = "b831381d-6324-4d53-ad4f-8cda48b30811"
	ssKey := base64.StdEncoding.EncodeToString(bytes.Repeat([]byte{7}, 16))
	cert, key, certSHA := selfSigned(t)
	p := map[string]int{"socks": freePort(t), "http": freePort(t), "vless": freePort(t), "vmess": freePort(t), "ss": freePort(t), "trojan": freePort(t)}

	server := map[string]any{
		"log": map[string]any{"loglevel": "none"},
		"inbounds": []any{
			map[string]any{"listen": "127.0.0.1", "port": p["socks"], "protocol": "socks", "settings": map[string]any{"auth": "password", "accounts": []any{map[string]any{"user": "u", "pass": "p"}}}},
			map[string]any{"listen": "127.0.0.1", "port": p["http"], "protocol": "http"},
			map[string]any{"listen": "127.0.0.1", "port": p["vless"], "protocol": "vless", "settings": map[string]any{"clients": []any{map[string]any{"id": uuid}}, "decryption": "none"}},
			map[string]any{"listen": "127.0.0.1", "port": p["vmess"], "protocol": "vmess", "settings": map[string]any{"clients": []any{map[string]any{"id": uuid}}},
				"streamSettings": map[string]any{"network": "ws", "wsSettings": map[string]any{"path": "/ray"}}},
			map[string]any{"listen": "127.0.0.1", "port": p["ss"], "protocol": "shadowsocks", "settings": map[string]any{"method": "2022-blake3-aes-128-gcm", "password": ssKey, "network": "tcp"}},
			map[string]any{"listen": "127.0.0.1", "port": p["trojan"], "protocol": "trojan", "settings": map[string]any{"clients": []any{map[string]any{"password": "tr0jan"}}},
				"streamSettings": map[string]any{"security": "tls", "tlsSettings": map[string]any{"certificates": []any{map[string]any{
					"certificate": strings.Split(strings.TrimSpace(cert), "\n"), "key": strings.Split(strings.TrimSpace(key), "\n")}}}}},
		},
		"outbounds": []any{map[string]any{"protocol": "freedom"}},
	}
	raw, _ := json.Marshal(server)
	cfg, err := serial.DecodeJSONConfig(bytes.NewReader(raw))
	if err != nil {
		t.Fatal(err)
	}
	pb, err := cfg.Build()
	if err != nil {
		t.Fatal(err)
	}
	srv, err := xcore.New(pb)
	if err != nil {
		t.Fatal(err)
	}
	if err := srv.Start(); err != nil {
		t.Fatal(err)
	}
	defer srv.Close()

	vmessJSON, _ := json.Marshal(map[string]any{"v": "2", "ps": "vm", "add": "127.0.0.1", "port": p["vmess"], "id": uuid, "aid": 0, "net": "ws", "path": "/ray", "tls": ""})
	links := map[string]string{
		"socks5":     fmt.Sprintf("socks5://u:p@127.0.0.1:%d", p["socks"]),
		"http":       fmt.Sprintf("http://127.0.0.1:%d", p["http"]),
		"vless":      fmt.Sprintf("vless://%s@127.0.0.1:%d?type=tcp&security=none#v", uuid, p["vless"]),
		"vmess+ws":   "vmess://" + base64.StdEncoding.EncodeToString(vmessJSON),
		"ss-2022":    fmt.Sprintf("ss://%s@127.0.0.1:%d#ss", base64.RawURLEncoding.EncodeToString([]byte("2022-blake3-aes-128-gcm:"+ssKey)), p["ss"]),
		"trojan+tls": fmt.Sprintf("trojan://tr0jan@127.0.0.1:%d?security=tls&sni=adm.test&pcs=%s#t", p["trojan"], certSHA),
	}
	for name, link := range links {
		t.Run(name, func(t *testing.T) {
			prof, err := Parse(link)
			if err != nil {
				t.Fatal(err)
			}
			tr, closer, err := NewTransport(&prof, nil)
			if err != nil {
				t.Fatal(err)
			}
			defer closer.Close()
			c := &http.Client{Transport: tr, Timeout: 20 * time.Second}
			// A ranged request, exactly what download segments send.
			req, _ := http.NewRequestWithContext(context.Background(), http.MethodGet, origin.URL+"/f.bin", nil)
			req.Header.Set("Range", "bytes=1000-1999")
			resp, err := c.Do(req)
			if err != nil {
				t.Fatal(err)
			}
			defer resp.Body.Close()
			got, _ := io.ReadAll(resp.Body)
			if resp.StatusCode != http.StatusPartialContent || !bytes.Equal(got, payload[1000:2000]) {
				t.Fatalf("status %d, %d bytes", resp.StatusCode, len(got))
			}
		})
	}
}

var _ = core.ProxyProfile{}
