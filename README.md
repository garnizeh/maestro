# Maestro

> *"The man in black fled across the desert, and the gunslinger followed."*

A modern, daemonless OCI container manager built in Go. Rootless by default, with native OCI v1.1 artifact support and a security-first architecture.

Made by **garnizeH labs** - 2026.

**Status:** Pre-implementation (design and specification phase complete)

---

## Vision

Maestro aims to be the container tool developers reach for when they want Docker-level UX with Podman-level security, plus native OCI v1.1 artifact support that no existing tool does well out of the box.

Key design decisions:

- **Daemonless** — no long-running daemon; each CLI invocation is self-contained
- **Rootless by default** — all operations run without root (Calla mode)
- **OCI v1.1 native** — first-class support for artifacts, referrers API, and supply chain security
- **Pluggable runtimes** — runc, crun, youki, gVisor, and Kata via the Eld interface
- **Library-first** — core logic as Go libraries; CLI is a thin wrapper

## Architecture

All internal components are named after Stephen King's *The Dark Tower* mythology:

| Component | Dark Tower Name | Role |
|-----------|----------------|------|
| Core engine | **Tower** | Orchestrates all subsystems |
| Image manager | **Maturin** | Content-addressable store, pull/push |
| Container lifecycle | **Gan** | Create, start, stop, kill, exec |
| Runtime abstraction | **Eld** | Interface for runc/crun/youki/gVisor/Kata |
| Network manager | **Beam** | CNI integration, DNS, port mapping |
| Storage manager | **Prim** | Snapshotter abstraction, volumes |
| Registry client | **Shardik** | OCI Distribution Spec v1.1.0 |
| Security | **White** | Seccomp, capabilities, rootless, signing |
| Artifact manager | **Rose** | OCI artifacts via ORAS |
| State store | **Waystation** | File-based state with flock locking |
| API server | **Positronics** | Optional gRPC/REST socket mode |
| TUI dashboard | **Glass** | Terminal UI with bubbletea |
| CLI layer | **Dinh** | cobra-based command interface |

See [docs/dark-tower-naming-map.md](docs/dark-tower-naming-map.md) for the full naming reference with justifications.

## Project Structure

```
maestro/
├── cmd/maestro/              # Application entry point
├── internal/
│   ├── tower/                # Core engine
│   ├── maturin/              # Image management
│   ├── gan/                  # Container lifecycle
│   ├── eld/                  # Runtime abstraction
│   ├── beam/                 # Network management
│   ├── prim/                 # Storage management
│   ├── shardik/              # Registry client
│   ├── white/                # Security subsystem
│   ├── rose/                 # Artifact management
│   ├── waystation/           # State management
│   ├── positronics/          # API server (socket mode)
│   ├── glass/                # TUI dashboard
│   └── cli/                  # CLI commands (Dinh)
├── pkg/
│   ├── types/                # Shared type definitions
│   ├── client/               # Positronics API client
│   └── specgen/              # OCI spec generator
├── configs/                  # Default configs (seccomp, CNI, katet.toml)
├── openspec/                 # OpenSpec behavioral specifications
│   ├── config.yaml           # Project config
│   ├── specs/                # Source of truth (13 component specs)
│   ├── changes/              # Active changes
│   └── schemas/maestro/      # Custom schema + templates
├── docs/
│   ├── design-document.md    # Full architecture design
│   ├── roadmap.md            # Implementation roadmap (209 tasks)
│   ├── agent-protocol.md     # Development protocol
│   ├── dark-tower-naming-map.md
│   └── oci-ecosystem-research.md
├── test/
│   ├── integration/          # Integration tests
│   ├── e2e/                  # End-to-end tests
│   └── testutil/             # Test helpers
├── go.mod
├── Makefile
├── CHANGELOG.md
└── README.md
```

## Prerequisites

- Go 1.26.1+
- Git
- An OCI runtime: [crun](https://github.com/containers/crun) (recommended), [runc](https://github.com/opencontainers/runc), or [youki](https://github.com/youki-dev/youki)
- [conmon-rs](https://github.com/containers/conmon-rs) (container monitor)
- CNI plugins: [containernetworking/plugins](https://github.com/containernetworking/plugins)
- For rootless: `newuidmap`/`newgidmap`, entries in `/etc/subuid` and `/etc/subgid`
- For rootless networking: [pasta](https://passt.top/passt/) (recommended) or slirp4netns

## Getting Started

> Implementation has not started yet. The commands below represent the target UX.

```bash
# Build
make build

# Run a container (rootless by default)
maestro run -d -p 8080:80 --name web nginx:latest

# List containers
maestro ps

# View logs
maestro logs -f web

# Stop and remove
maestro stop web
maestro rm web

# Pull an image
maestro pull alpine:latest

# TUI dashboard
maestro dashboard

# System diagnostics
maestro system check
```

## Configuration

Maestro uses TOML configuration at `~/.config/maestro/katet.toml`:

```toml
[runtime]
default = "crun"

[storage]
driver = "overlay"
max_size = "50GB"

[network]
default_subnet = "10.99.0.0/16"
dns_enabled = true

[security]
rootless = true
default_seccomp = "builtin"
```

See [configs/katet.toml.example](configs/katet.toml.example) for the full reference.

## Development

```bash
make build              # Compile binary
make test               # Unit tests (with -race)
make test-integration   # Integration tests
make test-e2e           # End-to-end tests
make lint               # golangci-lint
make fmt                # gofumpt
make clean              # Remove build artifacts
```

### Development Protocol

This project follows a strict development protocol. The four laws:

1. **Tests Passing** — writing tests is sine qua non; 70% coverage minimum
2. **Documentation Updated** — specs, help text, README
3. **Commit on Task Branch** — conventional commits, never commit to main
4. **Roadmap is Source of Truth** — [docs/roadmap.md](docs/roadmap.md) reflects reality at all times

### Branch Naming

```
p<phase>/<milestone>-<description>
```

Examples: `p1/1.2-drawing-image-pull`, `p1/3.1-dinh-root-command`

### Commit Format

```
<type>(<component>): <description>

Roadmap #N, <change-name>
```

Types: `feat`, `fix`, `test`, `refactor`, `docs`, `chore`, `perf`
Components: `tower`, `maturin`, `gan`, `eld`, `beam`, `prim`, `shardik`, `white`, `rose`, `waystation`, `positronics`, `glass`, `dinh`

## Roadmap

The implementation is organized in three phases:

### Phase 1 — "The Gunslinger" (MVP)

Pull and run a container rootless on Linux. 87 tasks, 243 requirements, 992 scenarios.

- 1.1 The Tower rises (scaffold, CLI, config, state store)
- 1.2 Drawing of the Three (image pull from registries)
- 1.3 Gan creates (container run with OCI runtime)
- 1.4 The Beam connects (networking with CNI)
- 1.5 The Calla stands (rootless mode, security defaults)

### Phase 2 — "The Drawing of the Three" (Core)

Feature-complete for daily development. 66 tasks.

- 2.1 Maturin grows (push, tag, save, load, GC)
- 2.2 Roland's arsenal (exec, logs, stats, pause, cp)
- 2.3 The Dogans persist (volumes)
- 2.4 Beams multiply (custom networks, DNS)
- 2.5 Eld's lineage (multi-runtime support)
- 2.6 Maerlyn's Rainbow (TUI dashboard)

### Phase 3 — "The Dark Tower" (Advanced)

Supply chain security, artifacts, API server. 39 tasks.

- 3.1 The Eld Mark (image signing, verification)
- 3.2 The Rose blooms (OCI artifact management)
- 3.3 Positronics awakens (gRPC API server)
- 3.4 Systemd integration
- 3.5 Lazy Drawing (eStargz lazy pull)
- 3.6 Other worlds (macOS/Windows via VM)

Full details: [docs/roadmap.md](docs/roadmap.md)

## Documentation

| Document | Description |
|----------|-------------|
| [Design Document](docs/design-document.md) | Full architecture with component design, data models, and API |
| [Roadmap](docs/roadmap.md) | 209 tasks with dependencies, complexity, and acceptance criteria |
| [OCI Research](docs/oci-ecosystem-research.md) | Comprehensive OCI ecosystem research |
| [Dark Tower Naming](docs/dark-tower-naming-map.md) | Component-to-mythology mapping with justifications |

## Tech Stack

| Category | Choice |
|----------|--------|
| Language | Go 1.26.1+ |
| CLI | [cobra](https://github.com/spf13/cobra) v1.10+ |
| TUI | [bubbletea](https://github.com/charmbracelet/bubbletea) v2 + [lipgloss](https://github.com/charmbracelet/lipgloss) v2 |
| Registry | [go-containerregistry](https://github.com/google/go-containerregistry) v0.21+ |
| Artifacts | [oras-go](https://github.com/oras-project/oras-go) v2 |
| Networking | [CNI](https://github.com/containernetworking/cni) v1.3 + [plugins](https://github.com/containernetworking/plugins) v1.9 |
| Config | TOML via [go-toml](https://github.com/pelletier/go-toml) v2 |
| Logging | [zerolog](https://github.com/rs/zerolog) |
| API | gRPC + grpc-gateway |

## License

[Apache 2.0](LICENSE)

---

> *"Go then, there are other worlds than these."* — Jake Chambers
