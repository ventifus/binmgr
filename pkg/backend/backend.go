package backend

import (
	"context"
	"net/url"

	"github.com/ventifus/binmgr/pkg/manifest"
)

// Backend is the interface implemented by each source backend.
type Backend interface {
	Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error)
	Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error)
	Type() string
	CanHandle(u *url.URL) bool
}

// ResolveOptions are the options passed to Backend.Resolve.
type ResolveOptions struct {
	Version string // if non-empty, resolve this specific release; empty = latest
}

// Resolution is the result of resolving a package version (version + asset list).
type Resolution struct {
	Version string  // github: tag; kubeurl: stable.txt content; shasumurl: SHA-256 of file
	Assets  []Asset // nil for kubeurl (manager constructs URLs from asset_glob + version)
}

// Asset is one downloadable file within a resolution.
type Asset struct {
	Name      string
	URL       string
	Checksums map[string]string // non-empty only for shasumurl
}
