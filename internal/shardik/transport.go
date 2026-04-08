package shardik

import "net/http"

// insecureTransport returns an HTTP transport that allows plain-HTTP registries.
// Used only in tests via [WithInsecure].
func insecureTransport() *http.Transport {
	return &http.Transport{} //nolint:exhaustruct // defaults are correct for plain-HTTP
}
