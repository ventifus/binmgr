package manager

import (
	"context"
	"encoding/json"
	"fmt"
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

// ========== Install tests ==========

// newInstallManager builds a *mgr wired with a mock backend, a controlled
// fetcher, and the provided verifier. HOME is set to a temp dir so the
// default install dir and manifest dir are isolated. Returns the manager,
// the mock backend (so tests can inspect calls), and the home directory.
func newInstallManager(
	t *testing.T,
	fetcher *MockFetcher,
	extractor *MockExtractor,
	verifier *MockVerifier,
	backendType string,
	resolution *backend.Resolution,
) (Manager, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	mb := &MockBackend{
		TypeFn: func() string { return backendType },
		CanHandleFn: func(u *url.URL) bool {
			return true // accept everything
		},
		ResolveFn: func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
			return resolution, nil
		},
		CheckFn: func(ctx context.Context, pkg *manifest.Package) (*backend.Resolution, error) {
			return resolution, nil
		},
	}

	reg := backend.NewRegistry()
	reg.Register(mb)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}

	m := New(reg, fetcher, extractor, verifier, libDir)
	return m, home
}

// defaultCompute returns a fixed checksum map for any input.
func defaultCompute(ctx context.Context, data []byte, algorithms []string) (map[string]string, error) {
	result := make(map[string]string, len(algorithms))
	for _, alg := range algorithms {
		result[alg] = fmt.Sprintf("deadbeef-%s", alg)
	}
	return result, nil
}

// noExtract returns the input as a single file with an empty SourcePath.
func noExtract(ctx context.Context, name string, data []byte, globs []string) ([]extract.ExtractedFile, error) {
	return []extract.ExtractedFile{{SourcePath: "", Data: data}}, nil
}

// TestInstall_NoneChecksum verifies that strategy "none" skips Verify but still
// calls Fetch once and saves a manifest.
func TestInstall_NoneChecksum(t *testing.T) {
	assetData := []byte("binary-content")
	fetchCount := 0
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			fetchCount++
			return assetData, nil
		},
	}
	verifyCount := 0
	verifier := &MockVerifier{
		VerifyFn: func(ctx context.Context, data []byte, expected map[string]string) error {
			verifyCount++
			return nil
		},
		ComputeFn: defaultCompute,
	}
	extractor := &MockExtractor{ExtractFn: noExtract}

	resolution := &backend.Resolution{
		Version: "v1.0.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}

	m, home := newInstallManager(t, fetcher, extractor, verifier, "github", resolution)

	opts := InstallOptions{
		SourceURL: "https://example.com/owner/mytool",
		Specs: []SpecOpts{
			{
				AssetGlob: "mytool-linux-amd64",
				LocalName: "mytool",
				Checksum:  ChecksumOpts{Strategy: "none"},
			},
		},
	}

	ctx := context.Background()
	if err := m.Install(ctx, opts); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	if fetchCount != 1 {
		t.Errorf("expected 1 Fetch call, got %d", fetchCount)
	}
	if verifyCount != 0 {
		t.Errorf("expected 0 Verify calls for strategy none, got %d", verifyCount)
	}

	// Manifest must exist.
	libDir := filepath.Join(home, ".local", "share", "binmgr")
	entries, err := os.ReadDir(libDir)
	if err != nil {
		t.Fatalf("read libDir: %v", err)
	}
	if len(entries) != 1 {
		t.Fatalf("expected 1 manifest file, got %d", len(entries))
	}

	// Installed file must exist at default location.
	installedPath := filepath.Join(home, ".local", "bin", "mytool")
	if _, err := os.Stat(installedPath); err != nil {
		t.Errorf("installed file not found at %q: %v", installedPath, err)
	}
}

// TestInstall_Deduplication verifies that two specs sharing the same expanded
// AssetGlob result in only one Fetch call.
func TestInstall_Deduplication(t *testing.T) {
	assetData := []byte("archive-content")
	fetchCount := 0
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			fetchCount++
			return assetData, nil
		},
	}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}
	extractCount := 0
	extractor := &MockExtractor{
		ExtractFn: func(ctx context.Context, name string, data []byte, globs []string) ([]extract.ExtractedFile, error) {
			extractCount++
			// Return one file named after the first glob.
			sourcePath := "bin"
			if len(globs) > 0 {
				sourcePath = globs[0]
			}
			return []extract.ExtractedFile{{SourcePath: sourcePath, Data: data}}, nil
		},
	}

	resolution := &backend.Resolution{
		Version: "v2.0.0",
		Assets: []backend.Asset{
			{Name: "bundle.tar.gz", URL: "https://example.com/bundle.tar.gz"},
		},
	}

	m, home := newInstallManager(t, fetcher, extractor, verifier, "github", resolution)

	opts := InstallOptions{
		SourceURL: "https://example.com/owner/tool",
		Specs: []SpecOpts{
			{
				AssetGlob:      "bundle.tar.gz",
				TraversalGlobs: []string{"toolA"},
				LocalName:      "toolA",
				Checksum:       ChecksumOpts{Strategy: "none"},
			},
			{
				AssetGlob:      "bundle.tar.gz",
				TraversalGlobs: []string{"toolB"},
				LocalName:      "toolB",
				Checksum:       ChecksumOpts{Strategy: "none"},
			},
		},
	}

	ctx := context.Background()
	if err := m.Install(ctx, opts); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	if fetchCount != 1 {
		t.Errorf("expected 1 Fetch call (dedup), got %d", fetchCount)
	}
	if extractCount != 2 {
		t.Errorf("expected 2 Extract calls (one per spec), got %d", extractCount)
	}

	// Both files must be present.
	for _, name := range []string{"toolA", "toolB"} {
		p := filepath.Join(home, ".local", "bin", name)
		if _, err := os.Stat(p); err != nil {
			t.Errorf("installed file %q not found: %v", p, err)
		}
	}
}

// TestInstall_KubeurlAssetURL verifies that the kubeurl code path constructs
// the correct asset URL from the version and spec AssetGlob, and that the
// package ID is host + "/" + assetGlob.
func TestInstall_KubeurlAssetURL(t *testing.T) {
	var fetchedURL string
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			fetchedURL = u
			return []byte("kubectl-binary"), nil
		},
	}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}
	extractor := &MockExtractor{ExtractFn: noExtract}

	// kubeurl backend returns nil Assets.
	resolution := &backend.Resolution{
		Version: "v1.35.0",
		Assets:  nil,
	}

	m, home := newInstallManager(t, fetcher, extractor, verifier, "kubeurl", resolution)

	opts := InstallOptions{
		SourceURL:   "https://dl.k8s.io/release/stable.txt",
		BackendType: "kubeurl",
		Specs: []SpecOpts{
			{
				AssetGlob: "bin/linux/amd64/kubectl",
				LocalName: "kubectl",
				Checksum:  ChecksumOpts{Strategy: "none"},
			},
		},
	}

	ctx := context.Background()
	if err := m.Install(ctx, opts); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	wantURL := "https://dl.k8s.io/v1.35.0/bin/linux/amd64/kubectl"
	if fetchedURL != wantURL {
		t.Errorf("fetched URL = %q, want %q", fetchedURL, wantURL)
	}

	// Check manifest ID.
	libDir := filepath.Join(home, ".local", "share", "binmgr")
	entries, err := os.ReadDir(libDir)
	if err != nil || len(entries) != 1 {
		t.Fatalf("expected 1 manifest, got %d (err: %v)", len(entries), err)
	}

	data, err := os.ReadFile(filepath.Join(libDir, entries[0].Name()))
	if err != nil {
		t.Fatalf("read manifest: %v", err)
	}
	var pkg manifest.Package
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}

	wantID := "dl.k8s.io/bin/linux/amd64/kubectl"
	if pkg.ID != wantID {
		t.Errorf("pkg.ID = %q, want %q", pkg.ID, wantID)
	}
}

// TestInstall_VersionExpansionUnexpandedInManifest verifies that ${VERSION} in
// an AssetGlob is expanded before glob matching (so it finds the right asset)
// but stored unexpanded in the manifest.
func TestInstall_VersionExpansionUnexpandedInManifest(t *testing.T) {
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			return []byte("binary"), nil
		},
	}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}
	extractor := &MockExtractor{ExtractFn: noExtract}

	resolution := &backend.Resolution{
		Version: "v1.46.0",
		Assets: []backend.Asset{
			// Asset name contains the literal version (as resolved by the backend).
			{Name: "just-1.46.0-x86_64-unknown-linux-musl.tar.gz",
				URL: "https://example.com/just-1.46.0-x86_64-unknown-linux-musl.tar.gz"},
		},
	}

	m, home := newInstallManager(t, fetcher, extractor, verifier, "github", resolution)

	opts := InstallOptions{
		SourceURL: "https://github.com/casey/just",
		Specs: []SpecOpts{
			{
				// Unexpanded pattern — ${VERSION} must be expanded at match time.
				AssetGlob: "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz",
				LocalName: "just",
				Checksum:  ChecksumOpts{Strategy: "none"},
			},
		},
	}

	ctx := context.Background()
	if err := m.Install(ctx, opts); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	// Read the manifest and verify AssetGlob is stored unexpanded.
	libDir := filepath.Join(home, ".local", "share", "binmgr")
	entries, _ := os.ReadDir(libDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(libDir, entries[0].Name()))
	var pkg manifest.Package
	if err := json.Unmarshal(data, &pkg); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if len(pkg.Specs) != 1 {
		t.Fatalf("expected 1 spec, got %d", len(pkg.Specs))
	}

	wantGlob := "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz"
	if pkg.Specs[0].AssetGlob != wantGlob {
		t.Errorf("AssetGlob in manifest = %q, want unexpanded %q", pkg.Specs[0].AssetGlob, wantGlob)
	}
}

// TestInstall_LocalNameResolution exercises the three local-name rules:
//  1. bare name → joined with DefaultDir
//  2. absolute path → used directly
//  3. empty name → basename of asset, joined with DefaultDir
func TestInstall_LocalNameResolution(t *testing.T) {
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			return []byte("data"), nil
		},
	}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}
	extractor := &MockExtractor{ExtractFn: noExtract}

	home := t.TempDir()
	t.Setenv("HOME", home)

	absDir := t.TempDir()
	absPath := filepath.Join(absDir, "tool-abs")

	customDir := t.TempDir()

	resolution := &backend.Resolution{
		Version: "v1.0.0",
		Assets: []backend.Asset{
			{Name: "tool-linux-amd64", URL: "https://example.com/tool-linux-amd64"},
			{Name: "tool-abs-linux-amd64", URL: "https://example.com/tool-abs-linux-amd64"},
			{Name: "tool-empty-linux-amd64", URL: "https://example.com/tool-empty-linux-amd64"},
		},
	}

	mb := &MockBackend{
		TypeFn:      func() string { return "github" },
		CanHandleFn: func(u *url.URL) bool { return true },
		ResolveFn: func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
			return resolution, nil
		},
		CheckFn: func(ctx context.Context, pkg *manifest.Package) (*backend.Resolution, error) {
			return resolution, nil
		},
	}
	reg := backend.NewRegistry()
	reg.Register(mb)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}

	m := New(reg, fetcher, extractor, verifier, libDir)

	opts := InstallOptions{
		SourceURL:  "https://example.com/owner/tool",
		DefaultDir: customDir,
		Specs: []SpecOpts{
			{
				// 1. Bare name → goes to DefaultDir.
				AssetGlob: "tool-linux-amd64",
				LocalName: "mytool",
				Checksum:  ChecksumOpts{Strategy: "none"},
			},
			{
				// 2. Absolute path → used directly.
				AssetGlob: "tool-abs-linux-amd64",
				LocalName: absPath,
				Checksum:  ChecksumOpts{Strategy: "none"},
			},
			{
				// 3. Empty local name → basename of asset name, in DefaultDir.
				AssetGlob: "tool-empty-linux-amd64",
				LocalName: "",
				Checksum:  ChecksumOpts{Strategy: "none"},
			},
		},
	}

	ctx := context.Background()
	if err := m.Install(ctx, opts); err != nil {
		t.Fatalf("Install returned error: %v", err)
	}

	cases := []struct {
		desc string
		path string
	}{
		{"bare name in DefaultDir", filepath.Join(customDir, "mytool")},
		{"absolute path", absPath},
		{"basename of asset in DefaultDir", filepath.Join(customDir, "tool-empty-linux-amd64")},
	}
	for _, tc := range cases {
		if _, err := os.Stat(tc.path); err != nil {
			t.Errorf("%s: file not found at %q: %v", tc.desc, tc.path, err)
		}
	}
}

// ========== Update tests ==========

// newUpdateManager creates a manager with a mock backend whose Check function
// returns checkResolution and whose Resolve function returns resolveResolution.
// It also writes the provided manifest into the libDir so Update can load it.
// The installed binary is created at binPath so Install can overwrite it.
func newUpdateManager(
	t *testing.T,
	pkg *manifest.Package,
	binPath string,
	checkResolution *backend.Resolution,
	resolveResolution *backend.Resolution,
) (Manager, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			return []byte("binary-content"), nil
		},
	}
	extractor := &MockExtractor{ExtractFn: noExtract}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}

	mb := &MockBackend{
		TypeFn: func() string { return pkg.Backend },
		CanHandleFn: func(u *url.URL) bool {
			return true
		},
		ResolveFn: func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
			return resolveResolution, nil
		},
		CheckFn: func(ctx context.Context, p *manifest.Package) (*backend.Resolution, error) {
			return checkResolution, nil
		},
	}

	reg := backend.NewRegistry()
	reg.Register(mb)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}

	// If a binPath is specified, update the package's InstalledFiles to point there
	// and create a placeholder file.
	if binPath != "" {
		if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
			t.Fatalf("create bin dir: %v", err)
		}
		if err := os.WriteFile(binPath, []byte("old-binary"), 0755); err != nil {
			t.Fatalf("create installed binary: %v", err)
		}
		// Patch the first spec's InstalledFiles to the provided binPath.
		if len(pkg.Specs) > 0 {
			pkg.Specs[0].InstalledFiles = []manifest.InstalledFile{
				{LocalPath: binPath, Checksums: map[string]string{}},
			}
		}
	}

	writeManifest(t, libDir, pkg)

	m := New(reg, fetcher, extractor, verifier, libDir)
	return m, home
}

// TestUpdate_NewVersionInstalled verifies that when the backend reports a new
// version, Update calls Install and returns Updated=true.
func TestUpdate_NewVersionInstalled(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "mytool")

	pkg := &manifest.Package{
		ID:        "example.com/owner/mytool",
		Backend:   "github",
		SourceURL: "https://example.com/owner/mytool",
		Version:   "v1.0.0",
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "mytool-linux-amd64",
				LocalName: "mytool",
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
			},
		},
	}

	checkResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}
	resolveResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}

	m, _ := newUpdateManager(t, pkg, binPath, checkResolution, resolveResolution)

	ctx := context.Background()
	results, err := m.Update(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.Updated {
		t.Errorf("expected Updated=true, got false")
	}
	if r.OldVersion != "v1.0.0" {
		t.Errorf("OldVersion = %q, want %q", r.OldVersion, "v1.0.0")
	}
	if r.NewVersion != "v1.1.0" {
		t.Errorf("NewVersion = %q, want %q", r.NewVersion, "v1.1.0")
	}
}

// TestUpdate_SameVersionSkipped verifies that when the backend reports the same
// version, Update does not call Install and returns Updated=false.
func TestUpdate_SameVersionSkipped(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "mytool")

	pkg := &manifest.Package{
		ID:        "example.com/owner/mytool",
		Backend:   "github",
		SourceURL: "https://example.com/owner/mytool",
		Version:   "v1.0.0",
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "mytool-linux-amd64",
				LocalName: "mytool",
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
			},
		},
	}

	sameResolution := &backend.Resolution{
		Version: "v1.0.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}

	fetchCount := 0
	home := t.TempDir()
	t.Setenv("HOME", home)

	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			fetchCount++
			return []byte("binary"), nil
		},
	}
	extractor := &MockExtractor{ExtractFn: noExtract}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}

	mb := &MockBackend{
		TypeFn:      func() string { return "github" },
		CanHandleFn: func(u *url.URL) bool { return true },
		ResolveFn: func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
			return sameResolution, nil
		},
		CheckFn: func(ctx context.Context, p *manifest.Package) (*backend.Resolution, error) {
			return sameResolution, nil
		},
	}
	reg := backend.NewRegistry()
	reg.Register(mb)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}

	// Create a placeholder installed file.
	if err := os.MkdirAll(filepath.Dir(binPath), 0755); err != nil {
		t.Fatalf("create bin dir: %v", err)
	}
	if err := os.WriteFile(binPath, []byte("binary"), 0755); err != nil {
		t.Fatalf("create installed binary: %v", err)
	}
	pkg.Specs[0].InstalledFiles = []manifest.InstalledFile{
		{LocalPath: binPath, Checksums: map[string]string{}},
	}
	writeManifest(t, libDir, pkg)

	m := New(reg, fetcher, extractor, verifier, libDir)

	ctx := context.Background()
	results, err := m.Update(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].Updated {
		t.Errorf("expected Updated=false when version unchanged, got true")
	}
	if fetchCount != 0 {
		t.Errorf("expected 0 Fetch calls (no install), got %d", fetchCount)
	}
}

// TestUpdate_NamedPinnedPackageUpdates verifies that a pinned package is still
// updated when named explicitly in opts.Packages.
func TestUpdate_NamedPinnedPackageUpdates(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "mytool")

	pkg := &manifest.Package{
		ID:        "example.com/owner/mytool",
		Backend:   "github",
		SourceURL: "https://example.com/owner/mytool",
		Version:   "v1.0.0",
		Pinned:    true, // pinned — but named explicitly, so should update
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "mytool-linux-amd64",
				LocalName: "mytool",
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
			},
		},
	}

	checkResolution := &backend.Resolution{
		Version: "v2.0.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}
	resolveResolution := &backend.Resolution{
		Version: "v2.0.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}

	m, _ := newUpdateManager(t, pkg, binPath, checkResolution, resolveResolution)

	ctx := context.Background()
	results, err := m.Update(ctx, UpdateOptions{
		Packages: []PackageTarget{{ID: "example.com/owner/mytool"}},
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Updated {
		t.Errorf("expected Updated=true for explicitly named pinned package, got false")
	}
}

// TestUpdate_Unpin_ClearsPinBeforeUpdate verifies that --unpin clears the pin
// on the manifest before the update proceeds.
func TestUpdate_Unpin_ClearsPinBeforeUpdate(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "mytool")

	pkg := &manifest.Package{
		ID:        "example.com/owner/mytool",
		Backend:   "github",
		SourceURL: "https://example.com/owner/mytool",
		Version:   "v1.0.0",
		Pinned:    true,
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "mytool-linux-amd64",
				LocalName: "mytool",
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
			},
		},
	}

	checkResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}
	resolveResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}

	m, home := newUpdateManager(t, pkg, binPath, checkResolution, resolveResolution)

	ctx := context.Background()
	results, err := m.Update(ctx, UpdateOptions{
		Packages: []PackageTarget{{ID: "example.com/owner/mytool"}},
		Unpin:    true,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(results) != 1 || !results[0].Updated {
		t.Fatalf("expected 1 updated result")
	}

	// Reload the manifest and verify the pin was cleared.
	libDir := filepath.Join(home, ".local", "share", "binmgr")
	entries, _ := os.ReadDir(libDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(libDir, entries[0].Name()))
	var updated manifest.Package
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if updated.Pinned {
		t.Errorf("expected Pinned=false after --unpin, got true")
	}
}

// TestUpdate_Pin_SetsPinAfterUpdate verifies that --pin sets the pin on the
// manifest after a successful update.
func TestUpdate_Pin_SetsPinAfterUpdate(t *testing.T) {
	binDir := t.TempDir()
	binPath := filepath.Join(binDir, "mytool")

	pkg := &manifest.Package{
		ID:        "example.com/owner/mytool",
		Backend:   "github",
		SourceURL: "https://example.com/owner/mytool",
		Version:   "v1.0.0",
		Pinned:    false,
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "mytool-linux-amd64",
				LocalName: "mytool",
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
			},
		},
	}

	checkResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}
	resolveResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "mytool-linux-amd64", URL: "https://example.com/mytool-linux-amd64"},
		},
	}

	m, home := newUpdateManager(t, pkg, binPath, checkResolution, resolveResolution)

	ctx := context.Background()
	results, err := m.Update(ctx, UpdateOptions{
		Packages: []PackageTarget{{ID: "example.com/owner/mytool"}},
		Pin:      true,
	})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(results) != 1 || !results[0].Updated {
		t.Fatalf("expected 1 updated result")
	}

	// Reload the manifest and verify the pin was set.
	libDir := filepath.Join(home, ".local", "share", "binmgr")
	entries, _ := os.ReadDir(libDir)
	if len(entries) != 1 {
		t.Fatalf("expected 1 manifest, got %d", len(entries))
	}
	data, _ := os.ReadFile(filepath.Join(libDir, entries[0].Name()))
	var updated manifest.Package
	if err := json.Unmarshal(data, &updated); err != nil {
		t.Fatalf("parse manifest: %v", err)
	}
	if !updated.Pinned {
		t.Errorf("expected Pinned=true after --pin, got false")
	}
}

// TestUpdate_NonDefaultPathPreserved verifies that when a package has specs
// installed into different non-default directories, each spec is reinstalled
// to its original absolute path rather than being redirected to the first
// spec's directory.
func TestUpdate_NonDefaultPathPreserved(t *testing.T) {
	// Two specs installed into two separate non-default directories.
	dirA := t.TempDir()
	dirB := t.TempDir()
	binPathA := filepath.Join(dirA, "toolA")
	binPathB := filepath.Join(dirB, "toolB")

	pkg := &manifest.Package{
		ID:        "example.com/owner/multitool",
		Backend:   "github",
		SourceURL: "https://example.com/owner/multitool",
		Version:   "v1.0.0",
		Specs: []manifest.InstallSpec{
			{
				AssetGlob: "toolA-linux-amd64",
				LocalName: "toolA", // bare name; was installed to dirA
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
				InstalledFiles: []manifest.InstalledFile{
					{LocalPath: binPathA, Checksums: map[string]string{}},
				},
			},
			{
				AssetGlob: "toolB-linux-amd64",
				LocalName: "toolB", // bare name; was installed to dirB
				Checksum:  manifest.ChecksumConfig{Strategy: "none"},
				InstalledFiles: []manifest.InstalledFile{
					{LocalPath: binPathB, Checksums: map[string]string{}},
				},
			},
		},
	}

	// Create the placeholder files that the manifest claims are installed.
	for _, p := range []string{binPathA, binPathB} {
		if err := os.WriteFile(p, []byte("old-binary"), 0755); err != nil {
			t.Fatalf("create installed binary %q: %v", p, err)
		}
	}

	checkResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "toolA-linux-amd64", URL: "https://example.com/toolA-linux-amd64"},
			{Name: "toolB-linux-amd64", URL: "https://example.com/toolB-linux-amd64"},
		},
	}
	resolveResolution := &backend.Resolution{
		Version: "v1.1.0",
		Assets: []backend.Asset{
			{Name: "toolA-linux-amd64", URL: "https://example.com/toolA-linux-amd64"},
			{Name: "toolB-linux-amd64", URL: "https://example.com/toolB-linux-amd64"},
		},
	}

	// newUpdateManager patches only the first spec's InstalledFiles; since we
	// set InstalledFiles ourselves above (and pass binPath=""), use it directly.
	home := t.TempDir()
	t.Setenv("HOME", home)

	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, u string) ([]byte, error) {
			return []byte("new-binary"), nil
		},
	}
	extractor := &MockExtractor{ExtractFn: noExtract}
	verifier := &MockVerifier{
		VerifyFn:  func(ctx context.Context, data []byte, expected map[string]string) error { return nil },
		ComputeFn: defaultCompute,
	}

	mb := &MockBackend{
		TypeFn:      func() string { return pkg.Backend },
		CanHandleFn: func(u *url.URL) bool { return true },
		ResolveFn: func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
			return resolveResolution, nil
		},
		CheckFn: func(ctx context.Context, p *manifest.Package) (*backend.Resolution, error) {
			return checkResolution, nil
		},
	}
	reg := backend.NewRegistry()
	reg.Register(mb)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}
	writeManifest(t, libDir, pkg)

	m := New(reg, fetcher, extractor, verifier, libDir)

	ctx := context.Background()
	results, err := m.Update(ctx, UpdateOptions{})
	if err != nil {
		t.Fatalf("Update returned error: %v", err)
	}

	if len(results) != 1 || !results[0].Updated {
		t.Fatalf("expected 1 updated result")
	}

	// Both tools must be updated in-place at their original paths.
	for _, p := range []string{binPathA, binPathB} {
		data, err := os.ReadFile(p)
		if err != nil {
			t.Errorf("file not found at original path %q: %v", p, err)
			continue
		}
		if string(data) != "new-binary" {
			t.Errorf("file at %q has content %q, want %q", p, string(data), "new-binary")
		}
	}

	// No files must have been created under dirA for toolB (the bug scenario).
	wrongPath := filepath.Join(dirA, "toolB")
	if _, err := os.Stat(wrongPath); err == nil {
		t.Errorf("toolB was incorrectly written to dirA (%q); bug not fixed", wrongPath)
	}
}

// ========== Status tests ==========

// newStatusManager creates a manager with a mock backend that reports the given
// checkResolution, and writes the provided manifest to libDir.
func newStatusManager(
	t *testing.T,
	pkg *manifest.Package,
	checkResolution *backend.Resolution,
) (Manager, string) {
	t.Helper()

	home := t.TempDir()
	t.Setenv("HOME", home)

	mb := &MockBackend{
		TypeFn:      func() string { return pkg.Backend },
		CanHandleFn: func(u *url.URL) bool { return true },
		ResolveFn: func(ctx context.Context, sourceURL *url.URL, opts backend.ResolveOptions) (*backend.Resolution, error) {
			return checkResolution, nil
		},
		CheckFn: func(ctx context.Context, p *manifest.Package) (*backend.Resolution, error) {
			return checkResolution, nil
		},
	}

	reg := backend.NewRegistry()
	reg.Register(mb)

	libDir := filepath.Join(home, ".local", "share", "binmgr")
	if err := os.MkdirAll(libDir, 0700); err != nil {
		t.Fatalf("create libDir: %v", err)
	}
	writeManifest(t, libDir, pkg)

	m := New(reg, &MockFetcher{}, &MockExtractor{}, &MockVerifier{}, libDir)
	return m, home
}

// TestStatus_UpdateAvailable verifies UpdateAvailable is true when the backend
// reports a newer version.
func TestStatus_UpdateAvailable(t *testing.T) {
	pkg := &manifest.Package{
		ID:      "example.com/owner/mytool",
		Backend: "github",
		Version: "v1.0.0",
	}
	checkResolution := &backend.Resolution{Version: "v1.1.0"}

	m, _ := newStatusManager(t, pkg, checkResolution)

	ctx := context.Background()
	results, err := m.Status(ctx, nil)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	r := results[0]
	if !r.UpdateAvailable {
		t.Errorf("expected UpdateAvailable=true, got false")
	}
	if r.InstalledVersion != "v1.0.0" {
		t.Errorf("InstalledVersion = %q, want %q", r.InstalledVersion, "v1.0.0")
	}
	if r.LatestVersion != "v1.1.0" {
		t.Errorf("LatestVersion = %q, want %q", r.LatestVersion, "v1.1.0")
	}
}

// TestStatus_NoUpdateAvailable verifies UpdateAvailable is false when the
// backend reports the same version.
func TestStatus_NoUpdateAvailable(t *testing.T) {
	pkg := &manifest.Package{
		ID:      "example.com/owner/mytool",
		Backend: "github",
		Version: "v1.0.0",
	}
	checkResolution := &backend.Resolution{Version: "v1.0.0"}

	m, _ := newStatusManager(t, pkg, checkResolution)

	ctx := context.Background()
	results, err := m.Status(ctx, nil)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if results[0].UpdateAvailable {
		t.Errorf("expected UpdateAvailable=false when up to date, got true")
	}
}

// TestStatus_PinnedFlagPropagated verifies that the Pinned field from the
// manifest is reflected in the StatusResult.
func TestStatus_PinnedFlagPropagated(t *testing.T) {
	pkg := &manifest.Package{
		ID:      "example.com/owner/mytool",
		Backend: "github",
		Version: "v1.0.0",
		Pinned:  true,
	}
	checkResolution := &backend.Resolution{Version: "v1.0.0"}

	m, _ := newStatusManager(t, pkg, checkResolution)

	ctx := context.Background()
	results, err := m.Status(ctx, nil)
	if err != nil {
		t.Fatalf("Status returned error: %v", err)
	}

	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].Pinned {
		t.Errorf("expected Pinned=true, got false")
	}
}
