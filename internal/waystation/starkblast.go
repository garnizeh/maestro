// starkblast.go — schema versioning and migration for the Waystation store.
//
// In The Dark Tower, a Starkblast is a catastrophic supernatural storm.
// Here it represents the controlled destruction and rebuilding of data
// structures as the schema evolves — managed chaos, not uncontrolled.

package waystation

import (
	"errors"
	"fmt"

	"github.com/rs/zerolog/log"
)

const (
	// CurrentSchemaVersion is the schema version this binary understands.
	CurrentSchemaVersion = 1

	// metaCollection is where the schema metadata record lives.
	metaCollection = "meta"
	// metaKey is the key for the schema version record.
	metaKey = "schema"
)

// schemaMeta is persisted to track the state store schema version.
type schemaMeta struct {
	Version int `json:"version"`
}

// CheckAndMigrate reads the persisted schema version and runs any necessary
// migrations to bring the store up to CurrentSchemaVersion.
func (s *Store) CheckAndMigrate() error {
	var meta schemaMeta
	err := s.Get(metaCollection, metaKey, &meta)
	if err != nil && !errors.Is(err, ErrNotFound) {
		return fmt.Errorf("read schema version: %w", err)
	}

	if errors.Is(err, ErrNotFound) {
		// Fresh store — write the current version and done.
		return s.Put(metaCollection, metaKey, schemaMeta{Version: CurrentSchemaVersion})
	}

	if meta.Version == CurrentSchemaVersion {
		return nil
	}

	if meta.Version > CurrentSchemaVersion {
		return fmt.Errorf(
			"state store schema version %d is newer than this binary (%d); upgrade maestro",
			meta.Version, CurrentSchemaVersion,
		)
	}

	// Run migrations from meta.Version up to CurrentSchemaVersion.
	for v := meta.Version; v < CurrentSchemaVersion; v++ {
		log.Info().Int("from", v).Int("to", v+1).Msg("migrating state store schema")
		if migrateErr := migrate(s, v, v+1); migrateErr != nil {
			return fmt.Errorf(
				"migrate schema v%d→v%d: %w",
				v,
				v+1,
				migrateErr,
			) //coverage:ignore unreachable: all defined migrations return nil; future versions will add cases
		}
	}

	return s.Put(metaCollection, metaKey, schemaMeta{Version: CurrentSchemaVersion})
}

// SchemaVersion returns the currently persisted schema version, or 0 if unset.
func (s *Store) SchemaVersion() (int, error) {
	var meta schemaMeta
	if err := s.Get(metaCollection, metaKey, &meta); err != nil {
		if errors.Is(err, ErrNotFound) {
			return 0, nil
		}
		return 0, err
	}
	return meta.Version, nil
}

// migrate runs the migration from schema version `from` to `to`.
// Add cases here as the schema evolves.
func migrate(_ *Store, from, to int) error {
	switch {
	case from == 0 && to == 1:
		// v0→v1: initial schema, nothing to transform.
		return nil
	default:
		return fmt.Errorf(
			"no migration defined from v%d to v%d",
			from,
			to,
		) //coverage:ignore defensive guard; only reachable if CurrentSchemaVersion advances without a new case
	}
}
