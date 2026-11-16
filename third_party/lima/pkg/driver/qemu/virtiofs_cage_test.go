// SPDX-License-Identifier: Apache-2.0
package qemu

import (
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"

	"github.com/lima-vm/lima/v2/pkg/limatype"
)

func TestVirtiofsUsesNamespaceSandboxAndSingleIdentity(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("unprivileged Linux host")
	}
	cfg := Config{InstanceDir: t.TempDir(), LimaYAML: &limatype.LimaYAML{User: limatype.User{UID: new(uint32(1234))}, Mounts: []limatype.Mount{{Location: t.TempDir(), Writable: new(true)}}}}
	args, err := VirtiofsdCmdline(cfg, 0)
	if err != nil {
		t.Fatal(err)
	}
	joined := strings.Join(args, " ")
	for _, expected := range []string{"--sandbox namespace", "--cache auto", "--uid-map=:1234:", "--gid-map=:1234:"} {
		if !strings.Contains(joined, expected) {
			t.Fatal(args)
		}
	}
	if slices.Contains(args, "--writeback") || slices.Contains(args, "none") {
		t.Fatal("unsafe caching or sandbox", args)
	}
	cfg.LimaYAML.Mounts[0].Writable = new(false)
	if _, err := VirtiofsdCmdline(cfg, 0); err == nil {
		t.Fatal("readonly export accepted without host enforcement")
	}
	cfg.LimaYAML.Mounts[0].Writable = new(true)
	cfg.LimaYAML.Mounts[0].SSHFS.ReadonlyNames = []string{".git"}
	if _, err := VirtiofsdCmdline(cfg, 0); err == nil {
		t.Fatal("protected names silently discarded")
	}
}
