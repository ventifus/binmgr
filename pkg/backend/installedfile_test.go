package backend

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
)

func TestInstallBin(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	testData := []byte("#!/bin/sh\necho test")
	testFile := tmpDir + "/test-binary"

	err = installBin(testData, testFile)
	if err != nil {
		t.Fatalf("installBin failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(testFile); os.IsNotExist(err) {
		t.Error("Binary file was not created")
	}

	// Verify content
	content, err := os.ReadFile(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(content, testData) {
		t.Error("File content doesn't match")
	}

	// Verify permissions
	info, err := os.Stat(testFile)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0755 {
		t.Errorf("Expected permissions 0755, got %o", info.Mode().Perm())
	}
}

func TestInstallBin_InvalidPath(t *testing.T) {
	testData := []byte("test")
	err := installBin(testData, "/nonexistent/directory/binary")
	if err == nil {
		t.Error("Expected error for invalid path")
	}
}

func TestDownloadFile(t *testing.T) {
	testData := []byte("test file content")
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Length", string(rune(len(testData))))
		w.WriteHeader(200)
		w.Write(testData)
	}))
	defer server.Close()

	artifact := &Artifact{
		RemoteFile: server.URL,
	}

	ctx := context.Background()
	data, err := DownloadFile(ctx, nil, artifact)
	if err != nil {
		t.Fatalf("DownloadFile failed: %v", err)
	}

	if !bytes.Equal(data, testData) {
		t.Error("Downloaded data doesn't match")
	}
}

func TestDownloadFile_404(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(404)
		w.Write([]byte("Not Found"))
	}))
	defer server.Close()

	artifact := &Artifact{
		RemoteFile: server.URL,
	}

	ctx := context.Background()
	_, err := DownloadFile(ctx, nil, artifact)
	if err == nil {
		t.Error("Expected error for 404 response")
	}
}

func TestInstallFile_PlainBinary(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a binary-like file (ELF header)
	binaryData := []byte{0x7f, 0x45, 0x4c, 0x46} // ELF magic number
	binaryData = append(binaryData, []byte("rest of binary content")...)

	sha256sum := sha256.Sum256(binaryData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/test-binary"

	err = InstallFile(artifact, binaryData, localFile, "")
	if err != nil {
		t.Fatalf("InstallFile failed: %v", err)
	}

	if !artifact.Installed {
		t.Error("Artifact not marked as installed")
	}
	if artifact.LocalFile != localFile {
		t.Errorf("LocalFile not set correctly: got %s", artifact.LocalFile)
	}

	// Verify file exists
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("Binary file was not created")
	}
}

func TestInstallFile_TarGz(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create tar.gz archive with a binary
	binaryContent := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02} // ELF header
	binaryContent = append(binaryContent, []byte("test binary")...)

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	header := &tar.Header{
		Name:     "mybinary",
		Mode:     0755,
		Size:     int64(len(binaryContent)),
		Typeflag: tar.TypeReg,
	}
	if err := tw.WriteHeader(header); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	tw.Close()

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	if _, err := gw.Write(tarBuf.Bytes()); err != nil {
		t.Fatal(err)
	}
	gw.Close()

	archiveData := gzBuf.Bytes()
	sha256sum := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/extracted-binary"

	err = InstallFile(artifact, archiveData, localFile, "mybinary")
	if err != nil {
		t.Fatalf("InstallFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("Extracted binary file was not created")
	}

	// Verify it's marked as installed
	if len(artifact.InnerArtifacts) == 0 {
		t.Error("No inner artifacts recorded")
	} else if !artifact.InnerArtifacts[0].Installed {
		t.Error("Inner artifact not marked as installed")
	}
}

func TestInstallFile_Zip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create zip archive with a binary
	binaryContent := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02} // ELF header
	binaryContent = append(binaryContent, []byte("test binary")...)

	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	fw, err := zw.Create("mybinary.exe")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := fw.Write(binaryContent); err != nil {
		t.Fatal(err)
	}
	zw.Close()

	archiveData := zipBuf.Bytes()
	sha256sum := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/extracted-binary.exe"

	err = InstallFile(artifact, archiveData, localFile, "mybinary.exe")
	if err != nil {
		t.Fatalf("InstallFile failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(localFile); os.IsNotExist(err) {
		t.Error("Extracted binary file was not created")
	}

	// Verify artifact is marked as installed
	if !artifact.Installed {
		t.Error("Artifact not marked as installed")
	}
}

func TestInstallFile_InvalidChecksum(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	binaryData := []byte("test binary")

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: "invalid_checksum"}

	localFile := tmpDir + "/test-binary"

	err = InstallFile(artifact, binaryData, localFile, "")
	if err == nil {
		t.Error("Expected error for invalid checksum")
	}
}

func TestInstallFile_NoMatchingFileInTar(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create tar.gz archive
	binaryContent := []byte{0x7f, 0x45, 0x4c, 0x46, 0x02}

	var tarBuf bytes.Buffer
	tw := tar.NewWriter(&tarBuf)

	header := &tar.Header{
		Name:     "other-file",
		Mode:     0755,
		Size:     int64(len(binaryContent)),
		Typeflag: tar.TypeReg,
	}
	tw.WriteHeader(header)
	tw.Write(binaryContent)
	tw.Close()

	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	gw.Write(tarBuf.Bytes())
	gw.Close()

	archiveData := gzBuf.Bytes()
	sha256sum := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/extracted-binary"

	err = InstallFile(artifact, archiveData, localFile, "nonexistent")
	if err == nil {
		t.Error("Expected error when no matching file in tar")
	}
	if err != nil && !containsMiddle(err.Error(), "no matching files") {
		t.Errorf("Expected 'no matching files' error, got: %v", err)
	}
}

func TestInstallFile_NoMatchingFileInZip(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create zip archive
	var zipBuf bytes.Buffer
	zw := zip.NewWriter(&zipBuf)

	fw, _ := zw.Create("other-file.exe")
	fw.Write([]byte("content"))
	zw.Close()

	archiveData := zipBuf.Bytes()
	sha256sum := sha256.Sum256(archiveData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/extracted-binary"

	err = InstallFile(artifact, archiveData, localFile, "nonexistent")
	if err == nil {
		t.Error("Expected error when no matching file in zip")
	}
	if err != nil && !containsMiddle(err.Error(), "no matching files") {
		t.Errorf("Expected 'no matching files' error, got: %v", err)
	}
}

func TestInstallFile_UnsupportedFileType(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Create a PDF file (unsupported type)
	pdfData := []byte("%PDF-1.4\ntest content")
	sha256sum := sha256.Sum256(pdfData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/test-file.pdf"

	err = InstallFile(artifact, pdfData, localFile, "")
	if err == nil {
		t.Error("Expected error for unsupported file type")
	}
}

func TestInstallFile_EmptyData(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "binmgr-test-")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(tmpDir)

	// Empty file should install as binary with no mime type
	emptyData := []byte{}
	sha256sum := sha256.Sum256(emptyData)
	checksum := hex.EncodeToString(sha256sum[:])

	artifact := NewArtifact()
	artifact.Checksums = map[string]string{AlgorithmSha256: checksum}

	localFile := tmpDir + "/empty-binary"

	err = InstallFile(artifact, emptyData, localFile, "")
	if err != nil {
		t.Fatalf("InstallFile failed on empty data: %v", err)
	}

	if !artifact.Installed {
		t.Error("Empty file should be marked as installed")
	}
}
