package manager

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"path/filepath"
	"strings"

	"github.com/ventifus/binmgr/pkg/backend"
	"github.com/ventifus/binmgr/pkg/extract"
	"github.com/ventifus/binmgr/pkg/manifest"
)

// Install downloads and installs binaries from a source URL, then records a
// manifest for future update and status operations.
func (m *mgr) Install(ctx context.Context, opts InstallOptions) error {
	// 1. Parse the source URL; prepend https:// if no scheme is present.
	sourceURL := opts.SourceURL
	if !strings.HasPrefix(sourceURL, "http") {
		sourceURL = "https://" + sourceURL
	}
	parsedURL, err := url.Parse(sourceURL)
	if err != nil {
		return fmt.Errorf("install: parse source URL %q: %w", sourceURL, err)
	}

	// 2. Select backend.
	var b backend.Backend
	if opts.BackendType != "" {
		b, err = m.registry.DispatchByType(opts.BackendType)
	} else {
		b, err = m.registry.Dispatch(parsedURL)
	}
	if err != nil {
		return fmt.Errorf("install: select backend: %w", err)
	}

	// 3. Resolve version and asset list.
	resolution, err := b.Resolve(ctx, parsedURL, backend.ResolveOptions{Version: opts.Version})
	if err != nil {
		return fmt.Errorf("install: resolve %q: %w", parsedURL, err)
	}

	// 4. Determine package ID and handle kubeurl special case.
	//    For kubeurl, Assets is nil; construct in-memory asset list from specs.
	var pkgID string
	if resolution.Assets == nil {
		// kubeurl: each spec names one binary path; build an asset per spec.
		// The package ID is host + first spec's AssetGlob.
		if len(opts.Specs) == 0 {
			return fmt.Errorf("install: kubeurl backend requires at least one spec")
		}
		pkgID = parsedURL.Host + "/" + opts.Specs[0].AssetGlob
		assets := make([]backend.Asset, 0, len(opts.Specs))
		for _, spec := range opts.Specs {
			assets = append(assets, backend.Asset{
				Name: spec.AssetGlob,
				URL:  fmt.Sprintf("https://%s/%s/%s", parsedURL.Host, resolution.Version, spec.AssetGlob),
			})
		}
		resolution.Assets = assets
	} else {
		pkgID = parsedURL.Host + parsedURL.Path
	}

	// Normalised source URL stored in manifest (no query/fragment).
	normalizedSourceURL := "https://" + parsedURL.Host + parsedURL.Path

	// 5-6. For each spec, expand vars and find the matching asset.
	//      Group by expanded AssetGlob to deduplicate downloads.
	type specWork struct {
		spec         SpecOpts
		expandedGlob string
		expandedCSum ChecksumOpts
		expandedTrav []string
		asset        *backend.Asset
	}
	works := make([]specWork, 0, len(opts.Specs))
	for i := range opts.Specs {
		spec := opts.Specs[i]
		expandedGlob := ExpandVars(spec.AssetGlob, resolution.Version)
		expandedTrav := make([]string, len(spec.TraversalGlobs))
		for j, g := range spec.TraversalGlobs {
			expandedTrav[j] = ExpandVars(g, resolution.Version)
		}
		expandedCSum := ChecksumOpts{
			Strategy:      spec.Checksum.Strategy,
			FileGlob:      ExpandVars(spec.Checksum.FileGlob, resolution.Version),
			OrderGlob:     ExpandVars(spec.Checksum.OrderGlob, resolution.Version),
			Suffix:        spec.Checksum.Suffix,
			TraversalGlob: ExpandVars(spec.Checksum.TraversalGlob, resolution.Version),
		}

		// Find matching asset for this spec's expanded glob.
		var matched *backend.Asset
		for j := range resolution.Assets {
			ok, err := filepath.Match(expandedGlob, resolution.Assets[j].Name)
			if err != nil {
				return fmt.Errorf("install: spec %d: invalid asset glob %q: %w", i, expandedGlob, err)
			}
			if ok {
				matched = &resolution.Assets[j]
				break
			}
		}
		if matched == nil {
			return fmt.Errorf("install: spec %d: no asset matched glob %q", i, expandedGlob)
		}

		works = append(works, specWork{
			spec:         spec,
			expandedGlob: expandedGlob,
			expandedCSum: expandedCSum,
			expandedTrav: expandedTrav,
			asset:        matched,
		})
	}

	// 7. Download and verify each unique asset URL exactly once.
	//    Key: asset URL → downloaded bytes + resolved checksums.
	type downloadResult struct {
		data      []byte
		checksums map[string]string
	}
	downloads := make(map[string]*downloadResult)

	for i := range works {
		assetURL := works[i].asset.URL
		if _, seen := downloads[assetURL]; seen {
			continue
		}

		// Fetch.
		data, err := m.fetcher.Fetch(ctx, assetURL)
		if err != nil {
			return fmt.Errorf("install: fetch %q: %w", assetURL, err)
		}

		// Resolve checksums.
		checksums, err := m.resolveChecksums(
			ctx,
			works[i].expandedCSum,
			works[i].asset.Name,
			data,
			assetURL,
			works[i].asset.Checksums,
			resolution,
			resolution.Version,
		)
		if err != nil {
			return fmt.Errorf("install: resolve checksums for %q: %w", works[i].asset.Name, err)
		}

		// Verify if checksums were returned.
		if checksums != nil {
			if err := m.verifier.Verify(ctx, data, checksums); err != nil {
				return fmt.Errorf("install: verify %q: %w", works[i].asset.Name, err)
			}
		}

		downloads[assetURL] = &downloadResult{data: data, checksums: checksums}
	}

	// 8. For each spec, extract and write files.
	defaultDir := opts.DefaultDir
	if defaultDir == "" {
		defaultDir = filepath.Join(os.Getenv("HOME"), ".local", "bin")
	} else if strings.HasPrefix(defaultDir, "~/") {
		defaultDir = filepath.Join(os.Getenv("HOME"), defaultDir[2:])
	}

	manifestSpecs := make([]manifest.InstallSpec, 0, len(opts.Specs))

	for i := range works {
		w := &works[i]
		dl := downloads[w.asset.URL]

		// Extract or treat as direct bytes.
		var files []extract.ExtractedFile
		if len(w.expandedTrav) > 0 {
			files, err = m.extractor.Extract(ctx, w.asset.Name, dl.data, w.expandedTrav)
			if err != nil {
				return fmt.Errorf("install: extract %q: %w", w.asset.Name, err)
			}
		} else {
			files = []extract.ExtractedFile{{SourcePath: "", Data: dl.data}}
		}

		var installedFiles []manifest.InstalledFile
		for _, file := range files {
			localPath := resolveLocalPath(w.spec.LocalName, file.SourcePath, w.asset.Name, defaultDir)

			if err := os.MkdirAll(filepath.Dir(localPath), 0755); err != nil {
				return fmt.Errorf("install: create directory for %q: %w", localPath, err)
			}

			fileChecksums, err := m.verifier.Compute(ctx, file.Data, []string{"sha-256"})
			if err != nil {
				return fmt.Errorf("install: compute checksums for %q: %w", localPath, err)
			}

			if err := os.WriteFile(localPath, file.Data, 0755); err != nil {
				return fmt.Errorf("install: write %q: %w", localPath, err)
			}

			installedFiles = append(installedFiles, manifest.InstalledFile{
				SourcePath: file.SourcePath,
				LocalPath:  localPath,
				Checksums:  fileChecksums,
			})
		}

		// Build manifest spec using original unexpanded patterns.
		// ChecksumOpts.FileGlob doubles as the data file glob for "multisum";
		// store it in the correct manifest field based on strategy.
		csumCfg := manifest.ChecksumConfig{
			Strategy:      w.spec.Checksum.Strategy,
			Suffix:        w.spec.Checksum.Suffix,
			OrderGlob:     w.spec.Checksum.OrderGlob,
			TraversalGlob: w.spec.Checksum.TraversalGlob,
		}
		switch w.spec.Checksum.Strategy {
		case "multisum":
			csumCfg.DataGlob = w.spec.Checksum.FileGlob
		default:
			csumCfg.FileGlob = w.spec.Checksum.FileGlob
		}

		mspec := manifest.InstallSpec{
			AssetGlob:      w.spec.AssetGlob,
			TraversalGlobs: w.spec.TraversalGlobs,
			LocalName:      w.spec.LocalName,
			Checksum:       csumCfg,
			Asset: &manifest.DownloadedAsset{
				URL:       w.asset.URL,
				Checksums: dl.checksums,
			},
			InstalledFiles: installedFiles,
		}

		manifestSpecs = append(manifestSpecs, mspec)
	}

	// 9. Build and save the manifest.
	pkg := &manifest.Package{
		ID:        pkgID,
		Backend:   b.Type(),
		SourceURL: normalizedSourceURL,
		Version:   resolution.Version,
		Pinned:    opts.Pin,
		Specs:     manifestSpecs,
	}

	// 10. Save.
	if err := manifest.Save(pkg, m.libDir); err != nil {
		return fmt.Errorf("install: save manifest for %q: %w", pkgID, err)
	}

	return nil
}

// resolveLocalPath determines the absolute local path for an installed file.
//
// Priority:
//  1. localName starts with "/" or "~/" → use it directly (absolute or home-relative).
//  2. localName is a bare filename → join with defaultDir.
//  3. localName is empty → use basename of sourcePath if non-empty, else basename
//     of assetName; join with defaultDir.
func resolveLocalPath(localName, sourcePath, assetName, defaultDir string) string {
	home := os.Getenv("HOME")

	if localName != "" {
		if strings.HasPrefix(localName, "/") {
			return localName
		}
		if strings.HasPrefix(localName, "~/") {
			return filepath.Join(home, localName[2:])
		}
		// Bare filename: join with defaultDir.
		return filepath.Join(defaultDir, localName)
	}

	// Empty localName: derive from source.
	var base string
	if sourcePath != "" {
		base = filepath.Base(sourcePath)
	} else {
		base = filepath.Base(assetName)
	}
	return filepath.Join(defaultDir, base)
}
