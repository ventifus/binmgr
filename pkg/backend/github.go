package backend

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"os"
	"path"
	"strings"

	"github.com/apex/log"
	"github.com/ventifus/binmgr/pkg/manifest"
	"go.yaml.in/yaml/v3"
)

// githubBackend implements Backend for GitHub Releases.
type githubBackend struct {
	token      string
	httpClient *http.Client // nil means use http.DefaultClient
}

// ghHostConfig holds per-host config from ~/.config/gh/hosts.yml.
type ghHostConfig struct {
	OauthToken string `yaml:"oauth_token"`
}

// ghConfig is the top-level gh CLI config file (hosts are inline keys).
type ghConfig struct {
	Hosts map[string]ghHostConfig `yaml:",inline"`
}

// loadGHToken reads the GitHub OAuth token from the gh CLI config file.
// Returns an empty string (and logs a warning) if unavailable.
func loadGHToken() string {
	p := path.Join(os.Getenv("HOME"), ".config/gh/hosts.yml")
	f, err := os.Open(p)
	if err != nil {
		log.WithError(err).Warn("could not open gh config; using unauthenticated GitHub access")
		return ""
	}
	defer f.Close()

	var cfg ghConfig
	dec := yaml.NewDecoder(f)
	if err := dec.Decode(&cfg); err != nil {
		log.WithError(err).Warn("could not parse gh config; using unauthenticated GitHub access")
		return ""
	}

	host, ok := cfg.Hosts["github.com"]
	if !ok || host.OauthToken == "" {
		log.Warn("no oauth_token for github.com in gh config; using unauthenticated GitHub access")
		return ""
	}
	return host.OauthToken
}

// NewGitHubBackend creates a new GitHub backend, loading auth token from gh CLI config if available.
func NewGitHubBackend() Backend {
	return &githubBackend{token: loadGHToken()}
}

// CanHandle returns true when the URL host is github.com.
func (g *githubBackend) CanHandle(u *url.URL) bool {
	return u.Host == "github.com"
}

// Type returns the backend type string.
func (g *githubBackend) Type() string {
	return "github"
}

// githubRelease is a minimal representation of the GitHub Releases API response.
type githubRelease struct {
	TagName string         `json:"tag_name"`
	Assets  []githubAsset  `json:"assets"`
}

type githubAsset struct {
	Name               string `json:"name"`
	BrowserDownloadURL string `json:"browser_download_url"`
}

// client returns the HTTP client to use, falling back to http.DefaultClient.
func (g *githubBackend) client() *http.Client {
	if g.httpClient != nil {
		return g.httpClient
	}
	return http.DefaultClient
}

// doRequest performs an authenticated (if token available) GET request to the GitHub API.
func (g *githubBackend) doRequest(ctx context.Context, apiURL string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, apiURL, nil)
	if err != nil {
		return nil, fmt.Errorf("creating request for %s: %w", apiURL, err)
	}
	req.Header.Set("User-Agent", "binmgr")
	req.Header.Set("Accept", "application/vnd.github+json")
	if g.token != "" {
		req.Header.Set("Authorization", "Bearer "+g.token)
	}
	return g.client().Do(req)
}

// ownerRepo extracts the owner and repo from a GitHub URL path (e.g. /casey/just).
func ownerRepo(sourceURL *url.URL) (owner, repo string, err error) {
	parts := strings.SplitN(strings.TrimPrefix(sourceURL.Path, "/"), "/", 3)
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return "", "", fmt.Errorf("invalid GitHub URL %q: expected /owner/repo path", sourceURL)
	}
	return parts[0], parts[1], nil
}

// resolveRelease resolves a GitHub release and returns a Resolution.
func (g *githubBackend) resolveRelease(ctx context.Context, owner, repo, version string) (*Resolution, error) {
	var apiURL string
	if version != "" {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/tags/%s", owner, repo, version)
	} else {
		apiURL = fmt.Sprintf("https://api.github.com/repos/%s/%s/releases/latest", owner, repo)
	}

	log.WithField("url", apiURL).Debug("resolving GitHub release")

	resp, err := g.doRequest(ctx, apiURL)
	if err != nil {
		return nil, fmt.Errorf("github request to %s: %w", apiURL, err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		// handled below
	case http.StatusNotFound:
		return nil, fmt.Errorf("github.com/%s/%s: release not found (404)", owner, repo)
	case http.StatusUnauthorized, http.StatusForbidden:
		return nil, fmt.Errorf("github.com/%s/%s: authentication error (%d)", owner, repo, resp.StatusCode)
	case http.StatusTooManyRequests:
		return nil, fmt.Errorf("github.com/%s/%s: rate limited (429)", owner, repo)
	default:
		return nil, fmt.Errorf("github.com/%s/%s: unexpected status %d", owner, repo, resp.StatusCode)
	}

	var rel githubRelease
	if err := json.NewDecoder(resp.Body).Decode(&rel); err != nil {
		return nil, fmt.Errorf("decoding GitHub release response: %w", err)
	}

	assets := make([]Asset, 0, len(rel.Assets))
	for _, a := range rel.Assets {
		assets = append(assets, Asset{
			Name: a.Name,
			URL:  a.BrowserDownloadURL,
			// Checksums intentionally empty; located via separate checksum files.
		})
	}

	return &Resolution{
		Version: rel.TagName,
		Assets:  assets,
	}, nil
}

// Resolve resolves a specific or latest GitHub release for the given URL.
func (g *githubBackend) Resolve(ctx context.Context, sourceURL *url.URL, opts ResolveOptions) (*Resolution, error) {
	owner, repo, err := ownerRepo(sourceURL)
	if err != nil {
		return nil, err
	}
	return g.resolveRelease(ctx, owner, repo, opts.Version)
}

// Check returns the latest release resolution, which the manager compares to pkg.Version.
func (g *githubBackend) Check(ctx context.Context, pkg *manifest.Package) (*Resolution, error) {
	u, err := url.Parse(pkg.SourceURL)
	if err != nil {
		return nil, fmt.Errorf("parsing source URL %q: %w", pkg.SourceURL, err)
	}
	owner, repo, err := ownerRepo(u)
	if err != nil {
		return nil, err
	}
	return g.resolveRelease(ctx, owner, repo, "")
}
