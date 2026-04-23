package beam

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// makeFakeTarGz builds an in-memory tar.gz containing a single file.
func makeFakeTarGz(files map[string][]byte) ([]byte, error) {
	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	for name, content := range files {
		hdr := &tar.Header{
			Name:     name,
			Mode:     0750,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			return nil, err
		}
		if _, err := tw.Write(content); err != nil {
			return nil, err
		}
	}
	if err := tw.Close(); err != nil {
		return nil, err
	}
	if err := gzw.Close(); err != nil {
		return nil, err
	}
	return buf.Bytes(), nil
}

func TestDownloadCNIPlugins_AlreadyPresent(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Create a fake "bridge" binary to simulate already-downloaded plugins
	bridgePath := filepath.Join(dir, "bridge")
	if err := os.WriteFile(bridgePath, []byte("fake"), 0750); err != nil {
		t.Fatal(err)
	}

	dl := NewCNIDownloader()
	// Should return immediately without making any HTTP request
	if err := dl.DownloadCNIPlugins(context.Background(), dir); err != nil {
		t.Errorf("expected no error with existing bridge, got: %v", err)
	}
}

func TestDownloadCNIPlugins_Success(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	// Build a minimal tar.gz that contains a "bridge" executable
	tgz, err := makeFakeTarGz(map[string][]byte{
		"bridge":   []byte("#!/bin/sh\necho bridge"),
		"loopback": []byte("#!/bin/sh\necho loopback"),
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/x-gzip")
		if _, writeErr := w.Write(tgz); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	if errSucc := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir); errSucc != nil {
		t.Fatalf("unexpected error: %v", errSucc)
	}

	// "bridge" should now exist in the target dir
	if _, statErr := os.Stat(filepath.Join(dir, "bridge")); statErr != nil {
		t.Errorf("bridge not extracted: %v", statErr)
	}
}

func TestDownloadCNIPlugins_WriteFileFailure(t *testing.T) {
	t.Parallel()
	tgz, err := makeFakeTarGz(map[string][]byte{"bridge": []byte("fake")})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write(tgz); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dir := t.TempDir()
	if errCh := os.Chmod(dir, 0555); errCh != nil {
		t.Fatal(errCh)
	}
	t.Cleanup(func() {
		if chmodErr := os.Chmod(dir, 0755); chmodErr != nil {
			t.Fatalf("failed to restore file permissions: %v", chmodErr)
		}
	})

	if os.Geteuid() == 0 {
		t.Skip("root ignores permission bits")
	}

	dl := NewCNIDownloader()
	err = dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir)
	if err == nil {
		t.Fatal("expected error writing to read-only dir, got nil")
	}
}

func TestDownloadCNIPlugins_HTTPError(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir)
	if err == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if !strings.Contains(err.Error(), "404") {
		t.Errorf("expected 404 in error message, got: %v", err)
	}
}

func TestDownloadCNIPlugins_InvalidGzip(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write([]byte("this is not gzip at all")); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir)
	if err == nil {
		t.Fatal("expected error for invalid gzip, got nil")
	}
}

func TestDownloadCNIPlugins_InvalidTar(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	if _, writeErr := gzw.Write([]byte("this is valid gzip but invalid tar content")); writeErr != nil {
		t.Fatalf("failed to write test response: %v", writeErr)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write(buf.Bytes()); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir)
	if err == nil {
		t.Fatal("expected error for invalid tar, got nil")
	}
}

func TestDownloadCNIPlugins_TarBombRejected(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	tgz, err := makeFakeTarGz(map[string][]byte{
		"../../etc/malicious": []byte("evil content"),
	})
	if err != nil {
		t.Fatal(err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write(tgz); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	err = dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir)
	if err == nil {
		t.Fatal("expected error for tar bomb path traversal, got nil")
	}
	if !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("unexpected error message: %v", err)
	}
}

func TestDownloadCNIPlugins_MkdirFailure(t *testing.T) {
	t.Parallel()
	tmpFile, err := os.CreateTemp(t.TempDir(), "notadir")
	if err != nil {
		t.Fatal(err)
	}
	if closeErr := tmpFile.Close(); closeErr != nil {
		t.Fatalf("failed to close temp file: %v", closeErr)
	}

	dl := NewCNIDownloader()
	err = dl.DownloadCNIPlugins(context.Background(), filepath.Join(tmpFile.Name(), "subdir"))
	if err == nil {
		t.Fatal("expected error for non-creatable dir, got nil")
	}
}

func TestGetCNIPluginsURL(t *testing.T) {
	t.Parallel()
	url := getCNIPluginsURL()
	if !strings.Contains(url, CNIPluginsVersion) {
		t.Errorf("URL missing version %s: %s", CNIPluginsVersion, url)
	}
	if !strings.Contains(url, runtime.GOOS) {
		t.Errorf("URL missing GOOS %s: %s", runtime.GOOS, url)
	}
	if !strings.Contains(url, runtime.GOARCH) {
		t.Errorf("URL missing GOARCH %s: %s", runtime.GOARCH, url)
	}
}

func TestDownloadCNIPlugins_DirectoryEntry(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "subdir/",
		Typeflag: tar.TypeDir,
		Mode:     0750,
	}); err != nil {
		t.Fatalf("failed to write directory header: %v", err)
	}
	content := []byte("#!/bin/sh")
	if err := tw.WriteHeader(&tar.Header{
		Name:     "subdir/loopback",
		Typeflag: tar.TypeReg,
		Mode:     0750,
		Size:     int64(len(content)),
	}); err != nil {
		t.Fatalf("failed to write file header: %v", err)
	}
	if _, writeErr := tw.Write(content); writeErr != nil {
		t.Fatalf("failed to write file content: %v", writeErr)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write(buf.Bytes()); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	if err := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, dir); err != nil {
		t.Fatalf("unexpected error with dir entry: %v", err)
	}
}

func TestDownloadCNIPlugins_InvalidRequestURL(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), "://bad-url", dir)
	if err == nil {
		t.Fatal("expected error for invalid URL, got nil")
	}
}

func TestDownloadCNIPlugins_ClientDoFailure(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), "http://localhost:0/cni.tgz", dir)
	if err == nil {
		t.Fatal("expected connection error, got nil")
	}
}

func TestExtractFile_DirectoryMkdirFailure(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	if err := os.WriteFile(filepath.Join(targetDir, "collision"), []byte("x"), 0600); err != nil {
		t.Fatalf("failed to write collision file: %v", err)
	}
	if err := tw.WriteHeader(&tar.Header{
		Name:     "collision/sub/",
		Typeflag: tar.TypeDir,
		Mode:     0750,
	}); err != nil {
		t.Fatalf("failed to write directory header: %v", err)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write(buf.Bytes()); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, targetDir)
	if err == nil {
		t.Fatal("expected error when MkdirAll conflicts with existing file, got nil")
	}
}

func TestExtractFile_DecompressionBomb(t *testing.T) {
	t.Parallel()
	targetDir := t.TempDir()

	var buf bytes.Buffer
	gzw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gzw)

	contentLarge := make([]byte, maxDecompressionSize+1)
	if err := tw.WriteHeader(&tar.Header{
		Name:     "bomb",
		Typeflag: tar.TypeReg,
		Mode:     0750,
		Size:     int64(len(contentLarge)),
	}); err != nil {
		t.Fatalf("failed to write directory header: %v", err)
	}
	if _, writeErr := tw.Write(contentLarge); writeErr != nil {
		t.Fatalf("failed to write file content: %v", writeErr)
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("failed to close tar writer: %v", err)
	}
	if err := gzw.Close(); err != nil {
		t.Fatalf("failed to close gzip writer: %v", err)
	}

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		if _, writeErr := w.Write(buf.Bytes()); writeErr != nil {
			t.Errorf("failed to write test response: %v", writeErr)
		}
	}))
	defer srv.Close()

	dl := NewCNIDownloader()
	err := dl.downloadCNIPluginsFromURL(context.Background(), srv.URL, targetDir)
	if err == nil {
		t.Fatal("expected error for decompression bomb, got nil")
	}
}

func TestDownloadCNIPlugins_Setters(t *testing.T) {
	dl := NewCNIDownloader().
		WithFS(RealFS{}).
		WithHTTPClient(&http.Client{}).
		WithExtractor(realExtractor{})
	if dl.fs == nil || dl.http == nil || dl.extract == nil {
		t.Fatal("setters failed to set fields")
	}
}

func TestDownloadCNIPlugins_PackageLevel(t *testing.T) {
	// This will likely fail or short-circuit, which is fine for coverage.
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, "bridge"), []byte("fake"), 0750); err != nil {
		t.Fatalf("failed to write file: %v", err)
	}
	err := DownloadCNIPlugins(context.Background(), dir)
	if err != nil {
		t.Errorf("unexpected error in package-level call: %v", err)
	}
}
