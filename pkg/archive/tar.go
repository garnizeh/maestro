package archive

import (
	"archive/tar"
	"compress/gzip"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"syscall"
)

const (
	// MaxDecompressionSize is the default limit for a single file in an archive
	// to prevent decompression bombs (100MB).
	MaxDecompressionSize = int64(100 * 1024 * 1024)
)

// WhiteoutFormat defines how OCI whiteouts should be handled.
type WhiteoutFormat int

const (
	// WhiteoutVFS handles whiteouts by deleting files (for full-copy drivers).
	WhiteoutVFS WhiteoutFormat = iota
	// WhiteoutOverlay handles whiteouts by creating 0:0 character devices.
	WhiteoutOverlay
)

// ExtractOptions configures the archive extraction.
type ExtractOptions struct {
	MaxFileSize int64
	// WhiteoutFormat defines the whiteout handling strategy.
	WhiteoutFormat WhiteoutFormat
}

// ExtractTarGz extracts a .tar.gz stream into targetDir.
func ExtractTarGz(r io.Reader, targetDir string, opts ExtractOptions) error {
	gzReader, err := gzip.NewReader(r)
	if err != nil {
		return fmt.Errorf("archive: create gzip reader: %w", err)
	}
	defer gzReader.Close()

	if opts.MaxFileSize == 0 {
		opts.MaxFileSize = MaxDecompressionSize
	}

	tarReader := tar.NewReader(gzReader)
	for {
		header, errTar := tarReader.Next()
		if errTar == io.EOF {
			break
		}
		if errTar != nil {
			return fmt.Errorf("archive: read tar: %w", errTar)
		}

		if errExt := extractFile(tarReader, header, targetDir, opts); errExt != nil {
			return errExt
		}
	}
	return nil
}

func extractFile(
	tarReader *tar.Reader,
	header *tar.Header,
	targetDir string,
	opts ExtractOptions,
) error {
	name := filepath.Clean(header.Name)
	targetPath := filepath.Join(targetDir, name)

	// Security: Prevent ZipSlip (path traversal outside targetDir)
	cleanTarget := filepath.Clean(targetDir)
	if targetPath != cleanTarget &&
		!strings.HasPrefix(targetPath, cleanTarget+string(os.PathSeparator)) {
		return fmt.Errorf("archive: invalid file path %s", targetPath)
	}

	// Handle OCI Whiteouts
	if strings.HasPrefix(filepath.Base(name), ".wh.") {
		return handleWhiteout(targetDir, name, opts.WhiteoutFormat)
	}

	switch header.Typeflag {
	case tar.TypeDir:
		return extractDir(targetPath, header)
	case tar.TypeReg:
		return extractReg(tarReader, targetPath, header, opts)
	case tar.TypeSymlink:
		return extractSymlink(targetPath, header)
	case tar.TypeLink:
		return extractLink(targetDir, targetPath, header)
	}

	return nil
}

func extractDir(targetPath string, header *tar.Header) error {
	mode := header.FileInfo().Mode() | 0o700 //nolint:mnd // permissions
	if err := mkdirAllFn(targetPath, mode); err != nil {
		return fmt.Errorf("archive: mkdir %s: %w", targetPath, err)
	}
	return nil
}

func extractReg(
	tarReader *tar.Reader,
	targetPath string,
	header *tar.Header,
	opts ExtractOptions,
) error {
	if err := mkdirAllFn(filepath.Dir(targetPath), 0o750); err != nil { //nolint:mnd // permissions for parent dir
		return fmt.Errorf("archive: mkdir parent for %s: %w", targetPath, err)
	}

	fFlags := os.O_CREATE | os.O_RDWR | os.O_TRUNC
	mode := header.FileInfo().Mode() | 0o600 //nolint:mnd // permissions
	f, errExt := openFileFn(targetPath, fFlags, mode)
	if errExt != nil {
		return fmt.Errorf("archive: open file %s: %w", targetPath, errExt)
	}

	n, errCopy := io.Copy(f, io.LimitReader(tarReader, opts.MaxFileSize+1))
	if closeErr := f.Close(); closeErr != nil && errCopy == nil {
		errCopy = closeErr
	}

	if errCopy != nil {
		return fmt.Errorf("archive: write file %s: %w", targetPath, errCopy)
	}
	if n > opts.MaxFileSize {
		return fmt.Errorf("archive: file %s exceeds max size %d", targetPath, opts.MaxFileSize)
	}
	return nil
}

func extractSymlink(targetPath string, header *tar.Header) error {
	if err := mkdirAllFn(filepath.Dir(targetPath), 0o750); err != nil { //nolint:mnd // permissions
		return fmt.Errorf("archive: mkdir parent for symlink %s: %w", targetPath, err)
	}
	if err := removeFn(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("archive: remove existing %s for symlink: %w", targetPath, err)
	}
	if err := symlinkFn(header.Linkname, targetPath); err != nil {
		return fmt.Errorf("archive: symlink %s -> %s: %w", targetPath, header.Linkname, err)
	}
	return nil
}

func extractLink(targetDir, targetPath string, header *tar.Header) error {
	targetLink := filepath.Join(targetDir, filepath.Clean(header.Linkname))
	if err := removeFn(targetPath); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("archive: remove existing %s for link: %w", targetPath, err)
	}
	if err := linkFn(targetLink, targetPath); err != nil {
		return fmt.Errorf("archive: link %s -> %s: %w", targetPath, targetLink, err)
	}
	return nil
}

func handleWhiteout(targetDir, name string, format WhiteoutFormat) error {
	base := filepath.Base(name)
	dir := filepath.Dir(name)

	if base == ".wh..wh..opq" {
		// Opaque whiteout: remove all siblings in this directory from lower layers.
		// For VFS, we empty the directory. For Overlay, we set the 'trusted.overlay.opaque' xattr.
		// For now, we only support VFS-style opaque whiteouts (emptying).
		return removeChildren(filepath.Join(targetDir, dir))
	}

	// Normal whiteout: remove or hide the specific sibling.
	targetName := strings.TrimPrefix(base, ".wh.")
	targetPath := filepath.Join(targetDir, dir, targetName)

	switch format {
	case WhiteoutVFS:
		if err := removeAllFn(targetPath); err != nil && !os.IsNotExist(err) {
			return fmt.Errorf("archive: vfs whiteout %s: %w", targetPath, err)
		}
	case WhiteoutOverlay:
		// Create a character device with 0/0 device numbers (OverlayFS whiteout marker).
		if err := mkdirAllFn(filepath.Dir(targetPath), 0o750); err != nil { //nolint:mnd // permissions
			return fmt.Errorf("archive: overlay whiteout mkdir: %w", err)
		}
		// Device number for 0,0 is 0.
		if err := mknod(targetPath, 0); err != nil {
			return fmt.Errorf("archive: overlay whiteout mknod %s: %w", targetPath, err)
		}
	}
	return nil
}

var (
	// Injectable for testing.
	mknodFn     = syscall.Mknod //nolint:gochecknoglobals // mock hook
	mkdirAllFn  = os.MkdirAll   //nolint:gochecknoglobals // mock hook
	openFileFn  = os.OpenFile   //nolint:gochecknoglobals // mock hook
	removeAllFn = os.RemoveAll  //nolint:gochecknoglobals // mock hook
	removeFn    = os.Remove     //nolint:gochecknoglobals // mock hook
	symlinkFn   = os.Symlink    //nolint:gochecknoglobals // mock hook
	linkFn      = os.Link       //nolint:gochecknoglobals // mock hook
	readDirFn   = os.ReadDir    //nolint:gochecknoglobals // mock hook
)

const whiteoutMode = 0o666

func removeChildren(dir string) error {
	entries, err := readDirFn(dir)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	for _, entry := range entries {
		if rmErr := removeAllFn(filepath.Join(dir, entry.Name())); rmErr != nil {
			return rmErr
		}
	}
	return nil
}

func mknod(path string, dev int) error {
	return mknodFn(path, syscall.S_IFCHR|whiteoutMode, dev)
}
