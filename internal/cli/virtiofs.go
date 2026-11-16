package cli

import (
	"errors"
	"fmt"
	"io"
	"runtime"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/mounts"
	"github.com/sylvinus/agent-vm/internal/paths"
	"github.com/sylvinus/agent-vm/internal/vm"
)

func (s *start) shareMountType() string {
	if s.fixedShares != nil {
		return vm.Virtiofs
	}
	return vm.ReverseSSHFS
}

func (s *start) prepareFixedShares() error {
	if s.cageProfile == "" || s.opts.Scratch {
		return nil
	}
	p, err := cage.LoadProjectSharing(s.cageProfile)
	if err != nil {
		return err
	}
	if !p.Virtiofs() {
		return nil
	}
	if runtime.GOOS != "linux" || !paths.Equal(s.dir, cage.EnvironmentDir(s.cageProfile)) {
		return errors.New("virtiofs 仅支持 Cage 的 Linux 持久环境")
	}
	if s.opts.ReadOnly || len(s.entries(s.dir, io.Discard)) != 0 {
		return errors.New("virtiofs 暂不支持 --readonly 或 volumes 条目；请使用 SSHFS")
	}
	for _, root := range p.Paths {
		if cage.StoragePathsOverlap(root, s.dir) {
			return fmt.Errorf("virtiofs 项目共享不能与下载目录重叠：%s", root)
		}
		// LoadProjectSharing protects profile storage and credentials; the CLI
		// additionally protects its own state and any installed executable.
		if _, unsafe := mounts.UnsafeProject(root, s.refs()); unsafe {
			return errors.New("virtiofs 共享目录包含私有状态或程序目录")
		}
	}
	s.fixedShares = p.Paths
	return nil
}
