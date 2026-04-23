package shardik

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/google/go-containerregistry/pkg/authn"
	"github.com/rs/zerolog/log"
)

// SigulConfig carries optional overrides for the credential resolution chain.
// Zero value means "use all sources in priority order, no explicit credentials".
type SigulConfig struct {
	// Username and Password from explicit CLI flags (priority 1).
	Username string
	Password string
	// AuthFilePath overrides the default ~/.config/maestro/auth.json (optional).
	AuthFilePath string
	// HomeDir is a function providing the user's home directory (DI point).
	HomeDir func() (string, error)
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
	if cfg.HomeDir == nil {
		cfg.HomeDir = os.UserHomeDir
	}
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
	if auth, err := resolveFromAuthFile(s.cfg, registry); err == nil {
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
func resolveFromAuthFile(cfg SigulConfig, registry string) (authn.Authenticator, error) {
	path, pathErr := authFilePath(cfg)
	if pathErr != nil {
		return nil, pathErr
	}

	data, readErr := os.ReadFile(path)
	if readErr != nil {
		return nil, readErr
	}

	// Warn if permissions are too open.
	if info, statErr := os.Stat(path); statErr == nil {
		if info.Mode().Perm()&0o077 != 0 {
			log.Warn().Str("path", path).Stringer("mode", info.Mode().Perm()).
				Msg("sigul: auth file has overly permissive permissions; recommend 0600")
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
func SaveCredentials(registry, username, password string, cfg SigulConfig) error {
	path, err := authFilePath(cfg)
	if err != nil {
		return err
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return fmt.Errorf("create config dir: %w", mkdirErr)
	}

	// Load existing file, or start fresh.
	var af authFile
	if data, readErr := os.ReadFile(path); readErr == nil {
		if jsonErr := json.Unmarshal(data, &af); jsonErr != nil {
			log.Warn().
				Err(jsonErr).
				Str("path", path).
				Msg("sigul: failed to parse existing auth file; starting fresh")
		}
	}
	if af.Auths == nil {
		af.Auths = make(map[string]authEntry)
	}

	af.Auths[registry] = authEntry{Username: username, Password: password}

	data, jsonErr := json.MarshalIndent(af, "", "  ")
	if jsonErr != nil {
		return fmt.Errorf("marshal auth file: %w", jsonErr)
	}

	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return fmt.Errorf("write auth file: %w", writeErr)
	}
	return nil
}

// RemoveCredentials removes credentials for a registry from auth.json.
func RemoveCredentials(registry string, cfg SigulConfig) error {
	path, err := authFilePath(cfg)
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

	out, jsonErr := json.MarshalIndent(af, "", "  ")
	if jsonErr != nil {
		return fmt.Errorf("marshal auth file: %w", jsonErr)
	}

	if writeErr := os.WriteFile(path, out, 0o600); writeErr != nil {
		return fmt.Errorf("write auth file: %w", writeErr)
	}
	return nil
}

// authFilePath returns the path to auth.json, using pathOverride when non-empty.
func authFilePath(cfg SigulConfig) (string, error) {
	if cfg.AuthFilePath != "" {
		return cfg.AuthFilePath, nil
	}
	homeDirFn := cfg.HomeDir
	if homeDirFn == nil {
		homeDirFn = os.UserHomeDir
	}
	home, err := homeDirFn()
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
