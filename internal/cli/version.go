package cli

import "runtime"

// Build-time variables injected via ldflags.
//
//nolint:gochecknoglobals // ldflags injection: set at link time, read-only at runtime
var (
	Version   = "dev"
	Commit    = "unknown"
	BuildDate = "unknown"
	GoVersion = runtime.Version()
)

// BuildInfo holds all version metadata.
type BuildInfo struct {
	Version   string `json:"version"   yaml:"version"`
	Commit    string `json:"commit"    yaml:"commit"`
	BuildDate string `json:"buildDate" yaml:"buildDate"`
	GoVersion string `json:"goVersion" yaml:"goVersion"`
	OS        string `json:"os"        yaml:"os"`
	Arch      string `json:"arch"      yaml:"arch"`
}

// GetBuildInfo returns the current build metadata.
func GetBuildInfo() BuildInfo {
	return BuildInfo{
		Version:   Version,
		Commit:    Commit,
		BuildDate: BuildDate,
		GoVersion: GoVersion,
		OS:        runtime.GOOS,
		Arch:      runtime.GOARCH,
	}
}
