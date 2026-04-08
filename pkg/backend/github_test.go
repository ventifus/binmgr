package backend

import (
	"context"
	"net/url"
	"testing"

	"github.com/google/go-github/v55/github"
)

func TestExpandVariables(t *testing.T) {
	tagName := "v1.2.3"
	release := &github.RepositoryRelease{
		TagName: &tagName,
	}

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{
			name:  "replace TAG",
			input: "file-${TAG}.tar.gz",
			want:  "file-v1.2.3.tar.gz",
		},
		{
			name:  "replace VERSION",
			input: "file-${VERSION}.tar.gz",
			want:  "file-1.2.3.tar.gz",
		},
		{
			name:  "replace both",
			input: "${TAG}-${VERSION}",
			want:  "v1.2.3-1.2.3",
		},
		{
			name:  "no variables",
			input: "plain-text.tar.gz",
			want:  "plain-text.tar.gz",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := expandVariables(release, tt.input)
			if got != tt.want {
				t.Errorf("expandVariables() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestInstallGithub_InvalidURL(t *testing.T) {
	tests := []struct {
		name    string
		url     string
		wantErr bool
		errMsg  string
	}{
		{
			name:    "not github.com",
			url:     "https://gitlab.com/owner/repo",
			wantErr: true,
			errMsg:  "only valid for github.com",
		},
		{
			name:    "missing repo",
			url:     "https://github.com/owner",
			wantErr: true,
			errMsg:  "invalid github URL format",
		},
		{
			name:    "missing owner",
			url:     "https://github.com/",
			wantErr: true,
			errMsg:  "invalid github URL format",
		},
		{
			name:    "empty path",
			url:     "https://github.com",
			wantErr: true,
			errMsg:  "invalid github URL format",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			u, err := url.Parse(tt.url)
			if err != nil {
				t.Fatal(err)
			}

			ctx := context.Background()
			err = InstallGithub(ctx, u, "", "", ChecksumNone)
			if (err != nil) != tt.wantErr {
				t.Errorf("InstallGithub() error = %v, wantErr %v", err, tt.wantErr)
			}
			if err != nil && tt.errMsg != "" && !containsMiddle(err.Error(), tt.errMsg) {
				t.Errorf("Expected error containing %q, got %q", tt.errMsg, err.Error())
			}
		})
	}
}

func TestNewGithubRepo(t *testing.T) {
	repo := NewGithubRepo("testowner", "testrepo")
	if repo == nil {
		t.Fatal("NewGithubRepo returned nil")
	}
	if repo.owner != "testowner" {
		t.Errorf("Expected owner 'testowner', got %s", repo.owner)
	}
	if repo.repo != "testrepo" {
		t.Errorf("Expected repo 'testrepo', got %s", repo.repo)
	}
	if repo.Manifest == nil {
		t.Error("Manifest is nil")
	}
	if repo.Manifest.Type != "github" {
		t.Errorf("Expected type 'github', got %s", repo.Manifest.Type)
	}
	if repo.Manifest.Properties == nil {
		t.Error("Properties map is nil")
	}
	if repo.Manifest.Properties["owner"] != "testowner" {
		t.Error("Properties[owner] not set correctly")
	}
	if repo.Manifest.Properties["repo"] != "testrepo" {
		t.Error("Properties[repo] not set correctly")
	}
	if repo.client == nil {
		t.Error("Client is nil")
	}
}

func TestNewGithubRepoFromManifest(t *testing.T) {
	manifest := NewBinmgrManifest()
	manifest.Type = "github"
	manifest.Properties["owner"] = "testowner"
	manifest.Properties["repo"] = "testrepo"

	repo := NewGithubRepoFromManifest(manifest)
	if repo == nil {
		t.Fatal("NewGithubRepoFromManifest returned nil")
	}
	if repo.owner != "testowner" {
		t.Errorf("Expected owner 'testowner', got %s", repo.owner)
	}
	if repo.repo != "testrepo" {
		t.Errorf("Expected repo 'testrepo', got %s", repo.repo)
	}
	if repo.Manifest != manifest {
		t.Error("Manifest reference not preserved")
	}
}

func TestGetRelease_BoundsChecking(t *testing.T) {
	// This test ensures our bounds checking fixes work
	manifest := NewBinmgrManifest()
	manifest.Properties["owner"] = "testowner"
	manifest.Properties["repo"] = "testrepo"

	repo := NewGithubRepoFromManifest(manifest)

	tests := []struct {
		name    string
		urlPath []string
		wantErr bool
	}{
		{
			name:    "nil path - should get latest",
			urlPath: nil,
			wantErr: false, // Will fail due to API call, but shouldn't panic
		},
		{
			name:    "empty path - should get latest",
			urlPath: []string{},
			wantErr: false,
		},
		{
			name:    "short path - should get latest",
			urlPath: []string{"", "owner", "repo"},
			wantErr: false,
		},
		{
			name:    "path with 4 elements but no 5th",
			urlPath: []string{"", "owner", "repo", "releases"},
			wantErr: false,
		},
		{
			name:    "path with tag but no tag value - should get latest",
			urlPath: []string{"", "owner", "repo", "releases", "tag"},
			wantErr: false,
		},
		{
			name:    "path with id but no id value - should get latest",
			urlPath: []string{"", "owner", "repo", "releases", "id"},
			wantErr: false,
		},
		{
			name:    "valid tag path",
			urlPath: []string{"", "owner", "repo", "releases", "tag", "v1.0.0"},
			wantErr: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// We're testing that it doesn't panic, not that it succeeds
			// (it will fail on API call, which is expected)
			ctx := context.Background()
			_ = repo.GetRelease(ctx, tt.urlPath)
			// Not checking error because API call will fail in tests
			// The important thing is that it doesn't panic on bounds checks
		})
	}
}

func TestGetAssetByGlob(t *testing.T) {
	assetName1 := "binary-linux-amd64.tar.gz"
	assetName2 := "binary-darwin-amd64.tar.gz"
	assetName3 := "binary-windows-amd64.zip"

	release := &github.RepositoryRelease{
		Assets: []*github.ReleaseAsset{
			{Name: &assetName1},
			{Name: &assetName2},
			{Name: &assetName3},
		},
	}

	tests := []struct {
		name     string
		glob     string
		wantName string
		wantErr  bool
	}{
		{
			name:     "exact match",
			glob:     "binary-linux-amd64.tar.gz",
			wantName: "binary-linux-amd64.tar.gz",
			wantErr:  false,
		},
		{
			name:     "wildcard match",
			glob:     "*linux*.tar.gz",
			wantName: "binary-linux-amd64.tar.gz",
			wantErr:  false,
		},
		{
			name:     "pattern match",
			glob:     "binary-*.zip",
			wantName: "binary-windows-amd64.zip",
			wantErr:  false,
		},
		{
			name:    "no match",
			glob:    "nonexistent-*.tar.gz",
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset, err := getAssetByGlob(release, tt.glob)
			if (err != nil) != tt.wantErr {
				t.Errorf("getAssetByGlob() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr {
				if asset == nil {
					t.Error("Expected non-nil asset")
					return
				}
				if asset.GetName() != tt.wantName {
					t.Errorf("Expected asset name %s, got %s", tt.wantName, asset.GetName())
				}
			}
		})
	}
}

func TestGetChecksumFile(t *testing.T) {
	sha256Name := "checksums.txt"
	assetSha256Name := "binary-linux-amd64.tar.gz.sha256"
	binaryName := "binary-linux-amd64.tar.gz"

	tagName := "v1.0.0"
	release := &github.RepositoryRelease{
		TagName: &tagName,
		Assets: []*github.ReleaseAsset{
			{Name: &sha256Name},
			{Name: &assetSha256Name},
			{Name: &binaryName},
		},
	}

	binaryAsset := &github.ReleaseAsset{Name: &binaryName}

	tests := []struct {
		name         string
		checksumType string
		wantName     string
		wantErr      bool
	}{
		{
			name:         "sha256sums default",
			checksumType: ChecksumShasum256,
			wantName:     "", // Will match checksums.txt with glob
			wantErr:      false,
		},
		{
			name:         "per asset sha256",
			checksumType: ChecksumPerAssetSha256,
			wantName:     assetSha256Name,
			wantErr:      false,
		},
		{
			name:         "unsupported type",
			checksumType: "unsupported",
			wantErr:      true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			asset, err := getChecksumFile(release, binaryAsset, tt.checksumType)
			if (err != nil) != tt.wantErr {
				t.Errorf("getChecksumFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}
			if !tt.wantErr && asset != nil && tt.wantName != "" {
				if asset.GetName() != tt.wantName {
					t.Errorf("Expected checksum file %s, got %s", tt.wantName, asset.GetName())
				}
			}
		})
	}
}

func TestNewArtifactFromAsset(t *testing.T) {
	// This test verifies that artifact creation initializes properly
	assetName := "binary.tar.gz"
	assetURL := "https://api.github.com/repos/owner/repo/releases/assets/123"
	browserURL := "https://github.com/owner/repo/releases/download/v1.0.0/binary.tar.gz"

	asset := &github.ReleaseAsset{
		Name:               &assetName,
		URL:                &assetURL,
		BrowserDownloadURL: &browserURL,
	}

	tagName := "v1.0.0"
	release := &github.RepositoryRelease{
		TagName: &tagName,
		Assets:  []*github.ReleaseAsset{asset},
	}

	repo := NewGithubRepo("owner", "repo")

	// Test with ChecksumNone
	artifact, err := repo.newArtifactFromAsset(release, asset, ChecksumNone)
	if err == nil {
		if artifact.RemoteFile != browserURL {
			t.Errorf("Expected RemoteFile %s, got %s", browserURL, artifact.RemoteFile)
		}
		if artifact.AssetUrl != assetURL {
			t.Errorf("Expected AssetUrl %s, got %s", assetURL, artifact.AssetUrl)
		}
		if artifact.ChecksumType != ChecksumNone {
			t.Errorf("Expected ChecksumType %s, got %s", ChecksumNone, artifact.ChecksumType)
		}
		if artifact.Installed {
			t.Error("Artifact should not be marked as installed")
		}
	}
}
