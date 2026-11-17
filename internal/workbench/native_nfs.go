package workbench

import (
	"context"
	"errors"
	"runtime"
	"time"

	"al.essio.dev/pkg/shellescape"
	"github.com/sylvinus/agent-vm/internal/cage"
)

func (s *Server) nativeNFS() (cage.NativeNFS, error) {
	cfg, err := loadConfig(s.configPath)
	return cfg.NativeNFS, err
}

func (s *Server) ensureNativeNFS(n cage.NativeNFS) error {
	if !n.Enabled() {
		return nil
	}
	if runtime.GOOS != "linux" || s.windowsGuest != nil || s.macosGuest != nil {
		return errors.New("原生 NFS 目前仅支持 Linux 主机上的 Linux 客机")
	}
	if err := n.Validate(); err != nil {
		return err
	}
	target, source := shellescape.Quote(n.Target), shellescape.Quote(n.Server+":"+n.Export)
	script := "set -eu\n" +
		"if findmnt -rn -C -M " + target + " >/dev/null; then\n" +
		" test \"$(findmnt -rn -C -M " + target + " -o FSTYPE)\" = nfs4\n" +
		" test \"$(findmnt -rn -C -M " + target + " -o SOURCE)\" = " + source + "\n" +
		"else\n sudo -n mkdir -p " + target + "\n" +
		" timeout 25s sudo -n /usr/sbin/mount.nfs " + source + " " + target +
		" -o vers=4.2,proto=tcp,port=2049,noresvport,hard,timeo=100,retrans=2,sec=sys,nosuid,nodev,retry=0,nconnect=4,rsize=1048576,wsize=1048576\nfi\n" +
		"test -d " + target + "\n" +
		// Modern kernels can leave the NFS read-ahead window at 128 KiB.
		// Keep a 15 MiB window for this mount's 1 MiB RPCs, not other devices.
		"device=$(findmnt -rn -C -M " + target + " -o MAJ:MIN)\n" +
		"case \"$device\" in ''|*[!0-9:]*) exit 1;; esac\n" +
		"ahead=/sys/class/bdi/$device/read_ahead_kb\n" +
		"if test -r \"$ahead\" && test \"$(cat \"$ahead\")\" -lt 15360; then printf '15360\\n' | sudo -n tee \"$ahead\" >/dev/null; fi\n" +
		// NFS keeps server ownership; trust Git only within this explicit grant.
		"for safe in " + target + " " + shellescape.Quote(n.Target+"/*") + "; do\n" +
		" git config --global --get-all safe.directory | grep -Fx -- \"$safe\" >/dev/null || git config --global --add safe.directory \"$safe\"\ndone\n"
	ctx, cancel := context.WithTimeout(s.operationContext(), 35*time.Second)
	defer cancel()
	if _, err := s.guest(ctx, script); err != nil {
		return errors.New("NAS 原生 NFS 挂载失败，请检查导出权限、TCP/2049 和客机 nfs-common")
	}
	return nil
}

// Unmount a child before replacing its parent share. Never lazily detach an
// active NFS mount: a busy tree must stop recovery instead of hiding live I/O.
func (s *Server) unmountNativeNFS(n cage.NativeNFS) error {
	ctx, cancel := context.WithTimeout(s.operationContext(), 10*time.Second)
	defer cancel()
	target := shellescape.Quote(n.Target)
	// Read mountinfo without stat/canonicalization: the parent FUSE transport
	// may be dead, but its NFS child must still prevent a lazy parent detach.
	_, err := s.guest(ctx, "if findmnt -rn -C -M "+target+" >/dev/null; then test \"$(findmnt -rn -C -M "+target+" -o FSTYPE)\" = nfs4 && test \"$(findmnt -rn -C -M "+target+" -o SOURCE)\" = "+shellescape.Quote(n.Server+":"+n.Export)+" && sudo -n umount "+target+"; fi")
	return err
}
