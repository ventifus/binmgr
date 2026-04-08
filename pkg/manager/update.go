package manager

import (
	"context"
	"fmt"
	"sync"

	"github.com/ventifus/binmgr/pkg/manifest"
)

// Update updates installed packages and manages pin status.
//
// If opts.Packages is empty, all non-pinned packages are updated to their
// latest versions. If opts.Packages is non-empty, only those packages are
// updated — pinned packages are not skipped when named directly.
//
// If opts.Unpin is set, each package's pin is cleared before checking for
// updates. If opts.Pin is set, each package is pinned after a successful
// update.
func (m *mgr) Update(ctx context.Context, opts UpdateOptions) ([]*UpdateResult, error) {
	// 1. Load packages.
	var pkgs []*manifest.Package
	explicitTargets := make(map[string]PackageTarget) // id → PackageTarget for version override

	if len(opts.Packages) == 0 {
		all, err := manifest.LoadAll(m.libDir)
		if err != nil {
			return nil, fmt.Errorf("update: load packages: %w", err)
		}
		// Filter to non-pinned packages when no packages are named explicitly.
		for _, p := range all {
			if !p.Pinned {
				pkgs = append(pkgs, p)
			}
		}
	} else {
		pkgs = make([]*manifest.Package, 0, len(opts.Packages))
		for _, target := range opts.Packages {
			pkg, err := manifest.Load(target.ID, m.libDir)
			if err != nil {
				return nil, fmt.Errorf("update: load package %q: %w", target.ID, err)
			}
			pkgs = append(pkgs, pkg)
			if target.Version != "" {
				explicitTargets[target.ID] = target
			}
		}
	}

	if len(pkgs) == 0 {
		return nil, nil
	}

	// 2. If opts.Unpin is set, clear pin on all packages and persist.
	if opts.Unpin {
		for _, pkg := range pkgs {
			if pkg.Pinned {
				pkg.Pinned = false
				if err := manifest.Save(pkg, m.libDir); err != nil {
					return nil, fmt.Errorf("update: clear pin for %q: %w", pkg.ID, err)
				}
			}
		}
	}

	// 3. Check for updates in parallel: for each package, call backend.Check.
	type checkResult struct {
		pkg        *manifest.Package
		newVersion string // version to install
		needUpdate bool   // true if installation should proceed
	}

	checks := make([]checkResult, len(pkgs))
	var mu sync.Mutex
	var wg sync.WaitGroup
	var firstCheckErr error

	for i, pkg := range pkgs {
		wg.Add(1)
		go func(idx int, p *manifest.Package) {
			defer wg.Done()

			b, err := m.registry.DispatchByType(p.Backend)
			if err != nil {
				mu.Lock()
				if firstCheckErr == nil {
					firstCheckErr = fmt.Errorf("update: package %q: dispatch backend %q: %w", p.ID, p.Backend, err)
				}
				mu.Unlock()
				return
			}

			resolution, err := b.Check(ctx, p)
			if err != nil {
				mu.Lock()
				if firstCheckErr == nil {
					firstCheckErr = fmt.Errorf("update: package %q: check: %w", p.ID, err)
				}
				mu.Unlock()
				return
			}

			target := explicitTargets[p.ID]
			var newVersion string
			var needUpdate bool

			if target.Version != "" {
				// User explicitly requested a specific version — always reinstall.
				newVersion = target.Version
				needUpdate = true
			} else {
				newVersion = resolution.Version
				needUpdate = resolution.Version != p.Version
			}

			mu.Lock()
			checks[idx] = checkResult{
				pkg:        p,
				newVersion: newVersion,
				needUpdate: needUpdate,
			}
			mu.Unlock()
		}(i, pkg)
	}

	wg.Wait()

	if firstCheckErr != nil {
		return nil, firstCheckErr
	}

	// 4. For each package that needs updating, run Install using stored specs.
	results := make([]*UpdateResult, 0, len(checks))

	for i := range checks {
		cr := &checks[i]
		pkg := cr.pkg

		result := &UpdateResult{
			ID:         pkg.ID,
			OldVersion: pkg.Version,
			NewVersion: cr.newVersion,
			Updated:    false,
		}

		if !cr.needUpdate {
			results = append(results, result)
			continue
		}

		// Reconstruct InstallOptions from the manifest's stored (unexpanded) specs.
		installOpts := InstallOptions{
			SourceURL: pkg.SourceURL,
			Version:   cr.newVersion,
			Pin:       pkg.Pinned,
		}

		// Reconstruct SpecOpts from each stored InstallSpec.
		// Use the absolute LocalPath of the first installed file (if present) as
		// LocalName so that each spec reinstalls to exactly the same location
		// regardless of DefaultDir. This avoids a bug where specs installed to
		// different directories would all be redirected to the first spec's
		// directory on update.
		installOpts.Specs = make([]SpecOpts, 0, len(pkg.Specs))
		for _, spec := range pkg.Specs {
			// For multisum, the manifest stores the data file glob in DataGlob;
			// SpecOpts / ChecksumOpts uses FileGlob for both shared-file and
			// multisum data files.
			fileGlob := spec.Checksum.FileGlob
			if spec.Checksum.Strategy == "multisum" {
				fileGlob = spec.Checksum.DataGlob
			}

			localName := spec.LocalName
			if len(spec.InstalledFiles) > 0 && spec.InstalledFiles[0].LocalPath != "" {
				localName = spec.InstalledFiles[0].LocalPath
			}

			installOpts.Specs = append(installOpts.Specs, SpecOpts{
				AssetGlob:      spec.AssetGlob,
				TraversalGlobs: spec.TraversalGlobs,
				LocalName:      localName,
				Checksum: ChecksumOpts{
					Strategy:      spec.Checksum.Strategy,
					FileGlob:      fileGlob,
					OrderGlob:     spec.Checksum.OrderGlob,
					Suffix:        spec.Checksum.Suffix,
					TraversalGlob: spec.Checksum.TraversalGlob,
				},
			})
		}

		if err := m.Install(ctx, installOpts); err != nil {
			return nil, fmt.Errorf("update: install %q: %w", pkg.ID, err)
		}

		result.Updated = true
		result.NewVersion = cr.newVersion

		// 5. If opts.Pin is set, reload the manifest and mark it pinned.
		if opts.Pin {
			updated, err := manifest.Load(pkg.ID, m.libDir)
			if err != nil {
				return nil, fmt.Errorf("update: reload manifest for pin %q: %w", pkg.ID, err)
			}
			updated.Pinned = true
			if err := manifest.Save(updated, m.libDir); err != nil {
				return nil, fmt.Errorf("update: save pin for %q: %w", pkg.ID, err)
			}
		}

		results = append(results, result)
	}

	return results, nil
}
