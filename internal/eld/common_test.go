package eld //nolint:testpackage // shared internal test helper

import (
	"github.com/rodrigo-baliza/maestro/internal/testutil"
)

// mockCommander implements Commander for testing.
type mockCommander = testutil.MockCommander
type mockFS = testutil.MockFS
