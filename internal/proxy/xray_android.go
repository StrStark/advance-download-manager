//go:build android && !noxray

package proxy

import (
	"context"
	"net"

	"github.com/StrStark/advance-download-manager/internal/links"

	xnet "github.com/xtls/xray-core/common/net"
	"github.com/xtls/xray-core/transport/internet"
)

// On Android, networks are chosen by handle, not interface name, so Xray's
// own SO_BINDTODEVICE handling can't be used. This system dialer maps the
// sockopt "interface" (our link ID) to a link and binds through it.
func init() {
	internet.UseAlternativeSystemDialer(&androidDialer{def: &internet.DefaultSystemDialer{}})
}

type androidDialer struct{ def *internet.DefaultSystemDialer }

func (d *androidDialer) Dial(ctx context.Context, src xnet.Address, dest xnet.Destination, sockopt *internet.SocketConfig) (net.Conn, error) {
	if sockopt != nil && sockopt.Interface != "" && dest.Network == xnet.Network_TCP {
		if l, ok := links.Find(sockopt.Interface); ok {
			return links.Dialer(l).DialContext(ctx, "tcp", dest.NetAddr())
		}
	}
	return d.def.Dial(ctx, src, dest, sockopt)
}

func (d *androidDialer) DestIpAddress() net.IP { return nil }
