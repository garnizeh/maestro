# Maestro Embedded Binaries

This directory manages the static binaries embedded into the Maestro executable to ensure full rootless portability.

## Contents

- `assets/fuse-overlayfs`: Static binary from [containers/fuse-overlayfs](https://github.com/containers/fuse-overlayfs).
- `assets/pasta`: Static binary from the `passt` project (extracted from `mgoltzsche/podman-static`).

## Why Embedding?

Maestro aims to be a "zero-dependency" container engine for unprivileged users. By embedding key tools like `fuse-overlayfs` (for storage) and `pasta` (for networking), we ensure that Maestro works out-of-the-box even on minimal distributions where these tools are not pre-installed.

## Management & Updates

### Obtaining Binaries

The binaries in `assets/` should be **statically linked** for Linux x86_64. 

- **fuse-overlayfs**: Download from the official [releases page](https://github.com/containers/fuse-overlayfs/releases).
- **pasta**: Can be compiled from source using `make LDFLAGS="-static"` in the [passt repository](https://passt.top/passt) or extracted from a reliable static distribution like `mgoltzsche/podman-static`.

### Development Workflow

1. The `assets/` directory is ignored by Git to avoid bloating the repository with large binaries.
2. Developers must ensure these binaries are present in `internal/bin/assets/` before building Maestro, otherwise the `go:embed` directive will fail.
3. To update a binary, simply replace the file in `assets/` and rebuild. Maestro's `internal/bin` package will automatically detect the change (via SHA256) and re-extract the updated binary to the user's local share directory (`~/.local/share/maestro/bin`).

## Future Support

Currently, only **Linux x86_64** is supported for binary embedding. Support for other architectures (AArch64) and platforms (macOS/Windows via VM-based wrappers) is planned for future milestones.
