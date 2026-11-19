package cage

import (
	"context"
	"errors"
	"net"
	"net/netip"
	"testing"
)

func TestNativeNFSScopeAndRevokedInternet(t *testing.T) {
	n := NativeNFS{Server: "192.168.60.55", Export: "/srv/cagenfs", Target: "/data/nas"}
	gate, err := NewEgressGate(DefaultConfig())
	if err != nil {
		t.Fatal(err)
	}
	network, err := WithDirectNetwork(Config{NativeNFS: n}, gate)
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		network, address string
		allowed          bool
	}{
		{"tcp", "192.168.60.55:2049", true},
		{"tcp", "192.168.60.55:22", false},
		{"tcp", "192.168.60.118:2049", false},
		{"udp", "192.168.60.55:2049", false},
		{"tcp", "1.1.1.1:443", false},
		{"tcp", "127.0.0.1:2049", false},
	} {
		if got := network.Allows(tc.network, netip.MustParseAddrPort(tc.address)); got != tc.allowed {
			t.Errorf("%s %s: %v", tc.network, tc.address, got)
		}
	}
	transport := network.(*directNetwork)
	calls := 0
	transport.dial = func(ctx context.Context, network, address string) (net.Conn, error) {
		calls++
		if network != "tcp4" || address != "192.168.60.55:2049" {
			t.Fatalf("unexpected direct connection %s %s", network, address)
		}
		return nil, errors.New("test NAS dial")
	}
	for _, address := range []string{"192.168.60.55:22", "192.168.60.118:2049", "1.1.1.1:443", "nas.local:2049"} {
		if _, err := network.DialContext(context.Background(), "tcp", address); !errors.Is(err, ErrGateClosed) {
			t.Fatalf("unexpected fallback %s: %v", address, err)
		}
	}
	if calls != 0 {
		t.Fatal("non-NAS connection bypassed the gate")
	}
	_, _ = network.DialContext(context.Background(), "tcp", "192.168.60.55:2049")
	if calls != 1 {
		t.Fatal("NAS did not use the direct transport")
	}
	gate.Close("test pause")
	if !network.Allows("tcp", netip.MustParseAddrPort("192.168.60.55:2049")) {
		t.Fatal("storage incorrectly depends on the internet lease")
	}
	for _, invalid := range []NativeNFS{
		{Server: "nas.local", Export: "/data", Target: "/data/nas"},
		{Server: "8.8.8.8", Export: "/data", Target: "/data/nas"},
		{Server: n.Server, Export: "/data", Target: "/etc"},
		{Server: n.Server, Export: "/data", Target: "/data/../root"},
		{Server: n.Server, Export: "/data\n", Target: n.Target},
	} {
		if _, err := WithDirectNetwork(Config{NativeNFS: invalid}, gate); err == nil {
			t.Errorf("invalid grant accepted: %+v", invalid)
		}
	}
}
