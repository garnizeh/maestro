package specgen_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	imagespec "github.com/opencontainers/image-spec/specs-go/v1"

	"github.com/rodrigo-baliza/maestro/pkg/specgen"
)

// ── helpers ───────────────────────────────────────────────────────────────────

func baseOpts(t *testing.T) specgen.Opts {
	t.Helper()
	return specgen.Opts{
		RootFS:      t.TempDir() + "/rootfs",
		ContainerID: "abc123def456",
		Rootless:    false,
	}
}

func imageConfig(cmd []string, entrypoint []string, env []string) imagespec.ImageConfig {
	return imagespec.ImageConfig{
		Cmd:        cmd,
		Entrypoint: entrypoint,
		Env:        env,
	}
}

// ── Generate tests ────────────────────────────────────────────────────────────

func TestGenerate_MinimalConfig(t *testing.T) {
	opts := baseOpts(t)
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec == nil {
		t.Fatal("expected non-nil spec")
	}
	if spec.OCIVersion != "1.0.2" {
		t.Errorf("OCIVersion = %q; want 1.0.2", spec.OCIVersion)
	}
}

func TestGenerate_ImageCmd(t *testing.T) {
	opts := baseOpts(t)
	imgCfg := imageConfig([]string{"nginx", "-g", "daemon off;"}, nil, nil)
	spec, err := specgen.Generate(imgCfg, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if len(spec.Process.Args) != 3 {
		t.Errorf("Args = %v; want 3 elements", spec.Process.Args)
	}
}

func TestGenerate_OptsOverrideCmd(t *testing.T) {
	opts := baseOpts(t)
	opts.Cmd = []string{"echo", "hello"}
	imgCfg := imageConfig([]string{"nginx"}, nil, nil)
	spec, err := specgen.Generate(imgCfg, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Process.Args[0] != "echo" {
		t.Errorf("Args[0] = %q; want echo", spec.Process.Args[0])
	}
}

func TestGenerate_OptsEntrypoint(t *testing.T) {
	opts := baseOpts(t)
	opts.Entrypoint = []string{"/custom-entrypoint.sh"}
	imgCfg := imageConfig([]string{"nginx"}, []string{"/docker-entrypoint.sh"}, nil)
	spec, err := specgen.Generate(imgCfg, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Process.Args[0] != "/custom-entrypoint.sh" {
		t.Errorf("Args[0] = %q; want /custom-entrypoint.sh", spec.Process.Args[0])
	}
}

func TestGenerate_EnvMerge(t *testing.T) {
	opts := baseOpts(t)
	opts.Env = []string{"MY_VAR=custom", "NEW_VAR=added"}
	imgCfg := imageConfig(nil, nil, []string{"MY_VAR=image", "BASE_VAR=base"})
	spec, err := specgen.Generate(imgCfg, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}

	envMap := make(map[string]string)
	for _, e := range spec.Process.Env {
		k, v, _ := strings.Cut(e, "=")
		envMap[k] = v
	}
	if envMap["MY_VAR"] != "custom" {
		t.Errorf("MY_VAR = %q; want custom (opts should override image)", envMap["MY_VAR"])
	}
	if envMap["BASE_VAR"] != "base" {
		t.Errorf("BASE_VAR = %q; want base", envMap["BASE_VAR"])
	}
	if envMap["NEW_VAR"] != "added" {
		t.Errorf("NEW_VAR = %q; want added", envMap["NEW_VAR"])
	}
}

func TestGenerate_WorkDir_FromOpts(t *testing.T) {
	opts := baseOpts(t)
	opts.WorkDir = "/custom/workdir"
	spec, err := specgen.Generate(imagespec.ImageConfig{WorkingDir: "/image/workdir"}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Process.Cwd != "/custom/workdir" {
		t.Errorf("Cwd = %q; want /custom/workdir", spec.Process.Cwd)
	}
}

func TestGenerate_WorkDir_FromImage(t *testing.T) {
	opts := baseOpts(t)
	spec, err := specgen.Generate(imagespec.ImageConfig{WorkingDir: "/app"}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Process.Cwd != "/app" {
		t.Errorf("Cwd = %q; want /app", spec.Process.Cwd)
	}
}

func TestGenerate_WorkDir_DefaultsToRoot(t *testing.T) {
	opts := baseOpts(t)
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Process.Cwd != "/" {
		t.Errorf("Cwd = %q; want /", spec.Process.Cwd)
	}
}

func TestGenerate_Hostname_FromOpts(t *testing.T) {
	opts := baseOpts(t)
	opts.Hostname = "my-container"
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Hostname != "my-container" {
		t.Errorf("Hostname = %q; want my-container", spec.Hostname)
	}
}

func TestGenerate_Hostname_FromContainerID(t *testing.T) {
	opts := baseOpts(t)
	opts.ContainerID = "abc123def456789"
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	// Should be truncated to 12 chars.
	if spec.Hostname != "abc123def456" {
		t.Errorf("Hostname = %q; want abc123def456", spec.Hostname)
	}
}

func TestGenerate_Hostname_ShortContainerID(t *testing.T) {
	opts := baseOpts(t)
	opts.ContainerID = "short"
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Hostname != "short" {
		t.Errorf("Hostname = %q; want short", spec.Hostname)
	}
}

func TestGenerate_ReadOnly(t *testing.T) {
	opts := baseOpts(t)
	opts.ReadOnly = true
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if !spec.Root.Readonly {
		t.Error("expected Root.Readonly = true")
	}
}

func TestGenerate_RootFSDefault(t *testing.T) {
	opts := baseOpts(t)
	opts.RootFS = ""
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	if spec.Root.Path != "rootfs" {
		t.Errorf("Root.Path = %q; want rootfs", spec.Root.Path)
	}
}

// ── Capabilities tests ────────────────────────────────────────────────────────

func TestGenerate_CapAdd(t *testing.T) {
	opts := baseOpts(t)
	opts.CapAdd = []string{"SYS_PTRACE"}
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, c := range spec.Process.Capabilities.Bounding {
		if c == "CAP_SYS_PTRACE" {
			found = true
		}
	}
	if !found {
		t.Errorf("CAP_SYS_PTRACE not found in capabilities: %v", spec.Process.Capabilities.Bounding)
	}
}

func TestGenerate_CapDrop(t *testing.T) {
	opts := baseOpts(t)
	opts.CapDrop = []string{"CAP_CHOWN"}
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, c := range spec.Process.Capabilities.Bounding {
		if c == "CAP_CHOWN" {
			t.Error("CAP_CHOWN should have been dropped")
		}
	}
}

// ── Namespace tests ───────────────────────────────────────────────────────────

func TestGenerate_Rootless_UserNamespace(t *testing.T) {
	opts := baseOpts(t)
	opts.Rootless = true
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == "user" {
			found = true
		}
	}
	if !found {
		t.Error("expected user namespace in rootless mode")
	}
	if len(spec.Linux.UIDMappings) == 0 {
		t.Error("expected UID mappings in rootless mode")
	}
}

func TestGenerate_NetworkNone(t *testing.T) {
	opts := baseOpts(t)
	opts.NetworkMode = "none"
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == "network" {
			found = true
		}
	}
	if !found {
		t.Error("expected network namespace for mode=none")
	}
}

func TestGenerate_NetworkHost(t *testing.T) {
	opts := baseOpts(t)
	opts.NetworkMode = "host"
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == "network" {
			t.Error("network namespace should not be present for host network mode")
		}
	}
}

func TestGenerate_NetworkPrivate(t *testing.T) {
	opts := baseOpts(t)
	opts.NetworkMode = "private"
	spec, err := specgen.Generate(imagespec.ImageConfig{}, opts)
	if err != nil {
		t.Fatalf("Generate: %v", err)
	}
	found := false
	for _, ns := range spec.Linux.Namespaces {
		if ns.Type == "network" {
			found = true
		}
	}
	if !found {
		t.Error("expected network namespace for mode=private")
	}
}

// ── Write tests ───────────────────────────────────────────────────────────────

func TestWrite_Success(t *testing.T) {
	dir := t.TempDir()
	sp, _ := specgen.Generate(imagespec.ImageConfig{}, baseOpts(t))

	if err := specgen.Write(dir, sp); err != nil {
		t.Fatalf("Write: %v", err)
	}

	data, readErr := os.ReadFile(filepath.Join(dir, "config.json"))
	if readErr != nil {
		t.Fatalf("read config.json: %v", readErr)
	}

	var parsed specgen.Spec
	if jsonErr := json.Unmarshal(data, &parsed); jsonErr != nil {
		t.Fatalf("parse config.json: %v", jsonErr)
	}
	if parsed.OCIVersion != "1.0.2" {
		t.Errorf("OCIVersion = %q; want 1.0.2", parsed.OCIVersion)
	}
}

func TestWrite_InvalidDir(t *testing.T) {
	sp, _ := specgen.Generate(imagespec.ImageConfig{}, baseOpts(t))
	err := specgen.Write("/nonexistent/path/that/does/not/exist", sp)
	if err == nil {
		t.Fatal("expected error for invalid dir")
	}
}
