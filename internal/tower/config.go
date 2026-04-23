// Package tower manages Maestro's configuration and first-run setup.
package tower

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"github.com/pelletier/go-toml/v2"
	"github.com/rs/zerolog/log"

	"github.com/garnizeh/maestro/internal/sys"
)

type FS interface {
	UserHomeDir() (string, error)
	ReadFile(name string) ([]byte, error)
	MkdirAll(path string, perm os.FileMode) error
	WriteFile(name string, data []byte, perm os.FileMode) error
	Stat(name string) (os.FileInfo, error)
	Getenv(key string) string
	IsNotExist(err error) bool
}

type Marshaller interface {
	Unmarshal(data []byte, v any) error
	Marshal(v any) ([]byte, error)
}

const (
	dirPerm  = 0o700
	filePerm = 0o600
)

// RealFS is the Thin Shell implementation that calls the standard library.
type RealFS = sys.RealFS

// realTOML is the Thin Shell implementation for TOML marshalling.
type realTOML struct{}

func (realTOML) Unmarshal(data []byte, v any) error { return toml.Unmarshal(data, v) }
func (realTOML) Marshal(v any) ([]byte, error)      { return toml.Marshal(v) }

// Loader handles configuration loading and initialization with injectable dependencies.
type Loader struct {
	fs   FS
	toml Marshaller
}

// NewLoader returns a Loader with the given implementations.
func NewLoader(fs FS, t Marshaller) *Loader {
	return &Loader{fs: fs, toml: t}
}

// defaultLoader is a global singleton for convenience.
//
//nolint:gochecknoglobals // singleton
var defaultLoader = NewLoader(RealFS{}, realTOML{})

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
func (l *Loader) defaults() (*Config, error) {
	home, err := l.fs.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("defaults: %w", err)
	}
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
	}, nil
}

// ConfigPath resolves the effective path to katet.toml.
// If override is non-empty it is returned as-is. Otherwise the XDG or HOME
// default is used.
func (l *Loader) ConfigPath(override string) (string, error) {
	if override != "" {
		return override, nil
	}
	base := l.fs.Getenv("XDG_CONFIG_HOME")
	if base == "" {
		home, err := l.fs.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("cannot determine home directory: %w", err)
		}
		base = filepath.Join(home, ".config")
	}
	return filepath.Join(base, "maestro", "katet.toml"), nil
}

// ConfigPath is a convenience function that uses the default loader.
func ConfigPath(override string) (string, error) {
	return defaultLoader.ConfigPath(override)
}

// LoadConfig loads configuration from the given path (or default path when
// empty), merges environment variable overrides, and returns the result.
func (l *Loader) LoadConfig(pathOverride string) (*Config, error) {
	path, err := l.ConfigPath(pathOverride)
	if err != nil {
		return nil, err
	}

	cfg, defErr := l.defaults()
	if defErr != nil {
		return nil, fmt.Errorf("defaults: %w", defErr)
	}

	data, readErr := l.fs.ReadFile(path)
	if readErr != nil && !l.fs.IsNotExist(readErr) {
		return nil, fmt.Errorf("read config %s: %w", path, readErr)
	}

	if readErr == nil {
		if unmarshalErr := l.toml.Unmarshal(data, cfg); unmarshalErr != nil {
			return nil, fmt.Errorf("parse config %s: %w", path, unmarshalErr)
		}
		log.Debug().Str("path", path).Msg("tower: config loaded from file")
	} else {
		log.Debug().Msg("tower: no config file found, using defaults")
	}

	l.applyEnvOverrides(cfg)
	return cfg, nil
}

// LoadConfig is a convenience function that uses the default loader.
func LoadConfig(pathOverride string) (*Config, error) {
	return defaultLoader.LoadConfig(pathOverride)
}

// applyEnvOverrides overwrites fields when corresponding env vars are set.
func (l *Loader) applyEnvOverrides(cfg *Config) {
	if v := l.fs.Getenv("MAESTRO_RUNTIME"); v != "" {
		cfg.Runtime.Default = v
	}
	if v := l.fs.Getenv("MAESTRO_STORAGE_DRIVER"); v != "" {
		cfg.Storage.Driver = v
	}
	if v := l.fs.Getenv("MAESTRO_LOG_LEVEL"); v != "" {
		cfg.Log.Level = v
	}
	if v := l.fs.Getenv("MAESTRO_ROOT"); v != "" {
		cfg.State.Root = v
	}
	if v := l.fs.Getenv("MAESTRO_ROOTLESS"); v != "" {
		cfg.Security.Rootless = strings.ToLower(v) != "false" && v != "0"
	}
}

// EnsureDefault creates the config file with defaults if it does not exist.
// Returns true when the file was newly created (first-run scenario).
func (l *Loader) EnsureDefault(pathOverride string) (bool, string, error) {
	path, err := l.ConfigPath(pathOverride)
	if err != nil {
		return false, "", err
	}

	if _, statErr := l.fs.Stat(path); statErr == nil {
		return false, path, nil
	}

	if mkdirErr := l.fs.MkdirAll(filepath.Dir(path), dirPerm); mkdirErr != nil {
		return false, path, fmt.Errorf("create config dir: %w", mkdirErr)
	}

	cfg, errDef := l.defaults()
	if errDef != nil {
		return false, path, errDef
	}
	data, marshalErr := l.toml.Marshal(cfg)
	if marshalErr != nil {
		return false, path, fmt.Errorf("marshal defaults: %w", marshalErr)
	}

	if writeErr := l.fs.WriteFile(path, data, filePerm); writeErr != nil {
		return false, path, fmt.Errorf("write default config: %w", writeErr)
	}

	log.Debug().Str("path", path).Msg("tower: default config created")

	return true, path, nil
}

// EnsureDefault is a convenience function that uses the default loader.
func EnsureDefault(pathOverride string) (bool, string, error) {
	return defaultLoader.EnsureDefault(pathOverride)
}

// ToTOML serialises the Config back to a TOML string.
func (c *Config) ToTOML() string {
	data, err := defaultLoader.toml.Marshal(c)
	if err != nil {
		return fmt.Sprintf("# error serialising config: %v\n", err)
	}
	return string(data)
}
