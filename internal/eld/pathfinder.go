package eld

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/rs/zerolog/log"
)

// runtimeCandidates is the ordered priority list of OCI runtimes to discover.
// Configurable for testing via pathfinderCandidates.
var runtimeCandidates = []string{ //nolint:gochecknoglobals // priority list for runtime discovery
	"crun", "runc", "youki",
}

// Pathfinder discovers available OCI runtimes by searching $PATH in priority
// order (crun → runc → youki), validates the binary, and resolves configuration
// overrides from the provided configPath and defaultRuntime.
type Pathfinder struct {
	commander Commander
	fs        FS
}

// NewPathfinder returns a [Pathfinder] using the system's $PATH.
func NewPathfinder() *Pathfinder {
	return &Pathfinder{
		commander: RealCommander{},
		fs:        RealFS{},
	}
}

// WithCommander sets a custom commander implementation.
func (p *Pathfinder) WithCommander(c Commander) *Pathfinder {
	p.commander = c
	return p
}

// WithFS sets a custom filesystem implementation.
func (p *Pathfinder) WithFS(f FS) *Pathfinder {
	p.fs = f
	return p
}

// Discover returns a [RuntimeInfo] for the best available OCI runtime.
func (p *Pathfinder) Discover(configPath, configName string) (*RuntimeInfo, error) {
	// Case 1: explicit override from configuration.
	if configPath != "" {
		log.Debug().Str("configPath", configPath).Str("configName", configName).
			Msg("eld: discover: using configured runtime")
		return p.validate(configName, configPath)
	}

	// Case 2: search PATH in priority order.
	for _, name := range runtimeCandidates {
		path, err := p.commander.LookPath(name)
		if err != nil {
			continue // not found in PATH, try next
		}
		info, err := p.validate(name, path)
		if err != nil {
			log.Debug().Err(err).Str("name", name).Str("path", path).
				Msg("eld: discover: validation failed")
			continue // binary exists but failed validation, try next
		}
		log.Debug().Str("name", name).Str("path", path).Msg("eld: discover: runtime selected")
		return info, nil
	}

	// Case 3: nothing found.
	return nil, fmt.Errorf(
		"%w: searched %s",
		ErrRuntimeNotFound,
		strings.Join(runtimeCandidates, ", "),
	)
}

// validate checks that the binary at path is a working OCI runtime and
// returns the populated [RuntimeInfo].
func (p *Pathfinder) validate(name, path string) (*RuntimeInfo, error) {
	// Resolve the absolute path.
	abs, err := p.fs.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime path %s: %w", path, err)
	}

	// Stat the binary — it must exist and be executable.
	if _, statErr := p.fs.Stat(abs); statErr != nil {
		return nil, fmt.Errorf("runtime binary %s: %w", abs, statErr)
	}

	// Run "--version" to confirm it responds and capture the version string.
	version, vErr := p.runVersion(abs)
	if vErr != nil {
		return nil, fmt.Errorf("runtime %s failed version check: %w", abs, vErr)
	}

	return &RuntimeInfo{
		Name:    name,
		Path:    abs,
		Version: strings.TrimSpace(version),
	}, nil
}

// runVersion executes "<binary> --version" and returns the combined output.
func (p *Pathfinder) runVersion(binary string) (string, error) {
	cmd := p.commander.CommandContext(context.Background(), binary, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", errors.Join(err, fmt.Errorf("output: %s", out.String()))
	}
	return out.String(), nil
}
