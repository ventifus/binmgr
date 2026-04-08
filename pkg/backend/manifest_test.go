package backend

import (
	"encoding/json"
	"os"
	"path"
	"testing"
)

func TestNewArtifact(t *testing.T) {
	artifact := NewArtifact()
	if artifact == nil {
		t.Fatal("NewArtifact returned nil")
	}
	if artifact.Checksums == nil {
		t.Error("Checksums map not initialized")
	}
	if artifact.InnerArtifacts == nil {
		t.Error("InnerArtifacts slice not initialized")
	}
	if len(artifact.Checksums) != 0 {
		t.Error("Checksums should be empty")
	}
	if len(artifact.InnerArtifacts) != 0 {
		t.Error("InnerArtifacts should be empty")
	}
}

func TestNewInnerArtifact(t *testing.T) {
	ia := NewInnerArtifact()
	if ia == nil {
		t.Fatal("NewInnerArtifact returned nil")
	}
}

func TestNewBinmgrManifest(t *testing.T) {
	// Save original args and restore after test
	origArgs := os.Args
	defer func() { os.Args = origArgs }()

	os.Args = []string{"binmgr", "install", "test"}

	manifest := NewBinmgrManifest()
	if manifest == nil {
		t.Fatal("NewBinmgrManifest returned nil")
	}
	if manifest.Artifacts == nil {
		t.Error("Artifacts slice not initialized")
	}
	if manifest.Properties == nil {
		t.Error("Properties map not initialized")
	}
	if len(manifest.Cmdline) != 2 {
		t.Errorf("Expected cmdline length 2, got %d", len(manifest.Cmdline))
	}
	if manifest.Cmdline[0] != "install" {
		t.Errorf("Expected first cmdline arg 'install', got %s", manifest.Cmdline[0])
	}
}

func TestBinmgrManifestString(t *testing.T) {
	manifest := &BinmgrManifest{
		Name:             "test-package",
		CurrentVersion:   "v1.0.0",
		CurrentRemoteUrl: "https://example.com/v1.0.0",
		LatestRemoteUrl:  "https://example.com/latest",
	}

	str := manifest.String()
	if str == "" {
		t.Error("String() returned empty string")
	}
	if !contains(str, "test-package") {
		t.Error("String() should contain package name")
	}
	if !contains(str, "v1.0.0") {
		t.Error("String() should contain version")
	}
}

func TestSaveAndLoadManifest(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override libDir for testing
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)

	testHome := tmpDir
	os.Setenv("HOME", testHome)

	// Create lib directory
	err = os.MkdirAll(libDir(), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create and save manifest
	manifest := NewBinmgrManifest()
	manifest.Name = "test-pkg"
	manifest.Type = "github"
	manifest.ManifestFileName = "test-manifest.json"
	manifest.CurrentVersion = "v1.2.3"
	manifest.Properties["owner"] = "testowner"
	manifest.Properties["repo"] = "testrepo"

	artifact := NewArtifact()
	artifact.LocalFile = "/tmp/test-binary"
	artifact.RemoteFile = "https://example.com/binary"
	artifact.Checksums = map[string]string{"sha-256": "abc123"}
	manifest.Artifacts = append(manifest.Artifacts, artifact)

	err = manifest.SaveManifest()
	if err != nil {
		t.Fatalf("SaveManifest failed: %v", err)
	}

	// Verify file exists
	manifestPath := path.Join(libDir(), manifest.ManifestFileName)
	if _, err := os.Stat(manifestPath); os.IsNotExist(err) {
		t.Fatal("Manifest file was not created")
	}

	// Read and verify content
	data, err := os.ReadFile(manifestPath)
	if err != nil {
		t.Fatal(err)
	}

	var loaded BinmgrManifest
	err = json.Unmarshal(data, &loaded)
	if err != nil {
		t.Fatalf("Failed to unmarshal manifest: %v", err)
	}

	if loaded.Name != "test-pkg" {
		t.Errorf("Expected name 'test-pkg', got %s", loaded.Name)
	}
	if loaded.Type != "github" {
		t.Errorf("Expected type 'github', got %s", loaded.Type)
	}
	if loaded.CurrentVersion != "v1.2.3" {
		t.Errorf("Expected version 'v1.2.3', got %s", loaded.CurrentVersion)
	}
	if len(loaded.Artifacts) != 1 {
		t.Errorf("Expected 1 artifact, got %d", len(loaded.Artifacts))
	}
	if loaded.Properties["owner"] != "testowner" {
		t.Errorf("Expected owner 'testowner', got %s", loaded.Properties["owner"])
	}
}

func TestGetAllManifests(t *testing.T) {
	// Create temp directory
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Override HOME
	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// Create lib directory
	err = os.MkdirAll(libDir(), 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create test manifests
	for i := 0; i < 3; i++ {
		manifest := NewBinmgrManifest()
		manifest.Name = "test-pkg-" + string(rune('a'+i))
		manifest.Type = "github"
		manifest.ManifestFileName = "test-" + string(rune('a'+i)) + ".json"
		manifest.CurrentVersion = "v1.0.0"

		err = manifest.SaveManifest()
		if err != nil {
			t.Fatal(err)
		}
	}

	// Create a subdirectory (should be skipped)
	os.Mkdir(path.Join(libDir(), "subdir"), 0755)

	// Get all manifests
	manifests, err := GetAllManifests()
	if err != nil {
		t.Fatalf("GetAllManifests failed: %v", err)
	}

	if len(manifests) != 3 {
		t.Errorf("Expected 3 manifests, got %d", len(manifests))
	}

	// Verify no nil entries
	for i, m := range manifests {
		if m == nil {
			t.Errorf("Manifest at index %d is nil", i)
		}
	}

	// Verify manifest filenames are set
	for _, m := range manifests {
		if m.ManifestFileName == "" {
			t.Error("ManifestFileName not set")
		}
	}
}

func TestGetAllManifests_EmptyDir(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	err = os.MkdirAll(libDir(), 0755)
	if err != nil {
		t.Fatal(err)
	}

	manifests, err := GetAllManifests()
	if err != nil {
		t.Fatalf("GetAllManifests failed: %v", err)
	}

	if len(manifests) != 0 {
		t.Errorf("Expected 0 manifests, got %d", len(manifests))
	}
}

func TestGetAllManifests_DirNotExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// Don't create the directory
	_, err = GetAllManifests()
	if err == nil {
		t.Error("Expected error when directory doesn't exist")
	}
}

func TestArtifactGetChecksumAlgorithms(t *testing.T) {
	artifact := NewArtifact()
	artifact.Checksums = map[string]string{
		"sha-256": "abc123",
		"sha-512": "def456",
	}

	algos := artifact.GetChecksumAlgorithms()
	if len(algos) != 2 {
		t.Errorf("Expected 2 algorithms, got %d", len(algos))
	}

	// Check both algorithms are present (order doesn't matter)
	hassha256 := false
	hassha512 := false
	for _, algo := range algos {
		if algo == "sha-256" {
			hassha256 = true
		}
		if algo == "sha-512" {
			hassha512 = true
		}
	}
	if !hassha256 || !hassha512 {
		t.Error("Not all algorithms present in result")
	}
}

func TestInnerArtifactGetChecksumAlgorithms(t *testing.T) {
	ia := NewInnerArtifact()
	ia.Checksums = map[string]string{
		"sha-256": "abc123",
	}

	algos := ia.GetChecksumAlgorithms()
	if len(algos) != 1 {
		t.Errorf("Expected 1 algorithm, got %d", len(algos))
	}
	if algos[0] != "sha-256" {
		t.Errorf("Expected 'sha-256', got %s", algos[0])
	}
}

// Helper function
func contains(s, substr string) bool {
	return len(s) > 0 && len(substr) > 0 && (s == substr || len(s) > len(substr) && (s[:len(substr)] == substr || s[len(s)-len(substr):] == substr || containsMiddle(s, substr)))
}

func containsMiddle(s, substr string) bool {
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
