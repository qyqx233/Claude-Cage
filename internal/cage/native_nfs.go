package cage

import (
	"errors"
	"net/netip"
	"path"
	"strings"
)

// NativeNFS is an explicit, cold-start grant to one NAS and one export. Its
// connection is storage access, independent of the external proxy's lease.
// The host-owned config remains outside every guest share.
type NativeNFS struct {
	Server string `json:"server,omitempty"`
	Export string `json:"export,omitempty"`
	Target string `json:"target,omitempty"`
}

func (n NativeNFS) Enabled() bool { return n != (NativeNFS{}) }

func (n NativeNFS) Validate() error {
	if !n.Enabled() {
		return nil
	}
	ip, err := netip.ParseAddr(n.Server)
	if err != nil || !ip.Is4() || !ip.IsPrivate() {
		return errors.New("native_nfs.server 必须是 NAS 的内网 IPv4 地址")
	}
	for _, p := range []string{n.Export, n.Target} {
		if len(p) > 1024 || !path.IsAbs(p) || path.Clean(p) != p || strings.ContainsAny(p, "\x00\r\n") {
			return errors.New("native_nfs 的导出和挂载目录必须是规范的绝对路径")
		}
	}
	// A NAS grant must not replace guest system directories or user homes.
	allowed := false
	for _, root := range []string{"/data/", "/mnt/", "/media/", "/srv/"} {
		allowed = allowed || strings.HasPrefix(n.Target, root)
	}
	if !allowed {
		return errors.New("native_nfs.target 必须是 /data、/mnt、/media 或 /srv 下的子目录")
	}
	return nil
}

func (n NativeNFS) Allows(network string, dst netip.AddrPort) bool {
	return n.Enabled() && network == "tcp" && dst.IsValid() && dst.Addr().String() == n.Server && dst.Port() == 2049
}
