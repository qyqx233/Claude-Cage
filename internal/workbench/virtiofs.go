package workbench

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/containerenv"
	"github.com/sylvinus/agent-vm/internal/paths"
	"go.yaml.in/yaml/v3"
)

var errFixedSharing = errors.New("virtiofs 不支持运行中更改共享范围，请先停止环境，在设置中修改固定共享目录后重新启动")

type fixedShareLayout struct {
	Type   string `yaml:"mountType"`
	VMType string `yaml:"vmType"`
	Mounts []struct {
		Location   string `yaml:"location"`
		MountPoint string `yaml:"mountPoint"`
		Writable   bool   `yaml:"writable"`
		SSHFS      struct {
			ReadonlyNames []string `yaml:"readonlyNames"`
		} `yaml:"sshfs"`
	} `yaml:"mounts"`
}

func (s *Server) fixedShareLayout() (fixedShareLayout, error) {
	var layout fixedShareLayout
	b, err := os.ReadFile(filepath.Join(s.instanceDir(), "lima.yaml"))
	if err == nil {
		err = yaml.Unmarshal(b, &layout)
	}
	return layout, err
}

func (p Preferences) sharing() cage.ProjectSharing {
	return cage.ProjectSharing{Type: p.ProjectShareType, Paths: p.ProjectSharePaths}
}

func (s *Server) usesVirtiofs() bool {
	s.mu.Lock()
	enabled := s.preferences.sharing().Virtiofs()
	s.mu.Unlock()
	if enabled {
		return true
	}
	layout, err := s.fixedShareLayout()
	return err == nil && layout.Type == "virtiofs"
}

func (s *Server) validateVirtiofsSettings(p Preferences) error {
	if !p.sharing().Virtiofs() {
		return nil
	}
	if runtime.GOOS != "linux" || s.windowsGuest != nil || s.macosGuest != nil || containerenv.Configured(s.profile) {
		return errors.New("virtiofs 仅支持 Linux 主机上的 Linux QEMU 客机")
	}
	for _, root := range p.ProjectSharePaths {
		if _, err := validateWorkspace(root, s.profile); err != nil {
			return err
		}
		if cage.StoragePathsOverlap(root, s.environmentDir()) {
			return errors.New("固定共享目录不能与下载目录重叠")
		}
	}
	return nil
}

func (s *Server) fixedLayoutMatches(layout fixedShareLayout) bool {
	s.mu.Lock()
	p := s.preferences.sharing()
	s.mu.Unlock()
	driver := layout.VMType
	// Lima fills in QEMU on Linux when older configuration files omit vmType.
	// Match that effective driver without rewriting a running VM's config.
	if driver == "" && runtime.GOOS == "linux" {
		driver = "qemu"
	}
	if !p.Virtiofs() || layout.Type != "virtiofs" || driver != "qemu" || s.validateVirtiofsSettings(Preferences{ProjectShareType: p.Type, ProjectSharePaths: p.Paths}) != nil {
		return false
	}
	expected := append([]string{s.environmentDir()}, p.Paths...)
	if len(expected) != len(layout.Mounts) {
		return false
	}
	seen := map[string]bool{}
	for _, m := range layout.Mounts {
		if !m.Writable || m.MountPoint != guestPath(m.Location) || len(m.SSHFS.ReadonlyNames) != 0 || seen[m.Location] {
			return false
		}
		found := false
		for _, root := range expected {
			found = found || paths.Equal(root, m.Location)
		}
		if !found {
			return false
		}
		seen[m.Location] = true
	}
	return true
}

func (s *Server) ensureVirtiofsProjectShare(path string) error {
	layout, err := s.fixedShareLayout()
	if err != nil || !s.fixedLayoutMatches(layout) {
		return errFixedSharing
	}
	root := ""
	s.mu.Lock()
	for _, p := range s.preferences.ProjectSharePaths {
		if cage.StorageContains(p, path) {
			root = p
			break
		}
	}
	s.mu.Unlock()
	if root == "" {
		return errFixedSharing
	}
	ctx, cancel := context.WithTimeout(s.operationContext(), 10*time.Second)
	defer cancel()
	// A probe validates the actual export as well as its mount type. Do not
	// remount live files or reconnect a device behind existing file handles.
	probe, err := os.CreateTemp(path, ".cage-mount-probe-*")
	if err != nil {
		return err
	}
	defer os.Remove(probe.Name())
	challenge := randomToken()
	_, err = probe.WriteString(challenge)
	closeErr := probe.Close()
	if err != nil {
		return err
	}
	if closeErr != nil {
		return closeErr
	}
	script := "test \"$(findmnt -rn -M " + shellescape.Quote(guestPath(root)) + " -o FSTYPE)\" = virtiofs && test \"$(cat " + shellescape.Quote(guestPath(probe.Name())) + ")\" = " + shellescape.Quote(challenge)
	if _, err = s.guest(ctx, script); err != nil {
		return errors.New("virtiofs 共享未就绪，请停止环境后重新启动并检查 virtiofsd 日志")
	}
	return nil
}

func sameProjectSharing(a, b Preferences) bool {
	x, y := a.sharing(), b.sharing()
	if x.Type == "" {
		x.Type = "sshfs"
	}
	if y.Type == "" {
		y.Type = "sshfs"
	}
	return x.Type == y.Type && slices.Equal(x.Paths, y.Paths)
}
