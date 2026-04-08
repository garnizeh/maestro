package maturin

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/go-containerregistry/pkg/name"
	"github.com/opencontainers/go-digest"
)

// shortIDLen is the number of hex characters used for a short image ID.
const shortIDLen = 12

// ImageSummary holds the display metadata for a locally stored tagged image.
type ImageSummary struct {
	Repository string        `json:"Repository"` // e.g. "docker.io/library/nginx"
	Tag        string        `json:"Tag"`        // e.g. "latest"
	Digest     digest.Digest `json:"Digest"`     // manifest digest
	ShortID    string        `json:"ShortID"`    // first 12 hex chars of manifest digest hex
	Created    time.Time     `json:"Created"`    // from OCI config JSON "created" field
	Size       int64         `json:"Size"`       // sum of compressed layer sizes from manifest
}

// HistoryEntry is one step in an image's layer creation history.
type HistoryEntry struct {
	Created    time.Time `json:"Created"`
	CreatedBy  string    `json:"CreatedBy,omitempty"`
	Size       int64     `json:"Size"`
	Comment    string    `json:"Comment,omitempty"`
	EmptyLayer bool      `json:"EmptyLayer,omitempty"`
}

// InspectResult holds combined manifest and config data for [Store.InspectImage].
type InspectResult struct {
	Ref          string          `json:"Ref"`
	ID           string          `json:"Id"`
	RepoTag      string          `json:"RepoTag"`
	Created      time.Time       `json:"Created"`
	Architecture string          `json:"Architecture"`
	Os           string          `json:"Os"`
	Size         int64           `json:"Size"`
	Manifest     json.RawMessage `json:"Manifest"`
	Config       json.RawMessage `json:"Config"`
}

// ── internal JSON shapes ──────────────────────────────────────────────────────

type ociManifest struct {
	Config struct {
		Digest string `json:"digest"`
		Size   int64  `json:"size"`
	} `json:"config"`
	Layers []struct {
		Size   int64  `json:"size"`
		Digest string `json:"digest"`
	} `json:"layers"`
}

type ociConfig struct {
	Created      time.Time    `json:"created"`
	Architecture string       `json:"architecture"`
	Os           string       `json:"os"`
	History      []ociHistory `json:"history"`
}

type ociHistory struct {
	Created    time.Time `json:"created"`
	CreatedBy  string    `json:"created_by"`
	Comment    string    `json:"comment"`
	EmptyLayer bool      `json:"empty_layer"`
}

// ── helpers ───────────────────────────────────────────────────────────────────

// readBlob reads all bytes of a CAS blob into memory.
func (s *Store) readBlob(dgst digest.Digest) ([]byte, error) {
	rc, err := s.Get(dgst)
	if err != nil {
		return nil, err
	}
	defer rc.Close()
	return io.ReadAll(rc)
}

// resolveRef resolves a parsed reference to a manifest digest by looking up
// the tag symlink in the manifest store.
func (s *Store) resolveRef(ref name.Reference) (digest.Digest, error) {
	tag, isTag := ref.(name.Tag)
	if !isTag {
		return "", fmt.Errorf("only tag references are supported (got %q)", ref.String())
	}
	return s.ResolveTag(ref.Context().RegistryStr(), ref.Context().RepositoryStr(), tag.TagStr())
}

// parseManifestAndConfig parses the manifest blob at dgst and returns the
// parsed manifest, raw manifest bytes, parsed config, and raw config bytes.
func (s *Store) parseManifestAndConfig(dgst digest.Digest) (ociManifest, []byte, ociConfig, []byte, error) {
	rawMan, err := s.readBlob(dgst)
	if err != nil {
		return ociManifest{}, nil, ociConfig{}, nil, fmt.Errorf("read manifest: %w", err)
	}

	var mf ociManifest
	if unmarshalErr := json.Unmarshal(rawMan, &mf); unmarshalErr != nil {
		return ociManifest{}, nil, ociConfig{}, nil, fmt.Errorf("parse manifest: %w", unmarshalErr)
	}

	configDgst := digest.Digest(mf.Config.Digest)
	rawCfg, cfgErr := s.readBlob(configDgst)
	if cfgErr != nil {
		return mf, rawMan, ociConfig{}, nil, fmt.Errorf("read config: %w", cfgErr)
	}

	var cfg ociConfig
	if unmarshalErr := json.Unmarshal(rawCfg, &cfg); unmarshalErr != nil {
		return mf, rawMan, ociConfig{}, nil, fmt.Errorf("parse config: %w", unmarshalErr)
	}

	return mf, rawMan, cfg, rawCfg, nil
}

// ── public API ────────────────────────────────────────────────────────────────

// ListImages walks the manifest symlink tree and returns metadata for all
// tagged images currently stored locally. Returns nil (not an error) if no
// images have been pulled yet.
//
// The manifest tree layout is:
//
//	manifests/<registry>/<repo-component...>/<tag>
//
// Because RepositoryStr() can return multi-component paths (e.g. "library/nginx"),
// the depth between registry and tag is variable. WalkDir handles all depths.
func (s *Store) ListImages(_ context.Context) ([]ImageSummary, error) {
	manifestsRoot := filepath.Join(s.root, "maturin", "manifests")

	var summaries []ImageSummary

	err := filepath.WalkDir(manifestsRoot, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			if os.IsNotExist(walkErr) {
				return fs.SkipAll
			}
			return walkErr
		}
		if d.IsDir() || strings.HasSuffix(path, ".tmp") {
			return nil
		}

		rel, relErr := filepath.Rel(manifestsRoot, path)
		if relErr != nil {
			return relErr //coverage:ignore filepath.Rel only errors when paths have different roots
		}

		parts := strings.Split(rel, string(filepath.Separator))
		const minParts = 3 // registry + at least one repo component + tag
		if len(parts) < minParts {
			return nil
		}

		registry := parts[0]
		tag := parts[len(parts)-1]
		repo := strings.Join(parts[1:len(parts)-1], "/")

		summary, infoErr := s.imageInfoFromTag(registry, repo, tag)
		if infoErr != nil {
			return nil //nolint:nilerr // skip malformed entries silently
		}
		summaries = append(summaries, summary)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("list manifests: %w", err)
	}

	return summaries, nil
}

func (s *Store) imageInfoFromTag(registry, repo, tag string) (ImageSummary, error) {
	dgst, err := s.ResolveTag(registry, repo, tag)
	if err != nil {
		return ImageSummary{}, err
	}

	mf, _, cfg, _, err := s.parseManifestAndConfig(dgst)
	if err != nil {
		return ImageSummary{}, err
	}

	var size int64
	for _, layer := range mf.Layers {
		size += layer.Size
	}

	shortID := ""
	if len(dgst.Hex()) >= shortIDLen {
		shortID = dgst.Hex()[:shortIDLen]
	}

	return ImageSummary{
		Repository: registry + "/" + repo,
		Tag:        tag,
		Digest:     dgst,
		ShortID:    shortID,
		Created:    cfg.Created,
		Size:       size,
	}, nil
}

// InspectImage returns combined manifest and config information for the given
// image reference (e.g. "nginx:latest").
func (s *Store) InspectImage(refStr string) (*InspectResult, error) {
	ref, parseErr := name.ParseReference(refStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse reference: %w", parseErr)
	}

	dgst, err := s.resolveRef(ref)
	if err != nil {
		return nil, err
	}

	mf, rawMan, cfg, rawCfg, err := s.parseManifestAndConfig(dgst)
	if err != nil {
		return nil, err
	}

	var size int64
	for _, layer := range mf.Layers {
		size += layer.Size
	}

	shortID := ""
	if len(dgst.Hex()) >= shortIDLen {
		shortID = dgst.Hex()[:shortIDLen]
	}

	return &InspectResult{
		Ref:          refStr,
		ID:           shortID,
		RepoTag:      ref.String(),
		Created:      cfg.Created,
		Architecture: cfg.Architecture,
		Os:           cfg.Os,
		Size:         size,
		Manifest:     json.RawMessage(rawMan),
		Config:       json.RawMessage(rawCfg),
	}, nil
}

// ImageHistory returns the layer history for the given image reference.
func (s *Store) ImageHistory(refStr string) ([]HistoryEntry, error) {
	ref, parseErr := name.ParseReference(refStr)
	if parseErr != nil {
		return nil, fmt.Errorf("parse reference: %w", parseErr)
	}

	dgst, err := s.resolveRef(ref)
	if err != nil {
		return nil, err
	}

	mf, _, cfg, _, err := s.parseManifestAndConfig(dgst)
	if err != nil {
		return nil, err
	}

	// Build a slice of layer sizes for non-empty history entries.
	layerSizes := make([]int64, len(mf.Layers))
	for i, l := range mf.Layers {
		layerSizes[i] = l.Size
	}

	entries := make([]HistoryEntry, 0, len(cfg.History))
	layerIdx := 0
	for _, h := range cfg.History {
		e := HistoryEntry{
			Created:    h.Created,
			CreatedBy:  h.CreatedBy,
			Comment:    h.Comment,
			EmptyLayer: h.EmptyLayer,
		}
		if !h.EmptyLayer && layerIdx < len(layerSizes) {
			e.Size = layerSizes[layerIdx]
			layerIdx++
		}
		entries = append(entries, e)
	}

	return entries, nil
}

// RemoveImage removes a tagged image from the local store. It deletes the tag
// symlink and removes the descriptor from index.json. Blob files are not
// deleted (they may be referenced by other images or future pull verification).
func (s *Store) RemoveImage(ctx context.Context, refStr string) error {
	ref, parseErr := name.ParseReference(refStr)
	if parseErr != nil {
		return fmt.Errorf("parse reference: %w", parseErr)
	}

	tag, isTag := ref.(name.Tag)
	if !isTag {
		return errors.New("rm requires a tag reference (digest references are not yet supported)")
	}

	registry := ref.Context().RegistryStr()
	repo := ref.Context().RepositoryStr()
	tagStr := tag.TagStr()

	dgst, err := s.ResolveTag(registry, repo, tagStr)
	if err != nil {
		return err
	}

	linkPath := s.tagLinkPath(registry, repo, tagStr)
	if removeErr := os.Remove(linkPath); removeErr != nil && !os.IsNotExist(removeErr) {
		return fmt.Errorf("remove tag symlink: %w", removeErr)
	}

	if indexErr := s.RemoveFromIndex(ctx, dgst); indexErr != nil {
		return fmt.Errorf("remove from index: %w", indexErr)
	}

	return nil
}
