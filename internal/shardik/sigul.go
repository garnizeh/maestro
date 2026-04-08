package shardik

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
)

// SigulConfig carries optional overrides for the credential resolution chain.
// Zero value means "use all sources in priority order, no explicit credentials".
type SigulConfig struct {
	// Username and Password from explicit CLI flags (priority 1).
	Username string
	Password string
	// AuthFilePath overrides the default ~/.config/maestro/auth.json (optional).
	AuthFilePath string
}

// authEntry is a single entry in the Maestro auth.json credential store.
type authEntry struct {
	Username string `json:"username"`
	Password string `json:"password"`
	Token    string `json:"token,omitempty"`
}

// authFile is the on-disk representation of ~/.config/maestro/auth.json.
type authFile struct {
	Auths map[string]authEntry `json:"auths"`
}

// sigulKeychain implements [authn.Keychain] using the Sigul credential chain.
type sigulKeychain struct {
	cfg SigulConfig
}

// NewSigulKeychain returns an [authn.Keychain] that resolves credentials using
// the Sigul priority chain:
//  1. CLI flags (SigulConfig.Username / Password)
//  2. $MAESTRO_REGISTRY_TOKEN environment variable
//  3. Maestro auth.json (~/.config/maestro/auth.json)
//  4. Docker config.json (~/.docker/config.json) via the default ggcr keychain
//  5. Credential helpers (via Docker keychain)
//  6. Anonymous fallback
func NewSigulKeychain(cfg SigulConfig) authn.Keychain {
	return &sigulKeychain{cfg: cfg}
}

// Resolve implements [authn.Keychain].
func (s *sigulKeychain) Resolve(resource authn.Resource) (authn.Authenticator, error) {
	registry := resource.RegistryStr()

	// Priority 1 — explicit CLI flags.
	if s.cfg.Username != "" {
		return authn.FromConfig(authn.AuthConfig{
			Username: s.cfg.Username,
			Password: s.cfg.Password,
		}), nil
	}

	// Priority 2 — $MAESTRO_REGISTRY_TOKEN.
	if tok := os.Getenv("MAESTRO_REGISTRY_TOKEN"); tok != "" {
		return authn.FromConfig(authn.AuthConfig{RegistryToken: tok}), nil
	}

	// Priority 3 — Maestro auth.json.
	if auth, err := resolveFromAuthFile(s.cfg.AuthFilePath, registry); err == nil {
		return auth, nil
	}

	// Priority 4+5 — Docker config + credential helpers (via ggcr default keychain).
	dockerAuth, err := authn.DefaultKeychain.Resolve(resource)
	if err == nil && dockerAuth != authn.Anonymous {
		return dockerAuth, nil
	}

	// Priority 6 — anonymous.
	return authn.Anonymous, nil
}

// resolveFromAuthFile looks up registry credentials from the Maestro auth.json.
func resolveFromAuthFile(pathOverride, registry string) (authn.Authenticator, error) {
	path, pathErr := authFilePath(pathOverride)
	if pathErr != nil {
		return nil, pathErr
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}

	// Warn if permissions are too open.
	if info, statErr := os.Stat(path); statErr == nil {
		if info.Mode().Perm()&0o177 != 0 {
			_, _ = fmt.Fprintf(os.Stderr,
				"warning: auth file %s has overly permissive permissions %v\n",
				path, info.Mode().Perm(),
			)
		}
	}

	var af authFile
	if jsonErr := json.Unmarshal(data, &af); jsonErr != nil {
		return nil, fmt.Errorf("parse auth file: %w", jsonErr)
	}

	// Try exact match, then bare hostname.
	for _, key := range []string{registry, bareHost(registry)} {
		if entry, ok := af.Auths[key]; ok {
			if entry.Token != "" {
				return authn.FromConfig(authn.AuthConfig{RegistryToken: entry.Token}), nil
			}
			return authn.FromConfig(authn.AuthConfig{
				Username: entry.Username,
				Password: entry.Password,
			}), nil
		}
	}

	return nil, errors.New("registry not found in auth file")
}

// SaveCredentials writes credentials for the given registry to auth.json.
func SaveCredentials(registry, username, password string, pathOverride string) error {
	path, err := authFilePath(pathOverride)
	if err != nil {
		return err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return fmt.Errorf("create config dir: %w", mkdirErr)
	}

	// Load existing file, or start fresh.
	var af authFile
	if data, readErr := os.ReadFile(path); readErr == nil {
		_ = json.Unmarshal(data, &af)
	}
	if af.Auths == nil {
		af.Auths = make(map[string]authEntry)
	}

	af.Auths[registry] = authEntry{Username: username, Password: password}

	// json.MarshalIndent cannot fail for the authFile struct (plain string fields).
	data, _ := json.MarshalIndent(af, "", "  ")

	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("write auth file: %w", writeErr)
	}
	return nil
}

// RemoveCredentials removes credentials for a registry from auth.json.
func RemoveCredentials(registry string, pathOverride string) error {
	path, err := authFilePath(pathOverride)
	if err != nil {
		return err
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		if os.IsNotExist(readErr) {
			return nil // nothing to remove
		}
		return fmt.Errorf("read auth file: %w", readErr)
	}

	var af authFile
	if jsonErr := json.Unmarshal(data, &af); jsonErr != nil {
		return fmt.Errorf("parse auth file: %w", jsonErr)
	}

	delete(af.Auths, registry)
	delete(af.Auths, bareHost(registry))

	// json.MarshalIndent cannot fail for the authFile struct (plain string fields).
	out, _ := json.MarshalIndent(af, "", "  ")

	if writeErr := os.WriteFile(path, out, 0o600); writeErr != nil {
		return fmt.Errorf("write auth file: %w", writeErr)
	}
	return nil
}

// userHomeDirFn is the function used to look up the user's home directory.
// Overridden in tests to simulate home-directory lookup failures.
//
//nolint:gochecknoglobals // dependency injection point: overridden in tests
var userHomeDirFn = os.UserHomeDir

// authFilePath returns the path to auth.json, using pathOverride when non-empty.
func authFilePath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	home, err := userHomeDirFn()
	if err != nil {
		return "", fmt.Errorf("cannot determine home directory: %w", err)
	}
	return filepath.Join(home, ".config", "maestro", "auth.json"), nil
}

// bareHost strips a port suffix from a registry hostname.
func bareHost(registry string) string {
	if idx := strings.LastIndex(registry, ":"); idx != -1 {
		return registry[:idx]
	}
	return registry
}
