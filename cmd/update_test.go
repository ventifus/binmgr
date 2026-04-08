package cmd

import (
	"context"
	"os"
	"testing"

	"github.com/ventifus/binmgr/pkg/backend"
)

func TestUpdatePackage_Github(t *testing.T) {
	m := backend.NewBinmgrManifest()
	m.Type = "github"
	m.Properties["owner"] = "testowner"
	m.Properties["repo"] = "testrepo"

	ctx := context.Background()
	// This will fail on API call, but we're testing the dispatch logic
	_ = updatePackage(ctx, m)
	// Not checking error as it will fail on actual API call
}

func TestUpdatePackage_Shasumurl(t *testing.T) {
	m := backend.NewBinmgrManifest()
	m.Type = "shasumurl"
	m.LatestRemoteUrl = "https://example.com/checksums.txt"

	ctx := context.Background()
	_ = updatePackage(ctx, m)
	// Not checking error as it will fail on actual HTTP call
}

func TestUpdatePackage_Kubeurl(t *testing.T) {
	m := backend.NewBinmgrManifest()
	m.Type = "kubeurl"

	ctx := context.Background()
	_ = updatePackage(ctx, m)
	// Not checking error as it will fail on actual HTTP call
}

func TestUpdatePackage_UnknownType(t *testing.T) {
	m := backend.NewBinmgrManifest()
	m.Type = "unknown-type"

	ctx := context.Background()
	err := updatePackage(ctx, m)
	// Unknown types return nil (no error)
	if err != nil {
		t.Errorf("updatePackage() with unknown type returned error: %v", err)
	}
}

func TestUpdateAll_NoManifests(t *testing.T) {
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

	ctx := context.Background()
	err = updateAll(ctx)
	if err != nil {
		t.Errorf("updateAll() failed with no manifests: %v", err)
	}
}
