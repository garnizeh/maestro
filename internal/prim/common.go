package prim

import (
	"context"
	"fmt"
	"path/filepath"
	"sync"

	"github.com/rs/zerolog/log"
)

// prepareHelper handles the common logic for creating snapshot directories and metadata.
func prepareHelper(
	_ context.Context,
	fs FS,
	mu sync.Locker,
	snapshotDir func(string) string,
	checkNotExists func(string) error,
	writeMeta func(string, VFSMeta) error,
	mounts func(string, string) ([]Mount, error),
	key, parent string,
) ([]Mount, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkNotExists(key); err != nil {
		return nil, err
	}

	snapDir := snapshotDir(key)
	if err := fs.MkdirAll(filepath.Join(snapDir, "work"), dirPerm); err != nil {
		return nil, fmt.Errorf("prepare %s: %w", key, err)
	}
	if err := fs.MkdirAll(filepath.Join(snapDir, "fs"), fsDirPerm); err != nil {
		return nil, fmt.Errorf("prepare %s: %w", key, err)
	}

	meta := VFSMeta{Key: key, Parent: parent, Kind: KindActive}
	if err := writeMeta(snapDir, meta); err != nil {
		if rmErr := fs.RemoveAll(snapDir); rmErr != nil {
			log.Warn().Err(rmErr).Str("snapDir", snapDir).
				Msg("prim: failed to cleanup directory after meta write failure")
		}
		return nil, fmt.Errorf("prepare %s: write meta: %w", key, err)
	}

	log.Debug().Str("key", key).Str("parent", parent).Msg("prim: snapshot prepared")

	return mounts(key, parent)
}

// viewHelper handles the common logic for creating read-only snapshots.
func viewHelper(
	_ context.Context,
	fs FS,
	mu sync.Locker,
	snapshotDir func(string) string,
	checkNotExists func(string) error,
	writeMeta func(string, VFSMeta) error,
	mounts func(string, string) ([]Mount, error),
	key, parent string,
	driverName string,
) ([]Mount, error) {
	mu.Lock()
	defer mu.Unlock()

	if err := checkNotExists(key); err != nil {
		return nil, err
	}

	snapDir := snapshotDir(key)
	if err := fs.MkdirAll(filepath.Join(snapDir, "fs"), fsDirPerm); err != nil {
		return nil, fmt.Errorf("%s: view %s: %w", driverName, key, err)
	}

	meta := VFSMeta{Key: key, Parent: parent, Kind: KindView}
	if err := writeMeta(snapDir, meta); err != nil {
		if rmErr := fs.RemoveAll(snapDir); rmErr != nil {
			log.Warn().Err(rmErr).Str("snapDir", snapDir).
				Msgf("%s: failed to cleanup view directory after meta write failure", driverName)
		}
		return nil, fmt.Errorf("%s: view %s: write meta: %w", driverName, key, err)
	}

	log.Debug().Str("key", key).Str("parent", parent).Str("driver", driverName).
		Msg("prim: view snapshot created")

	return mounts(key, parent)
}
