package cage

import (
	"os"
	"path/filepath"
	"runtime"
	"testing"
)

func TestProjectSharingValidatesExplicitDirectories(t *testing.T) {
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("unprivileged Linux host")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	root := filepath.Join(home, "arbitrary-projects")
	profile := filepath.Join(home, ".claude-cage")
	for _, dir := range []string{root, profile, filepath.Join(home, ".ssh")} {
		if err := os.MkdirAll(dir, 0700); err != nil {
			t.Fatal(err)
		}
	}
	p, err := NormalizeProjectSharing(ProjectSharing{Type: "virtiofs", Paths: []string{"  " + root + "  ", ""}}, profile)
	if err != nil || len(p.Paths) != 1 || p.Paths[0] != root {
		t.Fatalf("arbitrary root: %+v %v", p, err)
	}
	for _, list := range [][]string{nil, {"relative"}, {"/"}, {home}, {profile}, {filepath.Join(home, ".ssh")}, {root, root}} {
		if _, err := NormalizeProjectSharing(ProjectSharing{Type: "virtiofs", Paths: list}, profile); err == nil {
			t.Fatalf("unsafe roots accepted: %q", list)
		}
	}
	child := filepath.Join(root, "child")
	_ = os.Mkdir(child, 0700)
	if _, err := NormalizeProjectSharing(ProjectSharing{Type: "virtiofs", Paths: []string{root, child}}, profile); err == nil {
		t.Fatal("nested exports accepted")
	}
	alias := filepath.Join(home, "alias")
	if err := os.Symlink(profile, alias); err != nil {
		t.Fatal(err)
	}
	if _, err := NormalizeProjectSharing(ProjectSharing{Type: "virtiofs", Paths: []string{alias}}, profile); err == nil {
		t.Fatal("symlink into private storage accepted")
	}
}

func TestProjectSharingDefaultsToSSHFS(t *testing.T) {
	p, err := LoadProjectSharing(t.TempDir())
	if err != nil || p.Virtiofs() {
		t.Fatal(p, err)
	}
	if _, err := NormalizeProjectSharing(ProjectSharing{Type: "unknown"}, t.TempDir()); err == nil {
		t.Fatal("unknown transport accepted")
	}
}
