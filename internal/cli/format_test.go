package cli_test

import (
	"strings"
	"testing"

	"github.com/rodrigo-baliza/maestro/internal/cli"
)

type sampleData struct {
	Name  string `json:"name"  yaml:"name"`
	Value int    `json:"value" yaml:"value"`
}

func TestFormatter_JSON(t *testing.T) {
	f := cli.NewFormatter("json", false)
	out, err := f.Format(sampleData{Name: "test", Value: 42})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, `"name"`) || !strings.Contains(out, `"test"`) {
		t.Errorf("unexpected JSON output: %s", out)
	}
}

func TestFormatter_YAML(t *testing.T) {
	f := cli.NewFormatter("yaml", false)
	out, err := f.Format(sampleData{Name: "test", Value: 42})
	if err != nil {
		t.Fatal(err)
	}
	if !strings.Contains(out, "name: test") {
		t.Errorf("unexpected YAML output: %s", out)
	}
}

func TestFormatter_Template(t *testing.T) {
	f := cli.NewFormatter("{{.Name}}-{{.Value}}", false)
	out, err := f.Format(sampleData{Name: "foo", Value: 7})
	if err != nil {
		t.Fatal(err)
	}
	if out != "foo-7" {
		t.Errorf("template output = %q, want %q", out, "foo-7")
	}
}

func TestFormatter_InvalidTemplate(t *testing.T) {
	f := cli.NewFormatter("{{.Unclosed", false)
	_, err := f.Format(sampleData{})
	if err == nil {
		t.Error("expected error for invalid template")
	}
}

func TestFormatter_JSONMarshalError(t *testing.T) {
	f := cli.NewFormatter("json", false)
	_, err := f.Format(make(chan int)) // channels cannot be JSON-marshalled
	if err == nil {
		t.Error("expected error for non-serializable type")
	}
}

func TestFormatter_TableMarshalError(t *testing.T) {
	f := cli.NewFormatter("table", false)
	_, err := f.Format(make(chan int)) // table falls back to JSON; channels fail
	if err == nil {
		t.Error("expected error for non-serializable type")
	}
}

func TestFormatter_TemplateExecError(t *testing.T) {
	f := cli.NewFormatter("{{.NoSuchField}}", false)
	_, err := f.Format(sampleData{Name: "x", Value: 1})
	if err == nil {
		t.Error("expected error when template accesses non-existent field")
	}
}

func TestFormatter_Quiet(t *testing.T) {
	f := cli.NewFormatter("json", true).WithQuietFn(func(_ any) string {
		return "quiet-value"
	})
	out, err := f.Format(sampleData{})
	if err != nil {
		t.Fatal(err)
	}
	if out != "quiet-value" {
		t.Errorf("quiet output = %q", out)
	}
}
