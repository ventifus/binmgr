package backend

import (
	"bytes"
	"crypto/sha256"
	"crypto/sha512"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestChecksumTypes(t *testing.T) {
	types := ChecksumTypes()
	if len(types) == 0 {
		t.Error("ChecksumTypes returned empty slice")
	}

	expected := []string{
		ChecksumNone,
		ChecksumShasum256,
		ChecksumPerAssetSig,
		ChecksumPerAssetPem,
		ChecksumPerAssetSha256Sum,
		ChecksumPerAssetSha256,
		ChecksumMultiSum,
	}

	if len(types) != len(expected) {
		t.Errorf("Expected %d types, got %d", len(expected), len(types))
	}

	for i, exp := range expected {
		if types[i] != exp {
			t.Errorf("Expected types[%d] = %s, got %s", i, exp, types[i])
		}
	}
}

func TestChecksumAlgorithms(t *testing.T) {
	algos := ChecksumAlgorithms()
	if len(algos) != 1 {
		t.Errorf("Expected 1 algorithm, got %d", len(algos))
	}
	if algos[0] != AlgorithmSha256 {
		t.Errorf("Expected %s, got %s", AlgorithmSha256, algos[0])
	}
}

func TestGetHashFuncs(t *testing.T) {
	tests := []struct {
		name     string
		types    []string
		expected int
	}{
		{
			name:     "sha256",
			types:    []string{AlgorithmSha256},
			expected: 1,
		},
		{
			name:     "sha384",
			types:    []string{AlgorithmSha384},
			expected: 1,
		},
		{
			name:     "sha512",
			types:    []string{AlgorithmSha512},
			expected: 1,
		},
		{
			name:     "multiple",
			types:    []string{AlgorithmSha256, AlgorithmSha384, AlgorithmSha512},
			expected: 3,
		},
		{
			name:     "unknown",
			types:    []string{"unknown-algo"},
			expected: 0,
		},
		{
			name:     "mixed",
			types:    []string{AlgorithmSha256, "unknown", AlgorithmSha512},
			expected: 2,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			hashes := getHashFuncs(tt.types)
			if len(hashes) != tt.expected {
				t.Errorf("Expected %d hashes, got %d", tt.expected, len(hashes))
			}
		})
	}
}

func TestComputeChecksums(t *testing.T) {
	testData := []byte("Hello, World!")

	// Compute expected checksums
	sha256sum := sha256.Sum256(testData)
	expectedSha256 := hex.EncodeToString(sha256sum[:])

	sha384sum := sha512.Sum384(testData)
	expectedSha384 := hex.EncodeToString(sha384sum[:])

	tests := []struct {
		name  string
		data  []byte
		types []string
		want  map[string]string
	}{
		{
			name:  "sha256",
			data:  testData,
			types: []string{AlgorithmSha256},
			want:  map[string]string{AlgorithmSha256: expectedSha256},
		},
		{
			name:  "sha384",
			data:  testData,
			types: []string{AlgorithmSha384},
			want:  map[string]string{AlgorithmSha384: expectedSha384},
		},
		{
			name:  "multiple",
			data:  testData,
			types: []string{AlgorithmSha256, AlgorithmSha384},
			want: map[string]string{
				AlgorithmSha256: expectedSha256,
				AlgorithmSha384: expectedSha384,
			},
		},
		{
			name:  "empty data",
			data:  []byte{},
			types: []string{AlgorithmSha256},
			want:  map[string]string{AlgorithmSha256: "e3b0c44298fc1c149afbf4c8996fb92427ae41e4649b934ca495991b7852b855"},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := ComputeChecksums(tt.data, tt.types)
			if err != nil {
				t.Fatalf("ComputeChecksums failed: %v", err)
			}

			if len(got) != len(tt.want) {
				t.Errorf("Expected %d checksums, got %d", len(tt.want), len(got))
			}

			for k, v := range tt.want {
				if got[k] != v {
					t.Errorf("Checksum mismatch for %s: expected %s, got %s", k, v, got[k])
				}
			}
		})
	}
}

func TestGetChecksumUrl(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		wantErr    bool
		wantCount  int
	}{
		{
			name: "standard sha256sums",
			response: `abc123  file1.tar.gz
def456  file2.tar.gz
789xyz  file3.tar.gz`,
			statusCode: 200,
			wantErr:    false,
			wantCount:  3,
		},
		{
			name: "with asterisk prefix",
			response: `abc123  *file1.tar.gz
def456  *file2.tar.gz`,
			statusCode: 200,
			wantErr:    false,
			wantCount:  2,
		},
		{
			name:       "404 error",
			response:   "Not Found",
			statusCode: 404,
			wantErr:    true,
			wantCount:  0,
		},
		{
			name:       "empty file",
			response:   "",
			statusCode: 200,
			wantErr:    false,
			wantCount:  0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			csums, err := GetChecksumUrl(nil, server.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetChecksumUrl() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if len(csums) != tt.wantCount {
				t.Errorf("Expected %d checksums, got %d", tt.wantCount, len(csums))
			}

			if !tt.wantErr && tt.wantCount > 0 {
				// Verify first entry doesn't have asterisk
				if len(csums) > 0 && bytes.HasPrefix([]byte(csums[0].Name), []byte("*")) {
					t.Error("Asterisk prefix not removed from filename")
				}
			}
		})
	}
}

func TestGetChecksumMap(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		statusCode int
		wantErr    bool
		want       []string
	}{
		{
			name: "standard map",
			response: `sha-256
sha-384
sha-512`,
			statusCode: 200,
			wantErr:    false,
			want:       []string{"sha-256", "sha-384", "sha-512"},
		},
		{
			name: "mixed case",
			response: `SHA-256
Sha-384`,
			statusCode: 200,
			wantErr:    false,
			want:       []string{"sha-256", "sha-384"},
		},
		{
			name:       "404 error",
			statusCode: 404,
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			got, err := GetChecksumMap(nil, server.URL)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetChecksumMap() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("Expected %d items, got %d", len(tt.want), len(got))
				}
				for i, w := range tt.want {
					if i >= len(got) || got[i] != w {
						t.Errorf("Expected got[%d] = %s, got %s", i, w, got[i])
					}
				}
			}
		})
	}
}

func TestGetSumForFile(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		filename   string
		statusCode int
		want       string
		wantErr    bool
	}{
		{
			name: "file found",
			response: `abc123  file1.tar.gz
def456  file2.tar.gz
789xyz  file3.tar.gz`,
			filename:   "file2.tar.gz",
			statusCode: 200,
			want:       "def456",
			wantErr:    false,
		},
		{
			name: "file with asterisk",
			response: `abc123  *file1.tar.gz
def456  *file2.tar.gz`,
			filename:   "file2.tar.gz",
			statusCode: 200,
			want:       "def456",
			wantErr:    false,
		},
		{
			name: "file with ./ prefix",
			response: `abc123  ./file1.tar.gz
def456  ./file2.tar.gz`,
			filename:   "file2.tar.gz",
			statusCode: 200,
			want:       "def456",
			wantErr:    false,
		},
		{
			name: "file not found",
			response: `abc123  file1.tar.gz
def456  file2.tar.gz`,
			filename:   "file3.tar.gz",
			statusCode: 200,
			wantErr:    true,
		},
		{
			name:       "404 error",
			statusCode: 404,
			filename:   "any.tar.gz",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			got, err := GetSumForFile(nil, server.URL, tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetSumForFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr && got != tt.want {
				t.Errorf("GetSumForFile() = %s, want %s", got, tt.want)
			}
		})
	}
}

func TestGetMultiSumForFile(t *testing.T) {
	tests := []struct {
		name       string
		response   string
		csumMap    []string
		filename   string
		statusCode int
		want       map[string]string
		wantErr    bool
	}{
		{
			name: "file found with multiple checksums",
			response: `file1.tar.gz  abc123  def456  789xyz
file2.tar.gz  111222  333444  555666`,
			csumMap:    []string{"sha-256", "sha-384", "sha-512"},
			filename:   "file1.tar.gz",
			statusCode: 200,
			want: map[string]string{
				"sha-256": "abc123",
				"sha-384": "def456",
				"sha-512": "789xyz",
			},
			wantErr: false,
		},
		{
			name: "file not found",
			response: `file1.tar.gz  abc123  def456
file2.tar.gz  111222  333444`,
			csumMap:    []string{"sha-256", "sha-384"},
			filename:   "file3.tar.gz",
			statusCode: 200,
			wantErr:    true,
		},
		{
			name:       "404 error",
			statusCode: 404,
			csumMap:    []string{"sha-256"},
			filename:   "any.tar.gz",
			wantErr:    true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tt.statusCode)
				w.Write([]byte(tt.response))
			}))
			defer server.Close()

			got, err := GetMultiSumForFile(nil, tt.csumMap, server.URL, tt.filename)
			if (err != nil) != tt.wantErr {
				t.Errorf("GetMultiSumForFile() error = %v, wantErr %v", err, tt.wantErr)
				return
			}

			if !tt.wantErr {
				if len(got) != len(tt.want) {
					t.Errorf("Expected %d checksums, got %d", len(tt.want), len(got))
				}
				for k, v := range tt.want {
					if got[k] != v {
						t.Errorf("Expected %s = %s, got %s", k, v, got[k])
					}
				}
			}
		})
	}
}

func TestVerifyBytes(t *testing.T) {
	testData := []byte("Hello, World!")
	sha256sum := sha256.Sum256(testData)
	validSha256 := hex.EncodeToString(sha256sum[:])

	tests := []struct {
		name     string
		data     []byte
		checksum map[string]string
		wantErr  bool
	}{
		{
			name: "valid checksum",
			data: testData,
			checksum: map[string]string{
				AlgorithmSha256: validSha256,
			},
			wantErr: false,
		},
		{
			name: "invalid checksum",
			data: testData,
			checksum: map[string]string{
				AlgorithmSha256: "invalid_checksum",
			},
			wantErr: true,
		},
		{
			name: "empty checksum value skipped",
			data: testData,
			checksum: map[string]string{
				AlgorithmSha256: "",
			},
			wantErr: false,
		},
		{
			name: "multiple algorithms - all valid",
			data: testData,
			checksum: map[string]string{
				AlgorithmSha256: validSha256,
			},
			wantErr: false,
		},
		{
			name:     "empty checksum map",
			data:     testData,
			checksum: map[string]string{},
			wantErr:  false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyBytes(tt.data, tt.checksum)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyBytes() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}

func TestVerifyLocalFile(t *testing.T) {
	// Create temporary file
	tmpfile, err := os.CreateTemp("", "test-binary-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmpfile.Name())

	testData := []byte("test binary content")
	if _, err := tmpfile.Write(testData); err != nil {
		t.Fatal(err)
	}
	tmpfile.Close()

	sha256sum := sha256.Sum256(testData)
	validChecksum := hex.EncodeToString(sha256sum[:])

	tests := []struct {
		name     string
		artifact *Artifact
		wantErr  bool
	}{
		{
			name: "valid file with valid checksum",
			artifact: &Artifact{
				LocalFile: tmpfile.Name(),
				Checksums: map[string]string{
					AlgorithmSha256: validChecksum,
				},
			},
			wantErr: false,
		},
		{
			name: "valid file with invalid checksum",
			artifact: &Artifact{
				LocalFile: tmpfile.Name(),
				Checksums: map[string]string{
					AlgorithmSha256: "invalid_checksum",
				},
			},
			wantErr: true,
		},
		{
			name: "file does not exist",
			artifact: &Artifact{
				LocalFile: "/nonexistent/file/path",
				Checksums: map[string]string{
					AlgorithmSha256: validChecksum,
				},
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := VerifyLocalFile(tt.artifact)
			if (err != nil) != tt.wantErr {
				t.Errorf("VerifyLocalFile() error = %v, wantErr %v", err, tt.wantErr)
			}
		})
	}
}
