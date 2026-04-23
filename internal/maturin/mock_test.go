package maturin //nolint:testpackage // mock helpers are part of the internal package

import (
	"io"

	"github.com/garnizeh/maestro/pkg/archive"
)

type mockExtractor struct {
	extractFn func(io.Reader, string, archive.ExtractOptions) error
}

func (m *mockExtractor) Extract(r io.Reader, d string, o archive.ExtractOptions) error {
	if m.extractFn != nil {
		return m.extractFn(r, d, o)
	}
	return nil
}
