// Package tower manages Maestro's configuration and first-run setup.
package tower

import (
	"bytes"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
)

// Config holds the effective Maestro configuration after merging all sources.
type Config struct {
	Runtime  RuntimeConfig  `toml:"runtime"  json:"runtime"`
	Storage  StorageConfig  `toml:"storage"  json:"storage"`
	Network  NetworkConfig  `toml:"network"  json:"network"`
	Security SecurityConfig `toml:"security" json:"security"`
	Log      LogConfig      `toml:"log"      json:"log"`
	Registry RegistryConfig `toml:"registry" json:"registry"`
	State    StateConfig    `toml:"state"    json:"state"`
}

type RuntimeConfig struct {
	Default string `toml:"default" json:"default"`
}

type StorageConfig struct {
	Driver  string `toml:"driver"   json:"driver"`
	MaxSize string `toml:"max_size" json:"maxSize"`
	Root    string `toml:"root"     json:"root"`
}

type NetworkConfig struct {
	DefaultSubnet string `toml:"default_subnet" json:"defaultSubnet"`
	DNSEnabled    bool   `toml:"dns_enabled"    json:"dnsEnabled"`
	Driver        string `toml:"driver"         json:"driver"`
}

type SecurityConfig struct {
	Rootless       bool   `toml:"rootless"        json:"rootless"`
	DefaultSeccomp string `toml:"default_seccomp" json:"defaultSeccomp"`
}

type LogConfig struct {
	Level  string `toml:"level"  json:"level"`
	Driver string `toml:"driver" json:"driver"`
}

type RegistryConfig struct {
	Mirrors map[string]string `toml:"mirrors" json:"mirrors"`
}

type StateConfig struct {
	Root string `toml:"root" json:"root"`
}

// defaults returns a Config populated with sensible built-in values.
// It uses the current user's home directory to derive Storage.Root and State.Root
// (e.g., $HOME/.local/share/maestro[/maturin]) and sets reasonable defaults for
// runtime, storage driver/size, network, security, logging, and registry.
func defaults() *Config {
	home, _ := os.UserHomeDir()
	return &Config{
		Runtime: RuntimeConfig{Default: "auto"},
		Storage: StorageConfig{
			Driver:  "auto",
			MaxSize: "50GB",
			Root:    filepath.Join(home, ".local", "share", "maestro", "maturin"),
		},
		Network: NetworkConfig{
			DefaultSubnet: "10.99.0.0/16",
			DNSEnabled:    true,
			Driver:        "bridge",
		},
		Security: SecurityConfig{
			Rootless:       true,
			DefaultSeccomp: "builtin",
		},
		Log: LogConfig{
			Level:  "warn",
			Driver: "json-file",
		},
		Registry: RegistryConfig{
			Mirrors: map[string]string{},
		},
		State: StateConfig{
			Root: filepath.Join(home, ".local", "share", "maestro"),
		},
	}
}

// ConfigPath resolves the effective path to katet.toml.
// If override is non-empty it is returned as-is. Otherwise the XDG or HOME
// ConfigPath returns the path to the effective Maestro config file.
// If the provided override is non-empty, it is returned unchanged.
// Otherwise the function uses XDG_CONFIG_HOME if set, falling back to $HOME/.config,
// and returns $base/maestro/katet.toml.
// An error is returned only if the user's home directory cannot be determined.
func ConfigPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	base := os.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf(
				"cannot determine home directory: %w",
				err,
			) //coverage:ignore os.UserHomeDir failure requires a system without a $HOME, unreachable in unit tests
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "maestro", "katet.toml"), nil
}

// LoadConfig loads configuration from the given path (or default path when
// LoadConfig loads the effective Maestro configuration by combining built-in defaults,
// an optional on-disk TOML file, and environment variable overrides.
// 
// If pathOverride is non-empty it is used as the config file path; otherwise the path
// is resolved via ConfigPath. If the file does not exist, defaults are returned with
// environment overrides applied. An error is returned if path resolution fails,
// if reading the file fails for reasons other than "not exist", or if parsing the
// TOML file fails.
func LoadConfig(pathOverride string) (*Config, error) {
	path, err := ConfigPath(pathOverride)
	if err != nil {
		return nil, err //coverage:ignore delegates to ConfigPath; UserHomeDir failure unreachable in unit tests
	}

	cfg := defaults()

	data, err := os.ReadFile(path)
	if err != nil && !os.IsNotExist(err) {
		return nil, fmt.Errorf("read config %s: %w", path, err)
	}

	if err == nil {
		if unmarshalErr := toml.Unmarshal(data, cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, unmarshalErr)
		}
	}

	applyEnvOverrides(cfg)
	return cfg, nil
}

// applyEnvOverrides sets specific fields in cfg from environment variables when those
// variables are non-empty. It maps:
// - MAESTRO_RUNTIME -> cfg.Runtime.Default
// - MAESTRO_STORAGE_DRIVER -> cfg.Storage.Driver
// - MAESTRO_LOG_LEVEL -> cfg.Log.Level
// - MAESTRO_ROOT -> cfg.State.Root
// - MAESTRO_ROOTLESS -> cfg.Security.Rootless (treated as false for "false" or "0", true otherwise)
func applyEnvOverrides(cfg *Config) {
	if v := os.Getenv("MAESTRO_RUNTIME"); v != "" {
		cfg.Runtime.Default = v
	}
	if v := os.Getenv("MAESTRO_STORAGE_DRIVER"); v != "" {
		cfg.Storage.Driver = v
	}
	if v := os.Getenv("MAESTRO_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := os.Getenv("MAESTRO_ROOT"); v != "" {
		cfg.State.Root = v
	}
	if v := os.Getenv("MAESTRO_ROOTLESS"); v != "" {
		cfg.Security.Rootless = strings.ToLower(v) != "false" && v != "0"
	}
}

// EnsureDefault creates the config file with defaults if it does not exist.
// EnsureDefault ensures the default configuration file exists at the resolved path.
// If the file does not exist it creates the parent directory (permission 0700) and writes
// the built-in defaults to the file (permission 0600).
// It returns true and the resolved path when the file was newly created, or false and the
// resolved path when the file already existed. A non-nil error is returned if resolving the
// path, creating the directory, marshaling defaults, or writing the file fails.
func EnsureDefault(pathOverride string) (bool, string, error) {
	path, err := ConfigPath(pathOverride)
	if err != nil {
		return false, "", err //coverage:ignore delegates to ConfigPath; UserHomeDir failure unreachable in unit tests
	}

	if _, statErr := os.Stat(path); statErr == nil {
		return false, path, nil
	}

	if mkdirErr := os.MkdirAll(filepath.Dir(path), 0o700); mkdirErr != nil {
		return false, path, fmt.Errorf("create config dir: %w", mkdirErr)
	}

	cfg := defaults()
	data, marshalErr := toml.Marshal(cfg)
	if marshalErr != nil {
		return false, path, fmt.Errorf( //coverage:ignore Config only contains TOML-serializable primitive types; Marshal never fails
			"marshal defaults: %w",
			marshalErr,
		)
	}

	if writeErr := os.WriteFile(path, data, 0o600); writeErr != nil {
		return false, path, fmt.Errorf("write default config: %w", writeErr)
	}

	return true, path, nil
}

// ToTOML serialises the Config back to a TOML string.
func (c *Config) ToTOML() string {
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(c); err != nil {
		return fmt.Sprintf(
			"# error serialising config: %v\n",
			err,
		) //coverage:ignore Config only contains TOML-serializable primitive types; Encode never fails
	}
	return buf.String()
}
