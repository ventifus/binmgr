package backend

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ventifus/binmgr/pkg/manifest"
)

// fixtureRelease is a sample GitHub Releases API response.
var fixtureRelease = githubRelease{
	TagName: "v1.2.3",
	Assets: []githubAsset{
		{Name: "just-1.2.3-x86_64-unknown-linux-musl.tar.gz", BrowserDownloadURL: "https://github.com/casey/just/releases/download/v1.2.3/just-1.2.3-x86_64-unknown-linux-musl.tar.gz"},
		{Name: "SHA256SUMS", BrowserDownloadURL: "https://github.com/casey/just/releases/download/v1.2.3/SHA256SUMS"},
	},
}

// newTestServer creates a test HTTP server that serves a fixed release fixture.
func newTestServer(t *testing.T, release githubRelease, statusCode int) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(statusCode)
		if statusCode == http.StatusOK {
			if err := json.NewEncoder(w).Encode(release); err != nil {
				t.Errorf("encoding fixture: %v", err)
			}
		}
	}))
}

// rewriteTransport redirects all requests to a fixed base URL (test server).
type rewriteTransport struct {
	base string
}

func (rt *rewriteTransport) RoundTrip(req *http.Request) (*http.Response, error) {
	target, _ := url.Parse(rt.base)
	req2 := req.Clone(req.Context())
	req2.URL.Scheme = target.Scheme
	req2.URL.Host = target.Host
	return http.DefaultTransport.RoundTrip(req2)
}

// backendWithBaseURL returns a githubBackend that redirects API calls to baseURL.
func backendWithBaseURL(token string, baseURL string) *githubBackend {
	return &githubBackend{
		token:      token,
		httpClient: &http.Client{Transport: &rewriteTransport{base: baseURL}},
	}
}

func TestCanHandle(t *testing.T) {
	b := NewGitHubBackend().(*githubBackend)

	tests := []struct {
		rawURL string
		want   bool
	}{
		{"https://github.com/casey/just", true},
		{"https://github.com/cli/cli", true},
		{"https://gitlab.com/casey/just", false},
		{"https://example.com/foo/bar", false},
		{"https://dl.k8s.io/release/stable.txt", false},
	}

	for _, tt := range tests {
		u, err := url.Parse(tt.rawURL)
		if err != nil {
			t.Fatalf("parsing %q: %v", tt.rawURL, err)
		}
		got := b.CanHandle(u)
		if got != tt.want {
			t.Errorf("CanHandle(%q) = %v, want %v", tt.rawURL, got, tt.want)
		}
	}
}

func TestType(t *testing.T) {
	b := NewGitHubBackend()
	if got := b.Type(); got != "github" {
		t.Errorf("Type() = %q, want %q", got, "github")
	}
}

func TestResolveLatest(t *testing.T) {
	srv := newTestServer(t, fixtureRelease, http.StatusOK)
	defer srv.Close()

	b := backendWithBaseURL("", srv.URL)
	u, _ := url.Parse("https://github.com/casey/just")

	res, err := b.Resolve(context.Background(), u, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if res.Version != "v1.2.3" {
		t.Errorf("Version = %q, want %q", res.Version, "v1.2.3")
	}
	if len(res.Assets) != 2 {
		t.Fatalf("len(Assets) = %d, want 2", len(res.Assets))
	}
	if res.Assets[0].Name != "just-1.2.3-x86_64-unknown-linux-musl.tar.gz" {
		t.Errorf("Assets[0].Name = %q", res.Assets[0].Name)
	}
	if res.Assets[0].Checksums != nil {
		t.Errorf("Assets[0].Checksums should be nil for github backend")
	}
}

func TestResolveSpecificTag(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fixtureRelease)
	}))
	defer srv.Close()

	b := backendWithBaseURL("", srv.URL)
	u, _ := url.Parse("https://github.com/casey/just")

	_, err := b.Resolve(context.Background(), u, ResolveOptions{Version: "v1.2.3"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	wantPath := "/repos/casey/just/releases/tags/v1.2.3"
	if capturedPath != wantPath {
		t.Errorf("API path = %q, want %q", capturedPath, wantPath)
	}
}

func TestResolveLatestEndpoint(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fixtureRelease)
	}))
	defer srv.Close()

	b := backendWithBaseURL("", srv.URL)
	u, _ := url.Parse("https://github.com/casey/just")

	_, err := b.Resolve(context.Background(), u, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	wantPath := "/repos/casey/just/releases/latest"
	if capturedPath != wantPath {
		t.Errorf("API path = %q, want %q", capturedPath, wantPath)
	}
}

func TestCheck(t *testing.T) {
	var capturedPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fixtureRelease)
	}))
	defer srv.Close()

	b := backendWithBaseURL("", srv.URL)
	pkg := &manifest.Package{
		SourceURL: "https://github.com/casey/just",
		Version:   "v1.0.0",
	}

	res, err := b.Check(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}
	if res.Version != "v1.2.3" {
		t.Errorf("Version = %q, want %q", res.Version, "v1.2.3")
	}
	wantPath := "/repos/casey/just/releases/latest"
	if capturedPath != wantPath {
		t.Errorf("Check API path = %q, want %q", capturedPath, wantPath)
	}
}

func TestAuthHeaderPresent(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fixtureRelease)
	}))
	defer srv.Close()

	b := backendWithBaseURL("my-secret-token", srv.URL)
	u, _ := url.Parse("https://github.com/casey/just")

	_, err := b.Resolve(context.Background(), u, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	want := "Bearer my-secret-token"
	if capturedAuth != want {
		t.Errorf("Authorization header = %q, want %q", capturedAuth, want)
	}
}

func TestAuthHeaderAbsent(t *testing.T) {
	var capturedAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		_ = json.NewEncoder(w).Encode(fixtureRelease)
	}))
	defer srv.Close()

	b := backendWithBaseURL("", srv.URL)
	u, _ := url.Parse("https://github.com/casey/just")

	_, err := b.Resolve(context.Background(), u, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}
	if capturedAuth != "" {
		t.Errorf("Authorization header present when no token: %q", capturedAuth)
	}
}

func TestResolve404(t *testing.T) {
	srv := newTestServer(t, githubRelease{}, http.StatusNotFound)
	defer srv.Close()

	b := backendWithBaseURL("", srv.URL)
	u, _ := url.Parse("https://github.com/casey/nonexistent")

	_, err := b.Resolve(context.Background(), u, ResolveOptions{})
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
}
