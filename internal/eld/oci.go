package eld

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strconv"
	"syscall"

	"github.com/rs/zerolog/log"

	"github.com/rodrigo-baliza/maestro/internal/beam"
)

// OCIRuntime implements [Eld] by executing a generic OCI-compatible runtime
// binary (runc, crun, youki) via CLI invocation.
type OCIRuntime struct {
	info      RuntimeInfo
	commander Commander
	fs        FS
}

// NewOCIRuntime returns an [OCIRuntime] for the given runtime binary.
func NewOCIRuntime(info RuntimeInfo) *OCIRuntime {
	return &OCIRuntime{
		info:      info,
		commander: RealCommander{},
		fs:        RealFS{},
	}
}

// WithCommander sets a custom commander implementation.
func (r *OCIRuntime) WithCommander(c Commander) *OCIRuntime {
	r.commander = c
	return r
}

// WithFS sets a custom filesystem implementation.
func (r *OCIRuntime) WithFS(f FS) *OCIRuntime {
	r.fs = f
	return r
}

// Info returns the runtime metadata.
func (r *OCIRuntime) Info() RuntimeInfo { return r.info }

// Create creates a container from the OCI bundle at bundle.
// Invokes: <runtime> create --bundle <bundle> <id>.
// If opts.LauncherPath is set, it delegates execution to the namespace holder.
func (r *OCIRuntime) Create(ctx context.Context, id, bundle string, opts *CreateOpts) error {
	args := []string{"create", "--bundle", bundle}
	var stdout, stderr io.Writer
	var launcher string

	if opts != nil {
		if opts.NoPivot {
			args = append(args, "--no-pivot")
		}
		args = append(args, opts.ExtraArgs...)
		stdout = opts.Stdout
		stderr = opts.Stderr
		launcher = opts.LauncherPath
	}
	args = append(args, id)

	// Phase 2: Log raw OCI config.json payload
	configPath := filepath.Join(bundle, "config.json")
	if data, errRead := r.fs.ReadFile(configPath); errRead == nil {
		log.Debug().Str("containerID", id).RawJSON("config", data).Msg("OCI config.json payload")
	}

	if launcher != "" {
		r.cleanupCreatePipes(stdout, stderr)
		return r.runViaLauncher(ctx, launcher, args...)
	}

	return r.run(ctx, stdout, stderr, args...)
}

// Start starts the user process in a previously created container.
// Invokes: <runtime> start <id>.
// If opts.LauncherPath is set, it delegates execution to the namespace holder.
func (r *OCIRuntime) Start(ctx context.Context, id string, opts *StartOpts) error {
	args := []string{"start", id}
	if opts != nil && opts.LauncherPath != "" {
		return r.runViaLauncher(ctx, opts.LauncherPath, args...)
	}
	return r.run(ctx, nil, nil, args...)
}

// Kill sends signal to the container's init process.
// Invokes: <runtime> kill <id> <signal>.
func (r *OCIRuntime) Kill(ctx context.Context, id string, signal syscall.Signal) error {
	return r.run(ctx, nil, nil, "kill", id, strconv.Itoa(int(signal)))
}

// Delete removes the container's resources.
// Invokes: <runtime> delete [--force] <id>.
func (r *OCIRuntime) Delete(ctx context.Context, id string, opts *DeleteOpts) error {
	args := []string{"delete"}
	if opts != nil && opts.Force {
		args = append(args, "--force")
	}
	args = append(args, id)
	return r.run(ctx, nil, nil, args...)
}

// State returns the container's current Ka state.
// Invokes: <runtime> state <id> (returns JSON).
func (r *OCIRuntime) State(ctx context.Context, id string) (*State, error) {
	cmd := r.commander.CommandContext(ctx, r.info.Path, "state", id)
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
	log.Debug().Str("id", id).Str("status", string(s.Status)).Int("pid", s.Pid).
		Msg("oci: state retrieved")
	return &s, nil
}

// Features returns the runtime's capability set.
func (r *OCIRuntime) Features(ctx context.Context) (*Features, error) {
	cmd := r.commander.CommandContext(ctx, r.info.Path, "features")
	out, err := cmd.Output()
	if err != nil {
		// If features command fails, we assume a basic runtime (like runc)
		// that supports seccomp via static configuration.
		log.Warn().Err(err).Msg("runtime: failed to run features command, using defaults")
		return &Features{Seccomp: true}, nil // expected fallback
	}

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
		log.Warn().Err(jsonErr).Msg("runtime: failed to parse features output, using defaults")
		return &Features{Seccomp: true}, nil
	}

	cgroupsV2 := false
	for _, c := range raw.Linux.Cgroups {
		if c == "cgroups_v2" || c == "v2" {
			cgroupsV2 = true
		}
	}

	features := &Features{
		Namespaces: raw.Linux.Namespaces,
		CgroupsV2:  cgroupsV2,
		Seccomp:    raw.Seccomp.Enabled,
	}
	log.Debug().Interface("features", features).Msg("oci: features detected")
	return features, nil
}

func (r *OCIRuntime) runViaLauncher(ctx context.Context, launcher string, args ...string) error {
	fullArgs := append([]string{r.info.Path}, args...)
	req := beam.ExecRequest{
		Args: fullArgs,
		Wait: true,
	}
	log.Debug().Str("launcher", launcher).Strs("args", fullArgs).Msg("OCI runtime via launcher")
	_, err := beam.HolderInvoke(ctx, launcher, req)
	return err
}

// run executes a runtime subcommand and returns a wrapped error on failure.
func (r *OCIRuntime) run(ctx context.Context, stdout, stderr io.Writer, args ...string) error {
	cmd := r.commander.CommandContext(ctx, r.info.Path, args...)
	cmd.Stdout = stdout

	if stderr != nil {
		if f, ok := stderr.(*os.File); ok {
			cmd.Stderr = f
			log.Debug().Str("runtime", r.info.Name).Str("path", r.info.Path).
				Strs("args", args).Msg("executing OCI runtime")
			if runErr := cmd.Run(); runErr != nil {
				return fmt.Errorf(
					"runtime %s %v: %w (see logs for details)",
					r.info.Name,
					args,
					runErr,
				)
			}
			return nil
		}
	}

	tmpFile, err := r.fs.CreateTemp("", "maestro-runtime-stderr-*")
	if err != nil {
		return fmt.Errorf("runtime: create stderr temp file: %w", err)
	}
	tmpName := tmpFile.Name()
	defer func() {
		if errClose := tmpFile.Close(); errClose != nil {
			log.Debug().
				Err(errClose).
				Str("path", tmpName).
				Msg("oci: failed to close stderr temp file")
		}
		if errRem := r.fs.Remove(tmpName); errRem != nil {
			log.Debug().
				Err(errRem).
				Str("path", tmpName).
				Msg("oci: failed to remove stderr temp file")
		}
	}()

	cmd.Stderr = tmpFile
	if stderr != nil {
		cmd.Stderr = io.MultiWriter(tmpFile, stderr)
	}

	log.Debug().
		Str("runtime", r.info.Name).
		Str("path", r.info.Path).
		Strs("args", args).
		Msg("executing OCI runtime")
	if runErr := cmd.Run(); runErr != nil {
		if errSync := tmpFile.Sync(); errSync != nil {
			log.Debug().Err(errSync).Msg("runtime: failed to sync stderr temp file")
		}
		if _, errSeek := tmpFile.Seek(0, 0); errSeek != nil {
			log.Debug().Err(errSeek).Msg("runtime: failed to seek stderr temp file")
		}
		stderrContent, errRead := io.ReadAll(tmpFile)
		if errRead != nil {
			log.Debug().Err(errRead).Msg("runtime: failed to read stderr temp file")
		}
		return fmt.Errorf("runtime %s %v: %w: %s", r.info.Name, args, runErr, string(stderrContent))
	}
	return nil
}

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

func fmtError(err error, stderr *bytes.Buffer) error {
	if stderr != nil {
		if msg := bytes.TrimSpace(stderr.Bytes()); len(msg) > 0 {
			return fmt.Errorf("%w: %s", err, msg)
		}
	}
	return err
}

func (r *OCIRuntime) cleanupCreatePipes(stdout, stderr io.Writer) {
	// Bean protocol doesn't yet support FD passing.
	// Close the provided streams to avoid monitor hangs.
	if stdout != nil {
		if closer, ok := stdout.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Warn().Err(err).Msg("oci: failed to close stdout pipe")
			}
		}
	}
	if stderr != nil {
		if closer, ok := stderr.(io.Closer); ok {
			if err := closer.Close(); err != nil {
				log.Warn().Err(err).Msg("oci: failed to close stderr pipe")
			}
		}
	}
}
