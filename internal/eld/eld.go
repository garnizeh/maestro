// Package eld implements the Eld runtime abstraction layer for Maestro.
//
// Named after Arthur Eld from The Dark Tower — the legendary gunslinger-king
// from whom all gunslingers descend. Different OCI runtimes (runc, crun, youki,
// gVisor, Kata) are different gunslingers, all descended from the same lineage.
//
// Eld defines a common interface for all OCI-compatible container runtimes.
// Pathfinder discovers available runtimes. OCI implements the generic CLI driver.
// Monitor (Cort MVP) supervises container processes after Eld starts them.
package eld

import (
	"context"
	"errors"
	"io"
	"syscall"
	"time"
)

const (
	// dirPerm is the default permission for log and pid directories.
	dirPerm = 0o700
	// filePerm is the default permission for log and pid files.
	filePerm = 0o600
)

// ErrRuntimeNotFound is returned when no supported OCI runtime is found.
var ErrRuntimeNotFound = errors.New("no OCI runtime found (install crun, runc, or youki)")

// ErrContainerNotFound is returned when the runtime reports no such container.
var ErrContainerNotFound = errors.New("container not found in runtime")

// ErrInvalidSignal is returned when an unrecognised signal name/number is given.
var ErrInvalidSignal = errors.New("invalid signal")

// Status represents the Ka state as reported by the OCI runtime.
type Status string

const (
	StatusCreated Status = "created"
	StatusRunning Status = "running"
	StatusStopped Status = "stopped"
	StatusPaused  Status = "paused"
)

// State is the container state as returned by the OCI runtime's state command.
type State struct {
	// OCI runtime spec version.
	Version string `json:"ociVersion"`
	// Container identifier (matches what was passed to Create).
	ID string `json:"id"`
	// Ka state: created, running, stopped.
	Status Status `json:"status"`
	// PID of the container's init process (0 if not running).
	Pid int `json:"pid"`
	// Bundle path used when this container was created.
	Bundle string `json:"bundle"`
}

// Features holds the capabilities reported by the OCI runtime.
type Features struct {
	// Namespaces lists the namespace types supported by the runtime.
	Namespaces []string `json:"namespaces,omitempty"`
	// Cgroups reports whether the runtime supports cgroup v2.
	CgroupsV2 bool `json:"cgroupsV2"`
	// Seccomp reports whether the runtime supports seccomp filtering.
	Seccomp bool `json:"seccomp"`
}

// CreateOpts carries options for the Create operation.
type CreateOpts struct {
	// NoPivot disables pivot_root (used in certain rootless environments).
	NoPivot bool
	// Stdout is where the container's stdout is redirected.
	Stdout io.Writer
	// Stderr is where the container's stderr is redirected.
	Stderr io.Writer
	// ExtraArgs are additional runtime-specific arguments.
	ExtraArgs []string
	// LauncherPath is the absolute path to a namespace holder (e.g. for rootless).
	LauncherPath string
}

// StartOpts carries options for the Start operation.
type StartOpts struct {
	// LauncherPath is the absolute path to a namespace holder (e.g. for rootless).
	LauncherPath string
}

// DeleteOpts carries options for the Delete operation.
type DeleteOpts struct {
	// Force forcibly removes container resources even if not stopped.
	Force bool
}

// Eld defines the common interface for all OCI-compatible container runtimes.
// All runtimes descend from the same lineage — different gunslingers, same code.
type Eld interface {
	// Create creates a container from an OCI bundle (Gan's act of creation).
	// The container is placed in the "created" state; the user process does
	// not start yet.
	Create(ctx context.Context, id, bundle string, opts *CreateOpts) error

	// Start begins the user-specified process in a previously created container.
	// The container transitions from "created" to "running".
	Start(ctx context.Context, id string, opts *StartOpts) error

	// Kill sends signal to the container's init process (Roland fires).
	Kill(ctx context.Context, id string, signal syscall.Signal) error

	// Delete removes the container and releases its resources.
	// The container must be in the "stopped" state, unless opts.Force is set.
	Delete(ctx context.Context, id string, opts *DeleteOpts) error

	// State returns the container's current Ka (state) as reported by the runtime.
	State(ctx context.Context, id string) (*State, error)

	// Features returns the runtime's supported capabilities.
	// If the runtime does not support the features command, a default set
	// is returned without error.
	Features(ctx context.Context) (*Features, error)
}

// RuntimeInfo holds information about a discovered OCI runtime.
type RuntimeInfo struct {
	Name    string `json:"name"`
	Path    string `json:"path"`
	Version string `json:"version"`
}

// MonitorConfig configures the native Go container monitor.
type MonitorConfig struct {
	// ContainerID is the container being monitored.
	ContainerID string
	// BundlePath is the OCI bundle directory.
	BundlePath string
	// LogPath is the file where stdout/stderr are written in json-file format.
	LogPath string
	// PidFile is where the container's init PID is written.
	PidFile string
	// ExitFile is where the exit code is written after the process terminates.
	ExitFile string
	// Detach indicates whether the monitor should detach from the CLI terminal.
	Detach bool
	// Stdout is an optional writer where the container's stdout will be streamed in real-time.
	Stdout io.Writer
	// Stderr is an optional writer where the container's stderr will be streamed in real-time.
	Stderr io.Writer
	// Timeout is the maximum time to wait for the container to start.
	Timeout time.Duration
	// LauncherPath is the absolute path to a namespace holder (e.g. for rootless).
	LauncherPath string
}
