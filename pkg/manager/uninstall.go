package manager

import (
	"context"
	"errors"
	"fmt"
	"os"

	"github.com/ventifus/binmgr/pkg/manifest"
)

// Uninstall removes all installed files and the manifest for each named package.
// It returns an error if any package ID is not found in the manifest, or if
// removing an installed file fails for a reason other than it already being gone.
func (m *mgr) Uninstall(_ context.Context, packages []string) error {
	for _, id := range packages {
		pkg, err := manifest.Load(id, m.libDir)
		if err != nil {
			return err
		}

		for _, spec := range pkg.Specs {
			for _, f := range spec.InstalledFiles {
				if err := os.Remove(f.LocalPath); err != nil {
					if errors.Is(err, os.ErrNotExist) {
						continue
					}
					return fmt.Errorf("removing %s: %w", f.LocalPath, err)
				}
			}
		}

		if err := manifest.Delete(id, m.libDir); err != nil {
			return err
		}
	}

	return nil
}
