package cli

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
	"text/template"

	"gopkg.in/yaml.v3"
)

// Format represents an output format.
type Format string

const (
	FormatTable Format = "table"
	FormatJSON  Format = "json"
	FormatYAML  Format = "yaml"
)

// Formatter renders a value in the requested format.
type Formatter struct {
	format  string
	quiet   bool
	quietFn func(v any) string
}

// NewFormatter creates a Formatter configured with the provided format string and quiet flag.
// The returned Formatter has its quietFn unset; the format value is interpreted case-insensitively when used during formatting.
func NewFormatter(format string, quiet bool) *Formatter {
	return &Formatter{format: format, quiet: quiet}
}

// WithQuietFn sets the function used to extract the quiet-mode value (e.g., ID).
func (f *Formatter) WithQuietFn(fn func(v any) string) *Formatter {
	f.quietFn = fn
	return f
}

// Format formats v according to the configured output format and returns the result.
func (f *Formatter) Format(v any) (string, error) {
	if f.quiet && f.quietFn != nil {
		return f.quietFn(v), nil
	}

	switch strings.ToLower(f.format) {
	case "json":
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", fmt.Errorf("json marshal: %w", err)
		}
		return string(b), nil

	case "yaml":
		b, err := yaml.Marshal(v)
		if err != nil {
			return "", fmt.Errorf(
				"yaml marshal: %w",
				err,
			) //coverage:ignore yaml.Marshal panics rather than returning errors for unsupported types
		}
		return strings.TrimRight(string(b), "\n"), nil

	case "table", "":
		// Callers handle table rendering themselves; return JSON as fallback.
		b, err := json.MarshalIndent(v, "", "  ")
		if err != nil {
			return "", fmt.Errorf("format: %w", err)
		}
		return string(b), nil

	default:
		// Treat as Go template string.
		tmpl, err := template.New("custom").Parse(f.format)
		if err != nil {
			return "", fmt.Errorf("invalid template %q: %w", f.format, err)
		}
		var buf bytes.Buffer
		if execErr := tmpl.Execute(&buf, v); execErr != nil {
			return "", fmt.Errorf("template execute: %w", execErr)
		}
		return buf.String(), nil
	}
}
