package cage

import (
	"strings"
	"testing"
)

func TestVirtiofsRejectsNFSExportsAndNestedMounts(t *testing.T) {
	const mounts = "1 0 8:1 / / rw - ext4 /dev/sda1 rw\n" +
		"2 1 0:42 / /projects/nas rw shared:5 - nfs4 server:/export rw\n" +
		"3 1 0:43 / /with\\040space/nas rw - nfs server:/other rw\n"
	for _, tc := range []struct {
		root string
		bad  bool
	}{
		{"/projects/nas", true},
		{"/projects/nas/repo", true},
		{"/projects", true},
		{"/with space", true},
		{"/projects/nas-other", false},
		{"/local-projects", false},
	} {
		t.Run(tc.root, func(t *testing.T) {
			err := checkVirtiofsMountInfo(strings.NewReader(mounts), []string{tc.root})
			if (err != nil) != tc.bad {
				t.Fatalf("error = %v, want rejection = %v", err, tc.bad)
			}
		})
	}
	if err := checkVirtiofsMountInfo(strings.NewReader(mounts), []string{"/local", "/projects"}); err == nil {
		t.Fatal("second export containing NFS accepted")
	}
	if err := checkVirtiofsMountInfo(strings.NewReader("malformed"), []string{"/local"}); err == nil {
		t.Fatal("malformed mountinfo accepted")
	}
}
