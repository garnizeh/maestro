package cli_test

import (
	"bytes"
	"strings"
	"testing"

	"github.com/garnizeh/maestro/internal/cli"
)

func TestVersionCmd(t *testing.T) {
	h := cli.NewHandler()
	root := cli.NewRootCommand(h)
	buf := new(bytes.Buffer)
	root.SetOut(buf)

	t.Run("TableFormat", func(t *testing.T) {
		buf.Reset()
		root.SetArgs([]string{"version"})
		if err := root.Execute(); err != nil {
			t.Fatalf("version table: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "Version") || !strings.Contains(out, "garnizeH labs") {
			t.Errorf("version table output missing info: %s", out)
		}
	})

	t.Run("JSONFormat", func(t *testing.T) {
		buf.Reset()
		h.Format = "json"
		root.SetArgs([]string{"version"})
		if err := root.Execute(); err != nil {
			t.Fatalf("version json: %v", err)
		}
		out := buf.String()
		if !strings.Contains(out, "\"version\"") {
			t.Errorf("version json output missing info: %s", out)
		}
	})
}
