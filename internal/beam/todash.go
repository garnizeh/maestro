package beam

import (
	"fmt"
	"path/filepath"

	"github.com/rs/zerolog/log"
)

// Todash manages network namespace lifecycles for Maestro.
// "Todash space is the darkness between worlds" - The Dark Tower.
type Todash struct {
	basePath string
	fs       FS
	mounter  Mounter
}

// NewTodash creates a new Todash namespace manager.
func NewTodash(basePath string) *Todash {
	return &Todash{
		basePath: basePath,
		fs:       RealFS{},
		mounter:  newDefaultMounter(),
	}
}

// WithFS sets a custom filesystem implementation.
func (t *Todash) WithFS(fs FS) *Todash {
	t.fs = fs
	if rm, ok := t.mounter.(interface{ SetFS(FS) }); ok {
		rm.SetFS(fs)
	}
	return t
}

// WithMounter sets a custom mounter implementation.
func (t *Todash) WithMounter(m Mounter) *Todash {
	t.mounter = m
	if rm, ok := m.(interface{ SetFS(FS) }); ok {
		rm.SetFS(t.fs)
	}
	return t
}

// WithRootless enables or disables rootless mode for namespace operations.
func (t *Todash) WithRootless(rootless bool) namespaceManager {
	if rm, ok := t.mounter.(*RealMounter); ok {
		rm.rootless = rootless
	}
	return t
}

// NewNS creates a new persistent network namespace for a container.
// It returns the absolute path to the persistent bind mount and the launcher socket (if rootless).
func (t *Todash) NewNS(id string, mount *MountRequest) (string, string, error) {
	nsPath := t.NSPath(id)
	if errMkdir := t.fs.MkdirAll(t.basePath, dirPerm); errMkdir != nil {
		return "", "", fmt.Errorf("failed to create netns directory %s: %w", t.basePath, errMkdir)
	}

	nsPath, launcherPath, err := t.mounter.NewNS(nsPath, mount)
	if err == nil {
		log.Debug().Str("id", id).Str("ns", nsPath).Msg("todash: created network namespace")
	}
	return nsPath, launcherPath, err
}

// DeleteNS unmounts and removes the persistent network namespace file.
func (t *Todash) DeleteNS(id string) error {
	nsPath := filepath.Join(t.basePath, id)
	err := t.mounter.DeleteNS(nsPath)
	if err == nil {
		log.Debug().Str("id", id).Str("ns", nsPath).Msg("todash: deleted network namespace")
	}
	return err
}

// NSPath returns the expected bind-mount path for a namespace by container ID.
func (t *Todash) NSPath(id string) string {
	return filepath.Join(t.basePath, id)
}
