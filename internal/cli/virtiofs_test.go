package cli

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/vm"
)

func TestFixedVirtiofsSharesAreColdStartConfiguration(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("unprivileged Linux host")
	}
	se := newStartEnv(t)
	profile := filepath.Join(se.home, ".claude-cage")
	transfer := filepath.Join(se.home, "transfers")
	root := filepath.Join(se.home, "custom-root")
	for _, path := range []string{profile, transfer, root} {
		if err := os.MkdirAll(path, 0700); err != nil {
			t.Fatal(err)
		}
	}
	write := func(name string, v any) {
		t.Helper()
		b, _ := json.Marshal(v)
		if err := os.WriteFile(filepath.Join(profile, name), b, 0600); err != nil {
			t.Fatal(err)
		}
	}
	write("environment.json", cage.EnvironmentRecord{Directory: transfer})
	write("workbench.json", cage.ProjectSharing{Type: "virtiofs", Paths: []string{root}})
	se.app.cageProfile = profile
	s := &start{app: se.app, ctx: context.Background(), dir: transfer}
	if err := s.prepareFixedShares(); err != nil {
		t.Fatal(err)
	}
	ms, _ := s.shares(true)
	if s.shareMountType() != vm.Virtiofs || len(ms) != 2 || ms[0].Location != transfer || ms[1].Location != root {
		t.Fatal(ms, s.shareMountType())
	}
	inst := &vm.Instance{Mounts: ms, MountType: vm.Virtiofs, VMType: "qemu"}
	if err := vm.Confined(inst); err != nil {
		t.Fatal(err)
	}
	if sharesDiffer(inst, ms, vm.Virtiofs) {
		t.Fatal("identical static exports require a restart")
	}
	inst.MountType = vm.ReverseSSHFS
	if !sharesDiffer(inst, ms, vm.Virtiofs) {
		t.Fatal("transport change was ignored")
	}
	s.opts.ReadOnly = true
	if err := s.prepareFixedShares(); err == nil {
		t.Fatal("readonly protection silently dropped")
	}
}
