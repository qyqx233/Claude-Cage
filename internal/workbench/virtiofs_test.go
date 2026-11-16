package workbench

import (
	"bytes"
	"encoding/json"
	"errors"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/sylvinus/agent-vm/internal/cage"
	"go.yaml.in/yaml/v3"
)

func fixedShareServer(t *testing.T, running bool) (*Server, string) {
	t.Helper()
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		t.Skip("unprivileged Linux host")
	}
	s := transferMountServer(t, running, "current")
	root, _ := filepath.EvalSymlinks(t.TempDir())
	s.preferences.ProjectShareType = "virtiofs"
	s.preferences.ProjectSharePaths = []string{root}
	mounts := []any{}
	for _, path := range []string{s.environmentDir(), root} {
		mounts = append(mounts, map[string]any{"location": path, "mountPoint": guestPath(path), "writable": true})
	}
	b, err := yaml.Marshal(map[string]any{"mountType": "virtiofs", "vmType": "qemu", "mounts": mounts})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(s.instanceDir(), "lima.yaml"), b, 0600); err != nil {
		t.Fatal(err)
	}
	return s, root
}

func TestVirtiofsLayoutRequiresExactlyConfiguredExports(t *testing.T) {
	s, _ := fixedShareServer(t, false)
	if fs, net := s.inspectVMLayout(); !fs || !net {
		t.Fatal("fixed exports rejected", fs, net)
	}
	layout, _ := s.fixedShareLayout()
	layout.Mounts = append(layout.Mounts, layout.Mounts[0])
	if s.fixedLayoutMatches(layout) {
		t.Fatal("extra export accepted")
	}
	layout, _ = s.fixedShareLayout()
	layout.Mounts[1].Writable = false
	if s.fixedLayoutMatches(layout) {
		t.Fatal("unenforced readonly export accepted")
	}
}

func TestVirtiofsRejectsDynamicChangesWithoutSaving(t *testing.T) {
	s, _ := fixedShareServer(t, true)
	s.ready = true
	outside, _ := filepath.EvalSymlinks(t.TempDir())
	if err := s.ensureProjectShare(outside); !errors.Is(err, errFixedSharing) {
		t.Fatal(err)
	}
	body, _ := json.Marshal(map[string]any{"workspace": outside, "add": true})
	if w := request(s, "POST", "/api/projects/open", s.origin, string(body), true); w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
	if _, err := os.Stat(filepath.Join(s.profile, "projects.json")); !os.IsNotExist(err) {
		t.Fatal("failed connection saved grant", err)
	}
	cfg, err := cage.Load(s.configPath)
	if err != nil {
		t.Fatal(err)
	}
	next := s.preferences
	next.ProjectSharePaths = []string{outside}
	if err := s.apply(savedProfile{cfg, next}, false); !errors.Is(err, errFixedSharing) {
		t.Fatal("live settings changed", err)
	}
	if s.preferences.ProjectSharePaths[0] == outside || !s.vmRunning() {
		t.Fatal("rejected settings mutated live state")
	}
	w := request(s, "POST", "/api/projects/remove", s.origin, `{"workspace":"`+outside+`"}`, true)
	if w.Code != 409 {
		t.Fatal(w.Code, w.Body.String())
	}
}

func TestVirtiofsSubprojectAndOlderSettingsClient(t *testing.T) {
	s, root := fixedShareServer(t, false)
	child := filepath.Join(root, "new-project")
	if err := os.Mkdir(child, 0700); err != nil {
		t.Fatal(err)
	}
	if path, err := s.projectWorkspace(child); err != nil || path != child {
		t.Fatal(path, err)
	}
	cfg, _ := cage.Load(s.configPath)
	data, _ := json.Marshal(settingsView(cfg, s.preferences))
	var old map[string]any
	_ = json.Unmarshal(data, &old)
	delete(old, "project_share_type")
	delete(old, "project_share_paths")
	data, _ = json.Marshal(old)
	r := httptest.NewRequest("POST", "/api/settings", bytes.NewReader(data))
	r.Header.Set("Content-Type", "application/json")
	w := httptest.NewRecorder()
	v, ok := s.proposed(w, r)
	if !ok || v.Preferences.ProjectShareType != "virtiofs" || len(v.Preferences.ProjectSharePaths) != 1 || v.Preferences.ProjectSharePaths[0] != root {
		t.Fatal("older client lost sharing preference", v.Preferences, w.Body.String())
	}
}
