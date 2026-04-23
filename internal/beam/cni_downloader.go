package beam

import (
	"context"
	"fmt"
	"net/http"
	"path/filepath"
	"runtime"

	"github.com/rs/zerolog/log"

	"github.com/rodrigo-baliza/maestro/pkg/archive"
)

type realHTTPClient struct{}

func (realHTTPClient) Do(req *http.Request) (*http.Response, error) {
	//nolint:gosec // G107: Potential SSRF via variable url
	return http.DefaultClient.Do(req)
}

// CNIDownloader manages the downloading and extraction of CNI plugins.
type CNIDownloader struct {
	fs      FS
	http    HTTPClient
	extract Extractor
}

// NewCNIDownloader creates a new CNI plugin downloader.
func NewCNIDownloader() *CNIDownloader {
	return &CNIDownloader{
		fs:      RealFS{},
		http:    realHTTPClient{},
		extract: realExtractor{},
	}
}

// WithFS sets a custom filesystem implementation.
func (d *CNIDownloader) WithFS(fs FS) *CNIDownloader {
	d.fs = fs
	return d
}

// WithHTTPClient sets a custom HTTP client implementation.
func (d *CNIDownloader) WithHTTPClient(c HTTPClient) *CNIDownloader {
	d.http = c
	return d
}

// WithExtractor sets a custom archive extractor implementation.
func (d *CNIDownloader) WithExtractor(e Extractor) *CNIDownloader {
	d.extract = e
	return d
}

// defaultDownloader is a global singleton for convenience.
//
//nolint:gochecknoglobals // singleton
var defaultDownloader = NewCNIDownloader()

const (
	CNIPluginsVersion = "v1.6.0"
)

func getCNIPluginsURL() string {
	return fmt.Sprintf(
		"https://github.com/containernetworking/plugins/releases/download/%s/cni-plugins-%s-%s-%s.tgz",
		CNIPluginsVersion,
		runtime.GOOS,
		runtime.GOARCH,
		CNIPluginsVersion,
	)
}

// DownloadCNIPlugins downloads and extracts standard CNI plugins to targetDir
// if they are not already present.
func DownloadCNIPlugins(ctx context.Context, targetDir string) error {
	return defaultDownloader.DownloadCNIPlugins(ctx, targetDir)
}

// DownloadCNIPlugins downloads and extracts standard CNI plugins to targetDir
// if they are not already present.
func (d *CNIDownloader) DownloadCNIPlugins(ctx context.Context, targetDir string) error {
	// Check if bridge exists, assume others exist too
	if _, err := d.fs.Stat(filepath.Join(targetDir, "bridge")); err == nil {
		log.Debug().Str("targetDir", targetDir).Msg("beam: cni plugins already present")
		return nil
	}

	if err := d.fs.MkdirAll(targetDir, dirPerm); err != nil {
		return fmt.Errorf("failed to create CNI plugin directory %s: %w", targetDir, err)
	}

	return d.downloadCNIPluginsFromURL(ctx, getCNIPluginsURL(), targetDir)
}

// downloadCNIPluginsFromURL fetches and extracts a CNI plugin archive from the given URL.
func (d *CNIDownloader) downloadCNIPluginsFromURL(
	ctx context.Context,
	url, targetDir string,
) error {
	log.Debug().Str("url", url).Str("targetDir", targetDir).Msg("beam: downloading cni plugins")
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return fmt.Errorf("failed to create request for CNI plugins: %w", err)
	}

	resp, err := d.http.Do(req)
	if err != nil {
		return fmt.Errorf("failed to download CNI plugins from %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("unexpected status code %d when downloading CNI plugins", resp.StatusCode)
	}

	if extractErr := d.extract.Extract(resp.Body, targetDir, archive.ExtractOptions{
		MaxFileSize:    maxDecompressionSize,
		WhiteoutFormat: archive.WhiteoutVFS,
	}); extractErr != nil {
		return extractErr
	}

	log.Debug().Str("targetDir", targetDir).Msg("beam: cni plugins installed successfully")
	return nil
}

const maxDecompressionSize = int64(100 * 1024 * 1024)
