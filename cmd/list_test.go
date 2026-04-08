package cmd

import (
	"os"
	"testing"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/backend"
)

func TestListCommand_NoManifests(t *testing.T) {
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

	cmd := &cobra.Command{}
	err = list(cmd, []string{})
	if err != nil {
		t.Errorf("list() failed with empty manifests: %v", err)
	}
}

func TestListCommand_WithManifests(t *testing.T) {
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

	// Create test manifests
	for i := 0; i < 3; i++ {
		m := backend.NewBinmgrManifest()
		m.Name = "test-package-" + string(rune('a'+i))
		m.Type = "github"
		m.ManifestFileName = "test-" + string(rune('a'+i)) + ".json"
		m.CurrentVersion = "v1.0.0"
		m.Properties["owner"] = "owner" + string(rune('a'+i))
		m.Properties["repo"] = "repo" + string(rune('a'+i))

		err = m.SaveManifest()
		if err != nil {
			t.Fatal(err)
		}
	}

	cmd := &cobra.Command{}
	err = list(cmd, []string{})
	if err != nil {
		t.Errorf("list() failed: %v", err)
	}
}

func TestListCommand_DirNotExist(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	origHome := os.Getenv("HOME")
	defer os.Setenv("HOME", origHome)
	os.Setenv("HOME", tmpDir)

	// Don't create the lib directory
	cmd := &cobra.Command{}
	err = list(cmd, []string{})
	if err == nil {
		t.Error("Expected error when lib directory doesn't exist")
	}
}
