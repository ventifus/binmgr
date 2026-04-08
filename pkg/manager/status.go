package manager

import (
	"context"
	"fmt"
	"sync"

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
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstErr error

	for i, pkg := range pkgs {
		wg.Add(1)
		go func(idx int, p *manifest.Package) {
			defer wg.Done()

			b, err := m.registry.DispatchByType(p.Backend)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("status: package %q: dispatch backend %q: %w", p.ID, p.Backend, err)
				}
				mu.Unlock()
				return
			}

			resolution, err := b.Check(ctx, p)
			if err != nil {
				mu.Lock()
				if firstErr == nil {
					firstErr = fmt.Errorf("status: package %q: check: %w", p.ID, err)
				}
				mu.Unlock()
				return
			}

			mu.Lock()
			results[idx] = &StatusResult{
				ID:               p.ID,
				InstalledVersion: p.Version,
				LatestVersion:    resolution.Version,
				Pinned:           p.Pinned,
				UpdateAvailable:  resolution.Version != p.Version,
			}
			mu.Unlock()
		}(i, pkg)
	}

	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}

	return results, nil
}
