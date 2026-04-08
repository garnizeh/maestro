package eld

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
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
	// LookPathFn is exec.LookPath by default; replaced in tests.
	LookPathFn func(file string) (string, error)
	// RunVersionFn executes "<binary> --version" and returns stdout+stderr.
	RunVersionFn func(binary string) (string, error)
}

// NewPathfinder returns a [Pathfinder] using the system's $PATH.
func NewPathfinder() *Pathfinder {
	return &Pathfinder{
		LookPathFn:   exec.LookPath,
		RunVersionFn: DefaultRunVersionFn,
	}
}

// Discover returns a [RuntimeInfo] for the best available OCI runtime.
//
// Priority:
//  1. configPath and configName are set → validate that binary; error if missing.
//  2. Search runtimeCandidates (crun → runc → youki) in $PATH.
//  3. No runtime found → return [ErrRuntimeNotFound].
func (p *Pathfinder) Discover(configPath, configName string) (*RuntimeInfo, error) {
	// Case 1: explicit override from configuration.
	if configPath != "" {
		return p.validate(configName, configPath)
	}

	// Case 2: search PATH in priority order.
	for _, name := range runtimeCandidates {
		path, err := p.LookPathFn(name)
		if err != nil {
			continue // not found in PATH, try next
		}
		info, err := p.validate(name, path)
		if err != nil {
			continue // binary exists but failed validation, try next
		}
		return info, nil
	}

	// Case 3: nothing found.
	return nil, fmt.Errorf("%w: searched %s", ErrRuntimeNotFound, strings.Join(runtimeCandidates, ", "))
}

// validate checks that the binary at path is a working OCI runtime and
// returns the populated [RuntimeInfo].
func (p *Pathfinder) validate(name, path string) (*RuntimeInfo, error) {
	// Resolve the absolute path.
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, fmt.Errorf("resolve runtime path %s: %w", path, err)
	}

	// Stat the binary — it must exist and be executable.
	if _, statErr := os.Stat(abs); statErr != nil {
		return nil, fmt.Errorf("runtime binary %s: %w", abs, statErr)
	}

	// Run "--version" to confirm it responds and capture the version string.
	version, vErr := p.RunVersionFn(abs)
	if vErr != nil {
		return nil, fmt.Errorf("runtime %s failed version check: %w", abs, vErr)
	}

	return &RuntimeInfo{
		Name:    name,
		Path:    abs,
		Version: strings.TrimSpace(version),
	}, nil
}

// DefaultRunVersionFn runs "<binary> --version" and returns the combined output.
func DefaultRunVersionFn(binary string) (string, error) {
	cmd := exec.CommandContext(context.Background(), binary, "--version")
	var out bytes.Buffer
	cmd.Stdout = &out
	cmd.Stderr = &out
	if err := cmd.Run(); err != nil {
		return "", errors.Join(err, fmt.Errorf("output: %s", out.String()))
	}
	return out.String(), nil
}
