package manager

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/ventifus/binmgr/pkg/backend"
	"github.com/ventifus/binmgr/pkg/extract"
)

// ========== parseSha256Sums tests ==========

func TestParseSha256Sums(t *testing.T) {
	t.Run("standard format with multiple entries", func(t *testing.T) {
		input := "" +
			"abc123  foo.tar.gz\n" +
			"def456  bar.zip\n" +
			"789abc  baz.bin\n"
		got, err := parseSha256Sums([]byte(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		want := map[string]string{
			"foo.tar.gz": "abc123",
			"bar.zip":    "def456",
			"baz.bin":    "789abc",
		}
		if len(got) != len(want) {
			t.Fatalf("got %d entries, want %d", len(got), len(want))
		}
		for filename, hex := range want {
			if got[filename] != hex {
				t.Errorf("got[%q] = %q, want %q", filename, got[filename], hex)
			}
		}
	})

	t.Run("blank lines and comment lines are skipped", func(t *testing.T) {
		input := "" +
			"# This is a comment\n" +
			"\n" +
			"abc123  foo.tar.gz\n" +
			"\n" +
			"# Another comment\n" +
			"def456  bar.zip\n"
		got, err := parseSha256Sums([]byte(input))
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Fatalf("got %d entries, want 2", len(got))
		}
		if got["foo.tar.gz"] != "abc123" {
			t.Errorf("got[foo.tar.gz] = %q, want abc123", got["foo.tar.gz"])
		}
		if got["bar.zip"] != "def456" {
			t.Errorf("got[bar.zip] = %q, want def456", got["bar.zip"])
		}
	})

	t.Run("malformed line returns error", func(t *testing.T) {
		input := "abc123 foo.tar.gz\n" // only one space, not two
		_, err := parseSha256Sums([]byte(input))
		if err == nil {
			t.Fatal("expected error for malformed line, got nil")
		}
	})
}

// ========== parseMultisum tests ==========

func TestParseMultisum(t *testing.T) {
	orderFile := "SHA256\nSHA512\n"
	dataFile := "" +
		"foo.tar.gz  abc123def  999aabbcc\n" +
		"bar.zip     deadbeef01  cafebabe02\n"

	t.Run("standard format parsed correctly", func(t *testing.T) {
		got, err := parseMultisum([]byte(dataFile), []byte(orderFile), "foo.tar.gz")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != "abc123def" {
			t.Errorf("got sha-256 = %q, want abc123def", got["sha-256"])
		}
		if got["sha-512"] != "999aabbcc" {
			t.Errorf("got sha-512 = %q, want 999aabbcc", got["sha-512"])
		}
	})

	t.Run("column mapping from order file works for second entry", func(t *testing.T) {
		got, err := parseMultisum([]byte(dataFile), []byte(orderFile), "bar.zip")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != "deadbeef01" {
			t.Errorf("got sha-256 = %q, want deadbeef01", got["sha-256"])
		}
		if got["sha-512"] != "cafebabe02" {
			t.Errorf("got sha-512 = %q, want cafebabe02", got["sha-512"])
		}
	})

	t.Run("returns error for unknown target name", func(t *testing.T) {
		_, err := parseMultisum([]byte(dataFile), []byte(orderFile), "notexist.bin")
		if err == nil {
			t.Fatal("expected error for unknown target, got nil")
		}
		if !strings.Contains(err.Error(), "notexist.bin") {
			t.Errorf("error should mention target name, got: %v", err)
		}
	})
}

// ========== resolveChecksums tests ==========

// newTestMgr creates a minimal *mgr for unit tests, injecting mock
// fetcher and extractor.
func newTestMgr(fetcher *MockFetcher, extractor *MockExtractor) *mgr {
	return &mgr{
		fetcher:   fetcher,
		extractor: extractor,
	}
}

func TestResolveChecksums_None(t *testing.T) {
	m := newTestMgr(&MockFetcher{}, &MockExtractor{})
	opts := ChecksumOpts{Strategy: "none"}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "tool.tar.gz", URL: "https://example.com/tool.tar.gz"},
		},
	}

	got, err := m.resolveChecksums(context.Background(), opts, "tool.tar.gz", nil, "", nil, resolution, "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != nil {
		t.Errorf("expected nil for strategy none, got %v", got)
	}
}

func TestResolveChecksums_Auto_Found(t *testing.T) {
	checksumContent := "deadbeef  tool.tar.gz\n"
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, url string) ([]byte, error) {
			if url == "https://example.com/SHA256SUMS" {
				return []byte(checksumContent), nil
			}
			return nil, fmt.Errorf("unexpected fetch of %q", url)
		},
	}
	m := newTestMgr(fetcher, &MockExtractor{})

	opts := ChecksumOpts{Strategy: "auto"}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "tool.tar.gz", URL: "https://example.com/tool.tar.gz"},
			{Name: "SHA256SUMS", URL: "https://example.com/SHA256SUMS"},
		},
	}

	got, err := m.resolveChecksums(context.Background(), opts, "tool.tar.gz", nil, "", nil, resolution, "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["sha-256"] != "deadbeef" {
		t.Errorf("got sha-256 = %q, want deadbeef", got["sha-256"])
	}
}

func TestResolveChecksums_Auto_NotFound(t *testing.T) {
	m := newTestMgr(&MockFetcher{}, &MockExtractor{})

	opts := ChecksumOpts{Strategy: "auto"}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "tool.tar.gz", URL: "https://example.com/tool.tar.gz"},
			// No checksum file assets present.
		},
	}

	_, err := m.resolveChecksums(context.Background(), opts, "tool.tar.gz", nil, "", nil, resolution, "v1.0.0")
	if err == nil {
		t.Fatal("expected error when no checksum file found, got nil")
	}
	// Error should mention the tried names.
	if !strings.Contains(err.Error(), "SHA256SUMS") {
		t.Errorf("error should mention tried names, got: %v", err)
	}
	if !strings.Contains(err.Error(), "--checksum none") {
		t.Errorf("error should mention --checksum none, got: %v", err)
	}
}

func TestResolveChecksums_SharedFile(t *testing.T) {
	checksumContent := "cafebabe  mytool.zip\n"
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, url string) ([]byte, error) {
			if url == "https://example.com/checksums.txt" {
				return []byte(checksumContent), nil
			}
			return nil, fmt.Errorf("unexpected fetch of %q", url)
		},
	}
	m := newTestMgr(fetcher, &MockExtractor{})

	opts := ChecksumOpts{
		Strategy: "shared-file",
		FileGlob: "checksums.txt",
	}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "mytool.zip", URL: "https://example.com/mytool.zip"},
			{Name: "checksums.txt", URL: "https://example.com/checksums.txt"},
		},
	}

	got, err := m.resolveChecksums(context.Background(), opts, "mytool.zip", nil, "", nil, resolution, "v2.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["sha-256"] != "cafebabe" {
		t.Errorf("got sha-256 = %q, want cafebabe", got["sha-256"])
	}
}

func TestResolveChecksums_SharedFile_VersionExpansion(t *testing.T) {
	checksumContent := "112233aa  tool.tar.gz\n"
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, url string) ([]byte, error) {
			if url == "https://example.com/tool_1.2.3_checksums.txt" {
				return []byte(checksumContent), nil
			}
			return nil, fmt.Errorf("unexpected fetch of %q", url)
		},
	}
	m := newTestMgr(fetcher, &MockExtractor{})

	opts := ChecksumOpts{
		Strategy: "shared-file",
		FileGlob: "tool_${VERSION}_checksums.txt",
	}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "tool.tar.gz", URL: "https://example.com/tool.tar.gz"},
			{Name: "tool_1.2.3_checksums.txt", URL: "https://example.com/tool_1.2.3_checksums.txt"},
		},
	}

	got, err := m.resolveChecksums(context.Background(), opts, "tool.tar.gz", nil, "", nil, resolution, "v1.2.3")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["sha-256"] != "112233aa" {
		t.Errorf("got sha-256 = %q, want 112233aa", got["sha-256"])
	}
}

func TestResolveChecksums_PerAsset(t *testing.T) {
	t.Run("bare hex digest", func(t *testing.T) {
		fetcher := &MockFetcher{
			FetchFn: func(ctx context.Context, url string) ([]byte, error) {
				if url == "https://example.com/kubectl.sha256" {
					return []byte("  aabbccdd  \n"), nil // whitespace trimmed
				}
				return nil, fmt.Errorf("unexpected fetch of %q", url)
			},
		}
		m := newTestMgr(fetcher, &MockExtractor{})

		opts := ChecksumOpts{
			Strategy: "per-asset",
			Suffix:   ".sha256",
		}
		resolution := &backend.Resolution{
			Assets: []backend.Asset{
				{Name: "kubectl", URL: "https://example.com/kubectl"},
			},
		}

		got, err := m.resolveChecksums(context.Background(), opts, "kubectl", nil, "https://example.com/kubectl", nil, resolution, "v1.30.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != "aabbccdd" {
			t.Errorf("got sha-256 = %q, want aabbccdd", got["sha-256"])
		}
	})

	t.Run("sha256sums-format line in per-asset file", func(t *testing.T) {
		fetcher := &MockFetcher{
			FetchFn: func(ctx context.Context, url string) ([]byte, error) {
				if url == "https://example.com/kubectl.sha256" {
					// Some projects emit a full sha256sums-format line instead of a bare digest.
					return []byte("aabbccdd  kubectl\n"), nil
				}
				return nil, fmt.Errorf("unexpected fetch of %q", url)
			},
		}
		m := newTestMgr(fetcher, &MockExtractor{})

		opts := ChecksumOpts{
			Strategy: "per-asset",
			Suffix:   ".sha256",
		}
		resolution := &backend.Resolution{
			Assets: []backend.Asset{
				{Name: "kubectl", URL: "https://example.com/kubectl"},
			},
		}

		got, err := m.resolveChecksums(context.Background(), opts, "kubectl", nil, "https://example.com/kubectl", nil, resolution, "v1.30.0")
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if got["sha-256"] != "aabbccdd" {
			t.Errorf("got sha-256 = %q, want aabbccdd", got["sha-256"])
		}
	})

	t.Run("sha256sums-format line asset name mismatch returns error", func(t *testing.T) {
		fetcher := &MockFetcher{
			FetchFn: func(ctx context.Context, url string) ([]byte, error) {
				if url == "https://example.com/kubectl.sha256" {
					return []byte("aabbccdd  wrongname\n"), nil
				}
				return nil, fmt.Errorf("unexpected fetch of %q", url)
			},
		}
		m := newTestMgr(fetcher, &MockExtractor{})

		opts := ChecksumOpts{
			Strategy: "per-asset",
			Suffix:   ".sha256",
		}
		resolution := &backend.Resolution{
			Assets: []backend.Asset{
				{Name: "kubectl", URL: "https://example.com/kubectl"},
			},
		}

		_, err := m.resolveChecksums(context.Background(), opts, "kubectl", nil, "https://example.com/kubectl", nil, resolution, "v1.30.0")
		if err == nil {
			t.Fatal("expected error when asset name not found in checksum file, got nil")
		}
		if !strings.Contains(err.Error(), "kubectl") {
			t.Errorf("error should mention asset name, got: %v", err)
		}
	})
}

func TestResolveChecksums_ShasumURLShortcut(t *testing.T) {
	// Fetcher should never be called when assetChecksums is non-nil and non-empty.
	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, url string) ([]byte, error) {
			return nil, errors.New("fetcher should not be called")
		},
	}
	m := newTestMgr(fetcher, &MockExtractor{})

	// Even with strategy "shared-file", the shortcut should fire.
	opts := ChecksumOpts{Strategy: "shared-file", FileGlob: "SHA256SUMS"}
	prePopulated := map[string]string{"sha-256": "precomputed99"}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "oc.tar.gz", URL: "https://example.com/oc.tar.gz"},
		},
	}

	got, err := m.resolveChecksums(context.Background(), opts, "oc.tar.gz", nil, "", prePopulated, resolution, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["sha-256"] != "precomputed99" {
		t.Errorf("got sha-256 = %q, want precomputed99", got["sha-256"])
	}
}

func TestResolveChecksums_Multisum(t *testing.T) {
	orderContent := "SHA256\nSHA512\n"
	dataContent := "yq_linux_amd64.tar.gz  aa11bb22  cc33dd44\n"

	fetcher := &MockFetcher{
		FetchFn: func(ctx context.Context, url string) ([]byte, error) {
			switch url {
			case "https://example.com/checksums":
				return []byte(dataContent), nil
			case "https://example.com/checksums_hashes_order":
				return []byte(orderContent), nil
			default:
				return nil, fmt.Errorf("unexpected fetch of %q", url)
			}
		},
	}
	m := newTestMgr(fetcher, &MockExtractor{})

	opts := ChecksumOpts{
		Strategy:  "multisum",
		FileGlob:  "checksums",
		OrderGlob: "checksums_hashes_order",
	}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "yq_linux_amd64.tar.gz", URL: "https://example.com/yq_linux_amd64.tar.gz"},
			{Name: "checksums", URL: "https://example.com/checksums"},
			{Name: "checksums_hashes_order", URL: "https://example.com/checksums_hashes_order"},
		},
	}

	got, err := m.resolveChecksums(context.Background(), opts, "yq_linux_amd64.tar.gz", nil, "", nil, resolution, "v4.50.1")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["sha-256"] != "aa11bb22" {
		t.Errorf("got sha-256 = %q, want aa11bb22", got["sha-256"])
	}
	if got["sha-512"] != "cc33dd44" {
		t.Errorf("got sha-512 = %q, want cc33dd44", got["sha-512"])
	}
}

func TestResolveChecksums_Embedded(t *testing.T) {
	checksumFileContent := []byte("ffee1122  mytool.tar.gz\n")
	extractor := &MockExtractor{
		ExtractFn: func(ctx context.Context, name string, data []byte, globs []string) ([]extract.ExtractedFile, error) {
			return []extract.ExtractedFile{
				{SourcePath: "SHA256SUMS", Data: checksumFileContent},
			}, nil
		},
	}
	m := newTestMgr(&MockFetcher{}, extractor)

	opts := ChecksumOpts{
		Strategy:      "embedded",
		TraversalGlob: "SHA256SUMS",
	}
	resolution := &backend.Resolution{
		Assets: []backend.Asset{
			{Name: "mytool.tar.gz", URL: "https://example.com/mytool.tar.gz"},
		},
	}
	assetData := []byte("fake-archive-content")

	got, err := m.resolveChecksums(context.Background(), opts, "mytool.tar.gz", assetData, "", nil, resolution, "v1.0.0")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got["sha-256"] != "ffee1122" {
		t.Errorf("got sha-256 = %q, want ffee1122", got["sha-256"])
	}
}
