package workbench

import (
	"net/netip"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
)

func TestSettingsPreserveNativeNASGrant(t *testing.T) {
	s := testServer(t)
	old, err := loadConfig(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	old.NativeNFS = cage.NativeNFS{Server: "192.168.60.55", Export: "/srv/cagenfs", Target: "/data/nas"}
	old.DirectLAN = netip.MustParsePrefix("192.168.60.0/24")
	input := settingsView(old, s.preferences)
	next, _, err := proposedSettings(input, old, s.profile)
	if err != nil {
		t.Fatal(err)
	}
	if next.NativeNFS != old.NativeNFS {
		t.Fatal("normal settings changes dropped the NAS grant")
	}
	if next.DirectLAN != old.DirectLAN {
		t.Fatal("normal settings changes dropped the LAN grant")
	}
	changed := next
	changed.NativeNFS.Server = "192.168.60.56"
	if changed.Fingerprint() == next.Fingerprint() {
		t.Fatal("NAS destination is absent from the network configuration identity")
	}
}
