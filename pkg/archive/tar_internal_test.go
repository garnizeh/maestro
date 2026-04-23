package archive

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

type mockDirEntry struct {
	os.DirEntry

	name string
}

func (m *mockDirEntry) Name() string { return m.name }

func TestExtractTarGz_Success(t *testing.T) {
	tmpDir := t.TempDir()

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	files := []struct {
		Name     string
		Body     string
		Typeflag byte
		Linkname string
	}{
		{Name: "dir/", Typeflag: tar.TypeDir},
		{Name: "dir/file.txt", Body: "hello", Typeflag: tar.TypeReg},
		{Name: "dir/link.txt", Typeflag: tar.TypeSymlink, Linkname: "file.txt"},
		{Name: "dir/hardlink.txt", Typeflag: tar.TypeLink, Linkname: "dir/file.txt"},
	}

	for _, file := range files {
		hdr := &tar.Header{
			Name:     file.Name,
			Typeflag: file.Typeflag,
			Size:     int64(len(file.Body)),
			Linkname: file.Linkname,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if file.Body != "" {
			if _, err := tw.Write([]byte(file.Body)); err != nil {
				t.Fatal(err)
			}
		}
	}

	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, tmpDir, ExtractOptions{}); err != nil {
		t.Fatalf("ExtractTarGz failed: %v", err)
	}

	// Verify
	content, err := os.ReadFile(filepath.Join(tmpDir, "dir/file.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if string(content) != "hello" {
		t.Errorf("expected 'hello', got %q", string(content))
	}

	link, err := os.Readlink(filepath.Join(tmpDir, "dir/link.txt"))
	if err != nil {
		t.Fatal(err)
	}
	if link != "file.txt" {
		t.Errorf("expected 'file.txt', got %q", link)
	}
}

func TestExtractTarGz_Whiteouts(t *testing.T) {
	tmpDir := t.TempDir()

	// Pre-create some files to be whited out
	if err := os.MkdirAll(filepath.Join(tmpDir, "dir"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "dir/toremove.txt"), []byte("data"), 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(tmpDir, "opaque"), 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(tmpDir, "opaque/a.txt"), []byte("a"), 0644); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	whiteouts := []struct {
		Name     string
		Typeflag byte
	}{
		{Name: "dir/.wh.toremove.txt", Typeflag: tar.TypeReg},
		{Name: "opaque/.wh..wh..opq", Typeflag: tar.TypeReg},
	}

	for _, wh := range whiteouts {
		hdr := &tar.Header{
			Name:     wh.Name,
			Typeflag: wh.Typeflag,
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
	}

	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, tmpDir, ExtractOptions{WhiteoutFormat: WhiteoutVFS}); err != nil {
		t.Fatalf("ExtractTarGz failed: %v", err)
	}

	// Verify whiteouts
	if _, err := os.Stat(filepath.Join(tmpDir, "dir/toremove.txt")); !os.IsNotExist(err) {
		t.Error("expected dir/toremove.txt to be removed")
	}
	entries, err := os.ReadDir(filepath.Join(tmpDir, "opaque"))
	if err != nil {
		t.Fatalf("failed to read opaque directory: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("expected opaque directory to be empty, got %d entries", len(entries))
	}
}

func TestExtractTarGz_OverlayWhiteout(t *testing.T) {
	// Mock mknodFn
	oldMknod := mknodFn
	defer func() { mknodFn = oldMknod }()

	var capturedPath string
	mknodFn = func(path string, _ uint32, _ int) error {
		capturedPath = path
		return nil
	}

	tmpDir := t.TempDir()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name:     "dir/.wh.file",
		Typeflag: tar.TypeReg,
	}
	tw.WriteHeader(hdr)
	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, tmpDir, ExtractOptions{WhiteoutFormat: WhiteoutOverlay}); err != nil {
		t.Fatal(err)
	}

	if !strings.HasSuffix(capturedPath, "dir/file") {
		t.Errorf("expected mknod for dir/file, got %q", capturedPath)
	}
}

func TestExtractTarGz_ZipSlip(t *testing.T) {
	tmpDir := t.TempDir()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	hdr := &tar.Header{
		Name: "../outside.txt",
	}
	tw.WriteHeader(hdr)
	tw.Close()
	gw.Close()

	err := ExtractTarGz(&buf, tmpDir, ExtractOptions{})
	if err == nil || !strings.Contains(err.Error(), "invalid file path") {
		t.Errorf("expected ZipSlip error, got %v", err)
	}
}

func TestExtractTarGz_MaxFileSize(t *testing.T) {
	tmpDir := t.TempDir()
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)

	body := "too long"
	hdr := &tar.Header{
		Name:     "big.txt",
		Typeflag: tar.TypeReg,
		Size:     int64(len(body)),
	}
	tw.WriteHeader(hdr)
	tw.Write([]byte(body))
	tw.Close()
	gw.Close()

	err := ExtractTarGz(&buf, tmpDir, ExtractOptions{MaxFileSize: 3})
	if err == nil || !strings.Contains(err.Error(), "exceeds max size") {
		t.Errorf("expected max size error, got %v", err)
	}
}

func TestExtractTarGz_PanicOnGzip(t *testing.T) {
	// Trigger create gzip reader failure by passing non-gzip data
	err := ExtractTarGz(strings.NewReader("not-gzip"), t.TempDir(), ExtractOptions{})
	if err == nil {
		t.Fatal("expected error")
	}
}

func TestExtractTarGz_CopyError(t *testing.T) {
	// We need to trigger an error during io.Copy (after Header is read).
	// This can happen if the tar stream is truncated or has a checksum error.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	tw.WriteHeader(&tar.Header{Name: "file.txt", Typeflag: tar.TypeReg, Size: 10})
	// We don't write the content and close prematurely to cause a read error during copy.
	tw.Flush()
	gw.Close()

	err := ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
	if err == nil {
		t.Fatal("expected error during io.Copy")
	}
}

func TestExtractTarGz_MkdirErrors(t *testing.T) {
	t.Run("MkdirError", func(t *testing.T) {
		oldMkdirAll := mkdirAllFn
		defer func() { mkdirAllFn = oldMkdirAll }()
		mkdirAllFn = func(_ string, _ os.FileMode) error { return errors.New("mkdir fail") }

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(&tar.Header{Name: "dir/", Typeflag: tar.TypeDir})
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
		if err == nil || !strings.Contains(err.Error(), "mkdir fail") {
			t.Errorf("expected mkdir error, got %v", err)
		}
	})

	t.Run("MkdirParentRegError", func(t *testing.T) {
		oldMkdirAll := mkdirAllFn
		defer func() { mkdirAllFn = oldMkdirAll }()
		mkdirAllFn = func(_ string, _ os.FileMode) error { return errors.New("mkdir parent fail") }

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(&tar.Header{Name: "dir/file.txt", Typeflag: tar.TypeReg})
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
		if err == nil || !strings.Contains(err.Error(), "mkdir parent fail") {
			t.Errorf("expected mkdir parent error, got %v", err)
		}
	})

	t.Run("MkdirParentSymlinkError", func(t *testing.T) {
		oldMkdirAll := mkdirAllFn
		defer func() { mkdirAllFn = oldMkdirAll }()
		mkdirAllFn = func(_ string, _ os.FileMode) error { return errors.New("mkdir parent symlink fail") }

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(
			&tar.Header{Name: "dir/link", Typeflag: tar.TypeSymlink, Linkname: "target"},
		)
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
		if err == nil || !strings.Contains(err.Error(), "mkdir parent symlink fail") {
			t.Errorf("expected mkdir parent symlink error, got %v", err)
		}
	})

	t.Run("MkdirParentOverlayError", func(t *testing.T) {
		oldMkdirAll := mkdirAllFn
		defer func() { mkdirAllFn = oldMkdirAll }()
		mkdirAllFn = func(_ string, _ os.FileMode) error { return errors.New("mkdir overlay fail") }

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(&tar.Header{Name: "dir/.wh.file", Typeflag: tar.TypeReg})
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{WhiteoutFormat: WhiteoutOverlay})
		if err == nil || !strings.Contains(err.Error(), "mkdir overlay fail") {
			t.Errorf("expected mkdir overlay error, got %v", err)
		}
	})
}

func TestExtractTarGz_FileErrors(t *testing.T) {
	t.Run("OpenFileError", func(t *testing.T) {
		oldOpenFile := openFileFn
		defer func() { openFileFn = oldOpenFile }()
		openFileFn = func(_ string, _ int, _ os.FileMode) (*os.File, error) {
			return nil, errors.New("open fail")
		}

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(&tar.Header{Name: "file.txt", Typeflag: tar.TypeReg})
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
		if err == nil || !strings.Contains(err.Error(), "open fail") {
			t.Errorf("expected open error, got %v", err)
		}
	})
}

func TestExtractTarGz_LinkErrors(t *testing.T) {
	t.Run("SymlinkError", func(t *testing.T) {
		oldSymlink := symlinkFn
		defer func() { symlinkFn = oldSymlink }()
		symlinkFn = func(_, _ string) error { return errors.New("symlink fail") }

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(
			&tar.Header{Name: "link", Typeflag: tar.TypeSymlink, Linkname: "target"},
		)
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
		if err == nil || !strings.Contains(err.Error(), "symlink fail") {
			t.Errorf("expected symlink error, got %v", err)
		}
	})

	t.Run("HardlinkError", func(t *testing.T) {
		oldLink := linkFn
		defer func() { linkFn = oldLink }()
		linkFn = func(_, _ string) error { return errors.New("link fail") }

		var buf bytes.Buffer
		gw := gzip.NewWriter(&buf)
		tw := tar.NewWriter(gw)
		err := tw.WriteHeader(
			&tar.Header{Name: "hlink", Typeflag: tar.TypeLink, Linkname: "target"},
		)
		if err != nil {
			t.Fatalf("WriteHeader: %v", err)
		}
		tw.Close()
		gw.Close()

		err = ExtractTarGz(&buf, t.TempDir(), ExtractOptions{})
		if err == nil || !strings.Contains(err.Error(), "link fail") {
			t.Errorf("expected link error, got %v", err)
		}
	})
}

func TestExtractTarGz_WhiteoutReadDirError(t *testing.T) {
	oldReadDir := readDirFn
	defer func() { readDirFn = oldReadDir }()
	readDirFn = func(_ string) ([]os.DirEntry, error) { return nil, errors.New("readdir fail") }

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: ".wh..wh..opq", Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, t.TempDir(), ExtractOptions{}); err == nil ||
		!strings.Contains(err.Error(), "readdir fail") {
		t.Errorf("expected readdir error, got %v", err)
	}
}

func TestExtractTarGz_WhiteoutReadDirNotExists(t *testing.T) {
	oldReadDir := readDirFn
	defer func() { readDirFn = oldReadDir }()
	readDirFn = func(_ string) ([]os.DirEntry, error) { return nil, os.ErrNotExist }

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: ".wh..wh..opq", Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, t.TempDir(), ExtractOptions{}); err != nil {
		t.Errorf("expected no error for NotExist, got %v", err)
	}
}

func TestExtractTarGz_WhiteoutRemoveAllChildError(t *testing.T) {
	oldReadDir := readDirFn
	oldRemoveAll := removeAllFn
	defer func() {
		readDirFn = oldReadDir
		removeAllFn = oldRemoveAll
	}()

	readDirFn = func(_ string) ([]os.DirEntry, error) {
		return []os.DirEntry{&mockDirEntry{name: "child"}}, nil
	}
	removeAllFn = func(_ string) error {
		return errors.New("child rmall fail")
	}

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: ".wh..wh..opq", Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, t.TempDir(), ExtractOptions{}); err == nil ||
		!strings.Contains(err.Error(), "child rmall fail") {
		t.Errorf("expected child rmall error, got %v", err)
	}
}

func TestExtractTarGz_WhiteoutRemoveAllError(t *testing.T) {
	oldRemoveAll := removeAllFn
	defer func() { removeAllFn = oldRemoveAll }()
	removeAllFn = func(_ string) error { return errors.New("rmall fail") }

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: ".wh.file", Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, t.TempDir(), ExtractOptions{WhiteoutFormat: WhiteoutVFS}); err == nil ||
		!strings.Contains(err.Error(), "rmall fail") {
		t.Errorf("expected rmall error, got %v", err)
	}
}

func TestExtractTarGz_WhiteoutMknodError(t *testing.T) {
	oldMknod := mknodFn
	defer func() { mknodFn = oldMknod }()
	mknodFn = func(_ string, _ uint32, _ int) error { return errors.New("mknod fail") }

	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	if err := tw.WriteHeader(&tar.Header{Name: ".wh.file", Typeflag: tar.TypeReg}); err != nil {
		t.Fatalf("WriteHeader: %v", err)
	}
	tw.Close()
	gw.Close()

	if err := ExtractTarGz(&buf, t.TempDir(), ExtractOptions{WhiteoutFormat: WhiteoutOverlay}); err == nil ||
		!strings.Contains(err.Error(), "mknod fail") {
		t.Errorf("expected mknod error, got %v", err)
	}
}
