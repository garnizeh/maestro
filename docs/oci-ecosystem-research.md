# OCI Ecosystem Research for Maestro CLI Design

**Date:** 2026-01-01
**Purpose:** Comprehensive research document covering the OCI ecosystem, existing tools, specifications, libraries, and best practices. This serves as input for the Maestro design document.

---

## Table of Contents

1. [OCI Specifications](#1-oci-specifications)
2. [Existing Tools Deep Dive](#2-existing-tools-deep-dive)
3. [Container Runtimes](#3-container-runtimes)
4. [Image Management Details](#4-image-management-details)
5. [Network Management](#5-network-management)
6. [Storage Management](#6-storage-management)
7. [Security](#7-security)
8. [CLI Design Patterns](#8-cli-design-patterns)
9. [Go Libraries (Current Versions)](#9-go-libraries-current-versions)

---

## 1. OCI Specifications

### 1.1 OCI Image Specification (v1.1.1)

**Current Version:** v1.1.1 (latest stable)
**Repository:** https://github.com/opencontainers/image-spec
**Go Module:** `github.com/opencontainers/image-spec` (published Feb 2025)

#### Manifest Schema

The OCI Image Manifest (`application/vnd.oci.image.manifest.v1+json`) is the core descriptor for a container image targeting a specific architecture and OS:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.manifest.v1+json",
  "artifactType": "<string, optional>",
  "config": {
    "mediaType": "application/vnd.oci.image.config.v1+json",
    "digest": "sha256:<hex>",
    "size": 7023
  },
  "layers": [
    {
      "mediaType": "application/vnd.oci.image.layer.v1.tar+gzip",
      "digest": "sha256:<hex>",
      "size": 32654
    }
  ],
  "subject": {
    "mediaType": "application/vnd.oci.image.manifest.v1+json",
    "digest": "sha256:<hex>",
    "size": 1234
  },
  "annotations": {
    "org.opencontainers.image.created": "2025-01-01T00:00:00Z"
  }
}
```

**Field Requirements:**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `schemaVersion` | int | YES | Must be `2` for backward compatibility |
| `mediaType` | string | NO | Should be `application/vnd.oci.image.manifest.v1+json` |
| `artifactType` | string | Conditional | Required when `config.mediaType` is empty |
| `config` | descriptor | YES | References container configuration |
| `layers` | array | YES | Array of layer descriptors; base layer at index 0 |
| `subject` | descriptor | NO | References another manifest (used for artifacts) |
| `annotations` | map | NO | String-to-string metadata |

#### Config Schema

The image configuration (`application/vnd.oci.image.config.v1+json`) contains runtime parameters, environment variables, and the `rootfs` diff_ids that correspond to layers.

#### Layer Media Types

| Media Type | Description |
|------------|-------------|
| `application/vnd.oci.image.layer.v1.tar` | Uncompressed tar |
| `application/vnd.oci.image.layer.v1.tar+gzip` | Gzip-compressed tar |
| `application/vnd.oci.image.layer.v1.tar+zstd` | Zstandard-compressed tar |
| `application/vnd.oci.image.layer.nondistributable.v1.tar` | Non-distributable layer |
| `application/vnd.oci.image.layer.nondistributable.v1.tar+gzip` | Non-distributable gzipped |

**Empty Descriptor:** `application/vnd.oci.empty.v1+json` with digest `sha256:44136fa355b3678a1146ad16f7e8649e94fb4fc21fe77e8310c060f61caaff8a` (used for artifact manifests where config is not meaningful).

#### Content-Addressable Storage Model

All blobs are stored by their content digest (typically SHA-256). The storage model is a directed acyclic graph (DAG):
- **Index** -> points to platform-specific manifests
- **Manifest** -> points to config blob + ordered layer blobs
- **Config blob** -> contains runtime config and rootfs diff_ids
- **Layer blobs** -> filesystem changesets

Modifying any blob requires creating a new blob and updating all parent references up to the root `index.json` (as implemented by umoci).

### 1.2 OCI Distribution Specification (v1.1.0)

**Current Version:** v1.1.0 (released Feb 2024)
**Repository:** https://github.com/opencontainers/distribution-spec

#### API Endpoints

**Pull Operations (REQUIRED for all registries):**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v2/<name>/manifests/<reference>` | Pull manifest by tag or digest |
| `GET` | `/v2/<name>/blobs/<digest>` | Pull blob by digest |
| `HEAD` | `/v2/<name>/manifests/<reference>` | Check manifest existence |
| `HEAD` | `/v2/<name>/blobs/<digest>` | Check blob existence |

**Push Operations (SHOULD support):**

| Method | Path | Description |
|--------|------|-------------|
| `POST` | `/v2/<name>/blobs/uploads/` | Initiate blob upload |
| `PUT` | `<location>?digest=<digest>` | Complete blob upload (monolithic) |
| `PATCH` | `<location>` | Upload blob chunk (chunked) |
| `PUT` | `/v2/<name>/manifests/<reference>` | Push manifest |
| `POST` | `/v2/<name>/blobs/uploads/?mount=<digest>&from=<name>` | Cross-repo blob mount |

**Content Discovery (SHOULD support):**

| Method | Path | Description |
|--------|------|-------------|
| `GET` | `/v2/<name>/tags/list` | List repository tags (paginated) |
| `GET` | `/v2/<name>/referrers/<digest>` | List manifests referencing a digest (NEW in v1.1) |

**Content Management (SHOULD support):**

| Method | Path | Description |
|--------|------|-------------|
| `DELETE` | `/v2/<name>/manifests/<reference>` | Delete manifest |
| `DELETE` | `/v2/<name>/blobs/<digest>` | Delete blob |

#### Referrers API (New in v1.1)

The referrers API (`/v2/<name>/referrers/<digest>`) allows clients to discover artifacts associated with a given manifest. When a manifest with a `subject` field is pushed, the registry indexes it for referrer queries. Registries respond with the header `OCI-Subject: <digest>` to confirm referrer processing.

This API is critical for supply chain security (finding signatures, SBOMs, attestations attached to an image).

#### Content Negotiation

Clients SHOULD include `Accept` headers to indicate supported manifest media types. The registry MAY reject manifests that reference non-existent content (except for `subject` references).

#### Authentication

The distribution spec delegates authentication to the registry implementation. The standard flow is the Docker V2 token authentication:
1. Client sends request
2. Registry returns `401` with `Www-Authenticate: Bearer realm="...",service="...",scope="..."`
3. Client exchanges credentials at the token endpoint
4. Client retries with `Authorization: Bearer <token>`

### 1.3 OCI Runtime Specification (v1.3.0)

**Current Version:** v1.3.0 (released Nov 2025)
**Repository:** https://github.com/opencontainers/runtime-spec

This is the fourth minor release of the v1 series. Notably adds FreeBSD support.

#### config.json Structure

Top-level fields:

| Field | Type | Required | Description |
|-------|------|----------|-------------|
| `ociVersion` | string | YES | SemVer v2.0.0 format |
| `root` | object | YES* | Root filesystem (`path`, `readonly`) |
| `mounts` | array | NO | Additional mount points |
| `process` | object | NO** | Container process spec |
| `hostname` | string | NO | Container hostname |
| `domainname` | string | NO | Container domain name |
| `hooks` | object | NO | Lifecycle hooks |
| `annotations` | map | NO | Arbitrary metadata |
| `linux` | object | NO | Linux-specific configuration |

*Required on most platforms; optional on Hyper-V
**Required when `start` is called

#### Lifecycle (create/start/kill/delete)

1. **create**: Runtime creates container environment per config.json, runs `createRuntime` and `createContainer` hooks
2. **start**: Runtime runs the user-specified process, runs `startContainer` and `poststart` hooks
3. **kill**: Runtime sends signal to container process
4. **delete**: Runtime deletes container resources, runs `poststop` hooks

#### Hooks

| Hook | Phase | Description |
|------|-------|-------------|
| `createRuntime` | After runtime env created, before pivot_root | Runtime setup |
| `createContainer` | After createRuntime, before pivot_root | Container setup |
| `startContainer` | Before user process, after pivot_root | In-container setup (e.g., ldconfig) |
| `poststart` | After user process started | External notification |
| `poststop` | After container stopped | Cleanup |
| `prestart` | (DEPRECATED) | Use createRuntime instead |

Each hook has: `path` (required), `args`, `env`, `timeout`.

#### Linux-Specific Features

**Namespaces** (8 types):
- `pid` - Process isolation
- `network` - Network stack isolation
- `mount` - Mount table isolation
- `ipc` - IPC isolation
- `uts` - Hostname/domain isolation
- `user` - UID/GID remapping
- `cgroup` - Cgroup hierarchy isolation
- `time` - Clock isolation (CLOCK_BOOTTIME, CLOCK_MONOTONIC)

**Cgroups Resources:**
- `memory`: limit, reservation, swap, swappiness, disableOOMKiller
- `cpu`: shares, quota, burst, period, cpus, mems
- `blockIO`: weight, throttle (read/write bps and iops)
- `pids`: max process count
- `hugepages`: huge page limits
- `network`: class ID, priorities
- `rdma`: RDMA device limits

**Seccomp:**
- `defaultAction`: SCMP_ACT_ALLOW, SCMP_ACT_ERRNO, SCMP_ACT_KILL, etc.
- `architectures`: list of architectures
- `syscalls`: per-syscall rules with names, actions, args, errnoRet

**Devices:**
- Explicit device allowlist
- Default devices: /dev/null, /dev/zero, /dev/full, /dev/random, /dev/urandom, /dev/tty, /dev/console, /dev/ptmx
- `netDevices`: host-to-container network device mapping (new)

**Additional:**
- `maskedPaths`: paths hidden from container
- `readonlyPaths`: paths mounted read-only
- `rootfsMountPropagation`: mount propagation mode
- `sysctl`: kernel parameters
- `intelRdt`: Intel Resource Director Technology
- `personality`: architecture execution domain
- `memoryPolicy`: NUMA memory allocation policy

### 1.4 OCI Artifacts

Artifacts extend the image spec for non-container content using two key fields added in v1.1:

**`artifactType` field:** Stored on the image manifest, serves as a single point of reference for content type. Required when `config.mediaType` is set to `application/vnd.oci.empty.v1+json`.

**`subject` field:** Optional field pointing to another manifest digest, creating a reference relationship. Used for signatures, SBOMs, attestations, and other metadata that "refers to" an image.

**Workflow:**
1. Push a container image (produces manifest digest D)
2. Create artifact (e.g., signature) with `subject.digest = D`
3. Push artifact manifest to the same registry
4. Clients use Referrers API to discover all artifacts referencing D

**Projects using OCI Artifacts:** Helm charts, OPA policies, Wasm modules, Sigstore signatures, SBOMs (CycloneDX/SPDX), in-toto attestations.

---

## 2. Existing Tools Deep Dive

### 2.1 containerd

**Repository:** https://github.com/containerd/containerd
**Latest Version:** v2.2.1 (Dec 2025), with v2.3 planned Apr 2026
**Language:** Go
**License:** Apache 2.0

#### Architecture

containerd is built around a plugin-based architecture where everything is a plugin -- content store, snapshotter, runtime, CRI, metadata, etc. Communication occurs via gRPC APIs.

**Key architectural components:**
- **Content Store:** Content-addressable blob storage
- **Metadata Store:** BoltDB-backed metadata (labels, namespaces, GC)
- **Snapshotter:** Filesystem layer management (overlay, btrfs, zfs, native, devmapper)
- **Runtime (ShimV2):** Process management via shim binaries
- **CRI Plugin:** Kubernetes Container Runtime Interface implementation
- **Transfer Service:** Unified artifact transfer between sources/destinations

#### ShimV2

ShimV2 is the standard interface between the containerd runtime and container processes. containerd invokes v2 runtimes as binaries on the system, which start a shim process. Each container gets its own shim process, enabling:
- Independent container lifecycle management
- Crash isolation (shim crash doesn't affect other containers)
- Runtime pluggability (runc, crun, youki, kata, gVisor)

#### Snapshotter Plugins

Snapshotters provide the filesystem abstraction for image layers. The interface includes key methods:
- `Stat()`: Get snapshot info
- `Update()`: Update mutable properties
- `Usage()`: Get resource usage
- `Prepare()`: Create active snapshot from parent
- `View()`: Create readonly view
- `Commit()`: Seal active snapshot
- `Remove()`: Delete snapshot
- `Walk()`: Iterate snapshots

Built-in snapshotters: overlay (default), native, btrfs, zfs, devmapper.
Remote snapshotters: stargz (eStargz lazy pulling), nydus, overlaybd, SOCI.

#### Namespaces

containerd supports namespace isolation for resource management. Kubernetes containers run in the `k8s.io` namespace. Different clients (Docker, nerdctl, ctr) use separate namespaces to avoid conflicts.

#### Strengths
- Production-proven at massive scale (Kubernetes default runtime)
- Highly modular plugin architecture
- Strong API stability guarantees (v2 Go client API is stable)
- Remote snapshotter support enables lazy pulling
- Excellent Kubernetes integration via CRI plugin

#### Weaknesses
- Daemon-based architecture (single point of failure)
- Complex configuration surface
- Not designed for standalone CLI usage (needs nerdctl or ctr)
- Heavy dependency tree

### 2.2 Podman

**Repository:** https://github.com/containers/podman
**Latest Version:** v5.x (2025)
**Language:** Go
**License:** Apache 2.0

#### Architecture

Podman is built on a shared library stack:
- **containers/image**: Image management (pull, push, inspect, copy between transports)
- **containers/storage**: Local image/container storage (overlay, btrfs, vfs drivers)
- **conmon/conmon-rs**: Container monitor process (OCI runtime supervisor)
- **Netavark**: Network stack (Rust-based, replaces CNI in Podman 4.0+)
- **Aardvark-dns**: Container DNS resolution server

#### Daemonless Design

Each `podman` command forks its own process. Containers run as direct child processes of the invoking user, not a privileged daemon. Containers continue running independently after the CLI session ends. For long-running services, systemd integration via `podman generate systemd` or Quadlet files provides supervision.

#### Rootless Model

- Uses Linux user namespaces for isolation
- Maps unprivileged user to UID 0 inside container via `/etc/subuid` and `/etc/subgid`
- Requires at least 65,536 subordinate UIDs/GIDs per user
- Storage uses fuse-overlayfs (kernel < 5.13) or native overlay (kernel >= 5.13)
- Networking via pasta (default since Podman 5.0) or slirp4netns

#### Pod Model

Podman has first-class support for pods (groups of containers sharing namespaces), directly mirroring Kubernetes pods. `podman generate kube` and `podman play kube` enable bidirectional Kubernetes manifest compatibility.

#### Docker CLI Compatibility

Near drop-in replacement. Most commands are identical: `podman run`, `podman build`, `podman push`, etc. Users can alias `docker=podman` for existing scripts. Some edge cases and Docker Compose compatibility may require `podman-docker` compatibility package.

#### Strengths
- Daemonless = no single point of failure
- Rootless by default = superior security posture
- Pod-native = natural Kubernetes alignment
- Shared libraries with Buildah, Skopeo, CRI-O = consistent ecosystem
- SELinux integration

#### Weaknesses
- Not CRI-compliant (cannot serve as Kubernetes runtime directly)
- Some Docker Compose edge cases may break
- Rootless mode has networking limitations (no privileged ports < 1024 by default)
- macOS/Windows support requires VM (Podman Machine)

### 2.3 CRI-O

**Repository:** https://github.com/cri-o/cri-o
**Language:** Go
**License:** Apache 2.0

#### Architecture

CRI-O is a Kubernetes-only container runtime, implementing the CRI (Container Runtime Interface) and nothing more. It shares the containers/image and containers/storage libraries with Podman.

#### Design Philosophy

Minimalist approach: implements exactly what Kubernetes needs via CRI, with no general-purpose container management features. This results in:
- Smaller attack surface
- Faster pod start times (no daemon shim overhead)
- Version-locked to Kubernetes (CRI-O 1.x matches Kubernetes 1.x)

#### Strengths
- Minimal attack surface
- Optimized for Kubernetes
- Production-proven (Red Hat OpenShift default runtime)
- Shared seccomp policy with Podman/Buildah

#### Weaknesses
- Kubernetes-only (no standalone container management)
- Cannot be used for local development workflows
- Limited ecosystem compared to containerd

### 2.4 Docker Engine

**Repository:** https://github.com/moby/moby
**Language:** Go
**License:** Apache 2.0

#### Architecture

Docker Engine (dockerd) is a daemon-based architecture:
- **dockerd**: API server and orchestration daemon
- **containerd**: Container runtime (embedded)
- **BuildKit**: Image build engine (default since Docker 23.0)
- **docker CLI**: Client that communicates with dockerd via REST API

#### BuildKit Architecture

BuildKit is a modular build engine:
- **Frontend**: Parses build instructions (Dockerfile, custom LLBs)
- **Solver**: Resolves build graph, manages caching
- **Worker**: Executes build steps (OCI/runc worker or containerd worker)
- **Cache**: Layer-level caching with export/import capabilities
- **Multi-platform**: Cross-architecture builds via QEMU emulation or native builders

#### Strengths
- Largest ecosystem and community
- Docker Compose for multi-container applications
- Docker Desktop for developer experience
- BuildKit is best-in-class build engine
- Extensive plugin ecosystem

#### Weaknesses
- Daemon-based (security concern, single point of failure)
- Docker Desktop requires commercial license for large organizations
- Historical baggage (complex codebase)
- Root required for daemon (rootless mode is secondary)

### 2.5 skopeo

**Repository:** https://github.com/containers/skopeo
**Language:** Go
**License:** Apache 2.0

#### Architecture

Stateless image operations tool built on the containers/image library. No daemon required, no local storage needed for many operations.

#### Core Operations

- **`skopeo inspect`**: Inspect remote image metadata without pulling
- **`skopeo copy`**: Copy images between registries, local dirs, OCI layouts (registry-to-registry without intermediate storage)
- **`skopeo sync`**: Efficient bulk synchronization (optimized for re-syncs)
- **`skopeo delete`**: Delete images from registries
- **`skopeo list-tags`**: List tags in a repository

#### Supported Transports
- `docker://` - Docker registry
- `docker-archive:` - Docker tar archive
- `oci:` - OCI image layout
- `oci-archive:` - OCI tar archive
- `dir:` - Local directory
- `containers-storage:` - Local containers/storage

#### Strengths
- Zero-daemon, stateless operation
- Direct registry-to-registry transfers (no intermediate pull)
- Excellent for CI/CD pipelines and air-gapped environments
- Shared containers/image library = consistent behavior with Podman

#### Weaknesses
- No build capabilities
- No container runtime
- Limited to image/artifact operations

### 2.6 buildah

**Repository:** https://github.com/containers/buildah
**Language:** Go
**License:** Apache 2.0

#### Architecture

Buildah creates OCI/Docker images without requiring a daemon or Dockerfile. It provides both CLI and Go API.

#### Dockerfile-less Builds

Buildah allows scripting image builds in Bash:
```bash
container=$(buildah from fedora)
buildah run $container -- dnf install -y httpd
buildah config --cmd "/usr/sbin/httpd" $container
buildah commit $container my-httpd
```

#### Rootless Operation

- Uses user namespaces + fuse-overlayfs/native overlay
- Benchmark: 28s builds for 50-layer PyTorch image vs Docker's 52s (55% faster, per 2025 CNCF tests)

#### Strengths
- Dockerfile-less builds via scripting
- Rootless by default
- Shared Go API used by Podman internally
- Fine-grained control over image layers

#### Weaknesses
- Less familiar syntax than Dockerfile
- Smaller community than Docker/BuildKit

### 2.7 umoci

**Repository:** https://github.com/opencontainers/umoci
**Latest Version:** v0.5.0 (released May 2025 after a 4-year gap)
**Language:** Go
**License:** Apache 2.0

#### Architecture

umoci is the OCI reference implementation for image manipulation. It operates on OCI image layouts directly, without requiring a daemon.

**Key design:** When modifying a blob (which is content-addressable and immutable), umoci creates a new version and walks up the reference path, replacing all parent blobs until the change bubbles up to `index.json`.

Uses mtree(8) manifests for tracking filesystem changes and generating delta layers without requiring snapshots or overlays.

#### Core Operations
- `umoci unpack`: Extract OCI image to rootfs bundle
- `umoci repack`: Create new layer from modified rootfs
- `umoci config`: Modify image configuration
- `umoci raw`: Low-level blob operations
- `umoci new`: Create new empty image
- `umoci tag`: Manage image tags

#### Used By
- Stacker (Cisco appliance image builds)
- LXC/Incus (OCI image support)

### 2.8 ORAS (OCI Registry As Storage)

**Repository:** https://github.com/oras-project
**CLI Version:** v1.3.0 (Oct 2025)
**Go Library:** `oras.land/oras-go/v2` (v2 stable, v3 in development)
**License:** Apache 2.0

#### Architecture

ORAS enables using OCI registries as generic artifact stores, treating registries as content-addressable storage systems for any file type.

#### Key Features (v1.3.0)
- Multi-platform artifact management (`oras manifest index create/update`)
- Backup/restore (registry to local dir/tarball and vice versa)
- Structured output (JSON, Go templates via `--format`/`--template`)
- Full OCI distribution-spec v1.1.1 compliance
- Referrers discovery for supply chain artifacts

#### Go Library Capabilities
- Unified APIs for push/pull/manage across registries, filesystems, in-memory stores
- Docker credential helper protocol support
- PackManifest for Image Manifest v1.0 and v1.1
- Deletion and garbage collection in OCI package
- Warning handling from remote registries

---

## 3. Container Runtimes

### 3.1 runc

**Repository:** https://github.com/opencontainers/runc
**Latest Version:** v1.4.0 (stable), v1.5.0-rc.1 (prerelease, expected Apr 2026)
**Language:** Go
**License:** Apache 2.0

#### Features
- Reference OCI runtime implementation
- Full OCI Runtime Spec v1.3 support
- seccomp_unotify support (since v1.1.0-rc.1)
- ID-mapped mounts (MOUNT_ATTR_IDMAP)
- `runc features` command for capability introspection
- libpathrs default for hardened path resolution

#### Known Limitations
- No time namespace support yet (PR pending)
- Written in Go = higher memory footprint than C-based alternatives
- 2025 CVEs: CVE-2025-31133 (masked paths bypass), CVE-2025-52565 (/dev/console bind-mount bypass), CVE-2025-52881 -- all allowing potential container breakouts via /proc file manipulation

#### Release Schedule
6-month minor releases (end of April and October). cgroup v1 deprecated as of v1.4.0.

### 3.2 crun

**Repository:** https://github.com/containers/crun
**Language:** C
**License:** GPL-2.0

#### Performance vs runc

| Metric | crun | runc | Improvement |
|--------|------|------|-------------|
| Container start time | ~49% faster | Baseline | 49% |
| Memory usage | 3,752 KB | 15,120 KB | ~75% reduction |
| Exec performance | Faster | Baseline | Significant at scale |

#### Key Advantages
- C implementation interacts directly with Linux kernel
- Compiles to much smaller binary
- Drop-in replacement for runc (same CLI interface)
- Default runtime in RHEL 9+
- Supported by Mirantis Container Runtime since MCR 25.0.9

#### Compatibility
Same command-line interface as runc; tools configured for runc work with crun without modification.

### 3.3 youki

**Repository:** https://github.com/youki-dev/youki
**Latest Version:** v0.5.7 (early 2025)
**Language:** Rust
**License:** Apache 2.0

#### Maturity
- Passes containerd's e2e tests
- Adopted by several production environments
- Active development with regular releases (0.5.0 Jan 2025, 0.4.1 Sep 2024)

#### Features
- OCI runtime-spec compliant
- Rootless mode support
- Potential for better memory and performance than runc (Rust memory safety without GC overhead)
- Well-documented with developer documentation

#### Limitations
- Still pre-1.0 (API stability not guaranteed)
- Smaller community than runc/crun
- Less battle-tested in production

### 3.4 gVisor (runsc)

**Repository:** https://github.com/google/gvisor
**Language:** Go
**License:** Apache 2.0

#### Syscall Interception Model

gVisor implements a user-space application kernel that intercepts all container syscalls:
1. Container process makes syscall
2. Kernel intercepts via seccomp (SECCOMP_RET_TRAP) + SIGSYS signal (systrap platform)
3. gVisor's Sentry kernel handles syscall in user space
4. Only approved host syscalls reach actual kernel

**Platform implementations:**
- **Systrap** (default since mid-2023): Uses SECCOMP_RET_TRAP, best general-purpose performance
- **KVM**: Low-overhead syscall interception, poor with nested virtualization
- **ptrace**: Uses PTRACE_SYSEMU, most portable but slowest

#### Performance
- Overhead depends on syscall intensity of workload
- Google reports majority of applications see < 3% overhead
- CPU-bound workloads minimally affected; syscall-heavy workloads more impacted

#### Use Cases
- Multi-tenant environments
- Running untrusted code
- Compliance requirements for strong isolation

### 3.5 Kata Containers

**Repository:** https://github.com/kata-containers/kata-containers
**Language:** Go, Rust
**License:** Apache 2.0

#### VM-Based Isolation

Each container (or pod) runs inside a lightweight VM with:
- Stripped-down guest kernel (optimized for boot time and minimal footprint)
- Based on latest Linux LTS kernel
- Hardware virtualization as second layer of defense

**Supported hypervisors:**
- Cloud-Hypervisor
- Firecracker (AWS Lambda's hypervisor)
- QEMU/KVM

#### Performance Overhead
- Cold start latency: Higher than namespace-based containers (VM boot time)
- Memory: Additional overhead per VM
- Network: Bridge between VM and host adds configuration complexity
- Improving with faster hypervisors (Firecracker)

#### Use Cases
- High-security workloads (financial, regulated industries)
- Serverless computing (Firecracker integration)
- Multi-tenant cloud environments
- Running untrusted workloads

---

## 4. Image Management Details

### 4.1 Content-Addressable Storage Patterns

#### containerd's Approach
- Content store: flat blob storage keyed by digest
- Metadata in BoltDB with namespace isolation
- Garbage collection: mark-and-sweep in metadata package
- Snapshotter abstraction decouples storage from content

#### Podman's Approach (containers/storage)
- Shared store at `/var/lib/containers/storage` (rootful) or `~/.local/share/containers/storage` (rootless)
- Storage drivers: overlay (default), btrfs, vfs, zfs
- Shared between Podman, Buildah, CRI-O, Skopeo
- Layer deduplication via content-addressable digests
- Reference counting for garbage collection

### 4.2 Lazy Pulling

Pulling accounts for 76% of container start time, but only 6.4% of data is actually read during startup.

#### eStargz Format
- Based on CRFS stargz format with enhancements
- Compatible with standard OCI/Docker registries and runtimes
- Individual files can be fetched on-demand via HTTP range requests
- Optimization: prefetches likely-accessed files during startup
- Performance: up to 69.2% faster container creation (8s vs 26s in benchmarks)
- Supported by: Kubernetes, containerd, nerdctl, CRI-O, Podman, BuildKit, Kaniko

#### Nydus Format (CNCF Project)
- Replaces tar.gz with RAFS (Registry Acceleration File System)
- Separates metadata from data chunks (content-addressable)
- EROFS backend (Linux 5.19+): eliminates FUSE from data path entirely
  - Cached reads: kernel VFS -> EROFS driver -> page cache
  - ~10x lower per-operation latency vs FUSE
  - No stale mount failure mode

#### Other Lazy Pulling Technologies
- **OverlayBD**: Block-device based lazy pulling
- **SOCI** (AWS): Sparse OCI, content-addressable artifacts (Index + ZToCs)

### 4.3 Image Signing

#### Sigstore Ecosystem (Industry Standard)

**cosign**: Signs OCI containers and artifacts
- Keyless signing via Fulcio CA + Rekor transparency log
- Key-based signing with local/KMS keys
- OIDC identity-based (GitHub Actions, GitLab CI, etc.)
- OCI v1.1 support for storing signatures as referrers

**Fulcio**: Free code signing Certificate Authority
- Issues short-lived certificates (minutes)
- Private key discarded after signing ("keyless")

**Rekor**: Immutable transparency log
- All signing events publicly auditable
- Tamper-resistant ledger

**TUF (The Update Framework)**: Root of trust distribution
- Distributes Fulcio's root CA certificate and Rekor's public key
- Decentralized and federated trust delegation

#### Notation (Notary Project)
- Microsoft-backed alternative to Sigstore
- Uses OCI v1.1 referrers API for signature storage
- Hierarchical trust delegation model
- Trust stores for managing trusted identities

**Industry Trend:** Docker Content Trust (DCT/Notary v1) is being retired. Docker dropping DCT support starting Aug 2025. Sigstore has become the de facto standard.

### 4.4 Multi-Platform Images

#### Manifest Index (Image Index)

The Image Index (`application/vnd.oci.image.index.v1+json`) is a "fat manifest" that references platform-specific manifests:

```json
{
  "schemaVersion": 2,
  "mediaType": "application/vnd.oci.image.index.v1+json",
  "manifests": [
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:...",
      "size": 7023,
      "platform": {
        "architecture": "amd64",
        "os": "linux"
      }
    },
    {
      "mediaType": "application/vnd.oci.image.manifest.v1+json",
      "digest": "sha256:...",
      "size": 7023,
      "platform": {
        "architecture": "arm64",
        "os": "linux",
        "variant": "v8"
      }
    }
  ]
}
```

**Platform Selection:** Container runtimes match the host's OS + architecture against the manifest index entries. The `platform` object includes: `architecture`, `os`, `os.version`, `os.features`, `variant`.

**Usage:** OPTIONAL for image providers; consumers SHOULD be prepared to process them.

### 4.5 Garbage Collection

#### Registry GC (Mark and Sweep)
1. **Mark phase**: Iterate all manifests, build set of referenced blob digests
2. **Sweep phase**: Iterate all blobs, delete those not in the mark set

**Operational considerations:**
- Registry MUST be in read-only mode during GC (to avoid data corruption)
- Schedule during maintenance windows
- Cloud registries use lifecycle policies for automated cleanup
- Tag deletion triggers GC after configurable delay (e.g., 24 hours)

#### Local Store GC
- containerd: Metadata package handles GC with reference counting
- Podman: `podman system prune` removes unused images, containers, volumes
- Reference counting + mark-and-sweep hybrid approaches

### 4.6 Registry Authentication

#### Docker config.json
Location: `~/.docker/config.json`

```json
{
  "auths": {
    "registry.example.com": {
      "auth": "<base64(username:password)>"
    }
  },
  "credHelpers": {
    "gcr.io": "gcloud",
    "*.dkr.ecr.*.amazonaws.com": "ecr-login"
  },
  "credsStore": "desktop"
}
```

**Priority:** `credHelpers` > `credsStore` > `auths`

#### Credential Helpers
- External binaries named `docker-credential-<helper>`
- Protocol: JSON over stdin/stdout
- Operations: `get`, `store`, `erase`, `list`
- Implementations: gcloud, ecr-login, osxkeychain, secretservice, pass

#### OIDC Flows
- Docker Registry HTTP API v2 implements OAuth 2.0 bearer token flow
- Even basic auth goes through token exchange
- Cloud providers support OIDC for workload identity (GKE, EKS, AKS)
- Short-lived tokens preferred over long-lived credentials

---

## 5. Network Management

### 5.1 CNI Specification (v1.1.0)

**Current Spec Version:** 1.1.0
**Library:** `github.com/containernetworking/cni` v1.3.0
**Plugins:** `github.com/containernetworking/plugins` v1.9.1

#### Plugin Interface

CNI plugins are executable binaries that receive configuration via:
- **Environment variables**: `CNI_COMMAND`, `CNI_CONTAINERID`, `CNI_NETNS`, `CNI_IFNAME`, `CNI_ARGS`, `CNI_PATH`
- **stdin**: JSON network configuration
- **stdout**: JSON result on success
- **stderr**: Error output on failure

**Operations (Verbs):**
- `ADD`: Create network interface and attach to container
- `DEL`: Remove network interface
- `CHECK`: Verify container networking is correct
- `VERSION`: Report supported spec versions
- `GC` (new in v1.1): Clean up stale/leaked resources (IPAM reservations, etc.)
- `STATUS` (new in v1.1): Report plugin readiness to accept ADD requests

#### Plugin Chaining

Plugins chain via `prevResult` field. The runtime passes the result of the previous plugin as input to the next. Two categories:
- **Interface plugins**: Create network interfaces (bridge, macvlan, ipvlan, etc.)
- **Chained plugins**: Modify existing interfaces (bandwidth, firewall, tuning, etc.)

#### Network Configuration Format

```json
{
  "cniVersion": "1.1.0",
  "name": "my-network",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "cni0",
      "isGateway": true,
      "ipMasq": true,
      "ipam": {
        "type": "host-local",
        "subnet": "10.88.0.0/16"
      }
    },
    {
      "type": "bandwidth",
      "ingressRate": 1000000,
      "egressRate": 1000000
    }
  ]
}
```

### 5.2 Built-in CNI Plugins

**Interface plugins (main):**

| Plugin | Description |
|--------|-------------|
| `bridge` | Creates Linux bridge, adds host and container veth pairs |
| `macvlan` | Creates new MAC address, forwards traffic to container |
| `ipvlan` | Creates ipvlan interface in container |
| `host-device` | Moves existing host device into container namespace |
| `ptp` | Point-to-point veth pair |
| `vlan` | VLAN interface |

**IPAM plugins:**

| Plugin | Description |
|--------|-------------|
| `host-local` | IP allocation from local ranges |
| `dhcp` | DHCP-based IP allocation |
| `static` | Static IP assignment |

**Meta/Chained plugins:**

| Plugin | Description |
|--------|-------------|
| `bandwidth` | Traffic control (tbf) for ingress/egress limiting |
| `firewall` | iptables/firewalld/nftables rules for container traffic |
| `tuning` | Sysctl and interface attribute tuning |
| `portmap` | Port mapping via iptables/nftables DNAT |
| `sbr` | Source-based routing |

### 5.3 Podman and containerd Networking

#### Podman: Netavark (Default since v4.0)

Netavark is a Rust-based network stack that replaced CNI. Key differences from CNI:
- **No plugin model**: Performs all setup in a single binary (better performance)
- **Aardvark-dns**: Built-in DNS server for container name resolution across all joined networks
- **Full IPv6 support**: NAT, port forwarding, static addresses on per-network basis
- **Limitations**: No DHCP support, only bridge and macvlan configs, no Kubernetes CNI plugins (Flannel, Calico)

#### containerd: CNI-based

containerd uses standard CNI plugins via its CRI plugin. Network configuration is loaded from a conflist directory. nerdctl also uses CNI for its networking.

### 5.4 Network Namespace Lifecycle

1. **Creation**: `unshare(CLONE_NEWNET)` or `clone(CLONE_NEWNET)` creates isolated network namespace
2. **Setup**: CNI/Netavark creates veth pair, assigns IPs, sets up routes and NAT
3. **Container runs**: Network namespace is bind-mounted at `/var/run/netns/<id>` or container-specific path
4. **Cleanup**: CNI DEL / Netavark cleanup removes interfaces, NAT rules, IPAM reservations
5. **Deletion**: Namespace destroyed when no processes remain and bind-mount removed

For rootless containers, an extra network namespace is created for network operations. Podman: `podman unshare --rootless-netns` to enter.

### 5.5 Port Mapping

#### Rootful Containers
- **iptables**: Traditional DNAT rules via CNI portmap plugin
- **nftables**: Modern alternative, used by newer CNI portmap plugin (fixed CVE-2025-67499 in v1.9.0)

#### Rootless Containers
- **pasta** (default since Podman 5.0): Uses gateway address from host, replaces slirp4netns
- **slirp4netns** (legacy): User-mode networking stack creating virtual network interface. Emulates full TCP/IP stack. Can cause performance degradation for high-throughput workloads.
- Rootless containers cannot bind ports < 1024 by default (no CAP_NET_BIND_SERVICE)
- Docker rootless sets `ip_unprivileged_port_start=0` within container network namespace

### 5.6 Container DNS

#### Docker's Approach
- Embedded DNS server at `127.0.0.11` for user-defined networks
- Resolves container names and aliases
- Forwards external requests to host's DNS servers
- Filters localhost nameservers from host's `/etc/resolv.conf`
- Falls back to Google DNS (8.8.8.8, 8.8.4.4) if no nameservers remain

#### Podman's Approach (Aardvark)
- Dedicated DNS server per network
- Containers can resolve names across all joined networks
- Improvement over CNI's dnsname plugin which was per-network only

#### Performance Note
Default `ndots:5` in container `/etc/resolv.conf` causes unnecessary DNS lookups. Consider setting lower for performance-sensitive workloads.

### 5.7 IPv4/IPv6 Dual-Stack

- Kubernetes: Supported by default since v1.21
- Podman: `--ipv6` flag when creating network enables dual-stack (IPv4 + IPv6 together)
- Netavark: Full IPv6 NAT and port forwarding support
- CNI: Depends on plugin configuration; bridge plugin supports dual-stack
- Requires dual-stack support in the network plugin and host kernel

---

## 6. Storage Management

### 6.1 Snapshotter Interface (containerd)

The Snapshotter interface (`github.com/containerd/containerd/v2/core/snapshots`) provides:

```go
type Snapshotter interface {
    Stat(ctx context.Context, key string) (Info, error)
    Update(ctx context.Context, info Info, fieldpaths ...string) (Info, error)
    Usage(ctx context.Context, key string) (Usage, error)
    Mounts(ctx context.Context, key string) ([]mount.Mount, error)
    Prepare(ctx context.Context, key, parent string, opts ...Opt) ([]mount.Mount, error)
    View(ctx context.Context, key, parent string, opts ...Opt) ([]mount.Mount, error)
    Commit(ctx context.Context, name, key string, opts ...Opt) error
    Remove(ctx context.Context, key string) error
    Walk(ctx context.Context, fn WalkFunc, filters ...string) error
    Close() error
}
```

Two filesystem categories:
1. **Overlay filesystems** (overlay, fuse-overlayfs): Multiple directories with file diffs per layer. Work on EXT4, XFS.
2. **Snapshotting filesystems** (btrfs, zfs, devmapper): Handle diffs at block level. Require specific filesystem formats.

### 6.2 OverlayFS

| Feature | Details |
|---------|---------|
| Kernel requirement | >= 4.0 (or RHEL 3.10.0-514+) |
| Rootless (fuse-overlayfs) | Kernel >= 4.18, requires /dev/fuse |
| Rootless (native) | Kernel >= 5.11 (5.13 with SELinux fix) |
| Copy-on-Write | File-level (copies entire file) |
| Performance | Best general-purpose; write-heavy workloads create large writable layers |
| Default for | Docker, Podman, containerd |

### 6.3 Storage Driver Comparison

| Driver | CoW Level | Pros | Cons | Status |
|--------|-----------|------|------|--------|
| overlay2 | File | Best balance of perf/simplicity, wide FS support | Large writable layers for big files | Default, recommended |
| btrfs | Block | Snapshots, compression, block-level CoW | Higher CPU (CoW overhead) | Supported |
| zfs | Block | Data integrity (checksums), compression, deduplication | High memory (ARC cache) | Supported |
| devicemapper | Block | Works on older RHEL/CentOS | **Deprecated**, poor performance | Legacy |
| vfs | None | No kernel requirements, most portable | No CoW, copies full filesystem per layer | Debug/fallback |
| fuse-overlayfs | File | Rootless without kernel 5.11+ | ~2x overhead vs kernel overlay | Rootless fallback |

### 6.4 Volume Management Patterns

| Type | Description | Persistence | Use Case |
|------|-------------|-------------|----------|
| Named Volumes | Docker/Podman managed, lifecycle-independent | Persistent | Production app data |
| Bind Mounts | Host directory mapped into container | Host-dependent | Local development |
| tmpfs | RAM-backed filesystem | Ephemeral | Caches, secrets, temp data |

Best practices:
- Named volumes for persistent data (portable, isolated)
- Bind mounts for development (hot-reload, code sharing)
- tmpfs for sensitive ephemeral data (never written to disk)

### 6.5 Layer Diff and Apply

**Diff generation:** Compare two filesystem snapshots to produce a tar archive of changes (added/modified/deleted files). Deleted files represented by whiteout files (`.wh.<filename>`).

**Apply operation:** Extract layer tar onto parent snapshot, processing whiteout files for deletions.

**umoci's approach:** Uses mtree(8) manifests to track filesystem state and generate diffs without requiring snapshot or overlay support.

---

## 7. Security

### 7.1 Rootless Container Implementation

#### User Namespaces
- `unshare(CLONE_NEWUSER)` creates new user namespace
- Maps host UID (unprivileged) to UID 0 inside container
- Configuration via `/etc/subuid` and `/etc/subgid`
- Minimum: 65,536 subordinate UIDs/GIDs per user
- Values MUST be unique per user (overlap allows cross-user namespace corruption)
- `newuidmap`/`newgidmap` SETUID binaries perform the mapping

#### Podman Rootless User Namespace Modes
- **Default**: Allocates range from subuid/subgid
- **auto** (`--userns=auto`): Automatically determines minimum UIDs needed (default 1024, auto-expanded)
- **keep-id** (`--userns=keep-id`): Maps user as itself into container (for file ownership)

#### Networking (Rootless)
- Cannot create real bridges or manipulate iptables
- **pasta** (default since Podman 5.0): Uses host gateway, better performance
- **slirp4netns**: Full TCP/IP emulation in userspace, higher overhead
- Port < 1024 binding requires `sysctl net.ipv4.ip_unprivileged_port_start=0`

### 7.2 Seccomp

#### Default Profile
- Originally by Jesse Frazelle for Docker; shared by Docker, Podman, CRI-O, containerd
- Disables ~44 syscalls out of 300+ (allowlist approach)
- Default action: `SCMP_ACT_ERRNO` (deny and return error)
- Most containers need only 40-70 syscalls (Aqua Security report)

#### BPF Compilation
- JSON profile -> parsed by container engine -> compiled to BPF via libseccomp Go wrapper
- Loaded into kernel at container creation time

#### Profile Generation
- **oci-seccomp-bpf-hook**: OCI runtime hook using eBPF to trace syscalls and generate whitelist profiles
- Run container with tracing -> generate minimal profile -> deploy with custom profile

### 7.3 AppArmor and SELinux

#### AppArmor
- Default on Debian/Ubuntu-derived systems
- Docker generates `docker-default` profile automatically (loaded from tmpfs into kernel)
- Profile-based: applied to processes, restricts file access, network, capabilities
- **Limitation**: Cannot separate containers from each other (no MCS support)

#### SELinux
- Default on Red Hat-based systems
- Label-based: resources tagged, access controlled by labels + process properties
- **Advantage**: Separates containers from each other AND from host by default (MCS)
- Types: `container_t` (container process), `container_file_t` (container files)
- Red Hat tools share unified SELinux policy

### 7.4 Linux Capabilities

#### Docker Default Capabilities
```
CAP_CHOWN, CAP_DAC_OVERRIDE, CAP_FSETID, CAP_FOWNER,
CAP_MKNOD, CAP_NET_RAW, CAP_SETGID, CAP_SETUID,
CAP_SETFCAP, CAP_SETPCAP, CAP_NET_BIND_SERVICE,
CAP_SYS_CHROOT, CAP_KILL, CAP_AUDIT_WRITE
```

#### Podman Default Capabilities
Stricter than Docker's set (further restricts the default set for better security posture).

#### Recommendations
- Drop all capabilities, add only what's needed (`--cap-drop=ALL --cap-add=...`)
- Avoid `CAP_SYS_ADMIN` (almost equivalent to root)
- For privileged ports: use `sysctl` approach over `CAP_NET_BIND_SERVICE`

### 7.5 Image Vulnerability Scanning

#### Trivy
- Comprehensive scanner: vulnerabilities, misconfigurations, secrets, licenses
- Scans: container images, filesystems, Git repos, Kubernetes
- Fast with local vulnerability database caching
- Multiple output formats

#### Grype
- Focused specifically on package vulnerability detection
- Higher accuracy for vulnerability matching
- Pairs with Syft for SBOM-first workflow:
  1. `syft <image> -o spdx-json > sbom.json` (generate SBOM once)
  2. `grype sbom:sbom.json` (scan repeatedly against updated vulnerability DB)

### 7.6 Supply Chain Security

#### SBOM Generation (Syft)
- Generates SBOMs from container images, filesystems, and packaged binaries
- Output formats: SPDX, CycloneDX, Syft JSON
- Supports signed SBOM attestations via in-toto specification

#### Attestations (in-toto)
- Framework for verifiable build provenance
- Defines "layouts" (expected build steps) and "links" (evidence of execution)
- Cosign integrates in-toto attestations for container images

#### SLSA Framework (Supply-chain Levels for Software Artifacts)
- **Level 1**: Document build process, generate provenance
- **Level 2**: Source-aware builds, signed provenance
- **Level 3**: Build definitions from source, hardened CI
- **Level 4**: Full environment accounting, dependency tracking, insider threat mitigation

SLSA provenance is generated as in-toto attestations, signed via Sigstore, and stored as OCI artifacts using the referrers API.

---

## 8. CLI Design Patterns

### 8.1 Docker CLI Command Tree

```
docker
  container   (run, create, start, stop, restart, kill, rm, ps, logs, exec, attach, wait, pause, unpause, rename, inspect, diff, export, cp, stats, top, update, port, prune)
  image       (ls, pull, push, build, tag, rmi, inspect, history, save, load, import, prune)
  volume      (create, ls, inspect, rm, prune)
  network     (create, ls, inspect, rm, connect, disconnect, prune)
  system      (df, events, info, prune)
  manifest    (create, inspect, annotate, push, rm)
  buildx      (build, create, inspect, ls, rm, stop, use, bake, imagetools, prune)
  compose     (up, down, build, ps, logs, exec, run, start, stop, restart, pull, push, config, create, events, images, kill, pause, unpause, port, rm, top, version, cp)
  context     (create, ls, inspect, rm, use)
  trust       (key, sign, inspect, revoke, signer)
  plugin      (create, enable, disable, inspect, install, ls, push, rm, set, upgrade)
  config      (create, ls, inspect, rm)
  secret      (create, ls, inspect, rm)
  node        (inspect, ls, promote, demote, ps, rm, update)
  service     (create, inspect, logs, ls, ps, rm, rollback, scale, update)
  stack       (deploy, ls, ps, rm, services)
  swarm       (ca, init, join, join-token, leave, unlock, unlock-key, update)
  builder     (build, prune)
  scout       (cves, quickview, compare, recommendations, ...)
  checkpoint  (create, ls, rm)
  # Top-level shortcuts
  run, exec, ps, build, pull, push, images, login, logout, search, version, info, inspect
```

### 8.2 Podman CLI Command Tree

```
podman
  container   (same as docker container + checkpoint, restore, cleanup, exists, init, mount, unmount)
  image       (same as docker image + exists, mount, unmount, tree, sign, trust)
  volume      (create, ls, inspect, rm, prune, exists, export, import, reload)
  network     (create, ls, inspect, rm, prune, connect, disconnect, exists, reload, update)
  pod         (create, start, stop, restart, kill, rm, ps, inspect, logs, top, pause, unpause, exists, clone, stats)
  system      (df, events, info, prune, connection, migrate, renumber, reset, service)
  manifest    (create, inspect, annotate, push, rm, add, exists)
  machine     (init, start, stop, rm, ls, inspect, ssh, set, reset, info, os)
  secret      (create, ls, inspect, rm, exists)
  kube        (play, generate, down, apply)
  compose     (full Docker Compose compatibility)
  generate    (systemd, kube, spec)
  healthcheck (run)
  # Top-level shortcuts same as Docker
```

**Key Podman differences from Docker:**
- `pod` management command (unique to Podman)
- `machine` command for managing Podman VMs
- `kube` command for Kubernetes manifest operations
- `generate systemd` for systemd unit file generation
- `system connection` for managing remote Podman instances
- No `swarm`, `stack`, `config`, `plugin` commands

### 8.3 nerdctl CLI Command Tree

```
nerdctl
  container   (similar to docker container)
  image       (similar to docker + convert, encrypt, decrypt)
  volume      (create, ls, inspect, rm, prune)
  network     (create, ls, inspect, rm, prune)
  namespace   (create, inspect, ls, remove, update)    # containerd-specific
  system      (prune, events, info)
  manifest    (annotate, create, inspect, push, rm)
  compose     (full Compose support)
  builder     (prune, debug)
  apparmor    (inspect, load, ls, unload)               # containerd-specific
  checkpoint  (create, list, remove)
  ipfs        (registry serve)                           # unique to nerdctl
  # Special features
  --snapshotter=stargz|nydus|overlaybd|soci              # lazy pulling
  image encrypt/decrypt                                   # OCI encryption
  --namespace=<NS>                                        # containerd namespaces
  --verify=cosign / --sign=cosign                         # Sigstore integration
```

### 8.4 crictl Command Structure

```
crictl
  # Pod operations
  runp, stopp, rmp, pods, inspectp
  # Container operations
  create, start, stop, rm, ps, exec, attach, logs, inspect, update
  # Image operations
  pull, images, inspecti, rmi
  # Runtime operations
  info, version, stats, statsp
  # Config
  config
```

crictl is focused on CRI operations only. It speaks the CRI gRPC API directly to containerd/CRI-O.

### 8.5 Common CLI Patterns

#### Output Formatting
- `--format` with Go templates: `docker ps --format "{{.Names}}\t{{.Status}}"`
- `table` function for auto-width columns: `--format "table {{.Name}}\t{{.Image}}"`
- `--output json` or `--output yaml` for machine-readable output
- ORAS uses `--format json` + `--template` for Go template language

#### Filtering
- `--filter` or `-f`: `docker ps -f "status=running" -f "label=app=web"`
- Multiple filters ANDed together
- Common filter keys: name, label, status, id, ancestor, network

#### Plugin Mechanism (Docker CLI)
- Plugins are binaries matching `docker-[a-z][a-z0-9]*`
- Searched in `~/.docker/cli-plugins/` and system paths
- Must implement `docker-cli-plugin-metadata` subcommand returning JSON:
  ```json
  {"SchemaVersion": "0.1.0", "Vendor": "Example Inc.", "Version": "1.0.0", "ShortDescription": "A plugin example"}
  ```
- Plugin name must not conflict with built-in commands
- Examples: docker-buildx, docker-compose, docker-scout

---

## 9. Go Libraries (Current Versions)

### 9.1 OCI Specification Libraries

| Library | Latest Version | Notes |
|---------|---------------|-------|
| `github.com/opencontainers/image-spec` | v1.1.1 | Go types for OCI image manifest, config, index, descriptors |
| `github.com/opencontainers/runtime-spec` | v1.3.0 (Nov 2025) | Go types for runtime config.json, Linux-specific structs |

### 9.2 Container Registry Libraries

| Library | Latest Version | Key Features |
|---------|---------------|--------------|
| `github.com/google/go-containerregistry` | v0.21.2 (Mar 2026) | Immutable Image/Layer/ImageIndex interfaces; crane CLI; authn keychain; remote package for registry ops; registry package for testing |
| `oras.land/oras-go/v2` | v2.x stable (v3 in dev) | Push/pull artifacts; Docker credential helpers; OCI distribution-spec v1.1.1 compliant; in-memory and filesystem stores |

### 9.3 Container Runtime Libraries

| Library | Latest Version | Key Features |
|---------|---------------|--------------|
| `github.com/containerd/containerd/v2` | v2.2.1 (Dec 2025) | Client API, content store, snapshots, images, transfer service, sandbox service, plugin system |
| `github.com/opencontainers/runc/libcontainer` | v1.4.0 | Native Go implementation: namespaces, cgroups, capabilities, filesystem access controls |

### 9.4 Networking Libraries

| Library | Latest Version | Key Features |
|---------|---------------|--------------|
| `github.com/containernetworking/cni` | v1.3.0 | CNI spec v1.1.0 support; plugin loading; config parsing; GC and STATUS verbs |
| `github.com/containernetworking/plugins` | v1.9.1 | bridge, macvlan, ipvlan, host-device, bandwidth, firewall, tuning, portmap, sbr, host-local, dhcp, static |
| `github.com/vishvananda/netlink` | v1.x (actively maintained) | Netlink socket library; iproute2-like API; links, addresses, routes, iptables, tunnels, bonds, RDMA |

### 9.5 CLI and TUI Libraries

| Library | Latest Version | Key Features |
|---------|---------------|--------------|
| `github.com/spf13/cobra` | v1.10.2 (Dec 2025) | CLI framework; auto-generated shell completions (bash/zsh/fish/powershell); man pages; intelligent suggestions; used by Kubernetes, Docker, Hugo, GitHub CLI |
| `github.com/charmbracelet/bubbletea` | v2.0.0 (Sep 2025) | TUI framework; Elm Architecture; new Cursed Renderer; Mode 2026 support; clipboard ops; SSH-compatible |
| `github.com/charmbracelet/lipgloss` | v2.0.0 (2025) | Terminal styling; declarative approach; table rendering; border/margin/padding; color support (adaptive, ANSI, hex); built on termenv |

### 9.6 Library Ecosystem Map

```
                    +--------------------------+
                    |     CLI Layer (cobra)     |
                    +-----------+--------------+
                                |
                    +-----------v--------------+
                    |   TUI Layer (bubbletea)   |
                    |   Style Layer (lipgloss)  |
                    +-----------+--------------+
                                |
         +----------------------+----------------------+
         |                      |                      |
+--------v--------+  +---------v--------+  +----------v---------+
|  Registry Ops   |  |  Runtime Ops     |  |  Network Ops       |
| go-containerreg |  | runc/libcontainer|  | cni                |
| oras-go         |  | containerd/v2    |  | plugins            |
| image-spec      |  | runtime-spec     |  | netlink            |
+-----------------+  +------------------+  +--------------------+
```

### 9.7 Key Design Considerations for Library Selection

1. **go-containerregistry vs oras-go**: go-containerregistry has broader adoption (1,559 importers) and includes crane CLI. oras-go is more focused on artifact management and OCI v1.1 features. Consider using both: go-containerregistry for image operations, oras-go for artifact management.

2. **containerd/v2 client vs direct runc/libcontainer**: containerd provides higher-level abstractions (snapshotter, content store, transfer service) but requires a daemon. libcontainer provides direct container creation without daemon dependency.

3. **CNI vs Netavark model**: CNI is the standard with broad plugin ecosystem. Netavark (Podman) shows that a non-plugin monolithic approach can be faster. Maestro could support CNI plugins while optimizing common cases internally.

4. **containers/image + containers/storage**: The Podman/Buildah/CRI-O/Skopeo shared library stack is a proven approach for image and storage management without daemon dependency. Worth evaluating as an alternative to containerd's approach.

---

## Appendix A: Key URLs

### Specifications
- OCI Image Spec: https://github.com/opencontainers/image-spec
- OCI Distribution Spec: https://github.com/opencontainers/distribution-spec
- OCI Runtime Spec: https://github.com/opencontainers/runtime-spec
- CNI Spec: https://www.cni.dev/docs/spec/

### Tools
- containerd: https://github.com/containerd/containerd
- Podman: https://github.com/containers/podman
- CRI-O: https://github.com/cri-o/cri-o
- Docker/Moby: https://github.com/moby/moby
- BuildKit: https://github.com/moby/buildkit
- skopeo: https://github.com/containers/skopeo
- buildah: https://github.com/containers/buildah
- umoci: https://github.com/opencontainers/umoci
- ORAS: https://github.com/oras-project/oras
- nerdctl: https://github.com/containerd/nerdctl

### Runtimes
- runc: https://github.com/opencontainers/runc
- crun: https://github.com/containers/crun
- youki: https://github.com/youki-dev/youki
- gVisor: https://github.com/google/gvisor
- Kata Containers: https://github.com/kata-containers/kata-containers

### Security
- Sigstore/cosign: https://github.com/sigstore/cosign
- Notation: https://notaryproject.dev/
- Trivy: https://github.com/aquasecurity/trivy
- Grype: https://github.com/anchore/grype
- Syft: https://github.com/anchore/syft
- SLSA: https://slsa.dev/

### Go Libraries
- go-containerregistry: https://github.com/google/go-containerregistry
- oras-go: https://github.com/oras-project/oras-go
- cobra: https://github.com/spf13/cobra
- bubbletea: https://github.com/charmbracelet/bubbletea
- lipgloss: https://github.com/charmbracelet/lipgloss
- netlink: https://github.com/vishvananda/netlink

### Lazy Pulling
- stargz-snapshotter (eStargz): https://github.com/containerd/stargz-snapshotter
- nydus-snapshotter: https://github.com/containerd/nydus-snapshotter
- fuse-overlayfs: https://github.com/containers/fuse-overlayfs

## Appendix B: Architectural Lessons from Existing Tools

### Lesson 1: Daemon vs Daemonless
Docker's daemon model provides centralized management but is a single point of failure and security concern. Podman's daemonless approach eliminates both issues but requires different patterns for long-running services (systemd integration, Quadlet files).

**Recommendation for Maestro:** Consider a hybrid approach -- daemonless for CLI operations, optional lightweight daemon for advanced features (event streaming, background GC, watch operations).

### Lesson 2: Shared Libraries
The containers/image + containers/storage + conmon stack used by Podman/Buildah/Skopeo/CRI-O demonstrates the power of shared libraries. It ensures consistent behavior across tools and reduces maintenance burden.

**Recommendation for Maestro:** Design core operations as libraries first, CLI second. This enables future tooling to build on the same foundation.

### Lesson 3: Runtime Pluggability
containerd's ShimV2 and the OCI Runtime Spec enable runtime pluggability. Users can choose runc, crun, youki, gVisor, or Kata based on their security/performance needs.

**Recommendation for Maestro:** Support runtime pluggability from day one via OCI Runtime Spec compliance.

### Lesson 4: Storage Driver Abstraction
containerd's snapshotter interface cleanly abstracts storage operations. This enables features like lazy pulling (remote snapshotters) without changing upper layers.

**Recommendation for Maestro:** Design a clean storage abstraction layer similar to containerd's snapshotter interface.

### Lesson 5: Networking Evolution
The evolution from CNI to Netavark in Podman shows that the plugin model has overhead. However, CNI's plugin ecosystem is valuable.

**Recommendation for Maestro:** Support CNI plugins for compatibility, but implement optimized built-in networking for common cases (bridge, macvlan).

### Lesson 6: Security by Default
Podman's rootless-by-default approach is increasingly the industry standard. Docker now offers rootless but it's opt-in.

**Recommendation for Maestro:** Rootless by default, rootful as opt-in. Default seccomp profile, capability dropping, and user namespace isolation.

### Lesson 7: OCI v1.1 First
The OCI v1.1 spec with artifacts, referrers, and subject fields is the foundation for supply chain security. Tools that natively support it (ORAS, cosign) are better positioned.

**Recommendation for Maestro:** Build OCI v1.1 support as a first-class feature, not an afterthought. Native artifact and referrers support.
