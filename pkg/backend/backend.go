package backend

import (
	"context"
	"net/url"

	"github.com/ventifus/binmgr/pkg/manifest"
)

type Backend interface {
	Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error)
	Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error)
	Type() string
	CanHandle(u *url.URL) bool
}

type ResolveOptions struct {
	Version string // if non-empty, resolve this specific release; empty = latest
}

type Resolution struct {
	Version string  // github: tag; kubeurl: stable.txt content; shasumurl: SHA-256 of file
	Assets  []Asset // nil for kubeurl (manager constructs URLs from asset_glob + version)
}

type Asset struct {
	Name      string
	URL       string
	Checksums map[string]string // non-empty only for shasumurl
}
