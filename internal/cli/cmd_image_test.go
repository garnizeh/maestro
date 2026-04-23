package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"
	"time"

	"github.com/garnizeh/maestro/internal/cli"
	"github.com/garnizeh/maestro/internal/maturin"
)

func TestImageCmd_Ls(t *testing.T) {
	h := cli.NewHandler()
	h.ImageLsFn = func(_ context.Context, _ string) ([]maturin.ImageSummary, error) {
		return []maturin.ImageSummary{
			{
				Repository: "nginx",
				Tag:        "latest",
				Size:       1024 * 1024 * 50,
				Created:    time.Now().UTC().Add(-24 * time.Hour),
			},
		}, nil
	}

	root := cli.NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"image", "ls"})

	if err := root.Execute(); err != nil {
		t.Fatalf("image ls: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "nginx") || !strings.Contains(out, "latest") {
		t.Errorf("image ls output missing image details: %s", out)
	}
}

func TestImageCmd_Inspect(t *testing.T) {
	t.Run("Success", func(t *testing.T) {
		h := cli.NewHandler()
		h.ImageInspectFn = func(_, _ string) (*maturin.InspectResult, error) {
			return &maturin.InspectResult{
				ID: "sha256:123",
			}, nil
		}
		root := cli.NewRootCommand(h)
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetArgs([]string{"image", "inspect", "nginx"})
		if err := root.Execute(); err != nil {
			t.Fatalf("image inspect: %v", err)
		}
		if !strings.Contains(buf.String(), "sha256:123") {
			t.Errorf("inspect output missing ID: %s", buf.String())
		}
	})

	t.Run("Failure", func(t *testing.T) {
		h := cli.NewHandler()
		h.ImageInspectFn = func(_, _ string) (*maturin.InspectResult, error) {
			return nil, errors.New("not found")
		}
		root := cli.NewRootCommand(h)
		root.SilenceErrors = true
		root.SilenceUsage = true
		root.SetArgs([]string{"image", "inspect", "nonexistent"})
		if err := root.Execute(); err == nil {
			t.Fatal("expected error for nonexistent image")
		}
	})
}

func TestImageCmd_Rm(t *testing.T) {
	h := cli.NewHandler()
	removed := ""
	h.ImageRmFn = func(_ context.Context, _, ref string) error {
		removed = ref
		return nil
	}

	root := cli.NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)
	root.SetArgs([]string{"image", "rm", "nginx:latest"})

	if err := root.Execute(); err != nil {
		t.Fatalf("image rm: %v", err)
	}

	if removed != "nginx:latest" {
		t.Errorf("expected removed='nginx:latest', got %q", removed)
	}
}
