package backend

import (
	"context"
	"crypto/sha256"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"

	"github.com/ventifus/binmgr/pkg/manifest"
)

const fixtureShaSumTxt = `# This is a comment and should be skipped

abc123def456abc123def456abc123def456abc123def456abc123def456abc1  openshift-client-linux-amd64-rhel9-4.20.10.tar.gz
def456abc123def456abc123def456abc123def456abc123def456abc123def4  openshift-install-linux-4.20.10.tar.gz

`

func contentVersion(content string) string {
	return fmt.Sprintf("%x", sha256.Sum256([]byte(content)))
}

func newTestServer(t *testing.T, content string) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(content))
	}))
}

func TestShasumBackendCanHandle(t *testing.T) {
	b := NewShasumBackend()
	u, _ := url.Parse("https://mirror.openshift.com/pub/openshift-v4/x86_64/clients/ocp/stable/sha256sum.txt")
	if b.CanHandle(u) {
		t.Error("CanHandle should always return false for shasumurl backend")
	}
}

func TestShasumBackendType(t *testing.T) {
	b := NewShasumBackend()
	if got := b.Type(); got != "shasumurl" {
		t.Errorf("Type() = %q, want %q", got, "shasumurl")
	}
}

func TestShasumBackendResolve_AssetList(t *testing.T) {
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/pub/ocp/stable/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(res.Assets) != 2 {
		t.Fatalf("expected 2 assets, got %d", len(res.Assets))
	}
}

func TestShasumBackendResolve_AssetNames(t *testing.T) {
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/pub/ocp/stable/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	wantNames := []string{
		"openshift-client-linux-amd64-rhel9-4.20.10.tar.gz",
		"openshift-install-linux-4.20.10.tar.gz",
	}
	for i, want := range wantNames {
		if res.Assets[i].Name != want {
			t.Errorf("Assets[%d].Name = %q, want %q", i, res.Assets[i].Name, want)
		}
	}
}

func TestShasumBackendResolve_AssetURLs(t *testing.T) {
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/pub/ocp/stable/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	// Asset URLs should be relative to the checksum file's directory.
	wantSuffix := "/pub/ocp/stable/openshift-client-linux-amd64-rhel9-4.20.10.tar.gz"
	if got := res.Assets[0].URL; !hasURLPathSuffix(got, wantSuffix) {
		t.Errorf("Assets[0].URL = %q, want path suffix %q", got, wantSuffix)
	}
}

func TestShasumBackendResolve_Checksums(t *testing.T) {
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/pub/ocp/stable/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	wantChecksums := []string{
		"abc123def456abc123def456abc123def456abc123def456abc123def456abc1",
		"def456abc123def456abc123def456abc123def456abc123def456abc123def4",
	}
	for i, want := range wantChecksums {
		got, ok := res.Assets[i].Checksums["sha-256"]
		if !ok {
			t.Errorf("Assets[%d].Checksums missing sha-256 key", i)
			continue
		}
		if got != want {
			t.Errorf("Assets[%d].Checksums[sha-256] = %q, want %q", i, got, want)
		}
	}
}

func TestShasumBackendResolve_Version(t *testing.T) {
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/pub/ocp/stable/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	want := contentVersion(fixtureShaSumTxt)
	if res.Version != want {
		t.Errorf("Version = %q, want %q", res.Version, want)
	}
}

func TestShasumBackendResolve_ChangedContentGivesDifferentVersion(t *testing.T) {
	content1 := "abc123  file-1.0.tar.gz\n"
	content2 := "def456  file-2.0.tar.gz\n"

	srv1 := newTestServer(t, content1)
	defer srv1.Close()
	srv2 := newTestServer(t, content2)
	defer srv2.Close()

	b := NewShasumBackend()

	u1, _ := url.Parse(srv1.URL + "/sha256sum.txt")
	res1, err := b.Resolve(context.Background(), u1, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve(content1) error: %v", err)
	}

	u2, _ := url.Parse(srv2.URL + "/sha256sum.txt")
	res2, err := b.Resolve(context.Background(), u2, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve(content2) error: %v", err)
	}

	if res1.Version == res2.Version {
		t.Error("different file contents should produce different version hashes")
	}
}

func TestShasumBackendResolve_SkipsBlankLinesAndComments(t *testing.T) {
	content := `# header comment

aabbcc  tool-v1.tar.gz

# another comment
ddeeff  tool-v2.tar.gz
`
	srv := newTestServer(t, content)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	if len(res.Assets) != 2 {
		t.Fatalf("expected 2 assets (blank lines and comments skipped), got %d", len(res.Assets))
	}
	if res.Assets[0].Name != "tool-v1.tar.gz" {
		t.Errorf("Assets[0].Name = %q, want %q", res.Assets[0].Name, "tool-v1.tar.gz")
	}
	if res.Assets[1].Name != "tool-v2.tar.gz" {
		t.Errorf("Assets[1].Name = %q, want %q", res.Assets[1].Name, "tool-v2.tar.gz")
	}
}

func TestShasumBackendCheck(t *testing.T) {
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	pkg := &manifest.Package{
		SourceURL: srv.URL + "/pub/ocp/stable/sha256sum.txt",
		Version:   "oldversion",
	}

	res, err := b.Check(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	want := contentVersion(fixtureShaSumTxt)
	if res.Version != want {
		t.Errorf("Check Version = %q, want %q", res.Version, want)
	}
}

func TestShasumBackendCheck_DetectsChange(t *testing.T) {
	oldContent := "abc123  file-1.0.tar.gz\n"
	newContent := "def456  file-2.0.tar.gz\n"

	// Serve the new content.
	srv := newTestServer(t, newContent)
	defer srv.Close()

	b := NewShasumBackend()
	pkg := &manifest.Package{
		SourceURL: srv.URL + "/sha256sum.txt",
		Version:   contentVersion(oldContent),
	}

	res, err := b.Check(context.Background(), pkg)
	if err != nil {
		t.Fatalf("Check returned error: %v", err)
	}

	if res.Version == pkg.Version {
		t.Error("Check should return a different version when file content has changed")
	}
}

func TestShasumBackendResolve_IgnoresVersionOpt(t *testing.T) {
	// For shasumurl the version is a content hash; version pinning is not supported.
	// Resolve with opts.Version set should return the current latest anyway.
	srv := newTestServer(t, fixtureShaSumTxt)
	defer srv.Close()

	b := NewShasumBackend()
	sourceURL, _ := url.Parse(srv.URL + "/sha256sum.txt")

	res, err := b.Resolve(context.Background(), sourceURL, ResolveOptions{Version: "somepinhash"})
	if err != nil {
		t.Fatalf("Resolve returned error: %v", err)
	}

	want := contentVersion(fixtureShaSumTxt)
	if res.Version != want {
		t.Errorf("Version = %q, want current hash %q (opts.Version should be ignored)", res.Version, want)
	}
}

// hasURLPathSuffix returns true if rawURL's path ends with the given suffix.
func hasURLPathSuffix(rawURL, suffix string) bool {
	u, err := url.Parse(rawURL)
	if err != nil {
		return false
	}
	return len(u.Path) >= len(suffix) && u.Path[len(u.Path)-len(suffix):] == suffix
}
