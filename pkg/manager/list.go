package manager

import (
	"context"

	"github.com/ventifus/binmgr/pkg/manifest"
)

// List returns all installed packages from the manifest directory.
func (m *mgr) List(_ context.Context) ([]*manifest.Package, error) {
	return manifest.LoadAll(m.libDir)
}
