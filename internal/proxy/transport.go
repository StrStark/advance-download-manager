package proxy

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/links"
)

// System is a pseudo-profile that uses the environment's proxy settings.
var System = &core.ProxyProfile{ID: "system", Name: "System proxy", Type: "system"}

// NewTransport returns an HTTP transport that goes out through the given
// link (nil = OS default route) and proxy (nil = direct). The closer must be
// called when the transport is no longer used.
func NewTransport(p *core.ProxyProfile, l *links.Link) (*http.Transport, io.Closer, error) {
	d := &net.Dialer{Timeout: 20 * time.Second, KeepAlive: 30 * time.Second}
	if l != nil {
		d.Control = links.Control(*l)
	}
	t := &http.Transport{
		DialContext:           d.DialContext,
		ForceAttemptHTTP2:     false, // separate TCP connections are the point of segmenting
		MaxIdleConns:          128,
		MaxIdleConnsPerHost:   32,
		IdleConnTimeout:       60 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ResponseHeaderTimeout: 30 * time.Second,
	}
	var closer io.Closer = nopCloser{}
	switch {
	case p == nil:
	case p.Type == "system":
		t.Proxy = http.ProxyFromEnvironment
	case p.Type == "http" || p.Type == "https" || p.Type == "socks5":
		u, err := url.Parse(p.URL)
		if err != nil {
			return nil, nil, err
		}
		t.Proxy = http.ProxyURL(u)
	default:
		xd, err := newXrayDialer(p, l)
		if err != nil {
			return nil, nil, err
		}
		t.DialContext = xd.DialContext
		closer = xd
	}
	return t, closer, nil
}

type nopCloser struct{}

func (nopCloser) Close() error { return nil }

// TestResult reports whether a proxy (and link) works.
type TestResult struct {
	OK        bool   `json:"ok"`
	LatencyMs int64  `json:"latencyMs"`
	IP        string `json:"ip,omitempty"`
	Error     string `json:"error,omitempty"`
}

// TraceURL answers with "ip=<your address>" and is reachable in most places.
const TraceURL = "https://cloudflare.com/cdn-cgi/trace"

// Test fetches TraceURL through the proxy and reports latency and exit IP.
func Test(ctx context.Context, p *core.ProxyProfile, l *links.Link) TestResult {
	t, closer, err := NewTransport(p, l)
	if err != nil {
		return TestResult{Error: err.Error()}
	}
	defer closer.Close()
	defer t.CloseIdleConnections()
	ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
	defer cancel()
	req, _ := http.NewRequestWithContext(ctx, http.MethodGet, TraceURL, nil)
	start := time.Now()
	resp, err := (&http.Client{Transport: t}).Do(req)
	if err != nil {
		return TestResult{Error: cleanErr(err)}
	}
	defer resp.Body.Close()
	lat := time.Since(start).Milliseconds()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	res := TestResult{OK: resp.StatusCode < 500, LatencyMs: lat}
	for _, line := range strings.Split(string(body), "\n") {
		if ip, ok := strings.CutPrefix(line, "ip="); ok {
			res.IP = strings.TrimSpace(ip)
		}
	}
	if !res.OK {
		res.Error = resp.Status
	}
	return res
}

func cleanErr(err error) string {
	var ue *url.Error
	if errors.As(err, &ue) {
		err = ue.Err
	}
	if errors.Is(err, context.DeadlineExceeded) {
		return "timed out"
	}
	return err.Error()
}

// FetchSubscription downloads a subscription and parses its links. client
// decides the route (direct, or through an existing proxy).
func FetchSubscription(ctx context.Context, client *http.Client, subURL string) ([]core.ProxyProfile, []string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, subURL, nil)
	if err != nil {
		return nil, nil, err
	}
	// Many panels only return the plain base64 list to v2ray-style clients.
	req.Header.Set("User-Agent", "v2rayN/7.0 (ADM)")
	resp, err := client.Do(req)
	if err != nil {
		return nil, nil, errors.New(cleanErr(err))
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, nil, fmt.Errorf("subscription returned %s", resp.Status)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, 8<<20))
	if err != nil {
		return nil, nil, err
	}
	profiles, errs := ParseMany(string(body))
	if len(profiles) == 0 {
		if len(errs) > 0 {
			return nil, errs, errors.New("no usable servers in subscription")
		}
		return nil, nil, errors.New("subscription is empty or in an unsupported format (Clash/sing-box configs are not supported)")
	}
	return profiles, errs, nil
}

// Bypass reports whether host should skip the proxy. Rules: "localhost",
// "<local>" (private and single-label addresses), exact hosts, domain
// suffixes ("example.com" also matches "a.example.com", "*.x" works too),
// IPs and CIDR ranges.
func Bypass(host string, rules []string) bool {
	host = strings.ToLower(strings.Trim(host, "[]"))
	ip := net.ParseIP(host)
	for _, r := range rules {
		r = strings.ToLower(strings.TrimSpace(r))
		switch {
		case r == "":
		case r == "<local>":
			if (ip != nil && (ip.IsPrivate() || ip.IsLoopback() || ip.IsLinkLocalUnicast())) ||
				(ip == nil && !strings.Contains(host, ".")) || strings.HasSuffix(host, ".local") {
				return true
			}
		case strings.Contains(r, "/"):
			if _, n, err := net.ParseCIDR(r); err == nil && ip != nil && n.Contains(ip) {
				return true
			}
		default:
			r = strings.TrimPrefix(strings.TrimPrefix(r, "*"), ".")
			if host == r || strings.HasSuffix(host, "."+r) {
				return true
			}
		}
	}
	return false
}
