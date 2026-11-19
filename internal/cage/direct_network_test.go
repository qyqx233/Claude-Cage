package cage

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestDirectLANAllowsOnlyConfiguredTCPDestinationsOutsideHost(t *testing.T) {
	cfg := DefaultConfig()
	cfg.DirectLAN = netip.MustParsePrefix("192.168.60.0/24")
	gate, err := NewEgressGate(cfg)
	if err != nil {
		t.Fatal(err)
	}
	network, err := WithDirectNetwork(cfg, gate)
	if err != nil {
		t.Fatal(err)
	}
	direct := network.(*directNetwork)
	direct.offHost = func(addr netip.Addr) bool { return addr != netip.MustParseAddr("192.168.60.118") }
	dialed := errors.New("direct dial selected")
	direct.dial = func(_ context.Context, network, address string) (net.Conn, error) {
		if network != "tcp4" || address != "192.168.60.53:22" {
			t.Fatalf("unexpected direct dial: %s %s", network, address)
		}
		return nil, dialed
	}
	for _, tc := range []struct {
		transport string
		address   string
		allowed   bool
	}{
		{"tcp", "192.168.60.53:22", true},
		{"tcp", "192.168.60.53:443", true},
		{"udp", "192.168.60.53:22", false},
		{"tcp", "192.168.60.118:22", false},
		{"tcp", "192.168.61.53:22", false},
		{"tcp", "127.0.0.1:22", false},
		{"tcp", "1.1.1.1:443", false},
	} {
		if allowed := network.Allows(tc.transport, netip.MustParseAddrPort(tc.address)); allowed != tc.allowed {
			t.Errorf("%s %s allowed = %v", tc.transport, tc.address, allowed)
		}
		if !tc.allowed {
			if _, err := network.DialContext(context.Background(), tc.transport, tc.address); !errors.Is(err, ErrGateClosed) {
				t.Errorf("unexpected error for %s %s: %v", tc.transport, tc.address, err)
			}
		}
	}
	if _, err := network.DialContext(context.Background(), "tcp", "192.168.60.53:22"); !errors.Is(err, dialed) {
		t.Fatalf("office SSH did not use the direct dial: %v", err)
	}
}

func TestDirectLANRejectsPublicAndNoncanonicalPrefixes(t *testing.T) {
	for _, cidr := range []string{"0.0.0.0/0", "192.168.0.0/15", "10.0.0.0/7", "192.168.60.53/24", "127.0.0.0/8", "fd00::/8"} {
		cfg := DefaultConfig()
		cfg.DirectLAN = netip.MustParsePrefix(cidr)
		if err := cfg.Validate(); err == nil {
			t.Errorf("invalid LAN prefix accepted: %s", cidr)
		}
	}
}
