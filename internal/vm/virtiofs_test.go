package vm

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
)

func TestVirtiofsEnforcesConfiguredHostGrants(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("unprivileged Linux host")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	profile, transfer, root := filepath.Join(home, ".claude-cage"), filepath.Join(home, "transfers"), filepath.Join(home, "projects")
	for _, p := range []string{profile, transfer, root} {
		if err := os.MkdirAll(p, 0700); err != nil {
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
	t.Setenv(cage.ConfigEnv, filepath.Join(profile, "config.json"))
	inst := &Instance{MountType: Virtiofs, VMType: "qemu", Mounts: []Mount{{Location: transfer, MountPoint: transfer, Writable: true}, {Location: root, MountPoint: root, Writable: true}}}
	if err := cageVirtiofsGrants(inst); err != nil {
		t.Fatal(err)
	}
	if err := Confined(inst); err != nil {
		t.Fatal(err)
	}
	inst.Mounts[1].Writable = false
	if err := Confined(inst); err == nil {
		t.Fatal("readonly enforcement falsely claimed")
	}
	inst.Mounts[1].Writable = true
	inst.Mounts[1].SSHFS = &SSHFS{ReadonlyNames: []string{".git"}}
	if err := Confined(inst); err == nil {
		t.Fatal("protected names silently discarded")
	}
	inst.Mounts[1] = inst.Mounts[0]
	if err := cageVirtiofsGrants(inst); err == nil {
		t.Fatal("duplicate/extra export accepted")
	}
	t.Setenv(cage.ConfigEnv, "")
	if err := cageVirtiofsGrants(inst); err == nil {
		t.Fatal("unconfigured virtiofs VM accepted")
	}
}
