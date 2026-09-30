//go:build !noxray

package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"net"
	"runtime"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/links"

	xnet "github.com/xtls/xray-core/common/net"
	xcore "github.com/xtls/xray-core/core"
	"github.com/xtls/xray-core/infra/conf/serial"
	_ "github.com/xtls/xray-core/main/distro/all" // register protocols and transports
)

// XrayAvailable reports whether this build includes Xray-core.
const XrayAvailable = true

// xrayDialer is a tiny Xray instance with a single outbound.
type xrayDialer struct{ inst *xcore.Instance }

func newXrayDialer(p *core.ProxyProfile, l *links.Link) (*xrayDialer, error) {
	ob, _, err := outbound(p.URL)
	if err != nil {
		return nil, err
	}
	delete(ob, "_server")
	ob["tag"] = "proxy"
	if l != nil {
		// Pin the connection to the proxy server to this link. Desktop OSes
		// bind by interface name; Android resolves the link ID through our
		// own system dialer (see xray_android.go).
		iface := l.Name
		if runtime.GOOS == "android" {
			iface = l.ID
		}
		stream, _ := ob["streamSettings"].(map[string]any)
		if stream == nil {
			stream = map[string]any{}
			ob["streamSettings"] = stream
		}
		stream["sockopt"] = map[string]any{"interface": iface}
	}
	cfg := map[string]any{
		"log":       map[string]any{"loglevel": "none"},
		"outbounds": []any{ob},
	}
	raw, _ := json.Marshal(cfg)
	c, err := serial.DecodeJSONConfig(bytes.NewReader(raw))
	if err != nil {
		return nil, err
	}
	pb, err := c.Build()
	if err != nil {
		return nil, err
	}
	inst, err := xcore.New(pb)
	if err != nil {
		return nil, err
	}
	if err := inst.Start(); err != nil {
		return nil, err
	}
	return &xrayDialer{inst: inst}, nil
}

func (d *xrayDialer) DialContext(ctx context.Context, network, addr string) (net.Conn, error) {
	dest, err := xnet.ParseDestination("tcp:" + addr)
	if err != nil {
		return nil, err
	}
	return xcore.Dial(ctx, d.inst, dest)
}

func (d *xrayDialer) Close() error { return d.inst.Close() }
