package cage

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"time"

	"github.com/sylvinus/agent-vm/internal/netguard"
)

var privateIPv4 = [...]netip.Prefix{
	netip.MustParsePrefix("10.0.0.0/8"),
	netip.MustParsePrefix("172.16.0.0/12"),
	netip.MustParsePrefix("192.168.0.0/16"),
}

func validateDirectLAN(prefix netip.Prefix) error {
	if prefix == (netip.Prefix{}) {
		return nil
	}
	if prefix.IsValid() && prefix == prefix.Masked() {
		for _, private := range privateIPv4 {
			if prefix.Bits() >= private.Bits() && private.Contains(prefix.Addr()) {
				return nil
			}
		}
	}
	return errors.New("direct_lan 必须是规范的 IPv4 私有网段 CIDR")
}

type GuestNetwork interface {
	Allows(string, netip.AddrPort) bool
	DialContext(context.Context, string, string) (net.Conn, error)
}

type directNetwork struct {
	nas      NativeNFS
	lan      netip.Prefix
	fallback GuestNetwork
	offHost  func(netip.Addr) bool
	dial     func(context.Context, string, string) (net.Conn, error)
}

// WithDirectNetwork adds host-configured TCP grants for a NAS and a private
// LAN. These local services are independent of the external proxy's lease.
func WithDirectNetwork(cfg Config, fallback GuestNetwork) (GuestNetwork, error) {
	if err := cfg.NativeNFS.Validate(); err != nil {
		return nil, err
	}
	if err := validateDirectLAN(cfg.DirectLAN); err != nil {
		return nil, err
	}
	if !cfg.NativeNFS.Enabled() && !cfg.DirectLAN.IsValid() {
		return fallback, nil
	}
	dialer := &net.Dialer{Timeout: 10 * time.Second, KeepAlive: 30 * time.Second}
	return &directNetwork{nas: cfg.NativeNFS, lan: cfg.DirectLAN, fallback: fallback, offHost: netguard.OffHost, dial: dialer.DialContext}, nil
}

func (n *directNetwork) allowsDirect(network string, dst netip.AddrPort) bool {
	return network == "tcp" && dst.IsValid() && dst.Port() != 0 &&
		(n.nas.Allows(network, dst) || (n.lan.Contains(dst.Addr()) && n.offHost(dst.Addr())))
}

func (n *directNetwork) Allows(network string, dst netip.AddrPort) bool {
	return n.allowsDirect(network, dst) || n.fallback.Allows(network, dst)
}

func (n *directNetwork) DialContext(ctx context.Context, network, address string) (net.Conn, error) {
	dst, err := netip.ParseAddrPort(address)
	if err == nil && (network == "tcp" || network == "tcp4") && n.allowsDirect("tcp", dst) {
		return n.dial(ctx, "tcp4", dst.String())
	}
	return n.fallback.DialContext(ctx, network, address)
}
