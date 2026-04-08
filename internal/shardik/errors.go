package shardik

import "errors"

// As is a package-local alias so shardik.go does not need a direct import of
// the errors package while still being readable.
//
//nolint:gochecknoglobals // package-level alias; required by shardik.go to avoid a direct errors import
var As = errors.As
