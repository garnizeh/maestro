# Waystation State Specification

## Purpose

Waystation is the file-based state store for Maestro. It provides atomic CRUD operations for container state, image metadata, network configurations, and volume metadata. Without a daemon, all persistent state lives on the filesystem under Waystation's governance. Waystation includes Khef (flock-based locking) for concurrent access safety and Starkblast (schema migrations) for evolving the state store over time.

> *"The Way Station is a simple, durable structure in the desert that holds what travelers need."*

---

## Requirements

### Requirement: Directory Structure Initialization

Waystation MUST initialize its directory structure on first use. The root directory MUST default to the XDG data directory (`~/.local/share/maestro/`). The following subdirectories MUST be created during initialization:

- `containers/` -- container state and bundle storage
- `maturin/` -- image store (blobs, manifests, index)
- `dogan/` -- volume metadata and data
- `beam/` -- network configurations
- `thinnies/` -- lock files for concurrent access coordination

Waystation MUST also record a version file at the root to track the state schema version.

#### Scenario: First-run directory creation

GIVEN the Waystation root directory does not exist
WHEN Waystation is initialized
THEN all required subdirectories (`containers`, `maturin`, `dogan`, `beam`, `thinnies`) MUST be created under the root directory
AND a version file MUST be created at the root indicating the current schema version

#### Scenario: Idempotent initialization

GIVEN the Waystation root directory already exists with all required subdirectories
WHEN Waystation is initialized again
THEN no error MUST be returned
AND existing directory contents MUST NOT be modified

#### Scenario: Partial directory structure recovery

GIVEN the Waystation root directory exists but is missing some subdirectories (e.g., `beam/` is absent)
WHEN Waystation is initialized
THEN only the missing subdirectories MUST be created
AND existing subdirectories and their contents MUST NOT be modified

---

### Requirement: XDG Base Directory Compliance

Waystation MUST comply with the XDG Base Directory Specification for determining default paths.

- State data MUST default to `$XDG_DATA_HOME/maestro/` (typically `~/.local/share/maestro/`)
- If `$XDG_DATA_HOME` is not set, Waystation MUST fall back to `$HOME/.local/share/maestro/`
- The root path MUST be overridable via the `--root` CLI flag
- The root path MUST be overridable via the `storage.root` field in `katet.toml`

#### Scenario: Default path when XDG_DATA_HOME is set

GIVEN the environment variable `XDG_DATA_HOME` is set to `/custom/data`
WHEN Waystation resolves its root path with no explicit override
THEN the root path MUST be `/custom/data/maestro/`

#### Scenario: Default path when XDG_DATA_HOME is not set

GIVEN the environment variable `XDG_DATA_HOME` is not set
AND the user's home directory is `/home/user`
WHEN Waystation resolves its root path with no explicit override
THEN the root path MUST be `/home/user/.local/share/maestro/`

#### Scenario: CLI flag overrides XDG default

GIVEN the environment variable `XDG_DATA_HOME` is set to `/custom/data`
WHEN Waystation is initialized with `--root /explicit/path`
THEN the root path MUST be `/explicit/path`

#### Scenario: Config file overrides XDG default

GIVEN the `katet.toml` file contains `storage.root = "/toml/path"`
AND no `--root` CLI flag is provided
WHEN Waystation resolves its root path
THEN the root path MUST be `/toml/path`

#### Scenario: CLI flag takes precedence over config file

GIVEN the `katet.toml` file contains `storage.root = "/toml/path"`
AND the `--root` CLI flag is set to `/flag/path`
WHEN Waystation resolves its root path
THEN the root path MUST be `/flag/path`

---

### Requirement: Permission Enforcement

Waystation MUST enforce strict filesystem permissions to protect state data from unauthorized access.

- All directories created by Waystation MUST have permissions `0700` (owner read/write/execute only)
- All files created by Waystation MUST have permissions `0600` (owner read/write only)
- Waystation MUST verify permissions on initialization and SHOULD warn if existing directories or files have overly permissive modes

#### Scenario: Directory creation with correct permissions

GIVEN Waystation is initializing its directory structure
WHEN any directory is created
THEN that directory MUST have permissions `0700`

#### Scenario: File creation with correct permissions

GIVEN a container state file is being written
WHEN the file is created on disk
THEN that file MUST have permissions `0600`

#### Scenario: Warning on overly permissive existing directory

GIVEN the Waystation root directory exists with permissions `0755`
WHEN Waystation is initialized
THEN Waystation SHOULD emit a warning indicating that the directory has overly permissive mode
AND initialization MUST still succeed

#### Scenario: Temporary files inherit correct permissions

GIVEN a write operation creates a temporary file before atomic rename
WHEN the temporary file is created
THEN the temporary file MUST have permissions `0600`

---

### Requirement: Atomic Write Operations

All write operations in Waystation MUST be atomic. Waystation MUST use the write-to-temp-then-rename pattern to ensure that no partial writes are visible to concurrent readers or survive a crash.

- Data MUST first be written to a temporary file in the same directory as the target
- The temporary file MUST be flushed and synced to disk before rename
- The temporary file MUST then be renamed to the target path using an atomic rename operation
- If the write fails at any point before rename, the target file MUST remain unchanged

#### Scenario: Successful atomic write

GIVEN a container state file exists at `containers/<id>/state.json` with status "created"
WHEN the state is updated to "running"
THEN a temporary file MUST be written in the same directory
AND the temporary file MUST be renamed to `state.json`
AND any reader during the operation MUST see either the old complete state or the new complete state, never a partial write

#### Scenario: Write failure before rename preserves original

GIVEN a container state file exists with valid content
WHEN a write operation fails during the temporary file write (e.g., disk full)
THEN the original state file MUST remain unchanged
AND the temporary file MUST be cleaned up if partially written

#### Scenario: Crash during write does not corrupt state

GIVEN a container state file exists with valid content
WHEN the process crashes after writing the temporary file but before renaming
THEN on the next initialization, the original state file MUST still contain the previous valid content
AND any orphaned temporary files SHOULD be cleaned up

#### Scenario: Temporary file is in the same filesystem

GIVEN a write operation targets a file on a specific filesystem
WHEN the temporary file is created
THEN it MUST be created in the same directory as the target file to ensure the rename is atomic (same filesystem requirement)

---

### Requirement: Container State Persistence

Waystation MUST persist container state as JSON files under `containers/<id>/state.json`. The state file MUST conform to the `maestro/container-state/v1` schema.

Each container directory MUST contain:

- `state.json` -- the container's current state and metadata
- `config.json` -- the OCI runtime spec used to create the container
- `bundle/` -- the container's rootfs and runtime bundle

The state file MUST include at minimum: container ID, name, image reference, Ka status (created/running/paused/stopped/deleted), process information, configuration, mount information, network information, resource limits, security settings, runtime information, Cort PID, log path, and creation timestamp.

#### Scenario: Create new container state

GIVEN no state exists for container ID `abc123`
WHEN a new container is created
THEN a directory `containers/abc123/` MUST be created
AND a file `containers/abc123/state.json` MUST be written with the container's initial state
AND the `ka.status` field MUST be set to `"created"`
AND the `created_at` field MUST be set to the current UTC timestamp in RFC 3339 format

#### Scenario: Read existing container state

GIVEN a valid state file exists at `containers/abc123/state.json`
WHEN the state for container `abc123` is read
THEN all fields from the state file MUST be deserialized and returned without modification

#### Scenario: Update container state

GIVEN a container with ID `abc123` has Ka status `"created"`
WHEN the container transitions to `"running"`
THEN the state file MUST be atomically updated
AND the `ka.status` field MUST be `"running"`
AND the `ka.pid` field MUST contain the container process PID
AND the `ka.started_at` field MUST be set to the current UTC timestamp

#### Scenario: Delete container state

GIVEN a container with ID `abc123` has Ka status `"stopped"`
WHEN the container is removed
THEN the entire directory `containers/abc123/` MUST be removed
AND the container MUST no longer appear in container listings

#### Scenario: List all containers

GIVEN three containers exist with IDs `aaa`, `bbb`, and `ccc`
WHEN all container states are listed
THEN all three state files MUST be read and returned
AND containers with corrupted state files SHOULD be reported as errors but MUST NOT prevent listing of valid containers

#### Scenario: Container state file with unknown fields

GIVEN a state file contains fields not recognized by the current schema version
WHEN the state is read
THEN unknown fields MUST be preserved on read and written back on update (round-trip safety)

#### Scenario: Container state file is missing

GIVEN the directory `containers/abc123/` exists but `state.json` is absent
WHEN the state for container `abc123` is read
THEN an error MUST be returned indicating the state file is missing

#### Scenario: Container ID uniqueness

GIVEN a container with ID `abc123` already exists
WHEN a new container is created with the same ID
THEN an error MUST be returned indicating a duplicate container ID

---

### Requirement: Image Metadata Persistence

Waystation MUST persist image metadata within the `maturin/` subtree. The image store MUST follow the OCI Image Layout structure with additional Maestro metadata.

The image store MUST contain:

- `blobs/sha256/` -- content-addressable blob storage keyed by digest
- `manifests/<registry>/<repository>/<tag>` -- symlinks from tags to manifest digests
- `index.json` -- the local OCI image index tracking all stored images

#### Scenario: Store image metadata after pull

GIVEN an image `docker.io/library/nginx:latest` with digest `sha256:abc123` has been pulled
WHEN image metadata is persisted
THEN a symlink MUST exist at `maturin/manifests/docker.io/library/nginx/latest` pointing to the manifest digest
AND the digest MUST be recorded in `maturin/index.json`
AND all blob data MUST be stored under `maturin/blobs/sha256/`

#### Scenario: Multiple tags for same digest

GIVEN an image manifest has digest `sha256:abc123`
WHEN two tags `latest` and `1.25` are both associated with this digest
THEN both symlinks MUST point to the same manifest digest
AND blobs MUST NOT be duplicated

#### Scenario: Atomic index.json update

GIVEN `maturin/index.json` currently tracks 5 images
WHEN a new image is pulled
THEN `index.json` MUST be updated atomically using the write-to-temp-then-rename pattern
AND a concurrent reader MUST see either the old index (5 images) or the new index (6 images)

#### Scenario: Remove image from store

GIVEN an image with tag `nginx:latest` is stored locally
WHEN the image is removed
THEN the tag symlink MUST be deleted
AND if no other tags reference the same manifest digest, the manifest blob MAY be removed
AND `index.json` MUST be updated atomically to remove the entry

---

### Requirement: Network Configuration Persistence

Waystation MUST persist network configurations as JSON files under `beam/<name>/config.json`. Each network configuration MUST conform to the `maestro/beam/v1` schema.

#### Scenario: Create network configuration

GIVEN no network named `my-beam` exists
WHEN a network `my-beam` is created with driver `bridge` and subnet `10.100.0.0/24`
THEN a directory `beam/my-beam/` MUST be created
AND a file `beam/my-beam/config.json` MUST be written with the network's configuration
AND the file MUST include the CNI conflist (Guardian config), driver type, subnet, and creation timestamp

#### Scenario: Read network configuration

GIVEN a valid config file exists at `beam/my-beam/config.json`
WHEN the configuration for network `my-beam` is read
THEN all fields MUST be deserialized and returned without modification

#### Scenario: Update network with connected container

GIVEN a network `my-beam` exists with no connected containers
WHEN container `abc123` is connected to the network
THEN the config file MUST be atomically updated
AND the `connected_containers` field MUST include `abc123`

#### Scenario: Delete network configuration

GIVEN a network `my-beam` exists with no connected containers
WHEN the network is removed
THEN the directory `beam/my-beam/` MUST be removed

#### Scenario: Prevent deletion of network with connected containers

GIVEN a network `my-beam` has containers connected to it
WHEN deletion of the network is attempted
THEN an error MUST be returned indicating that containers are still connected

#### Scenario: List all networks

GIVEN networks `beam0` and `my-beam` exist
WHEN all networks are listed
THEN both configurations MUST be returned

---

### Requirement: Volume Metadata Persistence

Waystation MUST persist volume metadata as JSON files under `dogan/<name>/meta.json`. Volume data MUST be stored under `dogan/<name>/data/`. Each volume metadata file MUST conform to the `maestro/dogan/v1` schema.

#### Scenario: Create volume

GIVEN no volume named `my-dogan` exists
WHEN a volume `my-dogan` is created
THEN a directory `dogan/my-dogan/` MUST be created
AND a subdirectory `dogan/my-dogan/data/` MUST be created for actual volume data
AND a file `dogan/my-dogan/meta.json` MUST be written with volume metadata
AND the `mountpoint` field MUST reflect the absolute path to the `data/` directory

#### Scenario: Read volume metadata

GIVEN a valid metadata file exists at `dogan/my-dogan/meta.json`
WHEN the metadata for volume `my-dogan` is read
THEN all fields MUST be deserialized and returned without modification

#### Scenario: Update volume usage tracking

GIVEN a volume `my-dogan` is not used by any container
WHEN container `abc123` mounts the volume
THEN the metadata MUST be atomically updated
AND the `used_by` field MUST include `abc123`
AND the `last_used_at` field MUST be updated to the current UTC timestamp

#### Scenario: Delete volume

GIVEN a volume `my-dogan` exists and is not used by any container
WHEN the volume is removed
THEN the entire directory `dogan/my-dogan/` MUST be removed including data

#### Scenario: Prevent deletion of volume in use

GIVEN a volume `my-dogan` is currently mounted by container `abc123`
WHEN deletion of the volume is attempted
THEN an error MUST be returned indicating that the volume is in use by container `abc123`

#### Scenario: Volume name uniqueness

GIVEN a volume named `my-dogan` already exists
WHEN creation of another volume named `my-dogan` is attempted
THEN an error MUST be returned indicating a duplicate volume name

---

### Requirement: Khef Read Lock

Khef MUST provide shared read locks for inspection operations. Multiple concurrent readers MUST be able to hold read locks on the same resource simultaneously. Read locks MUST NOT block other read locks. Read locks MUST block write locks on the same resource.

Lock files MUST be stored at `thinnies/<resource-type>/<id>.lock` (e.g., `thinnies/containers/abc123.lock`).

#### Scenario: Multiple concurrent readers

GIVEN two processes both need to read the state of container `abc123`
WHEN both processes acquire a read lock on `thinnies/containers/abc123.lock`
THEN both locks MUST be granted simultaneously
AND both processes MUST be able to read the state concurrently

#### Scenario: Read lock blocks write lock

GIVEN process A holds a read lock on container `abc123`
WHEN process B attempts to acquire a write lock on the same container
THEN process B MUST block until process A releases the read lock

#### Scenario: Read lock does not block other reads

GIVEN process A holds a read lock on container `abc123`
WHEN process B attempts to acquire a read lock on the same container
THEN process B MUST immediately acquire the lock without blocking

#### Scenario: Read lock on nonexistent resource

GIVEN no lock file exists for container `xyz999`
WHEN a read lock is requested for container `xyz999`
THEN the lock file MUST be created
AND the read lock MUST be granted

---

### Requirement: Khef Write Lock

Khef MUST provide exclusive write locks for mutation operations. Only one writer MUST hold a write lock on a given resource at any time. Write locks MUST block both read and write locks from other processes on the same resource.

#### Scenario: Exclusive write access

GIVEN process A holds a write lock on container `abc123`
WHEN process B attempts to acquire a write lock on the same container
THEN process B MUST block until process A releases the write lock

#### Scenario: Write lock blocks read lock

GIVEN process A holds a write lock on container `abc123`
WHEN process B attempts to acquire a read lock on the same container
THEN process B MUST block until process A releases the write lock

#### Scenario: Write lock on different resources is independent

GIVEN process A holds a write lock on container `abc123`
WHEN process B attempts to acquire a write lock on container `def456`
THEN process B MUST immediately acquire the lock without blocking

#### Scenario: Lock release on completion

GIVEN process A holds a write lock on container `abc123`
WHEN process A completes its mutation and releases the lock
THEN a waiting process B MUST be able to acquire the lock

---

### Requirement: Khef Lock Timeout

Khef MUST support configurable lock acquisition timeouts. If a lock cannot be acquired within the timeout period, the operation MUST fail with a descriptive error. The default timeout MUST be 30 seconds. The timeout MUST be configurable via `katet.toml`.

#### Scenario: Lock acquired within timeout

GIVEN process A holds a write lock on container `abc123`
AND the lock timeout is configured to 30 seconds
WHEN process B attempts to acquire a write lock on the same container
AND process A releases the lock after 5 seconds
THEN process B MUST successfully acquire the lock

#### Scenario: Lock acquisition times out

GIVEN process A holds a write lock on container `abc123` and does not release it
AND the lock timeout is configured to 2 seconds
WHEN process B attempts to acquire a write lock on the same container
THEN after 2 seconds, process B MUST receive an error indicating lock acquisition timed out
AND the error MUST identify the resource (`containers/abc123`) that could not be locked

#### Scenario: Custom timeout from configuration

GIVEN `katet.toml` contains `storage.lock_timeout = "10s"`
WHEN a lock acquisition attempt blocks
THEN the timeout MUST be 10 seconds, not the default 30 seconds

#### Scenario: Zero timeout means non-blocking

GIVEN the lock timeout is configured to 0
WHEN a lock cannot be immediately acquired
THEN the operation MUST fail immediately with a lock contention error

---

### Requirement: Khef Dead Lock Detection via PID

Khef MUST detect dead locks caused by processes that have terminated while holding a lock. Each lock file MUST contain the PID of the process that holds the lock. On lock contention, Khef MUST check whether the PID recorded in the lock file corresponds to a running process. If the PID does not correspond to a running process, the lock MUST be considered stale and MUST be forcibly released.

#### Scenario: Detect stale lock from crashed process

GIVEN process A (PID 12345) acquired a write lock on container `abc123` and then crashed
AND the lock file at `thinnies/containers/abc123.lock` still contains PID 12345
AND PID 12345 is no longer running
WHEN process B attempts to acquire a write lock on the same container
THEN Khef MUST detect that PID 12345 is not running
AND Khef MUST release the stale lock
AND process B MUST acquire the write lock

#### Scenario: PID is still running

GIVEN process A (PID 12345) holds a write lock on container `abc123`
AND PID 12345 is still running
WHEN process B attempts to acquire a write lock on the same container
THEN Khef MUST NOT forcibly release the lock
AND process B MUST wait or time out normally

#### Scenario: PID recycling safety

GIVEN process A (PID 12345) acquired a lock and then terminated
AND a new unrelated process has been assigned PID 12345
WHEN process B attempts to acquire the lock
THEN Khef SHOULD use additional heuristics (e.g., process start time, command name) to determine whether the PID holder is the original locker
AND if the heuristic determines the PID belongs to a different process, the lock MUST be considered stale

#### Scenario: Lock file records PID on acquisition

GIVEN process B (PID 67890) acquires a write lock on container `abc123`
WHEN the lock is established
THEN the lock file at `thinnies/containers/abc123.lock` MUST contain PID 67890

#### Scenario: Stale lock detection is logged

GIVEN a stale lock is detected for container `abc123` from terminated PID 12345
WHEN the stale lock is forcibly released
THEN a warning-level log message MUST be emitted indicating the stale lock was reclaimed

---

### Requirement: Khef Lock File Structure

Lock files MUST be organized under `thinnies/` by resource type and resource ID. The directory structure MUST be:

- `thinnies/containers/<id>.lock` -- container locks
- `thinnies/maturin/<digest>.lock` -- image locks
- `thinnies/dogan/<name>.lock` -- volume locks
- `thinnies/beam/<name>.lock` -- network locks

#### Scenario: Lock file for container resource

GIVEN a write lock is requested for container `abc123`
WHEN the lock file is created
THEN it MUST be located at `thinnies/containers/abc123.lock`

#### Scenario: Lock file for image resource

GIVEN a write lock is requested for image digest `sha256:def456`
WHEN the lock file is created
THEN it MUST be located at `thinnies/maturin/sha256:def456.lock` or an appropriately escaped variant

#### Scenario: Lock file for volume resource

GIVEN a write lock is requested for volume `my-dogan`
WHEN the lock file is created
THEN it MUST be located at `thinnies/dogan/my-dogan.lock`

#### Scenario: Lock file for network resource

GIVEN a write lock is requested for network `my-beam`
WHEN the lock file is created
THEN it MUST be located at `thinnies/beam/my-beam.lock`

#### Scenario: Lock subdirectories are created automatically

GIVEN the `thinnies/containers/` directory does not exist
WHEN a lock is requested for container `abc123`
THEN the `thinnies/containers/` directory MUST be created with permissions `0700`
AND the lock file MUST be created

---

### Requirement: Starkblast Schema Versioning

Starkblast MUST track the schema version of the Waystation state store. The version MUST be persisted in a version file at the Waystation root. Starkblast MUST ensure that the state store schema version is checked on every Waystation initialization.

#### Scenario: Fresh installation version tracking

GIVEN Waystation is being initialized for the first time
WHEN the directory structure is created
THEN a version file MUST be written containing the current schema version (e.g., `1`)

#### Scenario: Version file is read on initialization

GIVEN a Waystation with schema version `1` exists
WHEN Waystation is initialized
THEN Starkblast MUST read the version file
AND MUST confirm that the current schema version matches or exceeds the stored version

#### Scenario: Version file is missing

GIVEN the Waystation root exists but the version file is absent
WHEN Waystation is initialized
THEN Starkblast MUST treat this as a legacy (pre-versioning) state store
AND MUST attempt to infer the version or set it to version `1`
AND a warning SHOULD be emitted

---

### Requirement: Starkblast Automatic Migration

Starkblast MUST automatically apply pending migrations when the state store schema version is older than the expected version. Migrations MUST be applied sequentially in version order. Each migration MUST be atomic -- if a migration fails partway through, the state store MUST be left in the pre-migration state.

#### Scenario: Migration from v1 to v2

GIVEN a Waystation state store at schema version `1`
AND the current expected schema version is `2`
AND a migration from v1 to v2 is defined
WHEN Waystation is initialized
THEN Starkblast MUST detect the version mismatch
AND MUST apply the v1-to-v2 migration
AND MUST update the version file to `2`

#### Scenario: Multiple sequential migrations

GIVEN a Waystation state store at schema version `1`
AND the current expected schema version is `3`
AND migrations v1-to-v2 and v2-to-v3 are defined
WHEN Waystation is initialized
THEN Starkblast MUST apply v1-to-v2 first, then v2-to-v3
AND the version file MUST read `3` after completion

#### Scenario: Migration failure rolls back

GIVEN a Waystation state store at schema version `1`
AND the v1-to-v2 migration encounters an error partway through
WHEN the migration is attempted
THEN the state store MUST remain at version `1`
AND an error MUST be returned describing the migration failure
AND no partially migrated state MUST be visible

#### Scenario: No migration needed

GIVEN a Waystation state store at schema version `2`
AND the current expected schema version is `2`
WHEN Waystation is initialized
THEN no migrations MUST be applied
AND initialization MUST proceed normally

#### Scenario: Future version is rejected

GIVEN a Waystation state store at schema version `5`
AND the current expected schema version is `3`
WHEN Waystation is initialized
THEN an error MUST be returned indicating that the state store was created by a newer version of Maestro
AND no data MUST be modified

#### Scenario: Migration acquires exclusive lock

GIVEN a migration from v1 to v2 needs to run
WHEN Starkblast begins the migration
THEN an exclusive write lock MUST be held on the entire Waystation (global lock)
AND no other Waystation operations MUST proceed until the migration completes or fails

---

### Requirement: Concurrent Access Safety

Waystation MUST guarantee data integrity under concurrent access from multiple processes. Because Maestro is daemonless, multiple CLI invocations MAY access the state store simultaneously.

#### Scenario: Concurrent container create operations

GIVEN two processes attempt to create containers simultaneously
WHEN both processes write to the state store
THEN each container MUST be created successfully with its own ID
AND no container state MUST be corrupted or lost

#### Scenario: Concurrent read and write on same container

GIVEN process A is reading the state of container `abc123`
AND process B is updating the state of container `abc123` concurrently
WHEN both operations complete
THEN process A MUST have seen either the pre-update or post-update state (never partial)
AND process B's update MUST be fully persisted

#### Scenario: Concurrent state listing and container deletion

GIVEN process A is listing all containers
AND process B deletes container `abc123` concurrently
WHEN process A's listing completes
THEN the listing MUST either include or exclude `abc123` consistently
AND the listing MUST NOT crash or return corrupted data

#### Scenario: Concurrent index.json updates

GIVEN two image pull operations complete simultaneously and both need to update `maturin/index.json`
WHEN both operations attempt to update the index
THEN Khef locking MUST serialize the updates
AND both images MUST appear in the final index

#### Scenario: Orphaned temporary files are cleaned up

GIVEN a previous process crashed leaving a temporary file (e.g., `state.json.tmp.abc123`)
WHEN Waystation initializes or a subsequent write operation occurs on the same resource
THEN orphaned temporary files SHOULD be detected and removed

---

### Requirement: Container State Schema Validation

Waystation MUST validate the schema of container state files on read. State files that fail validation MUST result in a descriptive error. The `$schema` field MUST be checked to determine the appropriate validation rules.

#### Scenario: Valid state file is accepted

GIVEN a state file contains all required fields and conforms to `maestro/container-state/v1`
WHEN the state is read
THEN the state MUST be deserialized successfully

#### Scenario: State file with missing required field

GIVEN a state file is missing the required `id` field
WHEN the state is read
THEN an error MUST be returned indicating which required field is missing

#### Scenario: State file with invalid Ka status

GIVEN a state file contains `ka.status` set to `"flying"` (an invalid status)
WHEN the state is read
THEN an error MUST be returned indicating the invalid status value
AND the error MUST list the valid status values (created, running, paused, stopped, deleted)

#### Scenario: State file with unsupported schema version

GIVEN a state file has `$schema` set to `maestro/container-state/v99`
WHEN the state is read
THEN an error MUST be returned indicating the unsupported schema version

---

### Requirement: Bulk Operations

Waystation MUST support bulk operations for system-wide tasks such as pruning and garbage collection. Bulk operations MUST acquire appropriate locks on each resource before modifying it.

#### Scenario: Bulk container prune

GIVEN containers `aaa` (stopped), `bbb` (running), and `ccc` (stopped) exist
WHEN a prune operation targets stopped containers
THEN containers `aaa` and `ccc` MUST be removed
AND container `bbb` MUST NOT be affected
AND each removal MUST acquire and release its own lock

#### Scenario: Bulk operation with one locked resource

GIVEN containers `aaa` and `ccc` are stopped
AND another process holds a write lock on `aaa`
WHEN a prune operation targets stopped containers
THEN container `ccc` MUST be removed successfully
AND the operation SHOULD report that `aaa` was skipped due to lock contention
AND the operation MUST NOT fail entirely due to one locked resource

---

### Requirement: State Store Integrity Check

Waystation MUST provide a mechanism to verify the integrity of the state store. This check MUST detect corrupted state files, orphaned directories, and inconsistencies between resources (e.g., a container referencing a nonexistent volume).

#### Scenario: Detect corrupted JSON

GIVEN the file `containers/abc123/state.json` contains invalid JSON
WHEN an integrity check is performed
THEN the corrupted file MUST be reported with the container ID and the nature of the corruption

#### Scenario: Detect orphaned container directory

GIVEN a directory `containers/xyz999/` exists but contains no `state.json`
WHEN an integrity check is performed
THEN the orphaned directory MUST be reported

#### Scenario: Detect dangling volume reference

GIVEN container `abc123` references volume `my-dogan` in its mounts
AND the directory `dogan/my-dogan/` does not exist
WHEN an integrity check is performed
THEN the dangling reference MUST be reported with both the container ID and volume name

#### Scenario: Clean state store passes check

GIVEN all state files are valid, all references are consistent, and no orphaned directories exist
WHEN an integrity check is performed
THEN no errors or warnings MUST be reported

---

### Requirement: Error Handling and Reporting

Waystation MUST provide clear, actionable error messages for all failure conditions. Errors MUST include the resource type, resource identifier, and the nature of the failure.

#### Scenario: Permission denied on write

GIVEN the Waystation directory has incorrect permissions preventing write access
WHEN a write operation is attempted
THEN the error MUST indicate permission denied
AND the error MUST include the file path that was inaccessible

#### Scenario: Disk full during write

GIVEN the filesystem containing the Waystation is full
WHEN a write operation is attempted
THEN the error MUST indicate insufficient disk space
AND the original state file MUST remain unchanged (atomic write guarantee)

#### Scenario: State file not found

GIVEN no container with ID `missing123` exists
WHEN the state for `missing123` is requested
THEN the error MUST indicate that the container was not found
AND the error MUST include the container ID

#### Scenario: Lock acquisition failure includes resource details

GIVEN lock acquisition for container `abc123` times out
WHEN the timeout error is returned
THEN the error MUST identify the resource type (`container`), resource ID (`abc123`), and the PID of the current lock holder (if available)
