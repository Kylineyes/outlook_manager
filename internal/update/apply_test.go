package update

import (
	"archive/zip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestFindArchiveAsset(t *testing.T) {
	prefix, err := AssetPrefixForPlatform()
	if err != nil {
		t.Skip(err)
	}
	release := &Release{Assets: []Asset{{Name: "README.md"}, {Name: prefix + "v9.zip"}}}
	asset, err := FindArchiveAsset(release)
	if err != nil {
		t.Fatal(err)
	}
	if asset.Name != prefix+"v9.zip" {
		t.Fatalf("asset = %s", asset.Name)
	}
}

func TestChecksumForFile(t *testing.T) {
	dir := t.TempDir()
	checksums := filepath.Join(dir, "SHA256SUMS")
	content := "abc123  package.zip\n"
	if err := os.WriteFile(checksums, []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
	value, err := checksumForFile(checksums, "package.zip")
	if err != nil {
		t.Fatal(err)
	}
	if value != "abc123" {
		t.Fatalf("checksum = %s", value)
	}
}

func TestDownloadReleaseVerifiesChecksum(t *testing.T) {
	dir := t.TempDir()
	prefix, err := AssetPrefixForPlatform()
	if err != nil {
		t.Skip(err)
	}
	archiveName := prefix + "v9.9.9.zip"
	archivePath := filepath.Join(dir, archiveName)
	binaryName := currentBinaryName()
	file, err := os.Create(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	writer := zip.NewWriter(file)
	entry, err := writer.Create(binaryName)
	if err != nil {
		t.Fatal(err)
	}
	_, _ = entry.Write([]byte("new-binary"))
	if err := writer.Close(); err != nil {
		t.Fatal(err)
	}
	file.Close()
	data, err := os.ReadFile(archivePath)
	if err != nil {
		t.Fatal(err)
	}
	sum := sha256.Sum256(data)
	checksum := hex.EncodeToString(sum[:])

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/package.zip":
			_, _ = w.Write(data)
		case "/SHA256SUMS":
			_, _ = fmt.Fprintf(w, "%s  %s\n", checksum, archiveName)
		default:
			http.NotFound(w, r)
		}
	}))
	defer server.Close()

	release := &Release{TagName: "v9.9.9", Assets: []Asset{
		{Name: archiveName, BrowserDownloadURL: server.URL + "/package.zip"},
		{Name: "SHA256SUMS", BrowserDownloadURL: server.URL + "/SHA256SUMS"},
	}}
	result, err := DownloadRelease(context.Background(), release, t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	if !result.ChecksumVerified {
		t.Fatal("checksum not verified")
	}
	binary, err := os.ReadFile(result.BinaryPath)
	if err != nil {
		t.Fatal(err)
	}
	if string(binary) != "new-binary" {
		t.Fatalf("binary = %q", binary)
	}
}
