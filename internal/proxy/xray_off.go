//go:build noxray

package proxy

import (
	"context"
	"errors"
	"net"

	"github.com/StrStark/advance-download-manager/internal/core"
	"github.com/StrStark/advance-download-manager/internal/links"
)

// XrayAvailable reports whether this build includes Xray-core.
const XrayAvailable = false

type xrayDialer struct{}

func newXrayDialer(p *core.ProxyProfile, l *links.Link) (*xrayDialer, error) {
	return nil, errors.New("this build of ADM has no V2Ray/Xray support (built with -tags noxray)")
}

func (d *xrayDialer) DialContext(context.Context, string, string) (net.Conn, error) {
	return nil, errors.New("no Xray support")
}
func (d *xrayDialer) Close() error { return nil }
