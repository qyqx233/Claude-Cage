package cage

import (
	"bufio"
	"fmt"
	"io"
	"os"
	"strings"
)

func validateVirtiofsFilesystems(roots []string) error {
	f, err := os.Open("/proc/self/mountinfo")
	if err != nil {
		return fmt.Errorf("无法检查 virtiofs 共享的文件系统：%w", err)
	}
	defer f.Close()
	return checkVirtiofsMountInfo(f, roots)
}

// Rootless virtiofsd retains O_PATH descriptors for cached guest inodes.
// On NFS, unlink may therefore leave busy .nfs files even with no guest file
// open. Check ancestors and descendants: a local export can contain NFS mounts.
func checkVirtiofsMountInfo(r io.Reader, roots []string) error {
	scanner := bufio.NewScanner(r)
	scanner.Buffer(make([]byte, 4096), 1024*1024)
	unescape := strings.NewReplacer(`\040`, " ", `\011`, "\t", `\012`, "\n", `\134`, `\`)
	for scanner.Scan() {
		parts := strings.SplitN(scanner.Text(), " - ", 2)
		if len(parts) != 2 {
			return fmt.Errorf("无法解析 virtiofs 文件系统检查所需的 mountinfo")
		}
		fields, fs := strings.Fields(parts[0]), strings.Fields(parts[1])
		if len(fields) < 6 || len(fs) < 3 {
			return fmt.Errorf("无法解析 virtiofs 文件系统检查所需的 mountinfo")
		}
		if fs[0] != "nfs" && fs[0] != "nfs4" {
			continue
		}
		mount := unescape.Replace(fields[4])
		for _, root := range roots {
			if StoragePathsOverlap(root, mount) {
				return fmt.Errorf("virtiofs 暂不支持 NFS 目录或包含 NFS 子挂载的共享（%s）；缓存句柄可能阻止删除文件，请使用 SSHFS", mount)
			}
		}
	}
	if err := scanner.Err(); err != nil {
		return fmt.Errorf("无法读取 virtiofs 文件系统检查所需的 mountinfo：%w", err)
	}
	return nil
}
