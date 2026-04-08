# Prim Storage Specification

## Purpose

Prim is the storage management layer for Maestro. It provides a snapshotter abstraction for managing image layers and container root filesystems, with multiple backend drivers: AllWorld (OverlayFS), fuse-overlayfs, VFS, and future support for Btrfs and ZFS. Prim also encompasses Dogan (volume management) for persistent, bind-mounted, and tmpfs storage, and Palaver (diff/apply) for computing and displaying filesystem changes.

---

## Requirements

### Requirement: Snapshotter Interface

The system MUST provide a snapshotter interface that abstracts filesystem operations for image layers and container root filesystems. All snapshotter implementations MUST conform to this interface.

#### Scenario: Prepare a writable snapshot

GIVEN a committed parent snapshot identified by key "layer-1"
WHEN a writable snapshot is prepared with a new key "container-rw" and parent "layer-1"
THEN the system MUST return mount instructions for a writable filesystem
AND writes to the mounted filesystem MUST be isolated to the new snapshot
AND the parent snapshot MUST NOT be modified

#### Scenario: Prepare a writable snapshot without parent

GIVEN no parent snapshot is specified
WHEN a writable snapshot is prepared with key "base-layer"
THEN the system MUST return mount instructions for an empty writable filesystem
AND writes to the mounted filesystem MUST be persisted in the new snapshot

#### Scenario: View a read-only snapshot

GIVEN a committed snapshot identified by key "image-layer-3"
WHEN a read-only view is requested with a new key "view-1" and parent "image-layer-3"
THEN the system MUST return mount instructions for a read-only filesystem
AND the mounted filesystem MUST contain the contents of "image-layer-3" and all its ancestors
AND any attempt to write to the mounted filesystem MUST fail

#### Scenario: Commit a writable snapshot

GIVEN an active writable snapshot identified by key "container-rw"
WHEN the snapshot is committed with the name "layer-2"
THEN the snapshot MUST become immutable
AND the snapshot MUST be referenceable by the name "layer-2" as a parent for future snapshots
AND further writes to the original key "container-rw" MUST NOT be possible

#### Scenario: Commit fails on read-only snapshot

GIVEN a read-only view snapshot identified by key "view-1"
WHEN the system attempts to commit "view-1"
THEN the operation MUST return an error indicating that only active (writable) snapshots can be committed

#### Scenario: Remove a snapshot

GIVEN a snapshot identified by key "old-layer" with no dependent snapshots
WHEN the snapshot is removed
THEN all storage associated with the snapshot MUST be released
AND the snapshot key MUST no longer be resolvable

#### Scenario: Remove a snapshot with dependents rejected

GIVEN a snapshot "parent-layer" has dependent child snapshots
WHEN the system attempts to remove "parent-layer"
THEN the operation MUST return an error indicating the snapshot has dependents

#### Scenario: Walk all snapshots

GIVEN three committed snapshots and two active snapshots exist
WHEN the walk operation is invoked
THEN the callback function MUST be invoked for each of the five snapshots
AND each invocation MUST provide the snapshot key, parent, and kind (committed or active)

#### Scenario: Query snapshot disk usage

GIVEN a committed snapshot that contains 50 MB of data
WHEN the usage is queried for that snapshot
THEN the result MUST report the disk space consumed by the snapshot
AND the reported size MUST reflect actual storage (not logical size)

---

### Requirement: AllWorld (OverlayFS) Snapshotter

The system MUST provide an OverlayFS-based snapshotter as the primary storage driver. AllWorld mounts image layers as a stacked lower directory set with a writable upper directory and work directory for container root filesystems.

#### Scenario: Mount image layers as overlay

GIVEN an image with three layers committed as snapshots "layer-1", "layer-2", "layer-3"
WHEN a writable snapshot is prepared for a container with parent "layer-3"
THEN the mount type MUST be "overlay"
AND the `lowerdir` option MUST list all three layers in correct order (layer-3 on top, layer-1 at bottom)
AND an `upperdir` MUST be created for capturing writes
AND a `workdir` MUST be created for overlay internal use

#### Scenario: Copy-on-write behavior

GIVEN a container root filesystem is mounted via AllWorld overlay
AND the lower layers contain a file `/etc/config` with content "original"
WHEN the container modifies `/etc/config` to "modified"
THEN the modified file MUST appear in the upper directory
AND the original file in the lower layers MUST NOT be changed
AND reading `/etc/config` from the overlay MUST return "modified"

#### Scenario: File deletion creates whiteout

GIVEN a container root filesystem is mounted via AllWorld overlay
AND the lower layers contain a file `/tmp/old-file`
WHEN the container deletes `/tmp/old-file`
THEN a whiteout marker MUST be created in the upper directory
AND reading the directory listing from the overlay MUST NOT show `/tmp/old-file`
AND the original file in the lower layers MUST NOT be deleted

#### Scenario: AllWorld in rootful mode

GIVEN the system is running in rootful mode
AND the kernel version is 4.0 or later
AND the filesystem is ext4 or xfs
WHEN AllWorld is selected as the snapshotter
THEN overlay mounts MUST succeed using the kernel overlay driver
AND no additional FUSE components MUST be required

#### Scenario: AllWorld rootless with kernel 5.13 or later

GIVEN the system is running in rootless mode
AND the kernel version is 5.13 or later
WHEN AllWorld is selected as the snapshotter
THEN overlay mounts MUST succeed using the native kernel overlay driver in the user namespace
AND no FUSE components MUST be required

---

### Requirement: Fuse-overlayfs Snapshotter

The system MUST provide a fuse-overlayfs snapshotter as a fallback for rootless mode on kernels between 4.18 and 5.12 (inclusive) where native kernel overlay in user namespaces is not available.

#### Scenario: Fuse-overlayfs mount for rootless

GIVEN the system is running in rootless mode
AND the kernel version is between 4.18 and 5.12
AND `/dev/fuse` is accessible
WHEN fuse-overlayfs is selected as the snapshotter
THEN the mount MUST use the fuse-overlayfs binary instead of the kernel overlay driver
AND the mount MUST provide the same overlay semantics (lowerdir, upperdir, workdir)
AND the container root filesystem MUST be functional

#### Scenario: Fuse-overlayfs requires /dev/fuse

GIVEN the system is running in rootless mode
AND the kernel version is between 4.18 and 5.12
AND `/dev/fuse` is NOT accessible
WHEN the system attempts to use fuse-overlayfs
THEN the operation MUST return an error indicating `/dev/fuse` is required
AND the error message SHOULD suggest how to enable the FUSE device

#### Scenario: Fuse-overlayfs layer stacking

GIVEN an image with four layers
WHEN fuse-overlayfs prepares a writable snapshot for a container
THEN all four layers MUST be stacked as lower directories
AND writes MUST go to the upper directory
AND the behavior MUST be equivalent to kernel OverlayFS

---

### Requirement: VFS Snapshotter

The system MUST provide a VFS snapshotter as a universal fallback that works on any filesystem without kernel support for overlay or FUSE.

#### Scenario: VFS copies full filesystem per layer

GIVEN an image with three layers totaling 200 MB
WHEN a VFS snapshot is prepared for a container
THEN the system MUST perform a full copy of the parent filesystem
AND the resulting snapshot MUST contain the complete filesystem state
AND writes MUST go directly to the snapshot's directory

#### Scenario: VFS works on any filesystem type

GIVEN the underlying filesystem is not ext4 or xfs (e.g., NFS, FAT32)
WHEN VFS is selected as the snapshotter
THEN snapshot operations MUST succeed
AND no kernel-level overlay or snapshot support MUST be required

#### Scenario: VFS is slower but correct

GIVEN an image with layers that share common files
WHEN VFS prepares a snapshot
THEN common files MUST be duplicated (no copy-on-write deduplication)
AND the resulting filesystem MUST be identical in content to one produced by AllWorld overlay

---

### Requirement: Btrfs Snapshotter Interface (Phase 2)

The system MUST define a snapshotter interface for Btrfs that uses Btrfs subvolumes and snapshots for efficient copy-on-write storage.

#### Scenario: Btrfs prepare creates snapshot from parent subvolume

GIVEN the storage root is on a Btrfs filesystem
AND a committed snapshot exists as a Btrfs readonly subvolume
WHEN a writable snapshot is prepared from the committed snapshot
THEN the system MUST create a Btrfs writable snapshot of the parent subvolume
AND the writable snapshot MUST support block-level copy-on-write

#### Scenario: Btrfs commit makes subvolume read-only

GIVEN an active writable Btrfs snapshot
WHEN the snapshot is committed
THEN the system MUST convert the subvolume to a read-only snapshot
AND the committed snapshot MUST be usable as a parent for future snapshots

#### Scenario: Btrfs requires Btrfs filesystem

GIVEN the storage root is NOT on a Btrfs filesystem
WHEN the system attempts to use the Btrfs snapshotter
THEN the operation MUST return an error indicating the filesystem does not support Btrfs operations

---

### Requirement: ZFS Snapshotter Interface (Phase 2)

The system MUST define a snapshotter interface for ZFS that uses ZFS datasets and snapshots for copy-on-write storage with data integrity guarantees.

#### Scenario: ZFS prepare clones from parent snapshot

GIVEN the storage root is on a ZFS filesystem
AND a committed snapshot exists as a ZFS snapshot
WHEN a writable snapshot is prepared from the committed snapshot
THEN the system MUST create a ZFS clone of the parent snapshot
AND the clone MUST support writes independently of the parent

#### Scenario: ZFS commit creates snapshot

GIVEN an active writable ZFS clone
WHEN the snapshot is committed
THEN the system MUST create a ZFS snapshot of the clone
AND the snapshot MUST be immutable

#### Scenario: ZFS requires ZFS filesystem

GIVEN the storage root is NOT on a ZFS filesystem
WHEN the system attempts to use the ZFS snapshotter
THEN the operation MUST return an error indicating the filesystem does not support ZFS operations

---

### Requirement: Snapshotter Auto-Detection

The system MUST automatically detect and select the most appropriate snapshotter based on the runtime environment. The selection MUST consider kernel version, filesystem type, and whether the system is running in rootless mode. The chosen driver MUST be persisted in configuration.

#### Scenario: Rootful on kernel 5.13+ with ext4

GIVEN the system is running in rootful mode
AND the kernel version is 5.13 or later
AND the storage filesystem is ext4
WHEN auto-detection runs
THEN the system MUST select the AllWorld (overlay) snapshotter

#### Scenario: Rootful on Btrfs filesystem (Phase 2)

GIVEN the system is running in rootful mode
AND the storage filesystem is Btrfs
WHEN auto-detection runs
THEN the system MUST select the Btrfs snapshotter

#### Scenario: Rootful on ZFS filesystem (Phase 2)

GIVEN the system is running in rootful mode
AND the storage filesystem is ZFS
WHEN auto-detection runs
THEN the system MUST select the ZFS snapshotter

#### Scenario: Rootless on kernel 5.13+

GIVEN the system is running in rootless mode
AND the kernel version is 5.13 or later
WHEN auto-detection runs
THEN the system MUST select the AllWorld (overlay) snapshotter with native rootless support

#### Scenario: Rootless on kernel 4.18-5.12 with FUSE available

GIVEN the system is running in rootless mode
AND the kernel version is between 4.18 and 5.12
AND `/dev/fuse` is accessible
AND fuse-overlayfs is installed
WHEN auto-detection runs
THEN the system MUST select the fuse-overlayfs snapshotter

#### Scenario: Rootless on kernel 4.18-5.12 without FUSE

GIVEN the system is running in rootless mode
AND the kernel version is between 4.18 and 5.12
AND `/dev/fuse` is NOT accessible
WHEN auto-detection runs
THEN the system MUST select the VFS snapshotter as fallback

#### Scenario: Kernel below 4.18

GIVEN the kernel version is below 4.18
WHEN auto-detection runs
THEN the system MUST select the VFS snapshotter regardless of rootful/rootless mode

#### Scenario: Persist auto-detected choice

GIVEN auto-detection selects the AllWorld snapshotter
WHEN the detection completes
THEN the system MUST persist the choice in the configuration file
AND subsequent runs MUST use the persisted choice without re-running detection

#### Scenario: User override of auto-detected driver

GIVEN the configuration file specifies `storage.driver = "vfs"`
WHEN the system initializes the snapshotter
THEN the system MUST use VFS regardless of what auto-detection would have selected

---

### Requirement: Dogan (Volume Management)

The system MUST provide volume management for persistent storage that survives container removal. Named volumes MUST be stored at a well-known path. Bind mounts and tmpfs mounts MUST also be supported.

#### Scenario: Create named volume

GIVEN no volume named "data-vol" exists
WHEN a user creates a volume named "data-vol"
THEN a directory MUST be created at the configured storage root under `dogan/data-vol/data/`
AND a metadata file MUST be created recording the volume name, creation time, and driver
AND the volume MUST appear in the volume list

#### Scenario: Create volume with auto-generated name

GIVEN a user creates a volume without specifying a name
WHEN the volume is created
THEN the system MUST generate a unique name for the volume
AND the generated name MUST be a valid identifier
AND the volume MUST be created at `dogan/<generated-name>/data/`

#### Scenario: Create duplicate volume rejected

GIVEN a volume named "data-vol" already exists
WHEN a user attempts to create another volume named "data-vol"
THEN the system MUST return an error indicating the volume name is already in use

#### Scenario: List volumes

GIVEN three named volumes exist: "vol-a", "vol-b", "vol-c"
WHEN a user lists all volumes
THEN the output MUST include all three volumes
AND each entry MUST display the volume name, driver, and mount point

#### Scenario: Inspect volume

GIVEN a volume named "data-vol" exists and is used by container "app-1"
WHEN a user inspects the volume "data-vol"
THEN the output MUST include the volume name, driver, mount point, creation timestamp, labels, and the list of containers using it

#### Scenario: Remove unused volume

GIVEN a volume named "old-vol" exists and is NOT used by any container
WHEN a user removes the volume "old-vol"
THEN the volume's data directory MUST be deleted
AND the volume's metadata MUST be removed
AND the volume MUST no longer appear in the volume list

#### Scenario: Reject removal of volume in use

GIVEN a volume named "active-vol" is used by a running container
WHEN a user attempts to remove the volume "active-vol"
THEN the system MUST return an error indicating the volume is in use
AND the volume MUST NOT be removed

#### Scenario: Prune orphan volumes only

GIVEN three volumes exist: "used-vol" (mounted by a container), "orphan-1" (not mounted), and "orphan-2" (not mounted)
WHEN a user runs volume prune
THEN "orphan-1" and "orphan-2" MUST be removed
AND "used-vol" MUST NOT be removed
AND the system MUST report the number of volumes removed and space reclaimed

---

### Requirement: Volume Mounting on Container Run

The system MUST support three types of volume mounts when running containers: named volumes, bind mounts, and tmpfs mounts.

#### Scenario: Mount named volume

GIVEN a volume named "app-data" exists
WHEN a container is started with `-v app-data:/app/data`
THEN the volume's data directory MUST be mounted at `/app/data` inside the container
AND writes to `/app/data` MUST be persisted in the named volume
AND the data MUST survive container removal

#### Scenario: Mount named volume auto-creates if missing

GIVEN no volume named "new-vol" exists
WHEN a container is started with `-v new-vol:/data`
THEN the system MUST automatically create a named volume "new-vol"
AND the volume MUST be mounted at `/data` inside the container

#### Scenario: Bind mount host directory

GIVEN a host directory `/home/user/src` exists
WHEN a container is started with `-v /home/user/src:/app/src`
THEN the host directory MUST be mounted at `/app/src` inside the container
AND changes made inside the container at `/app/src` MUST be visible on the host at `/home/user/src`
AND changes on the host MUST be visible inside the container

#### Scenario: Bind mount distinguishing

GIVEN a path specification `-v /absolute/path:/container/path`
WHEN the system parses the volume specification
THEN the system MUST identify this as a bind mount (absolute host path)
GIVEN a path specification `-v relative-name:/container/path`
WHEN the system parses the volume specification
THEN the system MUST identify this as a named volume (no leading slash)

#### Scenario: tmpfs mount

GIVEN a container is started with `--tmpfs /tmp:size=100m`
WHEN the mount is configured
THEN a tmpfs filesystem MUST be mounted at `/tmp` inside the container
AND the tmpfs MUST be limited to 100 MB
AND data written to `/tmp` MUST NOT be persisted to disk
AND data MUST be lost when the container is removed

#### Scenario: tmpfs mount with default options

GIVEN a container is started with `--tmpfs /tmp` (no size specified)
WHEN the mount is configured
THEN a tmpfs filesystem MUST be mounted at `/tmp` inside the container
AND the system MUST use default tmpfs options

---

### Requirement: Mount Options

The system MUST support mount options that control access mode and SELinux labeling for volume mounts.

#### Scenario: Read-only mount

GIVEN a volume named "config-vol" exists
WHEN a container is started with `-v config-vol:/config:ro`
THEN the volume MUST be mounted at `/config` as read-only
AND any attempt to write to `/config` inside the container MUST fail with a permission error

#### Scenario: Read-write mount (default)

GIVEN a volume named "data-vol" exists
WHEN a container is started with `-v data-vol:/data` (no explicit option)
THEN the volume MUST be mounted at `/data` as read-write
AND writes to `/data` MUST succeed

#### Scenario: Explicit read-write mount

GIVEN a volume named "data-vol" exists
WHEN a container is started with `-v data-vol:/data:rw`
THEN the volume MUST be mounted at `/data` as read-write

#### Scenario: SELinux shared label (:z)

GIVEN the host has SELinux enabled
AND a volume is mounted with the `:z` option
WHEN the volume is configured
THEN the volume content MUST be relabeled with a shared SELinux label
AND the volume MUST be accessible by multiple containers simultaneously

#### Scenario: SELinux private label (:Z)

GIVEN the host has SELinux enabled
AND a volume is mounted with the `:Z` option
WHEN the volume is configured
THEN the volume content MUST be relabeled with a private SELinux label
AND the volume MUST be accessible only by the specific container it is mounted into

#### Scenario: Combined mount options

GIVEN a volume is mounted with `-v data-vol:/data:ro,z`
WHEN the mount is configured
THEN the volume MUST be mounted as read-only
AND the SELinux shared relabeling MUST be applied

---

### Requirement: Volume Driver Tracking

The system MUST track which containers are using which volumes. This information MUST be persisted in the volume's metadata.

#### Scenario: Track container mount on volume

GIVEN a volume named "shared-vol" exists
AND container "app-1" is started with `-v shared-vol:/data`
WHEN the container starts successfully
THEN the volume metadata MUST list "app-1" in the `used_by` field

#### Scenario: Track multiple containers on same volume

GIVEN a volume named "shared-vol" exists
AND containers "app-1" and "app-2" both mount "shared-vol"
WHEN the volume metadata is inspected
THEN the `used_by` field MUST list both "app-1" and "app-2"

#### Scenario: Remove container from tracking on container removal

GIVEN container "app-1" uses volume "shared-vol"
WHEN container "app-1" is removed
THEN "app-1" MUST be removed from the `used_by` field of "shared-vol"
AND the volume itself MUST NOT be removed

#### Scenario: Volume prune uses tracking data

GIVEN a volume "orphan-vol" has an empty `used_by` field
WHEN volume prune is executed
THEN "orphan-vol" MUST be eligible for removal

---

### Requirement: Layer Diff

The system MUST be able to compute filesystem differences between two snapshots, producing a changeset that represents files added, modified, or deleted. Deleted files MUST be represented using OCI whiteout file conventions.

#### Scenario: Detect added files

GIVEN a parent snapshot containing files `/a` and `/b`
AND a child snapshot containing files `/a`, `/b`, and `/c`
WHEN the diff between parent and child is computed
THEN the changeset MUST include `/c` as an added file

#### Scenario: Detect modified files

GIVEN a parent snapshot containing file `/a` with content "original"
AND a child snapshot containing file `/a` with content "modified"
WHEN the diff between parent and child is computed
THEN the changeset MUST include `/a` as a modified file

#### Scenario: Detect deleted files with whiteout

GIVEN a parent snapshot containing files `/a`, `/b`, and `/c`
AND a child snapshot containing only files `/a` and `/b`
WHEN the diff between parent and child is computed
THEN the changeset MUST include a whiteout entry for `/c`
AND the whiteout MUST follow OCI convention (`.wh.c` in the parent directory)

#### Scenario: Detect deleted directory with opaque whiteout

GIVEN a parent snapshot containing directory `/dir/` with files inside
AND a child snapshot where `/dir/` has been deleted and recreated empty
WHEN the diff between parent and child is computed
THEN the changeset MUST include an opaque whiteout for `/dir/`
AND the opaque whiteout MUST follow OCI convention (`.wh..wh..opq` inside the directory)

#### Scenario: Apply changeset to snapshot

GIVEN a parent snapshot and a changeset archive (tar) containing added and modified files plus whiteout entries
WHEN the changeset is applied to the parent snapshot
THEN all added files MUST appear in the resulting snapshot
AND all modified files MUST reflect the new content
AND all whiteout entries MUST result in deletion of the corresponding files

---

### Requirement: Palaver (Filesystem Diff Display)

The system MUST be able to display filesystem changes in a container relative to its original image, showing each change categorized as Added (A), Changed (C), or Deleted (D).

#### Scenario: Show added files

GIVEN a container whose root filesystem has a new file `/app/newfile.txt` not present in the image
WHEN the filesystem diff is displayed
THEN the output MUST include an entry `A /app/newfile.txt`

#### Scenario: Show changed files

GIVEN a container whose root filesystem has a modified file `/etc/hostname` (different from image)
WHEN the filesystem diff is displayed
THEN the output MUST include an entry `C /etc/hostname`

#### Scenario: Show deleted files

GIVEN a container whose root filesystem has deleted the file `/tmp/setup.sh` present in the image
WHEN the filesystem diff is displayed
THEN the output MUST include an entry `D /tmp/setup.sh`

#### Scenario: Mixed changes

GIVEN a container with added file `/new`, changed file `/etc/config`, and deleted file `/old`
WHEN the filesystem diff is displayed
THEN the output MUST include `A /new`, `C /etc/config`, and `D /old`
AND each entry MUST be on its own line

#### Scenario: No changes

GIVEN a container whose root filesystem is identical to the original image
WHEN the filesystem diff is displayed
THEN the output MUST be empty (no changes detected)

---

### Requirement: Snapshot Chain for Multi-Layer Images

The system MUST correctly build snapshot chains for multi-layer images, where each image layer becomes a committed snapshot that serves as the parent of the next layer.

#### Scenario: Build layer chain from image

GIVEN an image with layers L1, L2, L3 (L1 is the base)
WHEN the system prepares snapshots for the image
THEN L1 MUST be prepared with no parent and then committed
AND L2 MUST be prepared with L1 as parent and then committed
AND L3 MUST be prepared with L2 as parent and then committed
AND the final committed snapshot MUST represent the complete image filesystem

#### Scenario: Container writable layer on top of image chain

GIVEN an image with committed snapshot chain L1 -> L2 -> L3
WHEN a container is created from this image
THEN the system MUST prepare a writable snapshot with L3 as parent
AND the writable layer MUST capture all container writes
AND the image layers L1, L2, L3 MUST remain immutable

#### Scenario: Shared layers across images

GIVEN image A with layers L1, L2, L3 and image B with layers L1, L2, L4
AND L1 and L2 are identical blobs (same content digest)
WHEN both images have their snapshot chains built
THEN L1 and L2 MUST be stored only once on disk
AND image A's chain MUST be L1 -> L2 -> L3
AND image B's chain MUST be L1 -> L2 -> L4

---

### Requirement: Storage Root Directory Structure

The system MUST maintain a well-defined directory structure for all storage operations. Named volumes, snapshotter data, and metadata MUST be organized under the storage root.

#### Scenario: Default storage root for rootless mode

GIVEN the system is running in rootless mode
AND no custom storage root is configured
WHEN the storage subsystem initializes
THEN the storage root MUST be at `~/.local/share/maestro/`
AND the volume storage MUST be under `dogan/`
AND directory permissions MUST be set to 0700

#### Scenario: Custom storage root

GIVEN the configuration specifies a custom storage root
WHEN the storage subsystem initializes
THEN all storage operations MUST use the configured root
AND the directory structure MUST be created under the custom root if it does not exist

#### Scenario: Volume data directory structure

GIVEN a named volume "my-vol" is created
WHEN the volume is stored
THEN the data directory MUST be at `<root>/dogan/my-vol/data/`
AND the metadata MUST be stored alongside (e.g., `<root>/dogan/my-vol/meta.json`)

---

### Requirement: Concurrent Access Safety

The system MUST ensure that concurrent operations on the same snapshot or volume do not corrupt data. All mutations MUST be safe under concurrent access.

#### Scenario: Concurrent snapshot prepare on different keys

GIVEN two concurrent operations each preparing a writable snapshot with different keys but the same parent
WHEN both operations complete
THEN both snapshots MUST be valid
AND neither MUST corrupt the other or the parent

#### Scenario: Concurrent volume creation with different names

GIVEN two concurrent operations each creating a volume with a different name
WHEN both operations complete
THEN both volumes MUST be created successfully
AND each MUST have its own data directory

#### Scenario: Concurrent volume creation with same name

GIVEN two concurrent operations each attempting to create a volume named "dup-vol"
WHEN both operations execute
THEN exactly one MUST succeed
AND the other MUST return an error indicating the name is already in use

---

### Requirement: Storage Cleanup on Container Removal

The system MUST clean up all storage resources associated with a container when the container is removed. This includes removing the writable snapshot and releasing any associated disk space.

#### Scenario: Remove container writable layer

GIVEN a container has a writable snapshot "ctr-abc-rw"
WHEN the container is removed
THEN the writable snapshot MUST be removed via the snapshotter
AND the disk space consumed by the writable layer MUST be freed

#### Scenario: Image layers preserved after container removal

GIVEN a container was created from an image with layers L1, L2, L3
WHEN the container is removed
THEN the image layer snapshots L1, L2, L3 MUST NOT be removed
AND other containers using the same image MUST NOT be affected

#### Scenario: Volume not removed on container removal

GIVEN a container uses a named volume "persist-vol"
WHEN the container is removed
THEN the volume "persist-vol" MUST NOT be removed
AND the volume's data MUST be intact
AND the container MUST be removed from the volume's `used_by` tracking

---

### Requirement: Tmpfs Mount Isolation

The system MUST ensure that tmpfs mounts are backed entirely by RAM and are ephemeral. Data written to tmpfs MUST never be written to the host disk.

#### Scenario: Tmpfs data is memory-only

GIVEN a container is running with `--tmpfs /scratch:size=50m`
WHEN the container writes 40 MB to `/scratch`
THEN the data MUST be stored in RAM
AND the host filesystem MUST NOT contain the data
AND the total RAM usage for the mount MUST NOT exceed 50 MB

#### Scenario: Tmpfs data lost on container stop

GIVEN a container with a tmpfs mount at `/scratch` containing data
WHEN the container is stopped and restarted
THEN the `/scratch` directory MUST be empty
AND all previously written data MUST be gone

#### Scenario: Tmpfs exceeds size limit

GIVEN a container with `--tmpfs /scratch:size=10m`
WHEN the container attempts to write 15 MB to `/scratch`
THEN the write MUST fail with a "no space left on device" error
AND the mount MUST NOT grow beyond the configured limit

---

### Requirement: Bind Mount Validation

The system MUST validate bind mount sources before mounting them into containers.

#### Scenario: Bind mount from existing host directory

GIVEN the host directory `/home/user/data` exists
WHEN a container is started with `-v /home/user/data:/container/data`
THEN the bind mount MUST succeed
AND the host directory's contents MUST be visible inside the container

#### Scenario: Bind mount from non-existent host path

GIVEN the host path `/nonexistent/path` does not exist
WHEN a container is started with `-v /nonexistent/path:/data`
THEN the system MUST create the host directory before mounting
AND the bind mount MUST succeed
AND the created directory MUST be owned by the calling user

#### Scenario: Bind mount of a single file

GIVEN a host file `/home/user/config.json` exists
WHEN a container is started with `-v /home/user/config.json:/app/config.json`
THEN the single file MUST be bind-mounted into the container
AND modifications to the file inside the container MUST be reflected on the host

---

### Requirement: Volume Metadata Schema

The system MUST persist volume metadata in a well-defined schema that includes all information needed to manage the volume lifecycle.

#### Scenario: Volume metadata includes required fields

GIVEN a volume is created
WHEN the metadata is persisted
THEN the stored metadata MUST include the volume name, driver, mount point (absolute path), labels, options, creation timestamp, last-used timestamp, and list of containers using it

#### Scenario: Volume metadata survives system restart

GIVEN a volume "my-vol" was created with specific metadata
WHEN the system restarts and loads the volume
THEN the loaded metadata MUST match the original metadata exactly
AND the volume's data directory MUST still be accessible

#### Scenario: Volume last-used timestamp updates on mount

GIVEN a volume "my-vol" was last used 24 hours ago
WHEN a new container mounts "my-vol"
THEN the `last_used_at` field in the metadata MUST be updated to the current time

---

### Requirement: Snapshotter Error Handling

The system MUST handle errors gracefully during snapshot operations and provide clear, actionable error messages.

#### Scenario: Insufficient disk space during prepare

GIVEN the storage filesystem has less than 1 MB of free space
WHEN the system attempts to prepare a writable snapshot
THEN the operation MUST return an error indicating insufficient disk space
AND the error MUST include the available and required disk space if determinable

#### Scenario: Parent snapshot not found

GIVEN a snapshot prepare is requested with parent key "nonexistent-layer"
WHEN the system attempts to resolve the parent
THEN the operation MUST return an error indicating the parent snapshot does not exist

#### Scenario: Filesystem does not support overlay

GIVEN the underlying filesystem is NFS (which does not support overlay)
AND AllWorld (overlay) is configured as the snapshotter
WHEN the system attempts to mount an overlay
THEN the operation MUST return an error indicating the filesystem does not support overlay mounts
AND the error message SHOULD suggest switching to VFS or changing the storage location
