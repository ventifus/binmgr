package cmd

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/ventifus/binmgr/pkg/backend"
)

func TestGetPackageTypes(t *testing.T) {
	types := getPackageTypes()
	if len(types) == 0 {
		t.Error("getPackageTypes returned empty slice")
	}

	expected := map[string]bool{
		"github":    true,
		"tarball":   true,
		"shasumurl": true,
		"kubefile":  true,
	}

	for _, typ := range types {
		if !expected[typ] {
			t.Errorf("Unexpected package type: %s", typ)
		}
	}
}

func TestInstallArgs_ValidArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("type", "github", "")
	cmd.Flags().String("checksumtype", "sha256sums", "")

	args := []string{"https://github.com/owner/repo"}

	err := installArgs(cmd, args)
	if err != nil {
		t.Errorf("installArgs failed with valid args: %v", err)
	}
}

func TestInstallArgs_NoArgs(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("type", "github", "")
	cmd.Flags().String("checksumtype", "sha256sums", "")

	args := []string{}

	err := installArgs(cmd, args)
	if err == nil {
		t.Error("Expected error when no args provided")
	}
}

func TestInstallArgs_InvalidType(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("type", "invalid-type", "")
	cmd.Flags().String("checksumtype", "sha256sums", "")

	args := []string{"https://example.com"}

	err := installArgs(cmd, args)
	if err == nil {
		t.Error("Expected error for invalid type")
	}
	if err != nil && !contains(err.Error(), "unsupported type") {
		t.Errorf("Expected 'unsupported type' error, got: %v", err)
	}
}

func TestInstallArgs_InvalidChecksumType(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.Flags().String("type", "github", "")
	cmd.Flags().String("checksumtype", "invalid-checksum", "")

	args := []string{"https://example.com"}

	err := installArgs(cmd, args)
	if err == nil {
		t.Error("Expected error for invalid checksum type")
	}
	if err != nil && !contains(err.Error(), "unsupported checksum type") {
		t.Errorf("Expected 'unsupported checksum type' error, got: %v", err)
	}
}

func TestInstallArgs_ValidChecksumTypes(t *testing.T) {
	validTypes := backend.ChecksumTypes()

	for _, csType := range validTypes {
		t.Run(csType, func(t *testing.T) {
			cmd := &cobra.Command{}
			cmd.Flags().String("type", "github", "")
			cmd.Flags().String("checksumtype", csType, "")

			args := []string{"https://github.com/owner/repo"}

			err := installArgs(cmd, args)
			if err != nil {
				t.Errorf("installArgs failed with valid checksum type %s: %v", csType, err)
			}
		})
	}
}

func TestInstallArgs_ChecksumTypeWithExclamation(t *testing.T) {
	// Test that we properly handle checksumtype with '!' separator
	cmd := &cobra.Command{}
	cmd.Flags().String("type", "github", "")
	cmd.Flags().String("checksumtype", "sha256sums!custom-pattern", "")

	args := []string{"https://github.com/owner/repo"}

	err := installArgs(cmd, args)
	if err != nil {
		t.Errorf("installArgs failed with checksum type containing '!': %v", err)
	}
}

// Helper function
func contains(s, substr string) bool {
	if len(s) == 0 || len(substr) == 0 {
		return false
	}
	for i := 0; i <= len(s)-len(substr); i++ {
		if s[i:i+len(substr)] == substr {
			return true
		}
	}
	return false
}
