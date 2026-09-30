// Package proxy turns proxy profiles into HTTP transports. HTTP(S) and SOCKS5
// proxies use Go's own client; V2Ray/Xray protocols (VMess, VLESS, Trojan,
// Shadowsocks, with TLS/REALITY and WebSocket/gRPC/HTTPUpgrade/XHTTP
// transports) run through an embedded Xray-core.
package proxy

import (
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/url"
	"strconv"
	"strings"

	"github.com/StrStark/advance-download-manager/internal/core"
)

// Parse reads one proxy URL or share link into a profile (without an ID).
func Parse(link string) (core.ProxyProfile, error) {
	link = strings.TrimSpace(link)
	scheme, _, ok := strings.Cut(link, "://")
	if !ok {
		return core.ProxyProfile{}, errors.New("not a proxy link")
	}
	p := core.ProxyProfile{URL: link}
	switch strings.ToLower(scheme) {
	case "http", "https", "socks5", "socks5h", "socks":
		u, err := url.Parse(link)
		if err != nil || u.Hostname() == "" || u.Port() == "" {
			return p, errors.New("proxy URL needs a host and port, e.g. socks5://127.0.0.1:1080")
		}
		p.Type = strings.ToLower(scheme)
		if p.Type == "socks" || p.Type == "socks5h" {
			p.Type = "socks5"
			u.Scheme = "socks5"
			p.URL = u.String()
		}
		p.Name = u.Fragment
		if p.Name == "" {
			p.Name = strings.ToUpper(p.Type) + " " + u.Host
		}
		p.Server = u.Host
		return p, nil
	case "vmess", "vless", "trojan", "ss":
		ob, name, err := outbound(link)
		if err != nil {
			return p, err
		}
		p.Type = ob["protocol"].(string)
		p.Server = ob["_server"].(string)
		p.Name = name
		if p.Name == "" {
			p.Name = strings.ToUpper(p.Type[:1]) + p.Type[1:] + " " + ob["_server"].(string)
		}
		return p, nil
	}
	return p, fmt.Errorf("unsupported proxy type %q", scheme)
}

// ParseMany reads a block of text with one link per line. It also accepts a
// base64-encoded block (the usual subscription format). Invalid lines are
// reported but don't stop the rest.
func ParseMany(text string) ([]core.ProxyProfile, []string) {
	text = strings.TrimSpace(text)
	if !strings.Contains(text, "://") {
		if dec, ok := decodeBase64(text); ok && strings.Contains(dec, "://") {
			text = dec
		}
	}
	var out []core.ProxyProfile
	var errs []string
	for _, line := range strings.FieldsFunc(text, func(r rune) bool { return r == '\n' || r == '\r' || r == ' ' }) {
		if !strings.Contains(line, "://") {
			continue
		}
		p, err := Parse(line)
		if err != nil {
			errs = append(errs, shorten(line)+": "+err.Error())
			continue
		}
		out = append(out, p)
	}
	return out, errs
}

func shorten(s string) string {
	if len(s) > 48 {
		return s[:45] + "…"
	}
	return s
}

func decodeBase64(s string) (string, bool) {
	s = strings.Map(func(r rune) rune {
		if r == '\n' || r == '\r' || r == ' ' || r == '\t' {
			return -1
		}
		return r
	}, s)
	for _, enc := range []*base64.Encoding{base64.StdEncoding, base64.RawStdEncoding, base64.URLEncoding, base64.RawURLEncoding} {
		if b, err := enc.DecodeString(s); err == nil {
			return string(b), true
		}
	}
	return "", false
}

// Server returns "host:port" of the proxy server, for display.
func Server(p core.ProxyProfile) string {
	switch p.Type {
	case "http", "https", "socks5":
		if u, err := url.Parse(p.URL); err == nil {
			return u.Host
		}
	default:
		if ob, _, err := outbound(p.URL); err == nil {
			return ob["_server"].(string)
		}
	}
	return ""
}

// ---------- share links → Xray outbound ----------

// outbound converts a share link into an Xray outbound object. The returned
// map has a private "_server" key (host:port) that callers strip before use.
func outbound(link string) (map[string]any, string, error) {
	scheme, rest, _ := strings.Cut(link, "://")
	switch strings.ToLower(scheme) {
	case "vmess":
		return vmess(rest)
	case "vless", "trojan":
		return uriOutbound(strings.ToLower(scheme), link)
	case "ss":
		return shadowsocks(link)
	}
	return nil, "", fmt.Errorf("unsupported share link %q", scheme)
}

// vmess:// is base64 of a JSON object (the "v2rayN" format).
func vmess(b64 string) (map[string]any, string, error) {
	if i := strings.IndexByte(b64, '#'); i >= 0 {
		b64 = b64[:i]
	}
	raw, ok := decodeBase64(b64)
	if !ok {
		return nil, "", errors.New("vmess link is not valid base64")
	}
	var v map[string]any
	if err := json.Unmarshal([]byte(raw), &v); err != nil {
		return nil, "", errors.New("vmess link has invalid JSON")
	}
	str := func(k string) string {
		switch x := v[k].(type) {
		case string:
			return x
		case float64:
			return strconv.FormatFloat(x, 'f', -1, 64)
		}
		return ""
	}
	host, id := str("add"), str("id")
	port, _ := strconv.Atoi(str("port"))
	if host == "" || port == 0 || id == "" {
		return nil, "", errors.New("vmess link is missing address, port or id")
	}
	aid, _ := strconv.Atoi(str("aid"))
	sec := str("scy")
	if sec == "" {
		sec = "auto"
	}
	security := str("tls")
	q := url.Values{}
	for _, k := range []string{"host", "path", "sni", "alpn", "fp"} {
		if s := str(k); s != "" {
			q.Set(k, s)
		}
	}
	q.Set("headerType", str("type"))
	if str("net") == "grpc" {
		q.Set("serviceName", str("path"))
		q.Set("mode", str("type"))
	}
	stream, err := streamSettings(str("net"), security, q, host)
	if err != nil {
		return nil, "", err
	}
	ob := map[string]any{
		"protocol": "vmess",
		"settings": map[string]any{"vnext": []any{map[string]any{
			"address": host, "port": port,
			"users": []any{map[string]any{"id": id, "alterId": aid, "security": sec}},
		}}},
		"streamSettings": stream,
		"_server":        net.JoinHostPort(host, strconv.Itoa(port)),
	}
	return ob, str("ps"), nil
}

// vless://uuid@host:port?… and trojan://password@host:port?…
func uriOutbound(proto, link string) (map[string]any, string, error) {
	u, err := url.Parse(link)
	if err != nil {
		return nil, "", fmt.Errorf("%s link: %v", proto, err)
	}
	host := u.Hostname()
	port, _ := strconv.Atoi(u.Port())
	secret := u.User.Username()
	if host == "" || port == 0 || secret == "" {
		return nil, "", fmt.Errorf("%s link is missing host, port or credentials", proto)
	}
	q := u.Query()
	security := q.Get("security")
	if proto == "trojan" && security == "" {
		security = "tls"
	}
	stream, err := streamSettings(q.Get("type"), security, q, host)
	if err != nil {
		return nil, "", err
	}
	var settings map[string]any
	if proto == "vless" {
		user := map[string]any{"id": secret, "encryption": "none"}
		if enc := q.Get("encryption"); enc != "" {
			user["encryption"] = enc
		}
		if flow := q.Get("flow"); flow != "" {
			user["flow"] = flow
		}
		settings = map[string]any{"vnext": []any{map[string]any{"address": host, "port": port, "users": []any{user}}}}
	} else {
		settings = map[string]any{"servers": []any{map[string]any{"address": host, "port": port, "password": secret}}}
	}
	ob := map[string]any{
		"protocol":       proto,
		"settings":       settings,
		"streamSettings": stream,
		"_server":        net.JoinHostPort(host, strconv.Itoa(port)),
	}
	return ob, u.Fragment, nil
}

// ss://base64(method:password)@host:port#name (SIP002), or the legacy
// ss://base64(method:password@host:port)#name.
func shadowsocks(link string) (map[string]any, string, error) {
	rest := strings.TrimPrefix(link[strings.Index(link, "://")+3:], "")
	name := ""
	if i := strings.IndexByte(rest, '#'); i >= 0 {
		name, _ = url.PathUnescape(rest[i+1:])
		rest = rest[:i]
	}
	if i := strings.IndexByte(rest, '?'); i >= 0 {
		q, _ := url.ParseQuery(rest[i+1:])
		if q.Get("plugin") != "" {
			return nil, "", errors.New("shadowsocks plugins (obfs, v2ray-plugin) are not supported")
		}
		rest = rest[:i]
	}
	rest = strings.TrimSuffix(rest, "/")
	var userinfo, hostport string
	if at := strings.LastIndexByte(rest, '@'); at >= 0 {
		userinfo, hostport = rest[:at], rest[at+1:]
		if dec, ok := decodeBase64(userinfo); ok && strings.Contains(dec, ":") {
			userinfo = dec
		} else if un, err := url.PathUnescape(userinfo); err == nil {
			userinfo = un
		}
	} else {
		dec, ok := decodeBase64(rest)
		if !ok {
			return nil, "", errors.New("shadowsocks link is not valid")
		}
		at := strings.LastIndexByte(dec, '@')
		if at < 0 {
			return nil, "", errors.New("shadowsocks link is not valid")
		}
		userinfo, hostport = dec[:at], dec[at+1:]
	}
	method, password, ok := strings.Cut(userinfo, ":")
	host, portStr, err := net.SplitHostPort(hostport)
	port, _ := strconv.Atoi(portStr)
	if !ok || err != nil || port == 0 || method == "" {
		return nil, "", errors.New("shadowsocks link is missing method, password, host or port")
	}
	ob := map[string]any{
		"protocol": "shadowsocks",
		"settings": map[string]any{"servers": []any{map[string]any{
			"address": host, "port": port, "method": strings.ToLower(method), "password": password,
		}}},
		"streamSettings": map[string]any{"network": "raw"},
		"_server":        net.JoinHostPort(host, strconv.Itoa(port)),
	}
	return ob, name, nil
}

// streamSettings builds Xray's transport + security section from the common
// share-link query parameters.
func streamSettings(network, security string, q url.Values, host string) (map[string]any, error) {
	s := map[string]any{}
	switch network {
	case "", "tcp", "raw":
		s["network"] = "raw"
		if q.Get("headerType") == "http" {
			req := map[string]any{"path": []string{orDefault(q.Get("path"), "/")}}
			if h := q.Get("host"); h != "" {
				req["headers"] = map[string]any{"Host": strings.Split(h, ",")}
			}
			s["rawSettings"] = map[string]any{"header": map[string]any{"type": "http", "request": req}}
		}
	case "ws":
		s["network"] = "ws"
		s["wsSettings"] = map[string]any{"path": orDefault(q.Get("path"), "/"), "host": q.Get("host")}
	case "grpc":
		s["network"] = "grpc"
		s["grpcSettings"] = map[string]any{"serviceName": q.Get("serviceName"), "multiMode": q.Get("mode") == "multi"}
	case "httpupgrade":
		s["network"] = "httpupgrade"
		s["httpupgradeSettings"] = map[string]any{"path": orDefault(q.Get("path"), "/"), "host": q.Get("host")}
	case "xhttp", "splithttp":
		s["network"] = "xhttp"
		x := map[string]any{"path": orDefault(q.Get("path"), "/"), "host": q.Get("host")}
		if m := q.Get("mode"); m != "" {
			x["mode"] = m
		}
		s["xhttpSettings"] = x
	default:
		return nil, fmt.Errorf("transport %q is not supported", network)
	}

	sni := q.Get("sni")
	if sni == "" {
		sni = q.Get("peer")
	}
	switch security {
	case "", "none":
	case "tls":
		t := map[string]any{"serverName": orDefault(sni, orDefault(q.Get("host"), host))}
		if fp := q.Get("fp"); fp != "" {
			t["fingerprint"] = fp
		}
		if alpn := q.Get("alpn"); alpn != "" {
			t["alpn"] = strings.Split(alpn, ",")
		}
		// Xray retired "allowInsecure"; self-signed servers are trusted by
		// pinning their certificate hash (pcs) or verifying another name (vcn).
		if pcs := q.Get("pcs"); pcs != "" {
			t["pinnedPeerCertSha256"] = pcs
		}
		if vcn := q.Get("vcn"); vcn != "" {
			t["verifyPeerCertByName"] = vcn
		}
		s["security"] = "tls"
		s["tlsSettings"] = t
	case "reality":
		if q.Get("pbk") == "" {
			return nil, errors.New("REALITY link is missing the public key (pbk)")
		}
		s["security"] = "reality"
		s["realitySettings"] = map[string]any{
			"serverName":  sni,
			"fingerprint": orDefault(q.Get("fp"), "chrome"),
			"publicKey":   q.Get("pbk"),
			"shortId":     q.Get("sid"),
			"spiderX":     q.Get("spx"),
		}
	default:
		return nil, fmt.Errorf("security %q is not supported", security)
	}
	return s, nil
}

func orDefault(s, def string) string {
	if s == "" {
		return def
	}
	return s
}
