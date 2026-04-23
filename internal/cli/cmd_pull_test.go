package cli_test

import (
	"bytes"
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
	"github.com/rodrigo-baliza/maestro/internal/maturin"
)

func TestImageCmd_Pull(t *testing.T) {
	h := cli.NewHandler()
	pulled := ""
	h.PullDrawFn = func(_ context.Context, _, ref string, _ maturin.DrawOptions) error {
		pulled = ref
		if ref == "fail" {
			return errors.New("mock-pull-fail")
		}
		return nil
	}

	root := cli.NewRootCommand(h)

	t.Run("Success", func(t *testing.T) {
		buf := new(bytes.Buffer)
		root.SetOut(buf)
		root.SetArgs([]string{"image", "pull", "nginx:latest"})
		if err := root.Execute(); err != nil {
			t.Fatalf("pull nginx:latest: %v", err)
		}
		if pulled != "nginx:latest" {
			t.Errorf("expected pulled='nginx:latest', got %q", pulled)
		}
	})

	t.Run("Failure", func(t *testing.T) {
		root.SilenceErrors = true
		root.SetArgs([]string{"image", "pull", "fail"})
		if err := root.Execute(); err == nil || !strings.Contains(err.Error(), "mock-pull-fail") {
			t.Fatalf("expected mock-pull-fail error, got: %v", err)
		}
	})
}
