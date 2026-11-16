package cage

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"github.com/sylvinus/agent-vm/internal/mounts"
	"github.com/sylvinus/agent-vm/internal/paths"
)

// ProjectSharing is the cold-start sharing configuration in workbench.json.
// Paths are explicit host grants, independent of the selected project.
type ProjectSharing struct {
	Type  string   `json:"project_share_type"`
	Paths []string `json:"project_share_paths"`
}

func (p ProjectSharing) Virtiofs() bool { return p.Type == "virtiofs" }

func LoadProjectSharing(profile string) (ProjectSharing, error) {
	var p ProjectSharing
	b, err := os.ReadFile(filepath.Join(profile, "workbench.json"))
	if os.IsNotExist(err) {
		return p, nil
	}
	if err == nil {
		err = json.Unmarshal(b, &p)
	}
	if err != nil {
		return p, err
	}
	return NormalizeProjectSharing(p, profile)
}

func NormalizeProjectSharing(p ProjectSharing, profile string) (ProjectSharing, error) {
	if p.Type == "" {
		p.Type = "sshfs"
	}
	if p.Type != "sshfs" && p.Type != "virtiofs" {
		return p, errors.New("项目共享方式必须是 sshfs 或 virtiofs")
	}
	if !p.Virtiofs() {
		return p, nil
	}
	if runtime.GOOS != "linux" || os.Getuid() == 0 {
		return p, errors.New("virtiofs 仅支持普通用户运行的 Linux 主机与 Linux QEMU 客机")
	}
	if len(p.Paths) == 0 {
		return p, errors.New("virtiofs 需要配置 1 至 32 个固定共享目录")
	}
	private, err := StoragePrivatePaths(profile)
	if err != nil {
		return p, err
	}
	private = append(private, profile)
	if exe, e := os.Executable(); e == nil {
		private = append(private, filepath.Dir(exe))
	}
	home, err := os.UserHomeDir()
	if err != nil {
		return p, err
	}
	defaultProfile := filepath.Join(home, ".claude-cage")
	if !paths.Equal(defaultProfile, profile) {
		other, err := StoragePrivatePaths(defaultProfile)
		if err != nil {
			return p, err
		}
		private = append(private, append(other, defaultProfile)...)
	}
	refs := mounts.Refs(home, "", profile, private...)
	normalized := make([]string, 0, len(p.Paths))
	for _, path := range p.Paths {
		path = strings.TrimSpace(path)
		if path == "" {
			continue
		}
		if !filepath.IsAbs(path) {
			return p, errors.New("virtiofs 共享目录必须使用绝对路径")
		}
		real, err := paths.Resolve(path)
		if err != nil {
			return p, err
		}
		info, err := os.Stat(real)
		if err != nil || !info.IsDir() {
			return p, errors.New("virtiofs 共享路径必须是可访问的目录")
		}
		if _, unsafe := mounts.UnsafeProject(real, refs); unsafe {
			return p, errors.New("virtiofs 不能共享系统根目录、凭据、程序或 Cage 私有目录")
		}
		for _, existing := range normalized {
			if StoragePathsOverlap(real, existing) {
				return p, errors.New("virtiofs 共享目录不能重复或相互嵌套")
			}
		}
		normalized = append(normalized, real)
	}
	if len(normalized) == 0 || len(normalized) > 32 {
		return p, errors.New("virtiofs 需要配置 1 至 32 个固定共享目录")
	}
	p.Paths = normalized
	return p, nil
}
