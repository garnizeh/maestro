package cli

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

// execRootForImage runs the root command for image tests and returns combined output.
func execRootForImage(args ...string) (string, error) {
	root := NewRootCommand()
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs(args)
	err := root.Execute()
	return buf.String(), err
}

// sampleSummaries returns a fixed set of image summaries for testing.
func sampleSummaries() []maturin.ImageSummary {
	return []maturin.ImageSummary{
		{
			Repository: "docker.io/library/nginx",
			Tag:        "latest",
			ShortID:    "aabbccddeeff",
			Created:    time.Date(2024, 1, 1, 0, 0, 0, 0, time.UTC),
			Size:       10 * 1024 * 1024,
		},
	}
}

// ── image ls ─────────────────────────────────────────────────────────────────

func TestImageLs_HelpFlag(t *testing.T) {
	out, err := execRootForImage("image", "ls", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "--format") {
		t.Errorf("expected --format in help, got: %s", out)
	}
}

func TestImageLs_Table(t *testing.T) {
	orig := imageLsFn
	imageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return sampleSummaries(), nil
	}
	t.Cleanup(func() { imageLsFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "ls")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nginx") {
		t.Errorf("expected 'nginx' in output, got: %s", out)
	}
	if !strings.Contains(out, "REPOSITORY") {
		t.Errorf("expected table header in output, got: %s", out)
	}
}

func TestImageLs_JSON(t *testing.T) {
	orig := imageLsFn
	imageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return sampleSummaries(), nil
	}
	t.Cleanup(func() { imageLsFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "ls", "--format", "json")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"Repository"`) {
		t.Errorf("expected JSON output, got: %s", out)
	}
}

func TestImageLs_Quiet(t *testing.T) {
	orig := imageLsFn
	imageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return sampleSummaries(), nil
	}
	t.Cleanup(func() { imageLsFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("--quiet", "image", "ls")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "aabbccddeeff") {
		t.Errorf("expected ShortID in quiet output, got: %s", out)
	}
	if strings.Contains(out, "REPOSITORY") {
		t.Errorf("quiet should not show table header, got: %s", out)
	}
}

func TestImageLs_Empty(t *testing.T) {
	orig := imageLsFn
	imageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return nil, nil
	}
	t.Cleanup(func() { imageLsFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "ls")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "REPOSITORY") {
		t.Errorf("expected empty table header, got: %s", out)
	}
}

func TestImageLs_Error(t *testing.T) {
	orig := imageLsFn
	imageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return nil, errors.New("store error")
	}
	t.Cleanup(func() { imageLsFn = orig; globalFlags = GlobalFlags{} })

	_, err := execRootForImage("image", "ls")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

func TestImagesShortcut_Table(t *testing.T) {
	orig := imageLsFn
	imageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return sampleSummaries(), nil
	}
	t.Cleanup(func() { imageLsFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("images")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "nginx") {
		t.Errorf("expected 'nginx' in images shortcut output, got: %s", out)
	}
}

// ── image inspect ─────────────────────────────────────────────────────────────

func TestImageInspect_HelpFlag(t *testing.T) {
	out, err := execRootForImage("image", "inspect", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "IMAGE") {
		t.Errorf("expected IMAGE in help, got: %s", out)
	}
}

func TestImageInspect_MissingArg(t *testing.T) {
	_, err := execRootForImage("image", "inspect")
	if err == nil {
		t.Fatal("expected error for missing IMAGE argument")
	}
}

func TestImageInspect_Success(t *testing.T) {
	orig := imageInspectFn
	imageInspectFn = func(_ string, refStr string) (*maturin.InspectResult, error) {
		return &maturin.InspectResult{
			Ref:     refStr,
			ID:      "aabbccddeeff",
			RepoTag: refStr,
		}, nil
	}
	t.Cleanup(func() { imageInspectFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "inspect", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "aabbccddeeff") {
		t.Errorf("expected ID in output, got: %s", out)
	}
}

func TestImageInspect_Error(t *testing.T) {
	orig := imageInspectFn
	imageInspectFn = func(_ string, _ string) (*maturin.InspectResult, error) {
		return nil, errors.New("image not found")
	}
	t.Cleanup(func() { imageInspectFn = orig; globalFlags = GlobalFlags{} })

	_, err := execRootForImage("image", "inspect", "nginx:latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── image history ─────────────────────────────────────────────────────────────

func TestImageHistory_HelpFlag(t *testing.T) {
	out, err := execRootForImage("image", "history", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "IMAGE") {
		t.Errorf("expected IMAGE in help, got: %s", out)
	}
}

func TestImageHistory_MissingArg(t *testing.T) {
	_, err := execRootForImage("image", "history")
	if err == nil {
		t.Fatal("expected error for missing IMAGE argument")
	}
}

func TestImageHistory_Table(t *testing.T) {
	orig := imageHistoryFn
	imageHistoryFn = func(_ string, _ string) ([]maturin.HistoryEntry, error) {
		return []maturin.HistoryEntry{
			{
				Created:   time.Now().Add(-24 * time.Hour),
				CreatedBy: "/bin/sh -c apt-get install -y nginx",
				Size:      5 * 1024 * 1024,
			},
		}, nil
	}
	t.Cleanup(func() { imageHistoryFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "history", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "CREATED") {
		t.Errorf("expected table header in output, got: %s", out)
	}
	if !strings.Contains(out, "nginx") {
		t.Errorf("expected 'nginx' in output, got: %s", out)
	}
}

func TestImageHistory_JSON(t *testing.T) {
	orig := imageHistoryFn
	imageHistoryFn = func(_ string, _ string) ([]maturin.HistoryEntry, error) {
		return []maturin.HistoryEntry{{CreatedBy: "test"}}, nil
	}
	t.Cleanup(func() { imageHistoryFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "history", "--format", "json", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, `"CreatedBy"`) {
		t.Errorf("expected JSON output, got: %s", out)
	}
}

func TestImageHistory_LongCreatedBy_Truncated(t *testing.T) {
	orig := imageHistoryFn
	imageHistoryFn = func(_ string, _ string) ([]maturin.HistoryEntry, error) {
		return []maturin.HistoryEntry{
			{CreatedBy: strings.Repeat("x", 80)},
		}, nil
	}
	t.Cleanup(func() { imageHistoryFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "history", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "...") {
		t.Errorf("expected truncation '...' for long CreatedBy, got: %s", out)
	}
}

func TestImageHistory_Error(t *testing.T) {
	orig := imageHistoryFn
	imageHistoryFn = func(_ string, _ string) ([]maturin.HistoryEntry, error) {
		return nil, errors.New("image not found")
	}
	t.Cleanup(func() { imageHistoryFn = orig; globalFlags = GlobalFlags{} })

	_, err := execRootForImage("image", "history", "nginx:latest")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
}

// ── image rm ──────────────────────────────────────────────────────────────────

func TestImageRm_HelpFlag(t *testing.T) {
	out, err := execRootForImage("image", "rm", "--help")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "IMAGE") {
		t.Errorf("expected IMAGE in help, got: %s", out)
	}
}

func TestImageRm_MissingArg(t *testing.T) {
	_, err := execRootForImage("image", "rm")
	if err == nil {
		t.Fatal("expected error for missing IMAGE argument")
	}
}

func TestImageRm_Success(t *testing.T) {
	orig := imageRmFn
	imageRmFn = func(_ context.Context, _, _ string) error { return nil }
	t.Cleanup(func() { imageRmFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("image", "rm", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(out, "Deleted") {
		t.Errorf("expected 'Deleted' in output, got: %s", out)
	}
}

func TestImageRm_Quiet(t *testing.T) {
	orig := imageRmFn
	imageRmFn = func(_ context.Context, _, _ string) error { return nil }
	t.Cleanup(func() { imageRmFn = orig; globalFlags = GlobalFlags{} })

	out, err := execRootForImage("--quiet", "image", "rm", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if strings.Contains(out, "Deleted") {
		t.Errorf("quiet should suppress 'Deleted', got: %s", out)
	}
}

func TestImageRm_MultipleRefs(t *testing.T) {
	orig := imageRmFn
	var removed []string
	imageRmFn = func(_ context.Context, _, ref string) error {
		removed = append(removed, ref)
		return nil
	}
	t.Cleanup(func() { imageRmFn = orig; globalFlags = GlobalFlags{} })

	_, err := execRootForImage("image", "rm", "nginx:latest", "nginx:1.25")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(removed) != 2 {
		t.Errorf("expected 2 removals, got %d", len(removed))
	}
}

func TestImageRm_PartialError(t *testing.T) {
	orig := imageRmFn
	imageRmFn = func(_ context.Context, _, ref string) error {
		if ref == "bad:tag" {
			return errors.New("not found")
		}
		return nil
	}
	t.Cleanup(func() { imageRmFn = orig; globalFlags = GlobalFlags{} })

	_, err := execRootForImage("image", "rm", "nginx:latest", "bad:tag")
	if err == nil {
		t.Fatal("expected error from partial failure, got nil")
	}
}

func TestImageRm_ForceFlag(t *testing.T) {
	orig := imageRmFn
	imageRmFn = func(_ context.Context, _, _ string) error { return nil }
	t.Cleanup(func() { imageRmFn = orig; globalFlags = GlobalFlags{} })

	_, err := execRootForImage("image", "rm", "--force", "nginx:latest")
	if err != nil {
		t.Fatalf("unexpected error with --force: %v", err)
	}
}

// ── formatAge ─────────────────────────────────────────────────────────────────

func TestFormatAge_Zero(t *testing.T) {
	t.Parallel()
	if got := formatAge(time.Time{}); got != "N/A" {
		t.Errorf("formatAge(zero) = %q, want N/A", got)
	}
}

func TestFormatAge_LessThanMinute(t *testing.T) {
	t.Parallel()
	got := formatAge(time.Now().Add(-5 * time.Second))
	if !strings.Contains(got, "second") {
		t.Errorf("formatAge(5s ago) = %q, expected 'second'", got)
	}
}

func TestFormatAge_Minutes(t *testing.T) {
	t.Parallel()
	got := formatAge(time.Now().Add(-30 * time.Minute))
	if !strings.Contains(got, "minutes") {
		t.Errorf("formatAge(30m ago) = %q, expected 'minutes'", got)
	}
}

func TestFormatAge_Hours(t *testing.T) {
	t.Parallel()
	got := formatAge(time.Now().Add(-5 * time.Hour))
	if !strings.Contains(got, "hours") {
		t.Errorf("formatAge(5h ago) = %q, expected 'hours'", got)
	}
}

func TestFormatAge_Days(t *testing.T) {
	t.Parallel()
	got := formatAge(time.Now().Add(-48 * time.Hour))
	if !strings.Contains(got, "days") {
		t.Errorf("formatAge(48h ago) = %q, expected 'days'", got)
	}
}
