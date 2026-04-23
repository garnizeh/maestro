package eld //nolint:testpackage // shared internal test helper

import (
	"github.com/garnizeh/maestro/internal/testutil"
)

// mockCommander implements Commander for testing.
type mockCommander = testutil.MockCommander
type mockFS = testutil.MockFS
