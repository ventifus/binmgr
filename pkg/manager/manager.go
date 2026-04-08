package manager

import (
	"context"

	"github.com/ventifus/binmgr/pkg/backend"
	"github.com/ventifus/binmgr/pkg/extract"
	"github.com/ventifus/binmgr/pkg/fetch"
	"github.com/ventifus/binmgr/pkg/manifest"
	"github.com/ventifus/binmgr/pkg/verify"
)

// Manager orchestrates install, update, status, list, and uninstall operations.
type Manager interface {
	Install(ctx context.Context, opts InstallOptions) error
	Update(ctx context.Context, opts UpdateOptions) ([]*UpdateResult, error)
	Status(ctx context.Context, packages []string) ([]*StatusResult, error)
	List(ctx context.Context) ([]*manifest.Package, error)
	Uninstall(ctx context.Context, packages []string) error
}

// InstallOptions carries parameters for an install operation.
type InstallOptions struct {
	SourceURL   string
	Version     string // from @VERSION suffix; empty = latest
	Specs       []SpecOpts
	DefaultDir  string // empty = ~/.local/bin/
	BackendType string // --type override; empty = auto-detect
	Pin         bool
}

// SpecOpts describes one file (or set of files) to install from a package.
type SpecOpts struct {
	AssetGlob      string
	TraversalGlobs []string
	LocalName      string
	Checksum       ChecksumOpts
}

// ChecksumOpts specifies how to locate and verify checksums for a downloaded asset.
type ChecksumOpts struct {
	Strategy      string // "auto"|"none"|"shared-file"|"per-asset"|"multisum"|"embedded"
	FileGlob      string // shared-file: asset glob; multisum: data file glob
	OrderGlob     string // multisum: algorithm ordering file glob
	Suffix        string // per-asset: suffix appended to asset URL
	TraversalGlob string // embedded: traversal glob inside archive
}

// UpdateOptions carries parameters for an update operation.
type UpdateOptions struct {
	Packages []PackageTarget // empty = all non-pinned packages
	Pin      bool
	Unpin    bool
}

// PackageTarget identifies a package and optional target version for update.
type PackageTarget struct {
	ID      string
	Version string // empty = latest
}

// UpdateResult reports what happened during an update for one package.
type UpdateResult struct {
	ID         string
	OldVersion string
	NewVersion string
	Updated    bool
}

// StatusResult reports the current and available versions for one package.
type StatusResult struct {
	ID               string
	InstalledVersion string
	LatestVersion    string
	Pinned           bool
	UpdateAvailable  bool
}

type mgr struct {
	registry  *backend.Registry
	fetcher   fetch.Fetcher
	extractor extract.Extractor
	verifier  verify.Verifier
	libDir    string
}

// New returns a new Manager.
func New(
	registry *backend.Registry,
	f fetch.Fetcher,
	e extract.Extractor,
	v verify.Verifier,
	libDir string,
) Manager {
	return &mgr{
		registry:  registry,
		fetcher:   f,
		extractor: e,
		verifier:  v,
		libDir:    libDir,
	}
}

// Install is implemented in install.go.
// Update is implemented in update.go.
// Status is implemented in status.go.
