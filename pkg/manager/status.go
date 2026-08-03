package manager

import (
	"context"
	"fmt"

	"golang.org/x/sync/errgroup"

	"github.com/ventifus/binmgr/pkg/manifest"
)

// Status reports whether a newer version is available for each installed
// package without making any changes. If packages is empty, all installed
// packages are checked; otherwise only the named packages are checked.
func (m *mgr) Status(ctx context.Context, packages []string) ([]*StatusResult, error) {
	// 1. Load packages.
	var pkgs []*manifest.Package
	if len(packages) == 0 {
		var err error
		pkgs, err = manifest.LoadAll(m.libDir)
		if err != nil {
			return nil, fmt.Errorf("status: load packages: %w", err)
		}
	} else {
		pkgs = make([]*manifest.Package, 0, len(packages))
		for _, id := range packages {
			pkg, err := manifest.Load(id, m.libDir)
			if err != nil {
				return nil, fmt.Errorf("status: load package %q: %w", id, err)
			}
			pkgs = append(pkgs, pkg)
		}
	}

	if len(pkgs) == 0 {
		return nil, nil
	}

	// 2. Check for updates in parallel.
	results := make([]*StatusResult, len(pkgs))

	g, gctx := errgroup.WithContext(ctx)

	for i, pkg := range pkgs {
		idx, p := i, pkg
		g.Go(func() error {
			b, err := m.registry.DispatchByType(p.Backend)
			if err != nil {
				return fmt.Errorf("status: package %q: dispatch backend %q: %w", p.ID, p.Backend, err)
			}

			resolution, err := b.Check(gctx, p)
			if err != nil {
				return fmt.Errorf("status: package %q: check: %w", p.ID, err)
			}

			// idx is unique per goroutine; no mutex needed for slice writes.
			results[idx] = &StatusResult{
				ID:               p.ID,
				InstalledVersion: p.Version,
				LatestVersion:    resolution.Version,
				Pinned:           p.Pinned,
				UpdateAvailable:  resolution.Version != p.Version,
			}
			return nil
		})
	}

	if err := g.Wait(); err != nil {
		return nil, err
	}

	return results, nil
}
