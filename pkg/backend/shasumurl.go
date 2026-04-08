package backend

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"

	"github.com/ventifus/binmgr/pkg/manifest"
)

type shasumBackend struct {
	client *http.Client
}

// NewShasumBackend returns a Backend for sha256sum.txt-based package sources.
func NewShasumBackend() Backend {
	return &shasumBackend{client: &http.Client{}}
}

// CanHandle always returns false; this backend requires explicit --type shasumurl.
func (s *shasumBackend) CanHandle(u *url.URL) bool {
	return false
}

// Type returns the backend type string.
func (s *shasumBackend) Type() string {
	return "shasumurl"
}

// Resolve fetches the sha256sum.txt at sourceURL, hashes its content to produce
// a version, and parses each line into an Asset with a resolved download URL and
// the embedded checksum.
func (s *shasumBackend) Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error) {
	content, err := s.fetchURL(ctx, sourceURL.String())
	if err != nil {
		return nil, fmt.Errorf("shasumurl: fetch %s: %w", sourceURL, err)
	}

	version := fmt.Sprintf("%x", sha256.Sum256(content))

	assets, err := parseShasumFile(content, sourceURL)
	if err != nil {
		return nil, fmt.Errorf("shasumurl: parse %s: %w", sourceURL, err)
	}

	return &Resolution{
		Version: version,
		Assets:  assets,
	}, nil
}

// Check re-fetches the sha256sum.txt and returns a Resolution whose Version is
// the SHA-256 hex of the current file content. The caller compares this to
// pkg.Version to detect updates.
func (s *shasumBackend) Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error) {
	content, err := s.fetchURL(ctx, pkg.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("shasumurl: check fetch %s: %w", pkg.SourceURL, err)
	}

	version := fmt.Sprintf("%x", sha256.Sum256(content))
	return &Resolution{Version: version}, nil
}

// fetchURL performs an HTTP GET and returns the response body as bytes.
func (s *shasumBackend) fetchURL(ctx context.Context, rawURL string) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, rawURL, nil)
	if err != nil {
		return nil, err
	}
	resp, err := s.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("HTTP %d from %s", resp.StatusCode, rawURL)
	}
	return io.ReadAll(resp.Body)
}

// parseShasumFile parses a sha256sums-format file (lines of "{hex}  {filename}")
// and builds an Asset list. Blank lines and lines starting with '#' are skipped.
func parseShasumFile(content []byte, sourceURL *url.URL) ([]Asset, error) {
	dirPath := path.Dir(sourceURL.Path)
	var assets []Asset

	scanner := bufio.NewScanner(bytes.NewReader(content))
	for scanner.Scan() {
		line := scanner.Text()
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}

		// Standard sha256sums format: exactly two spaces between digest and filename.
		parts := strings.SplitN(line, "  ", 2)
		if len(parts) != 2 {
			continue
		}
		hexDigest := strings.TrimSpace(parts[0])
		filename := strings.TrimSpace(parts[1])
		if hexDigest == "" || filename == "" {
			continue
		}

		assetURL := url.URL{
			Scheme: sourceURL.Scheme,
			Host:   sourceURL.Host,
			Path:   path.Join(dirPath, filename),
		}

		assets = append(assets, Asset{
			Name: filename,
			URL:  assetURL.String(),
			Checksums: map[string]string{
				"sha-256": hexDigest,
			},
		})
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return assets, nil
}
