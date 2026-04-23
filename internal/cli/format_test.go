package cli_test

import (
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
)

func TestFormatter(t *testing.T) {
	data := struct {
		ID   string
		Name string
	}{
		ID:   "123",
		Name: "test",
	}

	testFormatterJSON(t, data)
	testFormatterYAML(t, data)
	testFormatterTemplate(t, data)
	testFormatterErrors(t, data)
	testFormatterQuiet(t, data)
}

func testFormatterJSON(t *testing.T, data any) {
	t.Run("JSON", func(t *testing.T) {
		f := cli.NewFormatter("json", false)
		out, err := f.Format(data)
		if err != nil {
			t.Fatalf("JSON format: %v", err)
		}
		if !strings.Contains(out, "\"ID\": \"123\"") || !strings.Contains(out, "\"Name\": \"test\"") {
			t.Errorf("unexpected JSON output: %s", out)
		}
	})
}

func testFormatterYAML(t *testing.T, data any) {
	t.Run("YAML", func(t *testing.T) {
		f := cli.NewFormatter("yaml", false)
		out, err := f.Format(data)
		if err != nil {
			t.Fatalf("YAML format: %v", err)
		}
		if !strings.Contains(out, "id: \"123\"") || !strings.Contains(out, "name: test") {
			t.Errorf("unexpected YAML output: %s", out)
		}
	})
}

func testFormatterTemplate(t *testing.T, data any) {
	t.Run("Template", func(t *testing.T) {
		f := cli.NewFormatter("{{.ID}}-{{.Name}}", false)
		out, err := f.Format(data)
		if err != nil {
			t.Fatalf("Template format: %v", err)
		}
		if out != "123-test" {
			t.Errorf("expected 123-test, got %q", out)
		}
	})
}

func testFormatterErrors(t *testing.T, data any) {
	t.Run("TemplateError", func(t *testing.T) {
		f := cli.NewFormatter("{{.NoField}}", false)
		_, err := f.Format(data)
		if err == nil {
			t.Fatal("expected error for invalid field")
		}
	})

	t.Run("InvalidTemplate", func(t *testing.T) {
		f := cli.NewFormatter("{{", false)
		_, err := f.Format(data)
		if err == nil {
			t.Fatal("expected error for malformed template")
		}
	})
}

func testFormatterQuiet(t *testing.T, data any) {
	t.Run("Quiet", func(t *testing.T) {
		f := cli.NewFormatter("json", true).WithQuietFn(func(v any) string {
			return v.(struct {
				ID   string
				Name string
			}).ID
		})
		out, err := f.Format(data)
		if err != nil {
			t.Fatalf("Quiet format: %v", err)
		}
		if out != "123" {
			t.Errorf("expected 123, got %q", out)
		}
	})
}
