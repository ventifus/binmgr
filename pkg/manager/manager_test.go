package manager

import (
	"context"
	"encoding/json"
	"net/url"
	"os"
	"path/filepath"
	"testing"

	"github.com/ventifus/binmgr/pkg/backend"
	"github.com/ventifus/binmgr/pkg/extract"
	"github.com/ventifus/binmgr/pkg/manifest"
)

// ========== Mock implementations ==========

// MockFetcher is a configurable mock for fetch.Fetcher.
type MockFetcher struct {
	FetchFn func(ctx context.Context, url string) ([]byte, error)
}

func (m *MockFetcher) Fetch(ctx context.Context, url string) ([]byte, error) {
	return m.FetchFn(ctx, url)
}

// MockExtractor is a configurable mock for extract.Extractor.
type MockExtractor struct {
	ExtractFn func(ctx context.Context, name string, data []byte, globs []string) ([]extract.ExtractedFile, error)
}

func (m *MockExtractor) Extract(ctx context.Context, name string, data []byte, globs []string) ([]extract.ExtractedFile, error) {
	return m.ExtractFn(ctx, name, data, globs)
}

// MockVerifier is a configurable mock for verify.Verifier.
type MockVerifier struct {
	VerifyFn  func(ctx context.Context, data []byte, expected map[string]string) error
	ComputeFn func(ctx context.Context, data []byte, algorithms []string) (map[string]string, error)
}

func (m *MockVerifier) Verify(ctx context.Context, data []byte, expected map[string]string) error {
	return m.VerifyFn(ctx, data, expected)
}

func (m *MockVerifier) Compute(ctx context.Context, data []byte, algorithms []string) (map[string]string, error) {
	return m.ComputeFn(ctx, data, algorithms)
}

// MockBackend is a configurable mock for backend.Backend.
type MockBackend struct {
	ResolveFn   func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error)
	CheckFn     func(ctx context.Context, pkg *manifest.Package) (*backend.Resolution, error)
	TypeFn      func() string
	CanHandleFn func(u *url.URL) bool
}

func (m *MockBackend) Resolve(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
	return m.ResolveFn(ctx, sourceURL, opts)
}

func (m *MockBackend) Check(ctx context.Context, pkg *manifest.Package) (*backend.Resolution, error) {
	return m.CheckFn(ctx, pkg)
}

func (m *MockBackend) Type() string {
	return m.TypeFn()
}

func (m *MockBackend) CanHandle(u *url.URL) bool {
	return m.CanHandleFn(u)
}

// ========== Helpers ==========

// writeManifest writes a Package as JSON into dir using manifest naming conventions.
func writeManifest(t *testing.T, dir string, pkg *manifest.Package) {
	t.Helper()
	data, err := json.MarshalIndent(pkg, "", "  ")
	if err != nil {
		t.Fatalf("marshal manifest: %v", err)
	}
	path := filepath.Join(dir, manifest.IDToFilename(pkg.ID))
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatalf("write manifest: %v", err)
	}
}

// newTestManager creates a manager wired to a fake registry and returns it
// along with the manifest lib directory (which is HOME/.local/share/binmgr/).
func newTestManager(t *testing.T) (Manager, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}

	reg := backend.NewRegistry()
	m := New(reg, &MockFetcher{}, &MockExtractor{}, &MockVerifier{}, libDir)
	return m, libDir
}

// ========== List tests ==========

func TestList_ReturnsAllPackages(t *testing.T) {
	mgr, libDir := newTestManager(t)

	pkgA := &manifest.Package{
		ID:      "tool-a",
		Backend: "github",
		Version: "v1.0.0",
	}
	pkgB := &manifest.Package{
		ID:      "tool-b",
		Backend: "github",
		Version: "v2.3.0",
	}
	writeManifest(t, libDir, pkgA)
	writeManifest(t, libDir, pkgB)

	ctx := context.Background()
	packages, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(packages) != 2 {
		t.Fatalf("expected 2 packages, got %d", len(packages))
	}

	// Build an ID set for order-independent checking.
	ids := make(map[string]bool, len(packages))
	for _, p := range packages {
		ids[p.ID] = true
	}
	for _, want := range []string{"tool-a", "tool-b"} {
		if !ids[want] {
			t.Errorf("expected package %q in results", want)
		}
	}
}

func TestList_EmptyWhenNoneInstalled(t *testing.T) {
	mgr, _ := newTestManager(t)

	ctx := context.Background()
	packages, err := mgr.List(ctx)
	if err != nil {
		t.Fatalf("List returned error: %v", err)
	}

	if len(packages) != 0 {
		t.Fatalf("expected 0 packages, got %d", len(packages))
	}
}

// ========== Uninstall tests ==========

func TestUninstall_DeletesFilesAndManifest(t *testing.T) {
	mgr, libDir := newTestManager(t)

	// Create a real file on disk to be removed.
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "mytool")
	if err := os.WriteFile(binPath, []byte("binary"), 0755); err != nil {
		t.Fatalf("create installed file: %v", err)
	}

	pkg := &manifest.Package{
		ID:      "mytool",
		Backend: "github",
		Version: "v1.0.0",
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "mytool-linux-amd64",
				InstalledFiles: []manifest.InstalledFile{
					{LocalPath: binPath, Checksums: map[string]string{}},
				},
			},
		},
	}
	writeManifest(t, libDir, pkg)

	ctx := context.Background()
	if err := mgr.Uninstall(ctx, []string{"mytool"}); err != nil {
		t.Fatalf("Uninstall returned error: %v", err)
	}

	// Installed file must be gone.
	if _, err := os.Stat(binPath); !os.IsNotExist(err) {
		t.Errorf("expected installed file to be removed, got stat err: %v", err)
	}

	// Manifest must be gone.
	manifestPath := filepath.Join(libDir, manifest.IDToFilename("mytool"))
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Errorf("expected manifest to be removed, got stat err: %v", err)
	}
}

func TestUninstall_ToleratesMissingFiles(t *testing.T) {
	mgr, libDir := newTestManager(t)

	pkg := &manifest.Package{
		ID:      "ghosttool",
		Backend: "github",
		Version: "v1.0.0",
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "ghosttool-linux-amd64",
				InstalledFiles: []manifest.InstalledFile{
					{LocalPath: "/nonexistent/path/ghosttool", Checksums: map[string]string{}},
				},
			},
		},
	}
	writeManifest(t, libDir, pkg)

	ctx := context.Background()
	// Should not return an error even though the file does not exist on disk.
	if err := mgr.Uninstall(ctx, []string{"ghosttool"}); err != nil {
		t.Fatalf("Uninstall returned error for missing file: %v", err)
	}

	// Manifest must be gone.
	manifestPath := filepath.Join(libDir, manifest.IDToFilename("ghosttool"))
	if _, err := os.Stat(manifestPath); !os.IsNotExist(err) {
		t.Errorf("expected manifest to be removed, got stat err: %v", err)
	}
}

func TestUninstall_ErrorWhenPackageNotFound(t *testing.T) {
	mgr, _ := newTestManager(t)

	ctx := context.Background()
	err := mgr.Uninstall(ctx, []string{"no-such-package"})
	if err == nil {
		t.Fatal("expected error for unknown package, got nil")
	}
}
