package backend

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/ventifus/binmgr/pkg/manifest"
)

type kubeBackend struct {
	client *http.Client
}

func NewKubeBackend() Backend {
	return &kubeBackend{
		client: &http.Client{},
	}
}

func (k *kubeBackend) CanHandle(u *url.URL) bool {
	return u.Host == "dl.k8s.io"
}

func (k *kubeBackend) Type() string {
	return "kubeurl"
}

func (k *kubeBackend) fetchVersion(ctx context.Context, versionURL string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, versionURL, nil)
	if err != nil {
		return "", fmt.Errorf("kubeurl: create request: %w", err)
	}
	req.Header.Set("User-Agent", "binmgr")

	resp, err := k.client.Do(req)
	if err != nil {
		return "", fmt.Errorf("kubeurl: fetch %s: %w", versionURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("kubeurl: fetch %s: unexpected status %s", versionURL, resp.Status)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return "", fmt.Errorf("kubeurl: read response from %s: %w", versionURL, err)
	}

	return strings.TrimSpace(string(b)), nil
}

func (k *kubeBackend) Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error) {
	if opts.Version != "" {
		return &Resolution{Version: opts.Version, Assets: nil}, nil
	}

	version, err := k.fetchVersion(ctx, sourceURL.String())
	if err != nil {
		return nil, err
	}

	return &Resolution{Version: version, Assets: nil}, nil
}

func (k *kubeBackend) Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error) {
	version, err := k.fetchVersion(ctx, pkg.SourceURL)
	if err != nil {
		return nil, err
	}

	return &Resolution{Version: version, Assets: nil}, nil
}
