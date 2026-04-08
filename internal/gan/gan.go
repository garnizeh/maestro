package gan

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"time"
)

// ErrContainerNotFound is returned when a container ID is not in the store.
var ErrContainerNotFound = errors.New("container not found")

// ErrInvalidTransition is returned when a Ka state transition is not allowed.
var ErrInvalidTransition = errors.New("invalid Ka state transition")

// ErrContainerRunning is returned when trying to remove a running container
// without the --force flag.
var ErrContainerRunning = errors.New("container is running: use --force to remove")

// ErrNameAlreadyInUse is returned when a named container already exists.
var ErrNameAlreadyInUse = errors.New("container name already in use")

// Ka represents the lifecycle state of a container.
// Named after Ka from The Dark Tower — the wheel of fate/destiny.
type Ka int //nolint:recvcheck // Mixed receivers are necessary for enums with Marshal/UnmarshalText.

const (
	// KaCreated is the destiny of a container that exists but has not started.
	KaCreated Ka = iota
	// KaRunning is the destiny of an executing user process.
	KaRunning
	// KaStopped is the destiny of a terminated process.
	KaStopped
	// KaDeleted is the terminal destiny of a removed container.
	KaDeleted
)

// String returns the human-readable Ka state name.
func (k Ka) String() string {
	switch k {
	case KaCreated:
		return "created"
	case KaRunning:
		return "running"
	case KaStopped:
		return "stopped"
	case KaDeleted:
		return "deleted"
	default:
		return "unknown"
	}
}

// MarshalText implements [encoding.TextMarshaler] for JSON serialisation.
func (k Ka) MarshalText() ([]byte, error) {
	return []byte(k.String()), nil
}

// UnmarshalText implements [encoding.TextUnmarshaler] for JSON deserialisation.
func (k *Ka) UnmarshalText(text []byte) error {
	switch string(text) {
	case "created":
		*k = KaCreated
	case "running":
		*k = KaRunning
	case "stopped":
		*k = KaStopped
	case "deleted":
		*k = KaDeleted
	default:
		return fmt.Errorf("unknown Ka state: %q", string(text))
	}
	return nil
}

// validTransitions defines the allowed Ka state transitions.
// Key: current state → Value: set of allowed next states.
var validTransitions = map[Ka][]Ka{ //nolint:gochecknoglobals // state machine rules
	KaCreated: {KaRunning, KaDeleted},
	KaRunning: {KaStopped},
	KaStopped: {KaDeleted},
	KaDeleted: {},
}

// CanTransition reports whether transitioning from current to next is valid.
func CanTransition(current, next Ka) bool {
	return slices.Contains(validTransitions[current], next)
}

// Container is the full in-memory and on-disk state for a container.
type Container struct {
	// ID is the 64-character hex container identifier.
	ID string `json:"id"`
	// Name is the human-readable container name (auto-generated or user-specified).
	Name string `json:"name"`
	// Image is the image reference used to create this container (e.g. "nginx:latest").
	Image string `json:"image"`
	// ImageDigest is the digest of the image manifest used.
	ImageDigest string `json:"imageDigest"`
	// Ka is the current lifecycle state.
	Ka Ka `json:"ka"`
	// Pid is the init process PID (0 if not running).
	Pid int `json:"pid"`
	// ExitCode is the exit code of the stopped process (valid when Ka=KaStopped).
	ExitCode int `json:"exitCode"`
	// BundlePath is the OCI bundle directory path.
	BundlePath string `json:"bundlePath"`
	// RootFSPath is the path to the container rootfs.
	RootFSPath string `json:"rootfsPath"`
	// LogPath is the path to the container log file.
	LogPath string `json:"logPath"`
	// PidFile is the path to the PID file.
	PidFile string `json:"pidFile"`
	// RuntimeName is the OCI runtime used (e.g. "crun", "runc").
	RuntimeName string `json:"runtimeName"`
	// Created is the container creation timestamp.
	Created time.Time `json:"created"`
	// Started is the time the container's user process first started.
	Started *time.Time `json:"started,omitempty"`
	// Finished is the time the container's user process terminated.
	Finished *time.Time `json:"finished,omitempty"`
	// Labels are arbitrary key-value string annotations.
	Labels map[string]string `json:"labels,omitempty"`
}

// Summary is a lightweight view of a container for the ps command.
type Summary struct {
	// ID is the full container ID.
	ID string `json:"id"`
	// ShortID is the first 12 characters of the ID.
	ShortID string `json:"shortId"`
	// Name is the human-readable container name.
	Name string `json:"name"`
	// Image is the image reference used.
	Image string `json:"image"`
	// Ka is the current lifecycle state name.
	Ka string `json:"status"`
	// Pid is the init process PID.
	Pid int `json:"pid"`
	// Created is the container creation timestamp.
	Created time.Time `json:"created"`
}

// Store is the key–value persistence layer for Gan.
// It is implemented by waystation.Store; this interface lets us mock it in tests.
type Store interface {
	Put(collection, key string, v any) error
	Get(collection, key string, v any) error
	Delete(collection, key string) error
	List(collection string) ([]string, error)
}

const (
	// containersCollection is the Waystation collection name for containers.
	containersCollection = "containers"
)

// Manager orchestrates container lifecycle operations.
// It composes a Store for persistence and delegates to Eld/Prim/Monitor.
type Manager struct {
	store Store
	root  string
}

// NewManager returns a [Manager] using the given store and data root.
func NewManager(store Store, root string) *Manager {
	return &Manager{store: store, root: root}
}

// LoadContainer retrieves a container's state by ID.
// Returns ErrContainerNotFound if no container with that ID exists.
func (m *Manager) LoadContainer(ctx context.Context, id string) (*Container, error) {
	_ = ctx
	var c Container
	if err := m.store.Get(containersCollection, id, &c); err != nil {
		if isNotFound(err) {
			return nil, fmt.Errorf("%w: %s", ErrContainerNotFound, id)
		}
		return nil, fmt.Errorf("gan: load container %s: %w", id, err)
	}
	return &c, nil
}

// SaveContainer persists a container's state.
func (m *Manager) SaveContainer(ctx context.Context, c *Container) error {
	_ = ctx
	if err := m.store.Put(containersCollection, c.ID, c); err != nil {
		return fmt.Errorf("gan: save container %s: %w", c.ID, err)
	}
	return nil
}

// DeleteContainer removes a container's state record.
func (m *Manager) DeleteContainer(ctx context.Context, id string) error {
	_ = ctx
	if err := m.store.Delete(containersCollection, id); err != nil {
		if isNotFound(err) {
			return fmt.Errorf("%w: %s", ErrContainerNotFound, id)
		}
		return fmt.Errorf("gan: delete container %s: %w", id, err)
	}
	return nil
}

// ListContainers returns all containers in insertion order.
func (m *Manager) ListContainers(ctx context.Context) ([]*Container, error) {
	_ = ctx
	keys, err := m.store.List(containersCollection)
	if err != nil {
		return nil, fmt.Errorf("gan: list containers: %w", err)
	}

	var containers []*Container
	for _, key := range keys {
		c, loadErr := m.LoadContainer(ctx, key)
		if loadErr != nil {
			continue // skip corrupt entries
		}
		containers = append(containers, c)
	}
	return containers, nil
}

// Transition updates the container's Ka state.
// Returns ErrInvalidTransition if the transition is not permitted.
func (m *Manager) Transition(ctx context.Context, id string, next Ka) (*Container, error) {
	c, err := m.LoadContainer(ctx, id)
	if err != nil {
		return nil, err
	}

	if !CanTransition(c.Ka, next) {
		return nil, fmt.Errorf("%w: %s → %s", ErrInvalidTransition, c.Ka, next)
	}

	c.Ka = next
	if saveErr := m.SaveContainer(ctx, c); saveErr != nil {
		return nil, saveErr
	}
	return c, nil
}

// FindByName searches for a container with the given name.
// Returns nil if no container with that name is found.
func (m *Manager) FindByName(_ context.Context, name string) (*Container, error) {
	ctrs, err := m.ListContainers(context.Background())
	if err != nil {
		return nil, err
	}
	for _, c := range ctrs {
		if c.Name == name {
			return c, nil
		}
	}
	return nil, nil //nolint:nilnil // returning nil for "not found" is the intended API here
}

// Summarise converts a Container into a lightweight Summary for display.
func Summarise(c *Container) Summary {
	shortID := c.ID
	if len(shortID) > 12 { //nolint:mnd // standard short ID length
		shortID = shortID[:12]
	}
	return Summary{
		ID:      c.ID,
		ShortID: shortID,
		Name:    c.Name,
		Image:   c.Image,
		Ka:      c.Ka.String(),
		Pid:     c.Pid,
		Created: c.Created,
	}
}

// isNotFound reports whether err indicates a missing record in the store.
func isNotFound(err error) bool {
	if err == nil {
		return false
	}
	// waystation.ErrNotFound has message "not found"; we check by string to avoid
	// importing waystation here (keeping gan independent).
	return errors.Is(err, errNotFound) || err.Error() == "not found"
}

// errNotFound is a sentinel used internally to detect waystation.ErrNotFound.
var errNotFound = errors.New("not found")
