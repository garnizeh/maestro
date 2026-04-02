package cli_test

import (
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
)

func TestGetBuildInfo(t *testing.T) {
	info := cli.GetBuildInfo()

	if info.GoVersion == "" {
		t.Error("GoVersion should not be empty")
	}
	if info.OS == "" {
		t.Error("OS should not be empty")
	}
	if info.Arch == "" {
		t.Error("Arch should not be empty")
	}
}
