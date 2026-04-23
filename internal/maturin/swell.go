package maturin

import (
	"context"
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/go-digest"

	"github.com/garnizeh/maestro/internal/prim"
	"github.com/garnizeh/maestro/pkg/archive"
)

// Swell extracts the layers of the image identified by refStr into the prim
// snapshotter. It builds a chain of committed snapshots for each layer.
// Returns the key of the top-most layer snapshot.
func (s *Store) Swell(ctx context.Context, refStr string, p prim.Prim) (string, error) {
	ref, err := name.ParseReference(refStr)
	if err != nil {
		return "", fmt.Errorf("swell: parse reference: %w", err)
	}
	log.Debug().Str("ref", refStr).Msg("maturin: swelling image")

	dgst, err := s.resolveRef(ref)
	if err != nil {
		return "", fmt.Errorf("swell: resolve tag: %w", err)
	}

	mf, _, _, _, err := s.parseManifestAndConfig(dgst)
	if err != nil {
		return "", fmt.Errorf("swell: parse manifest: %w", err)
	}

	var parentKey string
	for i, layer := range mf.Layers {
		layerDgst := digest.Digest(layer.Digest)
		key := "layer-" + layerDgst.Encoded()
		log.Debug().Int("layer", i).Str("key", key).Msg("maturin: processing layer")

		exists, existsErr := s.layerExists(ctx, p, key)
		if existsErr != nil {
			return "", fmt.Errorf("swell: check layer %d: %w", i, existsErr)
		}

		if !exists {
			log.Info().
				Int("layer", i).
				Str("key", key).
				Msg("maturin: layer not found, starting extraction")
			if procErr := s.processLayer(ctx, p, i, layerDgst, key, parentKey); procErr != nil {
				return "", procErr
			}
		} else {
			log.Info().Int("layer", i).Str("key", key).Msg("maturin: layer already exists, skipping")
		}

		parentKey = key
	}

	return parentKey, nil
}

func (s *Store) layerExists(ctx context.Context, p prim.Prim, key string) (bool, error) {
	exists := false
	walkErr := p.Walk(ctx, func(info prim.Info) error {
		if info.Key == key {
			exists = true
		}
		return nil
	})
	return exists, walkErr
}

func (s *Store) processLayer(
	ctx context.Context,
	p prim.Prim,
	i int,
	layerDgst digest.Digest,
	key, parentKey string,
) error {
	// Prepare a temporary snapshot for extraction.
	// We use a predictable name so we can cleanup stale attempts.
	tmpKey := fmt.Sprintf("tmp-swell-%d-%s", i, layerDgst.Encoded()[:12])
	log.Debug().
		Str("tmp_key", tmpKey).
		Str("parent", parentKey).
		Msg("maturin: preparing temporary snapshot")

	// Cleaning up stale attempts ensures we don't fail Prepare on second pull.
	if errRem := p.Remove(ctx, tmpKey); errRem != nil &&
		!errors.Is(errRem, prim.ErrSnapshotNotFound) {
		log.Debug().Err(errRem).Str("tmp_key", tmpKey).
			Msg("maturin: failed to remove stale cleanup snapshot before swell")
	}

	mounts, err := p.Prepare(ctx, tmpKey, parentKey)
	if err != nil {
		return fmt.Errorf("swell: prepare layer %d: %w", i, err)
	}

	if len(mounts) == 0 {
		return fmt.Errorf("swell: no mount point for layer %d", i)
	}

	// Perform extraction
	extractErr := s.extractLayer(layerDgst, p.WritableDir(tmpKey), p.WhiteoutFormat())
	if extractErr != nil {
		if cleanupErr := p.Remove(ctx, tmpKey); cleanupErr != nil {
			log.Debug().Err(cleanupErr).Str("tmp_key", tmpKey).
				Msg("maturin: failed to remove temp snapshot after extraction error")
		}
		return fmt.Errorf("swell: extract layer %d: %w", i, extractErr)
	}

	// Commit the temporary snapshot as a permanent layer.
	// We do a best-effort Remove of the target key first; if a directory
	// exists but is invalid (which is why layerExists returned false),
	// this will clean it up and allow the Commit (rename) to succeed.
	// Ensure target key doesn't exist. If it appeared while we were extracting,
	// we just cleanup and return success (idempotency).
	exists, errExists := s.layerExists(ctx, p, key)
	if errExists != nil {
		return fmt.Errorf("swell: check layer exists before commit: %w", errExists)
	}
	if exists {
		log.Warn().Str("key", key).Msg("maturin: layer appeared during extraction, skipping commit")
		if cleanupErr := p.Remove(ctx, tmpKey); cleanupErr != nil {
			log.Debug().Err(cleanupErr).Str("tmp_key", tmpKey).
				Msg("maturin: failed to remove temp snapshot after idempotency skip")
		}
		return nil
	}

	log.Debug().Str("tmp_key", tmpKey).Str("key", key).Msg("maturin: committing layer")
	if commitErr := p.Commit(ctx, key, tmpKey); commitErr != nil {
		if cleanupErr := p.Remove(ctx, tmpKey); cleanupErr != nil {
			log.Debug().Err(cleanupErr).Str("tmp_key", tmpKey).
				Msg("maturin: failed to remove temp snapshot after commit failure")
		}
		return fmt.Errorf("swell: commit layer %d: %w", i, commitErr)
	}

	return nil
}

func (s *Store) extractLayer(
	dgst digest.Digest,
	targetDir string,
	format archive.WhiteoutFormat,
) error {
	rc, err := s.Get(dgst)
	if err != nil {
		return fmt.Errorf("get blob %s: %w", dgst, err)
	}
	defer rc.Close()

	log.Debug().Str("dgst", dgst.String()).Str("target", targetDir).
		Msg("maturin: extracting layer contents")

	return s.extractor.Extract(rc, targetDir, archive.ExtractOptions{
		WhiteoutFormat: format,
	})
}
