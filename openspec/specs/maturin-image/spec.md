# Maturin Image Specification

> *"See the TURTLE of enormous girth! On his shell he holds the earth."*

## Purpose

Maturin manages OCI container images: content-addressable storage of blobs, pulling (Drawing) from registries, pushing (Unfound) to registries, multi-platform selection (Keystone), garbage collection (Reap), and all local image lifecycle operations (tag, save, load, remove).

---

## Requirements

---

### Requirement: Content-Addressable Blob Storage

Maturin MUST store all blobs (layers, configs, manifests) in a content-addressable store (CAS) keyed by SHA256 digest. The store MUST reside under the Waystation root at `maturin/blobs/sha256/`. Each blob MUST be stored as a file whose name is the hex-encoded SHA256 digest of its content.

#### Scenario: Store a blob by digest

GIVEN a valid blob content and its corresponding SHA256 digest
WHEN a Put operation is invoked with the digest and a reader of the content
THEN the blob MUST be stored at the path `maturin/blobs/sha256/<hex-digest>`
AND the stored file content MUST be byte-identical to the original content

#### Scenario: Retrieve a blob by digest

GIVEN a blob has been previously stored with a known SHA256 digest
WHEN a Get operation is invoked with that digest
THEN the system MUST return a reader that produces the exact stored content
AND the content MUST match the original blob byte-for-byte

#### Scenario: Check blob existence for a stored blob

GIVEN a blob has been stored with a known digest
WHEN an Exists operation is invoked with that digest
THEN the system MUST return true

#### Scenario: Check blob existence for an absent blob

GIVEN no blob has been stored with a given digest
WHEN an Exists operation is invoked with that digest
THEN the system MUST return false

#### Scenario: Delete a blob by digest

GIVEN a blob exists in the store with a known digest
WHEN a Delete operation is invoked with that digest
THEN the blob file MUST be removed from disk
AND a subsequent Exists check for that digest MUST return false

#### Scenario: Delete a non-existent blob

GIVEN no blob exists with a given digest
WHEN a Delete operation is invoked with that digest
THEN the system MUST return an error indicating the blob was not found

#### Scenario: Store blob creates parent directories

GIVEN the CAS directory structure does not yet exist on disk
WHEN the first Put operation is invoked
THEN the system MUST create the necessary directory hierarchy
AND the blob MUST be stored successfully

---

### Requirement: Blob Integrity Verification on Write

Maturin MUST verify the SHA256 integrity of every blob during write operations. The system MUST compute the SHA256 digest of the incoming content and compare it against the declared digest. If the digests do not match, the blob MUST NOT be persisted.

#### Scenario: Write with matching digest succeeds

GIVEN blob content whose SHA256 digest matches the declared digest
WHEN a Put operation is invoked
THEN the blob MUST be stored successfully
AND no error MUST be returned

#### Scenario: Write with mismatched digest fails

GIVEN blob content whose actual SHA256 digest differs from the declared digest
WHEN a Put operation is invoked
THEN the system MUST return a digest mismatch error
AND the error MUST include both the expected and actual digests
AND no file MUST be written to the store

#### Scenario: Partial write is cleaned up on integrity failure

GIVEN blob content that will fail integrity verification
WHEN the Put operation detects the mismatch after streaming begins
THEN any partially written temporary file MUST be removed
AND the store MUST remain in a consistent state

---

### Requirement: Blob Integrity Verification on Read

Maturin MUST verify the SHA256 integrity of every blob during read operations. The system MUST compute the SHA256 digest of the content being read and compare it against the expected digest. If the digests do not match, the system MUST report corruption.

#### Scenario: Read of an intact blob succeeds

GIVEN a blob stored with a valid digest
WHEN a Get operation is invoked
THEN the content MUST be returned without error
AND the SHA256 of the returned content MUST match the stored digest

#### Scenario: Read of a corrupted blob fails

GIVEN a blob whose on-disk content has been modified after storage (corruption)
WHEN a Get operation is invoked for that digest
THEN the system MUST return a corruption error
AND the error MUST indicate the expected digest and the actual digest of the corrupted content

---

### Requirement: Manifest Storage with Tag Symlinks

Maturin MUST store image manifests in the CAS and MUST maintain a tag resolution structure using symbolic links. The symlink structure MUST follow the pattern `maturin/manifests/<registry>/<repository>/<tag>` where each symlink points to the corresponding manifest digest.

#### Scenario: Store a manifest and create a tag symlink

GIVEN a manifest blob and its digest for repository "docker.io/library/nginx" with tag "latest"
WHEN the manifest is stored
THEN a symlink MUST exist at `maturin/manifests/docker.io/library/nginx/latest`
AND the symlink MUST resolve to the manifest digest `sha256:<hex>`

#### Scenario: Multiple tags pointing to the same manifest

GIVEN a manifest digest for "docker.io/library/nginx"
WHEN tags "latest" and "1.25" are both assigned to this manifest
THEN both symlinks MUST exist
AND both MUST resolve to the same digest
AND the manifest blob MUST be stored only once in the CAS

#### Scenario: Overwrite an existing tag

GIVEN tag "latest" for "docker.io/library/nginx" currently points to digest A
WHEN a new manifest with digest B is stored with tag "latest"
THEN the symlink MUST be atomically updated to point to digest B
AND if no other tags reference digest A, the old manifest MUST remain in the CAS until garbage collection

#### Scenario: Resolve a tag to a digest

GIVEN tag "latest" exists for "docker.io/library/nginx"
WHEN the system resolves the tag
THEN it MUST return the digest that the symlink points to
AND the manifest content MUST be retrievable using that digest

#### Scenario: Resolve a non-existent tag

GIVEN no tag "beta" exists for "docker.io/library/nginx"
WHEN the system attempts to resolve the tag
THEN it MUST return an error indicating the tag was not found

---

### Requirement: Local OCI Image Index

Maturin MUST maintain a local `index.json` file that tracks all images present in the local store. The index MUST conform to the OCI Image Index structure. The index MUST be updated atomically on every operation that adds or removes images.

#### Scenario: Index updated on image pull

GIVEN an empty local store
WHEN an image is successfully pulled and stored
THEN `index.json` MUST contain an entry for the pulled image manifest
AND the entry MUST include the manifest digest, media type, and size

#### Scenario: Index updated on image removal

GIVEN `index.json` contains an entry for a stored image
WHEN that image is removed
THEN the entry MUST be removed from `index.json`
AND the file MUST be written atomically (write-to-temp + rename)

#### Scenario: Index survives crash during update

GIVEN an image operation is in progress
WHEN the process crashes during `index.json` update
THEN on next access, `index.json` MUST be in a consistent state (either the old version or the fully new version)
AND partial writes MUST NOT corrupt the index

#### Scenario: Concurrent index access

GIVEN two processes attempt to update `index.json` simultaneously
WHEN both acquire the Khef lock in sequence
THEN each update MUST see a consistent state
AND no entries MUST be lost or duplicated

---

### Requirement: Drawing (Image Pull Flow)

Maturin MUST implement a complete image pull flow (Drawing) that resolves a reference, retrieves the manifest, downloads all layers, and stores the result locally. The Drawing flow MUST coordinate with Shardik (registry client) for all remote operations.

#### Scenario: Pull a single-platform image by tag

GIVEN a registry contains a single-platform image "library/nginx:latest"
WHEN Drawing is invoked for "library/nginx:latest"
THEN the system MUST resolve the tag to a manifest digest via Shardik
AND the system MUST download the manifest
AND the system MUST download all referenced layer blobs
AND the system MUST download the config blob
AND all blobs MUST be stored in the CAS
AND a tag symlink MUST be created
AND `index.json` MUST be updated

#### Scenario: Pull downloads layers in parallel

GIVEN an image has 4 layer blobs
WHEN Drawing is invoked
THEN layers MUST be downloaded concurrently with up to 4 parallel downloads
AND all 4 layers MUST be present in the CAS after completion

#### Scenario: Pull respects configurable parallelism

GIVEN the parallel download limit is configured to 2
WHEN Drawing is invoked for an image with 4 layers
THEN at no point MUST more than 2 concurrent blob downloads be active

#### Scenario: Pull by digest

GIVEN a manifest digest "sha256:abc123..."
WHEN Drawing is invoked with the digest reference
THEN the system MUST fetch the manifest directly by digest without tag resolution
AND the image MUST be stored locally

#### Scenario: Pull of an already-present image is a no-op for blobs

GIVEN an image has been previously pulled and all its blobs exist locally
WHEN Drawing is invoked again for the same image reference
THEN the system MUST verify the manifest is current
AND if the manifest has not changed, no blob downloads MUST occur
AND the operation MUST complete successfully

#### Scenario: Pull fails when registry is unreachable

GIVEN the registry is not reachable
WHEN Drawing is invoked
THEN the system MUST return an error indicating the registry could not be contacted
AND no partial state MUST be persisted in `index.json`

#### Scenario: Pull fails when a layer download fails

GIVEN an image has 4 layers and the download of the third layer fails
WHEN Drawing is invoked
THEN the system MUST return an error indicating which layer failed
AND successfully downloaded layers MAY be retained in the CAS (for future deduplication)
AND the image MUST NOT appear in `index.json` as complete

---

### Requirement: Keystone (Multi-Platform Selection)

When an image reference resolves to an OCI Image Index (fat manifest), Maturin MUST select the platform-specific manifest that matches the host system. This is the Keystone selection. The system MUST support an explicit platform override via the `--platform` flag.

#### Scenario: Select manifest matching host OS and architecture

GIVEN an Image Index with entries for linux/amd64 and linux/arm64
AND the host system is linux/amd64
WHEN Keystone selection is performed
THEN the system MUST select the linux/amd64 manifest

#### Scenario: Select manifest matching OS, architecture, and variant

GIVEN an Image Index with entries for linux/arm/v6, linux/arm/v7, and linux/arm64
AND the host system is linux/arm with variant v7
WHEN Keystone selection is performed
THEN the system MUST select the linux/arm/v7 manifest

#### Scenario: Fallback when exact variant is not available

GIVEN an Image Index with entries for linux/arm64/v8 and linux/amd64
AND the host system is linux/arm64 with no explicit variant
WHEN Keystone selection is performed
THEN the system MUST select the linux/arm64/v8 manifest by matching OS and architecture and accepting any variant

#### Scenario: Override platform with --platform flag

GIVEN an Image Index with entries for linux/amd64 and linux/arm64
AND the host system is linux/amd64
WHEN Drawing is invoked with `--platform linux/arm64`
THEN the system MUST select the linux/arm64 manifest regardless of host platform

#### Scenario: No compatible platform found

GIVEN an Image Index with entries only for windows/amd64
AND the host system is linux/amd64
AND no `--platform` override is specified
THEN the system MUST return an error indicating no compatible platform was found
AND the error MUST list the available platforms

#### Scenario: Single-platform image bypasses Keystone

GIVEN a reference resolves to a single image manifest (not an Image Index)
WHEN Drawing is invoked
THEN the system MUST use that manifest directly without Keystone selection

#### Scenario: Platform override with OS, architecture, and variant

GIVEN an Image Index with entries for linux/arm/v6 and linux/arm/v7
WHEN Drawing is invoked with `--platform linux/arm/v7`
THEN the system MUST select the linux/arm/v7 manifest exactly

---

### Requirement: Unfound (Image Push Flow)

Maturin MUST implement a complete image push flow (Unfound) that reads a local manifest, determines which blobs are missing from the remote registry, uploads only the missing blobs, and then pushes the manifest. The Unfound flow MUST coordinate with Shardik for all remote operations.

#### Scenario: Push an image with all blobs missing remotely

GIVEN a locally stored image with 3 layers, a config, and a manifest
AND none of these blobs exist on the remote registry
WHEN Unfound is invoked
THEN the system MUST upload all 3 layers and the config blob via Shardik
AND the system MUST then push the manifest
AND the image MUST be retrievable from the remote registry

#### Scenario: Push skips blobs that already exist remotely

GIVEN a locally stored image with 3 layers
AND 2 of those layers already exist on the remote registry
WHEN Unfound is invoked
THEN the system MUST check remote blob existence for each layer (HEAD request)
AND the system MUST upload only the 1 missing layer
AND the total data transferred MUST be only the missing blob plus the manifest

#### Scenario: Push uploads blobs in parallel

GIVEN a locally stored image with multiple layers to upload
WHEN Unfound is invoked
THEN blob uploads MUST proceed concurrently with the configured parallelism limit

#### Scenario: Push fails when authentication fails

GIVEN valid local image data
AND the registry rejects credentials
WHEN Unfound is invoked
THEN the system MUST return an authentication error
AND no blobs MUST be uploaded

#### Scenario: Push fails when a blob upload fails

GIVEN a locally stored image where one blob upload fails on the registry
WHEN Unfound is invoked
THEN the system MUST return an error indicating which blob failed
AND the manifest MUST NOT be pushed (partial pushes are not allowed)

#### Scenario: Push manifest after all blobs succeed

GIVEN all blob uploads complete successfully
WHEN the manifest push is attempted
THEN it MUST be the last operation in the push sequence
AND the registry MUST receive the manifest only after all referenced blobs are present

---

### Requirement: Docker Manifest V2 Compatibility

Maturin MUST accept images formatted as Docker Manifest V2 Schema 2 (`application/vnd.docker.distribution.manifest.v2+json`) and Docker Manifest List (`application/vnd.docker.distribution.manifest.list.v2+json`). Docker-format manifests MUST be converted to OCI format on ingestion and stored natively as OCI content.

#### Scenario: Pull a Docker V2 manifest image

GIVEN a registry serves an image with media type `application/vnd.docker.distribution.manifest.v2+json`
WHEN Drawing is invoked
THEN the system MUST accept the manifest
AND the system MUST convert it to OCI image manifest format for local storage
AND the image MUST be usable for container creation

#### Scenario: Pull a Docker manifest list

GIVEN a registry serves a fat manifest with media type `application/vnd.docker.distribution.manifest.list.v2+json`
WHEN Drawing is invoked
THEN the system MUST treat it as equivalent to an OCI Image Index
AND Keystone selection MUST apply to the contained platform entries

#### Scenario: Docker layer media types are accepted

GIVEN a Docker image with layers using media type `application/vnd.docker.image.rootfs.diff.tar.gzip`
WHEN Drawing is invoked
THEN the system MUST accept and store these layers
AND the layers MUST be usable for container rootfs assembly

---

### Requirement: Layer Deduplication

Maturin MUST deduplicate layer blobs based on content digest. If a blob with a given digest already exists in the CAS, it MUST NOT be downloaded or stored again. This applies across all images in the local store.

#### Scenario: Shared layers between images are stored once

GIVEN image A with layers [L1, L2, L3] has been pulled
AND image B references layers [L1, L2, L4] (sharing L1 and L2)
WHEN image B is pulled
THEN only layer L4 MUST be downloaded
AND only one copy of L1 and one copy of L2 MUST exist on disk

#### Scenario: Deduplication check occurs before download

GIVEN layer L1 already exists in the CAS
WHEN Drawing encounters L1 in a new image manifest
THEN the system MUST check Exists before initiating a download
AND the download MUST be skipped

#### Scenario: Deduplication across registries

GIVEN an image from registry A contains layer L1
AND an image from registry B also references a layer with the identical digest as L1
WHEN the second image is pulled
THEN L1 MUST NOT be downloaded again

---

### Requirement: Image Tagging

Maturin MUST support tagging operations that create a new tag for an existing local image. Tagging MUST create a new symlink in the manifest structure without duplicating any blob data.

#### Scenario: Tag an existing image with a new tag

GIVEN image "docker.io/library/nginx:latest" exists locally with digest D
WHEN a tag operation creates "docker.io/library/nginx:v1.25"
THEN a new symlink MUST be created at `maturin/manifests/docker.io/library/nginx/v1.25`
AND the symlink MUST point to digest D
AND no blob data MUST be copied

#### Scenario: Tag to a different repository

GIVEN image "docker.io/library/nginx:latest" exists locally with digest D
WHEN a tag operation creates "registry.example.com/myapp:prod"
THEN a new symlink MUST be created under `maturin/manifests/registry.example.com/myapp/prod`
AND the symlink MUST point to digest D

#### Scenario: Tag a non-existent image fails

GIVEN no image "docker.io/library/nonexistent:latest" exists locally
WHEN a tag operation is attempted
THEN the system MUST return an error indicating the source image was not found

---

### Requirement: Image Save (Export to Tar)

Maturin MUST support exporting a local image to an OCI-layout tar archive. The archive MUST be a valid OCI image layout that can be imported by other OCI-compliant tools.

#### Scenario: Save a single image to tar

GIVEN image "docker.io/library/nginx:latest" exists locally
WHEN a save operation is invoked with output file "nginx.tar"
THEN the output file MUST be a tar archive
AND the archive MUST contain a valid `oci-layout` file
AND the archive MUST contain `index.json` referencing the manifest
AND the archive MUST contain all referenced blobs (manifest, config, layers)

#### Scenario: Save multiple images to a single tar

GIVEN images "nginx:latest" and "alpine:3.18" exist locally
WHEN a save operation is invoked for both images with output file "images.tar"
THEN the archive MUST contain both manifests and all their referenced blobs
AND `index.json` MUST reference both manifests

#### Scenario: Save a non-existent image fails

GIVEN no image "nonexistent:latest" exists locally
WHEN a save operation is attempted
THEN the system MUST return an error indicating the image was not found

---

### Requirement: Image Load (Import from Tar)

Maturin MUST support importing images from tar archives in both OCI layout format and Docker `docker save` format. Imported images MUST be stored in the CAS and tracked in `index.json`.

#### Scenario: Load an OCI-layout tar archive

GIVEN a tar archive containing a valid OCI image layout
WHEN a load operation is invoked
THEN all blobs MUST be extracted and stored in the CAS
AND the manifest MUST be indexed in `index.json`
AND the image MUST appear in the local image list

#### Scenario: Load a Docker-format tar archive

GIVEN a tar archive produced by `docker save`
WHEN a load operation is invoked
THEN the Docker-format manifest MUST be converted to OCI format
AND all layer tarballs MUST be stored in the CAS
AND the image MUST appear in the local image list

#### Scenario: Load an archive with multiple images

GIVEN a tar archive containing 3 images
WHEN a load operation is invoked
THEN all 3 images MUST be imported
AND each MUST have its own entry in `index.json`

#### Scenario: Load with duplicate blobs

GIVEN a tar archive contains blobs that already exist in the local CAS
WHEN a load operation is invoked
THEN existing blobs MUST NOT be overwritten
AND the operation MUST complete successfully using the already-present blobs

#### Scenario: Load a corrupted tar archive

GIVEN a tar archive with truncated or invalid content
WHEN a load operation is invoked
THEN the system MUST return an error indicating the archive is invalid
AND no partial images MUST be added to `index.json`

---

### Requirement: Reap (Garbage Collection)

Maturin MUST implement garbage collection (Reap) using a mark-and-sweep algorithm with reference counting. Reap MUST remove unreferenced blobs from the CAS while preserving all blobs that are reachable from any manifest in `index.json`.

#### Scenario: Mark-and-sweep removes unreferenced blobs

GIVEN blobs B1, B2, B3 exist in the CAS
AND only B1 and B2 are referenced by manifests in `index.json`
WHEN Reap is invoked
THEN blob B3 MUST be deleted
AND blobs B1 and B2 MUST be preserved

#### Scenario: Reap preserves transitively referenced blobs

GIVEN a manifest M1 references config C1 and layers [L1, L2]
AND config C1 and layers L1, L2 are reachable from M1
WHEN Reap is invoked
THEN M1, C1, L1, and L2 MUST all be preserved

#### Scenario: Reap after image removal cleans up orphaned layers

GIVEN image A has unique layers [L3, L4] not shared with any other image
WHEN image A is removed and then Reap is invoked
THEN layers L3 and L4 MUST be deleted from the CAS

#### Scenario: Reap preserves shared layers

GIVEN image A and image B share layer L1
AND image A is removed
WHEN Reap is invoked
THEN L1 MUST be preserved because image B still references it

#### Scenario: Reap with LRU eviction when storage threshold is exceeded

GIVEN the total CAS storage exceeds the configured `gc_threshold` percentage of `max_size`
WHEN Reap is invoked
THEN images MUST be evicted in least-recently-used order based on `last_used_at` timestamp
AND eviction MUST continue until storage falls below the threshold
AND images currently in use by running containers MUST NOT be evicted

#### Scenario: Reap reports freed space

GIVEN unreferenced blobs totaling 500 MB exist in the CAS
WHEN Reap is invoked
THEN the system MUST report the number of blobs removed and the total space reclaimed

#### Scenario: Reap is safe under concurrent access

GIVEN a Reap operation is running
AND a concurrent Drawing (pull) operation stores new blobs
WHEN both operations complete
THEN the newly stored blobs MUST NOT be deleted by the ongoing Reap
AND the store MUST remain consistent

#### Scenario: Manual prune of dangling images

GIVEN images exist that have no tags (dangling)
WHEN an image prune operation is invoked
THEN only untagged images MUST be removed
AND tagged images MUST be preserved

#### Scenario: Prune with age filter

GIVEN dangling images created at various times
WHEN an image prune operation is invoked with a filter "until=24h"
THEN only dangling images older than 24 hours MUST be removed

---

### Requirement: Image Metadata Schema

Maturin MUST maintain metadata for each stored image conforming to the image metadata schema. The metadata MUST track digest, tags, platform information, layer details, sizes, and timestamps.

#### Scenario: Metadata includes all required fields

GIVEN an image has been pulled and stored
WHEN image metadata is retrieved
THEN it MUST include `digest` (the SHA256 manifest digest)
AND it MUST include `media_type` (the manifest media type)
AND it MUST include `repository` (the full repository name including registry)
AND it MUST include `tags` (an array of all tags assigned to this manifest)
AND it MUST include `keystone.os` (the operating system)
AND it MUST include `keystone.architecture` (the CPU architecture)
AND it MUST include `config` (image configuration: env, cmd, exposed_ports, working_dir, labels, author)
AND it MUST include `layers` (array of layer descriptors: digest, size, media_type)
AND it MUST include `total_size` (sum of all layer sizes)
AND it MUST include `created_at` (when the image was originally created, from image config)
AND it MUST include `drawn_at` (when the image was pulled to this host)
AND it MUST include `last_used_at` (when the image was last used to create a container)

#### Scenario: Metadata timestamps are updated on use

GIVEN an image was pulled at time T1
AND the image is used to create a container at time T2
WHEN image metadata is retrieved
THEN `drawn_at` MUST equal T1
AND `last_used_at` MUST equal T2

#### Scenario: Metadata reflects current tags

GIVEN an image has tags ["latest", "v1.0"]
AND the "latest" tag is reassigned to a different manifest
WHEN metadata for the original image is retrieved
THEN its `tags` array MUST contain only ["v1.0"]

---

### Requirement: Image Removal with Dependency Check

Maturin MUST prevent removal of images that are in use by one or more containers. The system MUST check for container dependencies before removing an image.

#### Scenario: Remove an image with no dependent containers

GIVEN image "nginx:latest" exists locally
AND no containers are using this image
WHEN an image remove operation is invoked
THEN the image MUST be removed from `index.json`
AND the tag symlink MUST be removed
AND the manifest blob MAY be retained until Reap (if no other tags reference it)

#### Scenario: Remove an image with active containers fails

GIVEN image "nginx:latest" exists locally
AND a running container is using this image
WHEN an image remove operation is invoked without force
THEN the system MUST return an error indicating the image is in use
AND the error MUST list the container IDs that depend on this image
AND the image MUST NOT be removed

#### Scenario: Remove an image with stopped containers fails

GIVEN image "nginx:latest" exists locally
AND a stopped (but not deleted) container references this image
WHEN an image remove operation is invoked without force
THEN the system MUST return an error indicating the image is in use by stopped containers

#### Scenario: Force remove an image despite dependent containers

GIVEN image "nginx:latest" is in use by containers
WHEN an image remove operation is invoked with force
THEN the image MUST be removed
AND the tag symlink MUST be removed
AND the containers MUST remain in their current state (they retain their rootfs)

#### Scenario: Remove by digest removes all tags for that digest

GIVEN an image with digest D has tags ["latest", "v1.0"]
WHEN a remove operation is invoked by digest D
THEN all tag symlinks pointing to D MUST be removed
AND the image entry MUST be removed from `index.json`

#### Scenario: Untag removes only the specified tag

GIVEN an image with digest D has tags ["latest", "v1.0"]
WHEN an untag operation is invoked for tag "latest" only
THEN only the "latest" symlink MUST be removed
AND tag "v1.0" MUST remain
AND the image MUST remain in `index.json`

#### Scenario: Untag the last tag triggers image removal

GIVEN an image with digest D has only one tag "latest"
WHEN an untag operation is invoked for "latest"
THEN the tag symlink MUST be removed
AND the image MUST become a dangling image (no tags)
AND the image MAY be removed or retained for garbage collection

---

### Requirement: Atomic State Operations

All Maturin operations that modify state (index.json, manifest symlinks, blob files) MUST use atomic write patterns. For file writes, the system MUST write to a temporary file and rename it to the final path. This ensures crash safety and consistency.

#### Scenario: Index write uses atomic rename

GIVEN `index.json` needs to be updated
WHEN the update is performed
THEN the system MUST write the new content to a temporary file in the same directory
AND the system MUST rename the temporary file to `index.json`
AND if the process crashes between write and rename, the old `index.json` MUST remain intact

#### Scenario: Symlink update is atomic

GIVEN a tag symlink needs to be updated to point to a new digest
WHEN the update is performed
THEN the system MUST create a new symlink at a temporary name
AND the system MUST rename the temporary symlink to the final path
AND at no point MUST the tag resolve to an invalid or absent digest

---

### Requirement: Blob Storage Directory Permissions

Maturin MUST create all directories within the store with permissions 0700 to restrict access to the owning user. This is consistent with the rootless-first security model.

#### Scenario: CAS directories created with restricted permissions

GIVEN a fresh installation with no existing store directories
WHEN the first blob storage operation creates the directory hierarchy
THEN all created directories MUST have permissions 0700
AND no other users MUST have read, write, or execute access

#### Scenario: Blob files created with restricted permissions

GIVEN a blob is being stored
WHEN the blob file is written
THEN the file MUST have permissions no more permissive than 0600

---

### Requirement: Image List Retrieval

Maturin MUST provide an operation to list all locally stored images with their metadata. The list MUST be derived from the local `index.json` and associated metadata.

#### Scenario: List images returns all stored images

GIVEN 3 images have been pulled and stored
WHEN the image list operation is invoked
THEN the result MUST contain exactly 3 entries
AND each entry MUST include repository, tag, digest (truncated), creation time, and total size

#### Scenario: List images on an empty store

GIVEN no images have been pulled
WHEN the image list operation is invoked
THEN the result MUST be an empty list
AND no error MUST be returned

#### Scenario: List images shows all tags per image

GIVEN an image has been tagged with "latest" and "v1.0"
WHEN the image list operation is invoked
THEN the image MUST appear as two entries (one per tag) sharing the same digest

---

### Requirement: Image Inspection

Maturin MUST provide an operation to retrieve complete metadata for a specific image, including the full OCI manifest, image configuration, layer history, and platform information.

#### Scenario: Inspect an image by tag

GIVEN image "nginx:latest" exists locally
WHEN an inspect operation is invoked for "nginx:latest"
THEN the result MUST include the full manifest JSON
AND the result MUST include the image configuration (env, cmd, entrypoint, working_dir, exposed_ports, labels)
AND the result MUST include layer information (digest, size, media_type for each layer)
AND the result MUST include platform information (OS, architecture)

#### Scenario: Inspect an image by digest

GIVEN an image with digest "sha256:abc123..." exists locally
WHEN an inspect operation is invoked for "sha256:abc123..."
THEN the result MUST be identical to inspecting by any tag that resolves to this digest

#### Scenario: Inspect a non-existent image

GIVEN no image "nonexistent:latest" exists locally
WHEN an inspect operation is invoked
THEN the system MUST return an error indicating the image was not found

---

### Requirement: Image History

Maturin MUST provide an operation to retrieve the layer history of an image, showing the command that created each layer, its size, and whether it is an empty (metadata-only) layer.

#### Scenario: History shows all layers with commands

GIVEN an image built with 5 Dockerfile instructions, 3 of which produce filesystem changes
WHEN image history is retrieved
THEN the result MUST contain 5 entries
AND 3 entries MUST have non-zero sizes (real layers)
AND 2 entries MUST have zero size (empty/metadata layers)
AND each entry MUST include the "created by" command string

#### Scenario: History for an image with no history metadata

GIVEN an image whose config does not include history entries
WHEN image history is retrieved
THEN the system MUST return entries based on the layer list
AND the "created by" field MUST be empty or indicate that history is unavailable
