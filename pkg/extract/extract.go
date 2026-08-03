package extract

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/bzip2"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"path/filepath"
	"strings"
)

// maxExtractBytes is the upper bound on decompressed data from any single
// read. 2 GB is well above the largest real binary we expect (~248 MB) while
// still protecting against decompression-bomb archives.
//
// This is a var (not a const) so that tests can temporarily lower the limit
// to exercise the decompression-bomb guard without allocating gigabytes.
var maxExtractBytes int64 = 2 * 1024 * 1024 * 1024

// limitedReadAll reads from r and returns the contents. It returns an error
// if the stream exceeds maxExtractBytes, defending against decompression bombs.
func limitedReadAll(r io.Reader) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(r, maxExtractBytes+1))
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > maxExtractBytes {
		return nil, fmt.Errorf("extracted content exceeds %d byte limit", maxExtractBytes)
	}
	return data, nil
}

// ExtractedFile holds a single file produced by an extraction pass.
type ExtractedFile struct {
	// SourcePath is the path within the archive.  It is empty for
	// plain-compressed files (.gz / .bz2 without a tar wrapper).
	SourcePath string
	Data       []byte
}

// Extractor extracts files from an in-memory archive.
type Extractor interface {
	Extract(ctx context.Context, name string, data []byte, globs []string) ([]ExtractedFile, error)
}

type extractor struct{}

// NewExtractor returns a new Extractor.
func NewExtractor() Extractor {
	return &extractor{}
}

// Extract dispatches to the appropriate format handler based on the
// asset filename (name).  Format detection uses a fixed priority order
// (.tar.gz before .gz, .tar.bz2 before .bz2).
func (e *extractor) Extract(ctx context.Context, name string, data []byte, globs []string) ([]ExtractedFile, error) {
	lower := strings.ToLower(name)

	switch {
	case strings.HasSuffix(lower, ".tar.gz"), strings.HasSuffix(lower, ".tgz"):
		decompressed, err := gunzip(data)
		if err != nil {
			return nil, err
		}
		return extractTar(decompressed, globs)

	case strings.HasSuffix(lower, ".tar.bz2"),
		strings.HasSuffix(lower, ".tbz"),
		strings.HasSuffix(lower, ".tbz2"):
		decompressed, err := bunzip2(data)
		if err != nil {
			return nil, err
		}
		return extractTar(decompressed, globs)

	case strings.HasSuffix(lower, ".gz"):
		// plain gzip — globs are ignored
		decompressed, err := gunzip(data)
		if err != nil {
			return nil, err
		}
		return []ExtractedFile{{SourcePath: "", Data: decompressed}}, nil

	case strings.HasSuffix(lower, ".bz2"):
		// plain bzip2 — globs are ignored
		decompressed, err := bunzip2(data)
		if err != nil {
			return nil, err
		}
		return []ExtractedFile{{SourcePath: "", Data: decompressed}}, nil

	case strings.HasSuffix(lower, ".zip"):
		return extractZip(data, globs)

	default:
		// Return as-is; globs are not applied to plain files.
		return []ExtractedFile{{SourcePath: "", Data: data}}, nil
	}
}

// gunzip decompresses a gzip stream.
func gunzip(data []byte) ([]byte, error) {
	r, err := gzip.NewReader(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	defer r.Close()
	return limitedReadAll(r)
}

// bunzip2 decompresses a bzip2 stream.
func bunzip2(data []byte) ([]byte, error) {
	r := bzip2.NewReader(bytes.NewReader(data))
	return limitedReadAll(r)
}

// validateEntryPath rejects archive entry names that could escape the
// extraction directory via path traversal or absolute paths.
func validateEntryPath(name string) error {
	if filepath.IsAbs(name) {
		return fmt.Errorf("archive entry has absolute path: %q", name)
	}
	if strings.HasPrefix(filepath.Clean(name), "..") {
		return fmt.Errorf("archive entry escapes extraction directory: %q", name)
	}
	return nil
}

// matchesGlobs reports whether an archive entry at entryPath should be
// included given the caller's glob list.
//
// Rules:
//   - Empty globs list → include everything.
//   - Glob containing "/" → match against the full entry path only.
//   - Glob without "/"   → match against both the full entry path AND
//     the basename of the entry path.
func matchesGlobs(entryPath string, globs []string) (bool, error) {
	if len(globs) == 0 {
		return true, nil
	}
	base := filepath.Base(entryPath)
	for _, g := range globs {
		if strings.Contains(g, "/") {
			// Match full path only.
			ok, err := filepath.Match(g, entryPath)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		} else {
			// Match full path OR basename.
			ok, err := filepath.Match(g, entryPath)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
			ok, err = filepath.Match(g, base)
			if err != nil {
				return false, err
			}
			if ok {
				return true, nil
			}
		}
	}
	return false, nil
}

// extractTar iterates over a tar stream and returns all regular files
// that match globs.
func extractTar(data []byte, globs []string) ([]ExtractedFile, error) {
	tr := tar.NewReader(bytes.NewReader(data))
	var results []ExtractedFile
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if err := validateEntryPath(hdr.Name); err != nil {
			return nil, err
		}
		ok, err := matchesGlobs(hdr.Name, globs)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		contents, err := limitedReadAll(tr)
		if err != nil {
			return nil, err
		}
		results = append(results, ExtractedFile{SourcePath: hdr.Name, Data: contents})
	}
	return results, nil
}

// extractZip iterates over a zip archive and returns all files that
// match globs.
func extractZip(data []byte, globs []string) ([]ExtractedFile, error) {
	r, err := zip.NewReader(bytes.NewReader(data), int64(len(data)))
	if err != nil {
		return nil, err
	}
	var results []ExtractedFile
	for _, f := range r.File {
		if f.Mode().IsDir() {
			continue
		}
		if err := validateEntryPath(f.Name); err != nil {
			return nil, err
		}
		ok, err := matchesGlobs(f.Name, globs)
		if err != nil {
			return nil, err
		}
		if !ok {
			continue
		}
		rc, err := f.Open()
		if err != nil {
			return nil, err
		}
		contents, err := limitedReadAll(rc)
		rc.Close()
		if err != nil {
			return nil, err
		}
		results = append(results, ExtractedFile{SourcePath: f.Name, Data: contents})
	}
	return results, nil
}
