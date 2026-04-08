package extract

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"testing"
)

// ============================================================
// Helpers to build small in-memory archives
// ============================================================

// makeTarGz builds a .tar.gz archive in memory from the provided
// map of path → content.
func makeTarGz(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	for name, data := range files {
		hdr := &tar.Header{
			Name:     name,
			Typeflag: tar.TypeReg,
			Size:     int64(len(data)),
			Mode:     0644,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("WriteHeader(%q): %v", name, err)
		}
		if _, err := tw.Write(data); err != nil {
			t.Fatalf("Write(%q): %v", name, err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatal("tar close:", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal("gzip close:", err)
	}
	return buf.Bytes()
}

// makeZip builds a .zip archive in memory.
func makeZip(t *testing.T, files map[string][]byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := zip.NewWriter(&buf)
	for name, data := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip.Create(%q): %v", name, err)
		}
		if _, err := w.Write(data); err != nil {
			t.Fatalf("zip write(%q): %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal("zip close:", err)
	}
	return buf.Bytes()
}

// makeGz wraps raw bytes in a gzip stream.
func makeGz(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	if _, err := gz.Write(data); err != nil {
		t.Fatal("gzip write:", err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal("gzip close:", err)
	}
	return buf.Bytes()
}

// ============================================================
// Tests
// ============================================================

func TestExtract(t *testing.T) {
	ctx := context.Background()
	ex := NewExtractor()

	// Shared fixture: a tar.gz with three files across two directories.
	tarFiles := map[string][]byte{
		"bin/tool":              []byte("tool binary"),
		"bin/helper":           []byte("helper binary"),
		"lib/support.so":       []byte("shared lib"),
	}

	t.Run("tar_gz_empty_globs_returns_all", func(t *testing.T) {
		data := makeTarGz(t, tarFiles)
		got, err := ex.Extract(ctx, "archive.tar.gz", data, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(tarFiles) {
			t.Errorf("got %d files, want %d", len(got), len(tarFiles))
		}
	})

	t.Run("tar_gz_glob_basename_matches_basename_and_full_path", func(t *testing.T) {
		data := makeTarGz(t, tarFiles)
		// "tool" has no slash, so it should match by basename.
		got, err := ex.Extract(ctx, "archive.tar.gz", data, []string{"tool"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d files, want 1", len(got))
		}
		if got[0].SourcePath != "bin/tool" {
			t.Errorf("SourcePath = %q, want %q", got[0].SourcePath, "bin/tool")
		}
		if string(got[0].Data) != "tool binary" {
			t.Errorf("Data = %q, want %q", got[0].Data, "tool binary")
		}
	})

	t.Run("tar_gz_glob_with_slash_matches_full_path_only", func(t *testing.T) {
		data := makeTarGz(t, tarFiles)
		// "bin/tool" contains a slash → only full-path matching.
		got, err := ex.Extract(ctx, "archive.tar.gz", data, []string{"bin/tool"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d files, want 1", len(got))
		}
		if got[0].SourcePath != "bin/tool" {
			t.Errorf("SourcePath = %q, want %q", got[0].SourcePath, "bin/tool")
		}
	})

	t.Run("tar_gz_glob_with_slash_does_not_match_basename_alone", func(t *testing.T) {
		data := makeTarGz(t, tarFiles)
		// "lib/tool" contains a slash but the archive has "bin/tool", not "lib/tool".
		got, err := ex.Extract(ctx, "archive.tar.gz", data, []string{"lib/tool"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d files, want 0 (slash glob must not fall back to basename)", len(got))
		}
	})

	t.Run("tar_gz_wildcard_glob", func(t *testing.T) {
		data := makeTarGz(t, tarFiles)
		// "bin/*" matches both entries in bin/.
		got, err := ex.Extract(ctx, "archive.tar.gz", data, []string{"bin/*"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 2 {
			t.Errorf("got %d files, want 2", len(got))
		}
	})

	t.Run("tgz_suffix_detected", func(t *testing.T) {
		data := makeTarGz(t, tarFiles)
		got, err := ex.Extract(ctx, "archive.tgz", data, nil)
		if err != nil {
			t.Fatalf(".tgz suffix not recognized: %v", err)
		}
		if len(got) != len(tarFiles) {
			t.Errorf("got %d files, want %d", len(got), len(tarFiles))
		}
	})

	t.Run("plain_gz_ignores_globs", func(t *testing.T) {
		payload := []byte("just a file")
		data := makeGz(t, payload)
		// Even with a glob that would match nothing, we get the decompressed content.
		got, err := ex.Extract(ctx, "myfile.bin.gz", data, []string{"nomatch"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d files, want 1", len(got))
		}
		if got[0].SourcePath != "" {
			t.Errorf("SourcePath = %q, want empty string for plain gz", got[0].SourcePath)
		}
		if string(got[0].Data) != string(payload) {
			t.Errorf("Data mismatch: got %q, want %q", got[0].Data, payload)
		}
	})

	t.Run("zip_empty_globs_returns_all", func(t *testing.T) {
		zipFiles := map[string][]byte{
			"a/foo": []byte("foo"),
			"b/bar": []byte("bar"),
		}
		data := makeZip(t, zipFiles)
		got, err := ex.Extract(ctx, "archive.zip", data, nil)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != len(zipFiles) {
			t.Errorf("got %d files, want %d", len(got), len(zipFiles))
		}
	})

	t.Run("zip_glob_basename_matches", func(t *testing.T) {
		zipFiles := map[string][]byte{
			"a/foo": []byte("foo content"),
			"b/bar": []byte("bar content"),
		}
		data := makeZip(t, zipFiles)
		got, err := ex.Extract(ctx, "archive.zip", data, []string{"foo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d files, want 1", len(got))
		}
		if got[0].SourcePath != "a/foo" {
			t.Errorf("SourcePath = %q, want %q", got[0].SourcePath, "a/foo")
		}
	})

	t.Run("zip_glob_with_slash_matches_full_path_only", func(t *testing.T) {
		zipFiles := map[string][]byte{
			"a/foo": []byte("foo content"),
			"b/foo": []byte("other foo"),
		}
		data := makeZip(t, zipFiles)
		got, err := ex.Extract(ctx, "archive.zip", data, []string{"a/foo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d files, want 1", len(got))
		}
		if got[0].SourcePath != "a/foo" {
			t.Errorf("SourcePath = %q, want %q", got[0].SourcePath, "a/foo")
		}
	})

	t.Run("zip_glob_with_slash_does_not_match_basename_alone", func(t *testing.T) {
		zipFiles := map[string][]byte{
			"a/foo": []byte("foo content"),
		}
		data := makeZip(t, zipFiles)
		// "b/foo" has a slash but there is no "b/foo" entry.
		got, err := ex.Extract(ctx, "archive.zip", data, []string{"b/foo"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 0 {
			t.Errorf("got %d files, want 0", len(got))
		}
	})

	t.Run("plain_file_returned_as_is", func(t *testing.T) {
		payload := []byte("raw binary")
		got, err := ex.Extract(ctx, "mybinary", payload, []string{"anything"})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if len(got) != 1 {
			t.Fatalf("got %d files, want 1", len(got))
		}
		if got[0].SourcePath != "" {
			t.Errorf("SourcePath = %q, want empty for plain file", got[0].SourcePath)
		}
		if string(got[0].Data) != string(payload) {
			t.Errorf("Data mismatch")
		}
	})

	// ── bzip2 suffix variants ────────────────────────────────────────────
	// Note: compress/bzip2 is decompression-only; we cannot build a bzip2
	// stream in pure Go stdlib tests.  Instead we verify that the suffix
	// detection routes to the bzip2 path by confirming that a valid bzip2
	// header (magic bytes 'BZh') is attempted and an error is returned for
	// obviously invalid data — OR, more usefully, we test that the
	// .tar.bz2 / .tbz / .tbz2 suffixes do NOT fall through to the plain
	// file handler (which would return the raw bytes unchanged) and instead
	// attempt decompression (returning an error on garbage input).
	for _, suffix := range []string{".tar.bz2", ".tbz", ".tbz2"} {
		suffix := suffix
		t.Run("bzip2_suffix_detected_"+suffix[1:], func(t *testing.T) {
			// Garbage data — decompression must fail, proving the bzip2
			// path was entered rather than the plain-file fallback.
			_, err := ex.Extract(ctx, "archive"+suffix, []byte("not bzip2 data"), nil)
			if err == nil {
				t.Errorf("expected error for invalid bzip2 data with suffix %q, got nil", suffix)
			}
		})
	}

	t.Run("plain_bz2_suffix_detected", func(t *testing.T) {
		// Same reasoning: garbage data must trigger a bzip2 error.
		_, err := ex.Extract(ctx, "file.bz2", []byte("not bzip2"), nil)
		if err == nil {
			t.Error("expected error for invalid bzip2 data with .bz2 suffix, got nil")
		}
	})
}
