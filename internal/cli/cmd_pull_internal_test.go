package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// execRootForPull runs the root command for pull tests and returns stdout+stderr.
func execRootForPull(args ...string) (string, error) {
	root := NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

func TestPullCmd_MissingArg(t *testing.T) {
	_, err := execRootForPull("pull")
	if err == nil {
		t.Fatal("expected error for missing image argument")
	}
}

func TestPullCmd_HelpFlag(t *testing.T) {
	out, err := execRootForPull("pull", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "--platform") {
		t.Errorf("help output missing --platform flag, got: %s", out)
	}
}

func TestPullCmd_Success(t *testing.T) {
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, _, _ string, _ maturin.DrawOptions) error { return nil }
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	out, err := execRootForPull("pull", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Pull complete") {
		t.Errorf("expected 'Pull complete' in output, got: %s", out)
	}
}

func TestPullCmd_Success_Quiet(t *testing.T) {
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, _, _ string, _ maturin.DrawOptions) error { return nil }
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	out, err := execRootForPull("--quiet", "pull", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Pull complete") {
		t.Errorf("quiet mode should suppress 'Pull complete', got: %s", out)
	}
}

func TestPullCmd_DrawError(t *testing.T) {
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, _, _ string, _ maturin.DrawOptions) error {
		return errors.New("registry down")
	}
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	_, err := execRootForPull("pull", "nginx:latest")
	if err == nil {
		t.Fatal("expected draw error, got nil")
	}
}

func TestPullCmd_WithExplicitRoot(t *testing.T) {
	var capturedRoot string
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, root, _ string, _ maturin.DrawOptions) error {
		capturedRoot = root
		return nil
	}
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	if _, err := execRootForPull("--root", "/tmp/maestro-test", "pull", "nginx:latest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedRoot != "/tmp/maestro-test" {
		t.Errorf("root = %q, want /tmp/maestro-test", capturedRoot)
	}
}

func TestPullCmd_WithPlatformFlag(t *testing.T) {
	var capturedPlatform string
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, _, _ string, opts maturin.DrawOptions) error {
		capturedPlatform = opts.Platform
		return nil
	}
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	if _, err := execRootForPull("pull", "--platform", "linux/arm64", "nginx:latest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if capturedPlatform != "linux/arm64" {
		t.Errorf("platform = %q, want linux/arm64", capturedPlatform)
	}
}

func TestPullCmd_DefaultRoot_UsesHomeDir(t *testing.T) {
	// When no --root is specified, root is computed from os.UserHomeDir().
	var capturedRoot string
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, root, _ string, _ maturin.DrawOptions) error {
		capturedRoot = root
		return nil
	}
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	if _, err := execRootForPull("pull", "nginx:latest"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.HasSuffix(capturedRoot, ".local/share/maestro") {
		t.Errorf("default root = %q, want path ending with .local/share/maestro", capturedRoot)
	}
}

func TestPullCmd_ProgressWrittenToOutput(t *testing.T) {
	orig := pullDrawFn
	pullDrawFn = func(_ context.Context, _, _ string, opts maturin.DrawOptions) error {
		if opts.OnLayerDone != nil {
			opts.OnLayerDone(maturin.LayerEvent{Digest: "abc123456789", Skipped: false, Size: 4096})
		}
		return nil
	}
	t.Cleanup(func() {
		pullDrawFn = orig
		globalFlags = GlobalFlags{}
	})

	out, err := execRootForPull("pull", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "abc123456789") {
		t.Errorf("expected layer digest in output, got: %s", out)
	}
}
