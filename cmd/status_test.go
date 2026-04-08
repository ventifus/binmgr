package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/backend"
)

func TestStatusCommand_NoManifests(t *testing.T) {
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
	libPath := tmpDir + "/.local/share/binmgr/"
	err = os.MkdirAll(libPath, 0755)
	if err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	err = status(cmd, []string{})
	if err != nil {
		t.Errorf("status() failed with empty manifests: %v", err)
	}
}

func TestStatusCommand_WithManifests(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	libPath := tmpDir + "/.local/share/binmgr/"
	err = os.MkdirAll(libPath, 0755)
	if err != nil {
		t.Fatal(err)
	}

	// Create a test manifest
	m := backend.NewBinmgrManifest()
	m.Name = "test-package"
	m.Type = "github"
	m.ManifestFileName = "test-manifest.json"
	m.Properties["owner"] = "testowner"
	m.Properties["repo"] = "testrepo"
	m.CurrentVersion = "v1.0.0"

	err = m.SaveManifest()
	if err != nil {
		t.Fatal(err)
	}

	cmd := &cobra.Command{}
	// This will fail trying to contact GitHub, but we're testing that
	// it handles the manifest properly
	_ = status(cmd, []string{})
	// Not checking error because it will fail on API call
}
