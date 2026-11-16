package vm

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/sylvinus/agent-vm/internal/cage"
	"github.com/sylvinus/agent-vm/internal/paths"
)

// A Cage virtiofs VM must export exactly the explicit, currently configured
// grants plus its transfer directory. A hand-edited Lima file cannot add grants.
func cageVirtiofsGrants(inst *Instance) error {
	if inst.MountType != Virtiofs {
		return nil
	}
	config := os.Getenv(cage.ConfigEnv)
	if config == "" {
		return fmt.Errorf("%w: virtiofs requires an explicit Cage sharing configuration", ErrUnconfined)
	}
	profile := filepath.Dir(config)
	p, err := cage.LoadProjectSharing(profile)
	if err != nil {
		return err
	}
	if !p.Virtiofs() {
		return fmt.Errorf("%w: virtiofs is not selected", ErrUnconfined)
	}
	expected := append([]string{cage.EnvironmentDir(profile)}, p.Paths...)
	if len(expected) != len(inst.Mounts) {
		return fmt.Errorf("%w: virtiofs exports differ from configured grants", ErrUnconfined)
	}
	seen := map[string]bool{}
	for _, m := range inst.Mounts {
		found := false
		for _, root := range expected {
			found = found || paths.Equal(root, m.Location)
		}
		if !found || seen[m.Location] {
			return fmt.Errorf("%w: unexpected virtiofs export %s", ErrUnconfined, m.Location)
		}
		seen[m.Location] = true
	}
	return nil
}
