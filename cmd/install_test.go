package cmd

import (
	"testing"
)

func TestParseURL_SchemeMissing(t *testing.T) {
	rawURL, version := parseURL("github.com/casey/just")
	if rawURL != "https://github.com/casey/just" {
		t.Errorf("expected https scheme added, got %q", rawURL)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestParseURL_SchemePresent(t *testing.T) {
	rawURL, version := parseURL("https://github.com/casey/just")
	if rawURL != "https://github.com/casey/just" {
		t.Errorf("unexpected URL %q", rawURL)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestParseURL_WithVersion(t *testing.T) {
	rawURL, version := parseURL("github.com/casey/just@1.40.0")
	if rawURL != "https://github.com/casey/just" {
		t.Errorf("unexpected URL %q", rawURL)
	}
	if version != "1.40.0" {
		t.Errorf("expected version 1.40.0, got %q", version)
	}
}

func TestParseURL_AtInPathNotVersion(t *testing.T) {
	// The part after @ contains '/', so it is not a version.
	rawURL, version := parseURL("github.com/casey/just@1.40.0/extra/path")
	if rawURL != "https://github.com/casey/just@1.40.0/extra/path" {
		t.Errorf("unexpected URL %q", rawURL)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestParseURL_EmptyVersionAfterAt(t *testing.T) {
	// Trailing @ with nothing after it — not a version.
	rawURL, version := parseURL("github.com/casey/just@")
	if rawURL != "https://github.com/casey/just@" {
		t.Errorf("unexpected URL %q", rawURL)
	}
	if version != "" {
		t.Errorf("expected empty version, got %q", version)
	}
}

func TestParseFileSpec_SimpleGlob(t *testing.T) {
	assetGlob, traversalGlobs, localName := parseFileSpec("func_linux_amd64")
	if assetGlob != "func_linux_amd64" {
		t.Errorf("unexpected assetGlob %q", assetGlob)
	}
	if len(traversalGlobs) != 0 {
		t.Errorf("expected no traversalGlobs, got %v", traversalGlobs)
	}
	if localName != "" {
		t.Errorf("expected empty localName, got %q", localName)
	}
}

func TestParseFileSpec_GlobWithLocalName(t *testing.T) {
	assetGlob, traversalGlobs, localName := parseFileSpec("func_linux_amd64@kn-func")
	if assetGlob != "func_linux_amd64" {
		t.Errorf("unexpected assetGlob %q", assetGlob)
	}
	if len(traversalGlobs) != 0 {
		t.Errorf("expected no traversalGlobs, got %v", traversalGlobs)
	}
	if localName != "kn-func" {
		t.Errorf("expected localName kn-func, got %q", localName)
	}
}

func TestParseFileSpec_GlobWithTraversal(t *testing.T) {
	assetGlob, traversalGlobs, localName := parseFileSpec("just-${VERSION}-x86_64-unknown-linux-musl.tar.gz!just")
	if assetGlob != "just-${VERSION}-x86_64-unknown-linux-musl.tar.gz" {
		t.Errorf("unexpected assetGlob %q", assetGlob)
	}
	if len(traversalGlobs) != 1 || traversalGlobs[0] != "just" {
		t.Errorf("unexpected traversalGlobs %v", traversalGlobs)
	}
	if localName != "" {
		t.Errorf("expected empty localName, got %q", localName)
	}
}

func TestParseFileSpec_GlobWithTraversalAndLocalName(t *testing.T) {
	assetGlob, traversalGlobs, localName := parseFileSpec("kubelogin-linux-amd64.zip!bin/linux_amd64/kubelogin@kubelogin")
	if assetGlob != "kubelogin-linux-amd64.zip" {
		t.Errorf("unexpected assetGlob %q", assetGlob)
	}
	if len(traversalGlobs) != 1 || traversalGlobs[0] != "bin/linux_amd64/kubelogin" {
		t.Errorf("unexpected traversalGlobs %v", traversalGlobs)
	}
	if localName != "kubelogin" {
		t.Errorf("expected localName kubelogin, got %q", localName)
	}
}

func TestParseFileSpec_MultipleTraversals(t *testing.T) {
	assetGlob, traversalGlobs, localName := parseFileSpec("archive.tar.gz!inner.tar!binary")
	if assetGlob != "archive.tar.gz" {
		t.Errorf("unexpected assetGlob %q", assetGlob)
	}
	if len(traversalGlobs) != 2 || traversalGlobs[0] != "inner.tar" || traversalGlobs[1] != "binary" {
		t.Errorf("unexpected traversalGlobs %v", traversalGlobs)
	}
	if localName != "" {
		t.Errorf("expected empty localName, got %q", localName)
	}
}

func TestParseFileSpec_TildeExpansion(t *testing.T) {
	t.Setenv("HOME", "/home/testuser")
	assetGlob, _, localName := parseFileSpec("archive.tar.gz!libfoo.so@~/.local/lib/libfoo.so")
	if assetGlob != "archive.tar.gz" {
		t.Errorf("unexpected assetGlob %q", assetGlob)
	}
	if localName != "/home/testuser/.local/lib/libfoo.so" {
		t.Errorf("expected expanded localName, got %q", localName)
	}
}

func TestParseChecksumStrategy_Auto(t *testing.T) {
	opts, err := parseChecksumStrategy("auto")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Strategy != "auto" {
		t.Errorf("expected strategy auto, got %q", opts.Strategy)
	}
}

func TestParseChecksumStrategy_None(t *testing.T) {
	opts, err := parseChecksumStrategy("none")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Strategy != "none" {
		t.Errorf("expected strategy none, got %q", opts.Strategy)
	}
}

func TestParseChecksumStrategy_SharedFile(t *testing.T) {
	opts, err := parseChecksumStrategy("shared-file:SHA256SUMS")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Strategy != "shared-file" {
		t.Errorf("expected strategy shared-file, got %q", opts.Strategy)
	}
	if opts.FileGlob != "SHA256SUMS" {
		t.Errorf("expected FileGlob SHA256SUMS, got %q", opts.FileGlob)
	}
}

func TestParseChecksumStrategy_PerAsset(t *testing.T) {
	opts, err := parseChecksumStrategy("per-asset:.sha256")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Strategy != "per-asset" {
		t.Errorf("expected strategy per-asset, got %q", opts.Strategy)
	}
	if opts.Suffix != ".sha256" {
		t.Errorf("expected Suffix .sha256, got %q", opts.Suffix)
	}
}

func TestParseChecksumStrategy_MultisumDefaults(t *testing.T) {
	opts, err := parseChecksumStrategy("multisum")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Strategy != "multisum" {
		t.Errorf("expected strategy multisum, got %q", opts.Strategy)
	}
	if opts.FileGlob != "checksums" {
		t.Errorf("expected FileGlob checksums, got %q", opts.FileGlob)
	}
	if opts.OrderGlob != "checksums_hashes_order" {
		t.Errorf("expected OrderGlob checksums_hashes_order, got %q", opts.OrderGlob)
	}
}

func TestParseChecksumStrategy_MultisumDataOnly(t *testing.T) {
	opts, err := parseChecksumStrategy("multisum:mydata")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.FileGlob != "mydata" {
		t.Errorf("expected FileGlob mydata, got %q", opts.FileGlob)
	}
	if opts.OrderGlob != "checksums_hashes_order" {
		t.Errorf("expected default OrderGlob, got %q", opts.OrderGlob)
	}
}

func TestParseChecksumStrategy_MultisumDataAndOrder(t *testing.T) {
	opts, err := parseChecksumStrategy("multisum:mydata:myorder")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.FileGlob != "mydata" {
		t.Errorf("expected FileGlob mydata, got %q", opts.FileGlob)
	}
	if opts.OrderGlob != "myorder" {
		t.Errorf("expected OrderGlob myorder, got %q", opts.OrderGlob)
	}
}

func TestParseChecksumStrategy_Embedded(t *testing.T) {
	opts, err := parseChecksumStrategy("embedded:checksums.sha256")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if opts.Strategy != "embedded" {
		t.Errorf("expected strategy embedded, got %q", opts.Strategy)
	}
	if opts.TraversalGlob != "checksums.sha256" {
		t.Errorf("expected TraversalGlob checksums.sha256, got %q", opts.TraversalGlob)
	}
}

func TestParseChecksumStrategy_Invalid(t *testing.T) {
	_, err := parseChecksumStrategy("bogus")
	if err == nil {
		t.Error("expected error for invalid strategy")
	}
}
