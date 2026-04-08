package eld

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"os/exec"
	"strconv"
	"syscall"
)

// OCIRuntime implements [Eld] by executing a generic OCI-compatible runtime
// binary (runc, crun, youki) via CLI invocation.
type OCIRuntime struct {
	info RuntimeInfo
	// ExecCommandFn is configurable for testing — replaced by a fake binary.
	ExecCommandFn func(ctx context.Context, name string, arg ...string) *exec.Cmd
}

// NewOCIRuntime returns an [OCIRuntime] for the given runtime binary.
func NewOCIRuntime(info RuntimeInfo) *OCIRuntime {
	return &OCIRuntime{
		info:          info,
		ExecCommandFn: exec.CommandContext,
	}
}

// Info returns the runtime metadata.
func (r *OCIRuntime) Info() RuntimeInfo { return r.info }

// Create creates a container from the OCI bundle at bundle.
// Invokes: <runtime> create --bundle <bundle> <id>.
func (r *OCIRuntime) Create(ctx context.Context, id, bundle string, opts *CreateOpts) error {
	args := []string{"create", "--bundle", bundle}
	if opts != nil && opts.NoPivot {
		args = append(args, "--no-pivot")
	}
	if opts != nil {
		args = append(args, opts.ExtraArgs...)
	}
	args = append(args, id)
	return r.run(ctx, args...)
}

// Start starts the user process in a previously created container.
// Invokes: <runtime> start <id>.
func (r *OCIRuntime) Start(ctx context.Context, id string) error {
	return r.run(ctx, "start", id)
}

// Kill sends signal to the container's init process.
// Invokes: <runtime> kill <id> <signal>.
func (r *OCIRuntime) Kill(ctx context.Context, id string, signal syscall.Signal) error {
	return r.run(ctx, "kill", id, strconv.Itoa(int(signal)))
}

// Delete removes the container's resources.
// Invokes: <runtime> delete [--force] <id>.
func (r *OCIRuntime) Delete(ctx context.Context, id string, opts *DeleteOpts) error {
	args := []string{"delete"}
	if opts != nil && opts.Force {
		args = append(args, "--force")
	}
	args = append(args, id)
	return r.run(ctx, args...)
}

// State returns the container's current Ka state.
// Invokes: <runtime> state <id> (returns JSON).
func (r *OCIRuntime) State(ctx context.Context, id string) (*State, error) {
	cmd := r.ExecCommandFn(ctx, r.info.Path, "state", id)
	var stderrBuf bytes.Buffer
	cmd.Stderr = &stderrBuf
	out, err := cmd.Output()
	if err != nil {
		if isNotFoundErr(err, stderrBuf.Bytes()) {
			return nil, fmt.Errorf("%w: %s", ErrContainerNotFound, id)
		}
		return nil, fmt.Errorf("runtime state %s: %w", id, fmtError(err, &stderrBuf))
	}

	var s State
	if jsonErr := json.Unmarshal(out, &s); jsonErr != nil {
		return nil, fmt.Errorf("parse runtime state for %s: %w", id, jsonErr)
	}
	return &s, nil
}

// Features returns the runtime's capability set.
// Invokes: <runtime> features (OCI runtime spec ≥ 1.1).
// If the runtime does not support the features subcommand, a safe default is
// returned without error.
func (r *OCIRuntime) Features(ctx context.Context) (*Features, error) {
	cmd := r.ExecCommandFn(ctx, r.info.Path, "features")
	out, err := cmd.Output()
	if err != nil {
		// runtime does not support features — return a conservative default.
		return &Features{Seccomp: true}, nil //nolint:nilerr // feature discovery fallback
	}

	// Parse the OCI features JSON document.
	// We only extract the fields we care about.
	var raw struct {
		Linux struct {
			Namespaces []string `json:"namespaces"`
			Cgroups    []string `json:"cgroups"`
		} `json:"linux"`
		Seccomp struct {
			Enabled bool `json:"enabled"`
		} `json:"seccomp"`
	}
	if jsonErr := json.Unmarshal(out, &raw); jsonErr != nil {
		// Unparseable features response — treat as missing, return defaults.
		return &Features{Seccomp: true}, nil //nolint:nilerr // feature discovery fallback
	}

	cgroupsV2 := false
	for _, c := range raw.Linux.Cgroups {
		if c == "cgroups_v2" || c == "v2" {
			cgroupsV2 = true
		}
	}

	return &Features{
		Namespaces: raw.Linux.Namespaces,
		CgroupsV2:  cgroupsV2,
		Seccomp:    raw.Seccomp.Enabled,
	}, nil
}

// run executes a runtime subcommand and returns a wrapped error on failure.
func (r *OCIRuntime) run(ctx context.Context, args ...string) error {
	cmd := r.ExecCommandFn(ctx, r.info.Path, args...)
	var stderr bytes.Buffer
	cmd.Stderr = &stderr
	if err := cmd.Run(); err != nil {
		return fmt.Errorf("runtime %s %v: %w: %s", r.info.Name, args, err, stderr.String())
	}
	return nil
}

// isNotFoundErr reports whether the runtime error indicates a missing container.
func isNotFoundErr(err error, stderrOutput []byte) bool {
	if err == nil {
		return false
	}
	combined := err.Error() + string(stderrOutput)
	for _, needle := range []string{"does not exist", "not found", "no such container"} {
		if containsInsensitive(combined, needle) {
			return true
		}
	}
	return false
}

// containsInsensitive is a simple case-insensitive Contains without importing strings.
func containsInsensitive(s, sub string) bool {
	if len(sub) == 0 {
		return true
	}
	if len(s) < len(sub) {
		return false
	}
	for i := 0; i <= len(s)-len(sub); i++ {
		match := true
		for j := range sub {
			a, b := s[i+j], sub[j]
			if a >= 'A' && a <= 'Z' {
				a += 32
			}
			if b >= 'A' && b <= 'Z' {
				b += 32
			}
			if a != b {
				match = false
				break
			}
		}
		if match {
			return true
		}
	}
	return false
}

// fmtError extracts a useful error string from an exec command failure.
func fmtError(err error, stderr *bytes.Buffer) error {
	if stderr != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}
