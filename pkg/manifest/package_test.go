package manifest

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// ==============================
// IDToFilename tests
// ==============================

func TestIDToFilename(t *testing.T) {
	tests := []struct {
		name string
		id   string
		want string
	}{
		{
			name: "plain id unchanged",
			id:   "mybinary",
			want: "mybinary",
		},
		{
			name: "slash replaced",
			id:   "owner/repo",
			want: "owner_repo",
		},
		{
			name: "colon replaced",
			id:   "example.com:owner/repo",
			want: "example.com_owner_repo",
		},
		{
			name: "multiple slashes",
			id:   "a/b/c",
			want: "a_b_c",
		},
		{
			name: "mixed separators",
			id:   "host:8080/path/to/bin",
			want: "host_8080_path_to_bin",
		},
		{
			name: "empty string",
			id:   "",
			want: "",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got := IDToFilename(tc.id)
			if got != tc.want {
				t.Errorf("IDToFilename(%q) = %q, want %q", tc.id, got, tc.want)
			}
		})
	}
}

// ==============================
// Save + Load round-trip tests
// ==============================

func TestSaveAndLoad(t *testing.T) {
	dir := t.TempDir()

	pkg := &Package{
		ID:        "owner/repo",
		Backend:   "github",
		SourceURL: "https://github.com/owner/repo",
		Version:   "v1.2.3",
		Specs: []InstallSpec{
			{
				AssetGlob: "*.tar.gz",
				Checksum: ChecksumConfig{
					Strategy: "sha256sums",
					FileGlob: "SHA256SUMS",
				},
				Asset: &DownloadedAsset{
					URL:       "https://github.com/owner/repo/releases/download/v1.2.3/binary.tar.gz",
					Checksums: map[string]string{"sha-256": "deadbeef"},
				},
				InstalledFiles: []InstalledFile{
					{
						SourcePath: "binary",
						LocalPath:  "/home/user/.local/bin/binary",
						Checksums:  map[string]string{"sha-256": "cafebabe"},
					},
				},
			},
		},
	}

	if err := Save(pkg, dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	loaded, err := Load(pkg.ID, dir)
	if err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	if loaded.ID != pkg.ID {
		t.Errorf("ID: got %q, want %q", loaded.ID, pkg.ID)
	}
	if loaded.Backend != pkg.Backend {
		t.Errorf("Backend: got %q, want %q", loaded.Backend, pkg.Backend)
	}
	if loaded.SourceURL != pkg.SourceURL {
		t.Errorf("SourceURL: got %q, want %q", loaded.SourceURL, pkg.SourceURL)
	}
	if loaded.Version != pkg.Version {
		t.Errorf("Version: got %q, want %q", loaded.Version, pkg.Version)
	}
	if len(loaded.Specs) != 1 {
		t.Fatalf("Specs: got %d, want 1", len(loaded.Specs))
	}

	spec := loaded.Specs[0]
	if spec.AssetGlob != "*.tar.gz" {
		t.Errorf("AssetGlob: got %q, want %q", spec.AssetGlob, "*.tar.gz")
	}
	if spec.Checksum.Strategy != "sha256sums" {
		t.Errorf("Checksum.Strategy: got %q, want %q", spec.Checksum.Strategy, "sha256sums")
	}
	if spec.Asset == nil {
		t.Fatal("Asset is nil after round-trip")
	}
	if spec.Asset.Checksums["sha-256"] != "deadbeef" {
		t.Errorf("Asset.Checksums[sha-256]: got %q, want %q", spec.Asset.Checksums["sha-256"], "deadbeef")
	}
	if len(spec.InstalledFiles) != 1 {
		t.Fatalf("InstalledFiles: got %d, want 1", len(spec.InstalledFiles))
	}
	if spec.InstalledFiles[0].LocalPath != "/home/user/.local/bin/binary" {
		t.Errorf("InstalledFile.LocalPath: got %q", spec.InstalledFiles[0].LocalPath)
	}
}

func TestSaveProducesValidJSON(t *testing.T) {
	dir := t.TempDir()

	pkg := &Package{
		ID:      "test/pkg",
		Backend: "github",
		Version: "v0.1.0",
		Specs:   []InstallSpec{},
	}

	if err := Save(pkg, dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	path := filepath.Join(dir, IDToFilename(pkg.ID))
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("ReadFile failed: %v", err)
	}

	var decoded map[string]any
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Saved file is not valid JSON: %v", err)
	}

	if decoded["id"] != "test/pkg" {
		t.Errorf("JSON id field: got %v, want %q", decoded["id"], "test/pkg")
	}
	if decoded["backend"] != "github" {
		t.Errorf("JSON backend field: got %v, want %q", decoded["backend"], "github")
	}
	if decoded["version"] != "v0.1.0" {
		t.Errorf("JSON version field: got %v, want %q", decoded["version"], "v0.1.0")
	}

	// Pinned is omitempty=false (zero value), so it must not appear
	if _, ok := decoded["pinned"]; ok {
		t.Error("pinned field should be omitted when false")
	}
}

func TestLoad_NotFound(t *testing.T) {
	dir := t.TempDir()

	_, err := Load("nonexistent/pkg", dir)
	if err == nil {
		t.Fatal("expected error for missing package, got nil")
	}

	want := "package not found: nonexistent/pkg"
	if err.Error() != want {
		t.Errorf("error message: got %q, want %q", err.Error(), want)
	}
}

func TestSave_CreatesLibDir(t *testing.T) {
	// Use a path inside a fresh temp dir that does not yet exist.
	base := t.TempDir()
	dir := filepath.Join(base, "subdir", "binmgr")

	pkg := &Package{ID: "create/dir/test", Backend: "github", Version: "v1.0.0", Specs: []InstallSpec{}}
	if err := Save(pkg, dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if _, err := os.Stat(dir); err != nil {
		t.Errorf("dir was not created: %v", err)
	}
}

func TestSave_FilePermissions(t *testing.T) {
	dir := t.TempDir()

	pkg := &Package{ID: "perms/test", Backend: "github", Version: "v1.0.0", Specs: []InstallSpec{}}
	if err := Save(pkg, dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	path := filepath.Join(dir, IDToFilename(pkg.ID))
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("Stat failed: %v", err)
	}

	if mode := info.Mode().Perm(); mode != 0600 {
		t.Errorf("file mode: got %o, want %o", mode, 0600)
	}
}

// ==============================
// Delete tests
// ==============================

func TestDelete(t *testing.T) {
	dir := t.TempDir()

	pkg := &Package{ID: "delete/me", Backend: "github", Version: "v1.0.0", Specs: []InstallSpec{}}
	if err := Save(pkg, dir); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	if err := Delete(pkg.ID, dir); err != nil {
		t.Fatalf("Delete failed: %v", err)
	}

	// Confirm it's gone
	path := filepath.Join(dir, IDToFilename(pkg.ID))
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file still exists after Delete")
	}
}

func TestDelete_NotFound(t *testing.T) {
	dir := t.TempDir()

	err := Delete("does/not/exist", dir)
	if err == nil {
		t.Fatal("expected error deleting nonexistent package, got nil")
	}

	want := "package not found: does/not/exist"
	if err.Error() != want {
		t.Errorf("error message: got %q, want %q", err.Error(), want)
	}
}

// ==============================
// LoadAll tests
// ==============================

func TestLoadAll_MixedFiles(t *testing.T) {
	dir := t.TempDir()

	// Save two valid packages
	for _, id := range []string{"pkg/alpha", "pkg/beta"} {
		pkg := &Package{ID: id, Backend: "github", Version: "v1.0.0", Specs: []InstallSpec{}}
		if err := Save(pkg, dir); err != nil {
			t.Fatalf("Save(%q) failed: %v", id, err)
		}
	}

	// Write an invalid JSON file into dir
	badPath := filepath.Join(dir, "invalid_json_file")
	if err := os.WriteFile(badPath, []byte("{not valid json"), 0600); err != nil {
		t.Fatalf("WriteFile failed: %v", err)
	}

	// Write a subdirectory (should be skipped)
	subdir := filepath.Join(dir, "subdir")
	if err := os.Mkdir(subdir, 0755); err != nil {
		t.Fatalf("Mkdir failed: %v", err)
	}

	packages, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(packages) != 2 {
		t.Errorf("LoadAll returned %d packages, want 2 (invalid file should be skipped)", len(packages))
	}

	// All returned packages must have non-empty IDs
	for i, pkg := range packages {
		if pkg.ID == "" {
			t.Errorf("package[%d] has empty ID", i)
		}
	}
}

func TestLoadAll_EmptyDir(t *testing.T) {
	dir := t.TempDir()

	packages, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(packages) != 0 {
		t.Errorf("LoadAll returned %d packages, want 0", len(packages))
	}
}

func TestLoadAll_DirNotExist(t *testing.T) {
	base := t.TempDir()
	dir := filepath.Join(base, "nonexistent")

	packages, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll should return nil error when dir does not exist, got: %v", err)
	}
	if packages != nil {
		t.Errorf("LoadAll should return nil slice when dir does not exist, got: %v", packages)
	}
}

func TestLoadAll_AllValid(t *testing.T) {
	dir := t.TempDir()

	ids := []string{"vendor/tool-a", "vendor/tool-b", "vendor/tool-c"}
	for _, id := range ids {
		pkg := &Package{
			ID:        id,
			Backend:   "shasumurl",
			SourceURL: "https://example.com/" + id,
			Version:   "v2.0.0",
			Specs:     []InstallSpec{},
		}
		if err := Save(pkg, dir); err != nil {
			t.Fatalf("Save(%q) failed: %v", id, err)
		}
	}

	packages, err := LoadAll(dir)
	if err != nil {
		t.Fatalf("LoadAll failed: %v", err)
	}

	if len(packages) != len(ids) {
		t.Errorf("LoadAll returned %d packages, want %d", len(packages), len(ids))
	}
}
