# Dinh CLI Specification

## Purpose

Dinh is the CLI layer of Maestro -- the root of command authority from which all subcommands branch. Dinh provides the user-facing interface for managing containers, images, volumes, networks, artifacts, and system operations. It handles global flags, output formatting, configuration loading, shell completions, and error reporting.

> *"The dinh is the leader of a ka-tet -- the one who commands and from whom all orders flow."*

---

## Requirements

### Requirement: Root Command

The root command `maestro` MUST serve as the entry point for all CLI operations. When invoked without a subcommand, Dinh MUST display help text listing all available subcommand groups, top-level shortcuts, and global flags.

#### Scenario: Root command with no arguments

GIVEN the user invokes `maestro` with no arguments
WHEN the command executes
THEN help text MUST be displayed on standard output
AND the help text MUST list all subcommand groups (container, image, volume, network, artifact, system, service, generate, config)
AND the help text MUST list all top-level shortcuts (run, exec, ps, pull, push, images, login, logout)
AND the exit code MUST be 0

#### Scenario: Root command with --help flag

GIVEN the user invokes `maestro --help`
WHEN the command executes
THEN help text MUST be displayed on standard output
AND all global flags MUST be listed with their descriptions and default values
AND the exit code MUST be 0

#### Scenario: Root command with unknown subcommand

GIVEN the user invokes `maestro foobar`
WHEN the command executes
THEN an error message MUST be written to standard error indicating that `foobar` is not a recognized command
AND the error SHOULD suggest similar valid commands if any exist
AND the exit code MUST be non-zero

---

### Requirement: Global Flags

Dinh MUST support the following global flags on every command and subcommand. Global flags MUST be accepted before or after the subcommand name.

- `--config <path>` -- Path to the Ka-tet config file. Default: `~/.config/maestro/katet.toml`
- `--log-level <level>` -- Log verbosity level. Valid values: `debug`, `info`, `warn`, `error`. Default: `warn`
- `--runtime <name>` -- Eld runtime override. Valid values: `runc`, `crun`, `youki`, `runsc`, `kata`, `auto`. Default: `auto`
- `--storage-driver <driver>` -- Prim driver override. Valid values: `overlay`, `btrfs`, `zfs`, `vfs`, `auto`. Default: `auto`
- `--root <path>` -- Waystation root directory. Default: `~/.local/share/maestro`
- `--host <uri>` -- Positronics socket URI for API mode
- `--format <format>` -- Output format. Valid values: `table`, `json`, `yaml`, or a Go template string. Default: `table`
- `--no-color` -- Disable colored output
- `--quiet` / `-q` -- Show only resource IDs in output

#### Scenario: Config flag overrides default config path

GIVEN a config file exists at `/custom/config/katet.toml`
WHEN the user invokes `maestro --config /custom/config/katet.toml version`
THEN the configuration MUST be loaded from `/custom/config/katet.toml`

#### Scenario: Log level flag controls verbosity

GIVEN the user invokes `maestro --log-level debug ps`
WHEN the command executes
THEN debug-level log messages MUST be written to standard error
AND normal command output MUST still be written to standard output

#### Scenario: Runtime flag overrides auto-detection

GIVEN the user invokes `maestro --runtime crun run nginx`
WHEN a container is created
THEN the `crun` runtime MUST be used regardless of auto-detection result

#### Scenario: Invalid global flag value

GIVEN the user invokes `maestro --log-level verbose ps`
WHEN the command is parsed
THEN an error MUST be returned indicating `verbose` is not a valid log level
AND valid values MUST be listed in the error message

#### Scenario: Global flags before subcommand

GIVEN the user invokes `maestro --format json container ps`
WHEN the command executes
THEN the output MUST be in JSON format

#### Scenario: Global flags after subcommand

GIVEN the user invokes `maestro container ps --format json`
WHEN the command executes
THEN the output MUST be in JSON format

#### Scenario: Quiet flag produces IDs only

GIVEN the user invokes `maestro ps -q`
WHEN the command executes and containers exist
THEN only container IDs MUST be printed, one per line, with no headers or extra formatting

#### Scenario: No-color flag disables ANSI codes

GIVEN the user invokes `maestro --no-color ps`
WHEN the command executes
THEN the output MUST NOT contain ANSI escape codes for color

---

### Requirement: Subcommand Groups

Dinh MUST organize commands into the following subcommand groups, each corresponding to a Dark Tower component. Every group MUST display its own help text when invoked with `--help` or without a subcommand.

- `container` (Gunslinger) -- container lifecycle operations
- `image` (Archivist) -- image management operations
- `volume` (Keeper) -- volume management operations
- `network` (Beamseeker) -- network management operations
- `artifact` (Collector) -- OCI artifact operations
- `system` (An-tet) -- system introspection and maintenance
- `service` (Positronics) -- API server management
- `generate` -- code generation (systemd units, completions)
- `config` -- configuration management

#### Scenario: Container subcommand group lists all commands

GIVEN the user invokes `maestro container --help`
WHEN the help is displayed
THEN it MUST list all container subcommands: create, start, stop, restart, kill, rm, ps, logs, exec, attach, inspect, top, stats, port, cp, diff, wait, pause, unpause, rename, prune

#### Scenario: Image subcommand group lists all commands

GIVEN the user invokes `maestro image --help`
WHEN the help is displayed
THEN it MUST list all image subcommands: pull, push, ls, rm, inspect, history, tag, save, load, sign, verify, prune

#### Scenario: Volume subcommand group lists all commands

GIVEN the user invokes `maestro volume --help`
WHEN the help is displayed
THEN it MUST list all volume subcommands: create, ls, inspect, rm, prune

#### Scenario: Network subcommand group lists all commands

GIVEN the user invokes `maestro network --help`
WHEN the help is displayed
THEN it MUST list all network subcommands: create, ls, inspect, rm, connect, disconnect, prune

#### Scenario: Artifact subcommand group lists all commands

GIVEN the user invokes `maestro artifact --help`
WHEN the help is displayed
THEN it MUST list all artifact subcommands: push, pull, attach, ls, inspect

#### Scenario: System subcommand group lists all commands

GIVEN the user invokes `maestro system --help`
WHEN the help is displayed
THEN it MUST list all system subcommands: info, df, prune, events, check

#### Scenario: Service subcommand group lists all commands

GIVEN the user invokes `maestro service --help`
WHEN the help is displayed
THEN it MUST list all service subcommands: start, stop, status

#### Scenario: Generate subcommand group lists all commands

GIVEN the user invokes `maestro generate --help`
WHEN the help is displayed
THEN it MUST list all generate subcommands: systemd, completion

#### Scenario: Config subcommand group lists all commands

GIVEN the user invokes `maestro config --help`
WHEN the help is displayed
THEN it MUST list all config subcommands: show, edit

#### Scenario: Subcommand group without subcommand shows help

GIVEN the user invokes `maestro container` with no subcommand
WHEN the command executes
THEN help text for the container group MUST be displayed
AND the exit code MUST be 0

---

### Requirement: Top-Level Shortcuts

Dinh MUST provide the following top-level shortcuts that delegate to their corresponding subcommand. Each shortcut MUST accept the same flags and arguments as the full subcommand path. The help text for a shortcut MUST be identical to its corresponding full command.

- `maestro run` delegates to `maestro container run`
- `maestro exec` delegates to `maestro container exec`
- `maestro ps` delegates to `maestro container ps`
- `maestro pull` delegates to `maestro image pull`
- `maestro push` delegates to `maestro image push`
- `maestro images` delegates to `maestro image ls`
- `maestro login` delegates to `maestro login` (authentication, top-level)
- `maestro logout` delegates to `maestro logout` (deauthentication, top-level)

#### Scenario: Run shortcut equivalence

GIVEN the user invokes `maestro run -d -p 8080:80 nginx:latest`
WHEN the command executes
THEN the behavior MUST be identical to `maestro container run -d -p 8080:80 nginx:latest`

#### Scenario: Ps shortcut equivalence

GIVEN the user invokes `maestro ps --all`
WHEN the command executes
THEN the behavior MUST be identical to `maestro container ps --all`

#### Scenario: Pull shortcut equivalence

GIVEN the user invokes `maestro pull nginx:latest`
WHEN the command executes
THEN the behavior MUST be identical to `maestro image pull nginx:latest`

#### Scenario: Images shortcut equivalence

GIVEN the user invokes `maestro images --format json`
WHEN the command executes
THEN the behavior MUST be identical to `maestro image ls --format json`

#### Scenario: Shortcut help matches full command help

GIVEN the user invokes `maestro run --help`
WHEN the help is displayed
THEN the content MUST be identical to the output of `maestro container run --help`

#### Scenario: Exec shortcut equivalence

GIVEN the user invokes `maestro exec -it abc123 bash`
WHEN the command executes
THEN the behavior MUST be identical to `maestro container exec -it abc123 bash`

#### Scenario: Push shortcut equivalence

GIVEN the user invokes `maestro push myregistry.io/myapp:v1`
WHEN the command executes
THEN the behavior MUST be identical to `maestro image push myregistry.io/myapp:v1`

---

### Requirement: Output Formatting -- Table

When `--format table` is specified or no format is specified, Dinh MUST render output as a human-readable, column-aligned table. Table output MUST include a header row. Column widths MUST adapt to the data content. When output is connected to a TTY, table output SHOULD be styled with colors and alignment.

#### Scenario: Default table output for container list

GIVEN containers exist in the system
WHEN the user invokes `maestro ps`
THEN the output MUST be a table with columns: CONTAINER ID, IMAGE, COMMAND, CREATED, STATUS, PORTS, NAMES
AND the header row MUST be present
AND columns MUST be aligned

#### Scenario: Default table output for image list

GIVEN images exist in the system
WHEN the user invokes `maestro images`
THEN the output MUST be a table with columns: REPOSITORY, TAG, IMAGE ID, CREATED, SIZE

#### Scenario: Table output with no data

GIVEN no containers exist
WHEN the user invokes `maestro ps`
THEN only the header row MUST be displayed (or an informational message)
AND the exit code MUST be 0

#### Scenario: Table column widths adapt to content

GIVEN a container with a name of 50 characters exists
WHEN the user invokes `maestro ps`
THEN the NAMES column MUST be wide enough to display the full name without truncation

---

### Requirement: Output Formatting -- JSON

When `--format json` is specified, Dinh MUST render output as valid, parseable JSON. The output MUST be written to standard output. List commands MUST produce a JSON array. Single-item commands MUST produce a JSON object.

#### Scenario: JSON output for container list

GIVEN containers exist in the system
WHEN the user invokes `maestro ps --format json`
THEN the output MUST be a valid JSON array
AND each element MUST contain at minimum: `id`, `image`, `status`, `names`

#### Scenario: JSON output for single item inspect

GIVEN a container with ID `abc123` exists
WHEN the user invokes `maestro container inspect abc123 --format json`
THEN the output MUST be a valid JSON object containing all container state fields

#### Scenario: JSON output with empty results

GIVEN no containers exist
WHEN the user invokes `maestro ps --format json`
THEN the output MUST be an empty JSON array: `[]`

#### Scenario: JSON output is machine-parseable

GIVEN any command produces JSON output
WHEN the output is parsed by a standard JSON parser
THEN it MUST parse without errors
AND the output MUST NOT contain ANSI escape codes or decorative characters

#### Scenario: Version command with JSON format

GIVEN the user invokes `maestro version --format json`
WHEN the command executes
THEN the output MUST be a valid JSON object containing version, commit, build date, Go version, and OS/architecture

---

### Requirement: Output Formatting -- YAML

When `--format yaml` is specified, Dinh MUST render output as valid, parseable YAML. The output MUST be written to standard output.

#### Scenario: YAML output for container inspect

GIVEN a container with ID `abc123` exists
WHEN the user invokes `maestro container inspect abc123 --format yaml`
THEN the output MUST be valid YAML containing all container state fields

#### Scenario: YAML output for list commands

GIVEN containers exist in the system
WHEN the user invokes `maestro ps --format yaml`
THEN the output MUST be a valid YAML sequence (list)

---

### Requirement: Output Formatting -- Go Templates

When `--format` is specified with a Go template string (detected by the presence of `{{` and `}}`), Dinh MUST evaluate the template against each output item. This provides Docker-compatible custom formatting.

#### Scenario: Go template for container list

GIVEN containers exist in the system
WHEN the user invokes `maestro ps --format '{{.ID}}\t{{.Image}}\t{{.Status}}'`
THEN each line of output MUST contain the container ID, image, and status separated by tabs
AND no header row MUST be printed

#### Scenario: Go template accessing nested fields

GIVEN a container with network information exists
WHEN the user invokes `maestro container inspect abc123 --format '{{.Beam.Networks.beam0.IPAddress}}'`
THEN the output MUST be the container's IP address on the beam0 network

#### Scenario: Invalid Go template

GIVEN the user invokes `maestro ps --format '{{.Invalid'`
WHEN the command executes
THEN an error MUST be returned indicating the template is malformed
AND the error MUST be written to standard error

#### Scenario: Go template with json function

GIVEN a container exists
WHEN the user invokes `maestro container inspect abc123 --format '{{json .Ka}}'`
THEN the output MUST be the `ka` field serialized as JSON

---

### Requirement: Quiet Mode

When `--quiet` or `-q` is specified, Dinh MUST output only resource identifiers (IDs), one per line, with no headers, decorations, or additional fields. Quiet mode MUST take precedence over `--format` for list commands.

#### Scenario: Quiet mode for container list

GIVEN three containers exist with IDs `aaa111`, `bbb222`, `ccc333`
WHEN the user invokes `maestro ps -q`
THEN the output MUST be exactly three lines, each containing one container ID
AND no header row or extra formatting MUST be present

#### Scenario: Quiet mode for image list

GIVEN two images exist with IDs `img111` and `img222`
WHEN the user invokes `maestro images -q`
THEN the output MUST be exactly two lines, each containing one image ID

#### Scenario: Quiet mode with no results

GIVEN no containers exist
WHEN the user invokes `maestro ps -q`
THEN the output MUST be empty (no output at all)
AND the exit code MUST be 0

#### Scenario: Quiet mode overrides format flag

GIVEN containers exist
WHEN the user invokes `maestro ps -q --format json`
THEN the output MUST be IDs only, one per line (quiet mode takes precedence)

---

### Requirement: Shell Completions

Dinh MUST support generating shell completion scripts for bash, zsh, and fish. Completions MUST cover all commands, subcommands, flags, and flag values where applicable.

#### Scenario: Generate bash completions

GIVEN the user invokes `maestro generate completion bash`
WHEN the command executes
THEN valid bash completion script MUST be written to standard output
AND the exit code MUST be 0

#### Scenario: Generate zsh completions

GIVEN the user invokes `maestro generate completion zsh`
WHEN the command executes
THEN valid zsh completion script MUST be written to standard output

#### Scenario: Generate fish completions

GIVEN the user invokes `maestro generate completion fish`
WHEN the command executes
THEN valid fish completion script MUST be written to standard output

#### Scenario: Unsupported shell

GIVEN the user invokes `maestro generate completion powershell`
WHEN the command executes
THEN an error MUST be returned indicating that `powershell` is not a supported shell
AND supported shells MUST be listed in the error message

#### Scenario: Completions include subcommands

GIVEN bash completions have been installed
WHEN the user types `maestro con<TAB>`
THEN `container` MUST be offered as a completion
AND `config` MUST also be offered as a completion

#### Scenario: Completions include flag values

GIVEN bash completions have been installed
WHEN the user types `maestro --log-level <TAB>`
THEN `debug`, `info`, `warn`, `error` MUST be offered as completions

---

### Requirement: Error Output Format

All error messages MUST be written to standard error (stderr), never to standard output (stdout). Error messages MUST be human-readable by default. When `--format json` is active, error output SHOULD also be structured JSON on standard error.

#### Scenario: Error written to stderr

GIVEN the user invokes `maestro container inspect nonexistent`
WHEN the container is not found
THEN the error message MUST be written to standard error
AND standard output MUST be empty

#### Scenario: Error message includes context

GIVEN the user invokes `maestro container stop nonexistent`
WHEN the container is not found
THEN the error message MUST include the container identifier `nonexistent`
AND the error message MUST indicate the nature of the failure (e.g., "container not found")

#### Scenario: Error with JSON format active

GIVEN the user invokes `maestro container inspect nonexistent --format json`
WHEN the container is not found
THEN the error SHOULD be written to standard error as a JSON object with at minimum an `error` field
AND standard output MUST be empty

#### Scenario: Multiple errors reported together

GIVEN the user invokes `maestro container rm aaa bbb` where `aaa` exists and `bbb` does not
WHEN the command executes
THEN `aaa` MUST be removed successfully
AND an error for `bbb` MUST be written to standard error
AND the exit code MUST be non-zero (indicating partial failure)

---

### Requirement: Exit Codes

Dinh MUST use consistent exit codes to communicate the result of a command. Exit codes MUST follow these conventions:

- `0` -- Success
- `1` -- General error (command failed)
- `2` -- Usage error (invalid flags, missing arguments, unknown command)
- `125` -- Maestro internal error (engine failure, state corruption)
- `126` -- Container command cannot be invoked (permission denied inside container)
- `127` -- Container command not found (executable not found inside container)
- `128+N` -- Container exited with signal N (e.g., 137 for SIGKILL, 143 for SIGTERM)

#### Scenario: Successful command returns 0

GIVEN the user invokes `maestro version`
WHEN the command completes successfully
THEN the exit code MUST be 0

#### Scenario: General error returns 1

GIVEN the user invokes `maestro container stop nonexistent`
WHEN the container is not found
THEN the exit code MUST be 1

#### Scenario: Usage error returns 2

GIVEN the user invokes `maestro --invalid-flag`
WHEN the command is parsed
THEN the exit code MUST be 2

#### Scenario: Missing required argument returns 2

GIVEN the user invokes `maestro container stop` with no container ID
WHEN the command is parsed
THEN the exit code MUST be 2
AND the error message MUST indicate that a container ID is required

#### Scenario: Container command not found returns 127

GIVEN a container is running
WHEN the user invokes `maestro exec abc123 nonexistent-binary`
AND the binary does not exist in the container
THEN the exit code MUST be 127

#### Scenario: Container killed by SIGKILL returns 137

GIVEN a container is running
WHEN the container process is killed by SIGKILL (signal 9)
AND the user invokes `maestro wait abc123`
THEN the exit code MUST be 137 (128 + 9)

#### Scenario: Container killed by SIGTERM returns 143

GIVEN a container is running
WHEN the container process is terminated by SIGTERM (signal 15)
AND the user invokes `maestro wait abc123`
THEN the exit code MUST be 143 (128 + 15)

#### Scenario: Internal engine error returns 125

GIVEN the Waystation state store is corrupted and unreadable
WHEN the user invokes `maestro ps`
THEN the exit code MUST be 125
AND the error message MUST indicate an internal error

---

### Requirement: Color Output

Dinh MUST support colored terminal output. Color MUST be auto-detected based on the output target and MAY be controlled by the user.

- When standard output is connected to a TTY, colored output MUST be enabled by default
- When standard output is piped or redirected, colored output MUST be disabled by default
- The `--no-color` flag MUST disable colored output regardless of TTY detection
- The `NO_COLOR` environment variable (per the no-color.org convention) MUST disable colored output when set to any non-empty value
- The `TERM=dumb` environment variable MUST disable colored output

#### Scenario: Color enabled on TTY

GIVEN standard output is connected to an interactive terminal (TTY)
AND no color-disabling flags or environment variables are set
WHEN the user invokes `maestro ps`
THEN the output MAY contain ANSI escape codes for styling

#### Scenario: Color disabled when piped

GIVEN standard output is piped to another command (not a TTY)
WHEN the user invokes `maestro ps`
THEN the output MUST NOT contain ANSI escape codes

#### Scenario: No-color flag disables color on TTY

GIVEN standard output is connected to an interactive terminal (TTY)
WHEN the user invokes `maestro --no-color ps`
THEN the output MUST NOT contain ANSI escape codes

#### Scenario: NO_COLOR environment variable disables color

GIVEN the environment variable `NO_COLOR` is set to `1`
AND standard output is connected to a TTY
WHEN the user invokes `maestro ps`
THEN the output MUST NOT contain ANSI escape codes

#### Scenario: TERM=dumb disables color

GIVEN the environment variable `TERM` is set to `dumb`
AND standard output is connected to a TTY
WHEN the user invokes `maestro ps`
THEN the output MUST NOT contain ANSI escape codes

---

### Requirement: Configuration Loading Order

Dinh MUST load configuration from multiple sources with a defined precedence order. Higher-priority sources MUST override lower-priority ones. The loading order (highest to lowest priority) MUST be:

1. CLI flags (e.g., `--runtime crun`)
2. Environment variables (e.g., `MAESTRO_RUNTIME=crun`)
3. Configuration file (`katet.toml`)
4. Built-in defaults

#### Scenario: CLI flag overrides everything

GIVEN `katet.toml` contains `runtime.default = "runc"`
AND `MAESTRO_RUNTIME` is set to `youki`
WHEN the user invokes `maestro --runtime crun run nginx`
THEN the runtime MUST be `crun`

#### Scenario: Environment variable overrides config file

GIVEN `katet.toml` contains `runtime.default = "runc"`
AND `MAESTRO_RUNTIME` is set to `youki`
AND no `--runtime` flag is specified
WHEN a container is created
THEN the runtime MUST be `youki`

#### Scenario: Config file overrides defaults

GIVEN `katet.toml` contains `runtime.default = "runc"`
AND no environment variable or CLI flag overrides the runtime
WHEN a container is created
THEN the runtime MUST be `runc`

#### Scenario: Default when nothing is specified

GIVEN no `katet.toml` exists
AND no environment variables are set
AND no CLI flags are specified
WHEN a container is created
THEN the runtime MUST be `auto` (auto-detection via Pathfinder)

#### Scenario: Missing config file uses defaults silently

GIVEN the config file at the default path does not exist
WHEN the user invokes `maestro ps`
THEN the command MUST succeed using built-in defaults
AND no error MUST be displayed (a missing default config is not an error)

#### Scenario: Explicit config file that does not exist is an error

GIVEN the user invokes `maestro --config /nonexistent/katet.toml ps`
AND the file `/nonexistent/katet.toml` does not exist
WHEN the command executes
THEN an error MUST be returned indicating the config file was not found
AND the exit code MUST be non-zero

#### Scenario: Malformed config file is an error

GIVEN the config file at the default path contains invalid TOML syntax
WHEN the user invokes `maestro ps`
THEN an error MUST be returned indicating the config file has a syntax error
AND the error MUST include the file path and line number (if available)

---

### Requirement: Version Command

Dinh MUST provide a `maestro version` command that displays build-time metadata. The version information MUST include the version string, Git commit hash, build date, Go version, and OS/architecture.

#### Scenario: Version with default output

GIVEN the user invokes `maestro version`
WHEN the command executes
THEN the output MUST include the Maestro version string
AND the output MUST include the Git commit hash
AND the output MUST include the build date
AND the output MUST include the Go version
AND the output MUST include the operating system and architecture

#### Scenario: Version with JSON output

GIVEN the user invokes `maestro version --format json`
WHEN the command executes
THEN the output MUST be a valid JSON object
AND the JSON MUST contain fields for `version`, `commit`, `date`, `go_version`, `os`, and `arch`

---

### Requirement: Help Command

Dinh MUST provide a `maestro help` command and a `--help` flag on every command and subcommand. Help text MUST include a description, usage syntax, available subcommands (for groups), available flags, and examples.

#### Scenario: Help for any command

GIVEN the user invokes `maestro help container run`
WHEN the help is displayed
THEN the help text MUST include a description of the command
AND usage syntax showing required and optional arguments
AND a list of flags specific to the command
AND at least one usage example

#### Scenario: Help flag on any command

GIVEN the user invokes `maestro container run --help`
WHEN the help is displayed
THEN the output MUST be identical to `maestro help container run`

#### Scenario: Help includes examples

GIVEN the user invokes `maestro run --help`
WHEN the help is displayed
THEN the help text MUST include at least two usage examples

---

### Requirement: Container Subcommand Group (Gunslinger)

The `container` subcommand group MUST provide the following commands for container lifecycle management. Each command MUST support `--help` and the global output flags.

Commands:

- `create <image>` -- create a container without starting it
- `start <container>` -- start a stopped or created container
- `stop <container>` -- stop a running container (SIGTERM with timeout, then SIGKILL)
- `restart <container>` -- stop and start a container
- `kill <container>` -- send a signal to a container (default SIGKILL)
- `rm <container>...` -- remove one or more stopped containers
- `ps` -- list containers (running by default, `--all` for all states)
- `logs <container>` -- display container log output
- `exec <container> <command>...` -- execute a command inside a running container
- `attach <container>` -- attach to a container's standard streams
- `inspect <container>` -- display detailed container information
- `top <container>` -- display running processes inside a container
- `stats [container...]` -- display live resource usage statistics
- `port <container>` -- list port mappings
- `cp <src> <dst>` -- copy files between host and container
- `diff <container>` -- show filesystem changes
- `wait <container>...` -- block until container(s) stop
- `pause <container>` -- pause a container
- `unpause <container>` -- unpause a container
- `rename <old> <new>` -- rename a container
- `prune` -- remove all stopped containers

#### Scenario: Container create with image argument

GIVEN a valid image `nginx:latest` exists locally
WHEN the user invokes `maestro container create nginx:latest`
THEN a container MUST be created in the `created` state
AND the container ID MUST be printed to standard output

#### Scenario: Container ps with all flag

GIVEN containers in various states exist (running, stopped, created)
WHEN the user invokes `maestro container ps --all`
THEN all containers regardless of state MUST be listed

#### Scenario: Container ps without all flag

GIVEN containers in various states exist
WHEN the user invokes `maestro container ps`
THEN only running containers MUST be listed

#### Scenario: Container rm with multiple arguments

GIVEN stopped containers `aaa` and `bbb` exist
WHEN the user invokes `maestro container rm aaa bbb`
THEN both containers MUST be removed

#### Scenario: Container rm of running container fails

GIVEN a running container `abc123` exists
WHEN the user invokes `maestro container rm abc123`
THEN an error MUST be returned indicating the container is running
AND the error SHOULD suggest using `--force` to force removal

#### Scenario: Container logs with follow flag

GIVEN a running container `abc123` exists and is producing output
WHEN the user invokes `maestro container logs --follow abc123`
THEN existing log output MUST be displayed
AND the command MUST continue streaming new log output until interrupted

#### Scenario: Container logs with tail flag

GIVEN a container `abc123` has 100 lines of log output
WHEN the user invokes `maestro container logs --tail 10 abc123`
THEN only the last 10 lines of log output MUST be displayed

#### Scenario: Container exec with interactive TTY

GIVEN a running container `abc123` exists
WHEN the user invokes `maestro container exec -it abc123 /bin/sh`
THEN an interactive shell session MUST be opened inside the container
AND the terminal MUST be in raw mode for proper interactive behavior

#### Scenario: Container stop with custom timeout

GIVEN a running container `abc123` exists
WHEN the user invokes `maestro container stop --time 5 abc123`
THEN SIGTERM MUST be sent first
AND if the container does not stop within 5 seconds, SIGKILL MUST be sent

#### Scenario: Container kill with custom signal

GIVEN a running container `abc123` exists
WHEN the user invokes `maestro container kill --signal SIGUSR1 abc123`
THEN the signal SIGUSR1 MUST be sent to the container's main process

---

### Requirement: Image Subcommand Group (Archivist)

The `image` subcommand group MUST provide the following commands for image management.

Commands:

- `pull <image>` -- pull an image from a registry
- `push <image>` -- push an image to a registry
- `ls` -- list local images
- `rm <image>...` -- remove one or more images
- `inspect <image>` -- display detailed image information
- `history <image>` -- show image layer history
- `tag <source> <target>` -- create a new tag for an image
- `save <image> -o <file>` -- export an image to a tar archive
- `load -i <file>` -- import an image from a tar archive
- `sign <image>` -- sign an image with Sigstore/cosign
- `verify <image>` -- verify an image signature
- `prune` -- remove dangling images

#### Scenario: Image pull with default platform

GIVEN the user invokes `maestro image pull nginx:latest`
WHEN the pull completes
THEN the image matching the host OS and architecture MUST be stored locally

#### Scenario: Image pull with platform override

GIVEN the user invokes `maestro image pull --platform linux/arm64 nginx:latest`
WHEN the pull completes
THEN the `linux/arm64` variant MUST be stored regardless of the host architecture

#### Scenario: Image ls output format

GIVEN images exist locally
WHEN the user invokes `maestro image ls`
THEN a table with columns REPOSITORY, TAG, IMAGE ID, CREATED, SIZE MUST be displayed

#### Scenario: Image rm with dependent container

GIVEN image `nginx:latest` is used by a running container
WHEN the user invokes `maestro image rm nginx:latest`
THEN an error MUST be returned indicating the image is in use
AND the error SHOULD list the dependent container(s)

#### Scenario: Image tag creates new reference

GIVEN image `nginx:latest` exists locally
WHEN the user invokes `maestro image tag nginx:latest myregistry.io/mynginx:v1`
THEN the image MUST be accessible as both `nginx:latest` and `myregistry.io/mynginx:v1`

#### Scenario: Image prune removes dangling images

GIVEN dangling images (images with no tag) exist
WHEN the user invokes `maestro image prune`
THEN dangling images MUST be removed
AND tagged images MUST NOT be affected
AND the amount of disk space reclaimed MUST be reported

---

### Requirement: Volume Subcommand Group (Keeper)

The `volume` subcommand group MUST provide the following commands for volume management.

Commands:

- `create [name]` -- create a named volume (auto-generate name if omitted)
- `ls` -- list volumes
- `inspect <volume>` -- display detailed volume information
- `rm <volume>...` -- remove one or more volumes
- `prune` -- remove volumes not used by any container

#### Scenario: Volume create with auto-generated name

GIVEN the user invokes `maestro volume create` with no name argument
WHEN the volume is created
THEN a unique name MUST be auto-generated
AND the volume name MUST be printed to standard output

#### Scenario: Volume create with explicit name

GIVEN the user invokes `maestro volume create my-data`
WHEN the volume is created
THEN the volume MUST be named `my-data`

#### Scenario: Volume rm of in-use volume fails

GIVEN volume `my-data` is mounted by a running container
WHEN the user invokes `maestro volume rm my-data`
THEN an error MUST be returned indicating the volume is in use

#### Scenario: Volume prune with confirmation

GIVEN unused volumes exist
WHEN the user invokes `maestro volume prune`
THEN a confirmation prompt MUST be displayed before removal
AND if the user confirms, unused volumes MUST be removed
AND the amount of disk space reclaimed MUST be reported

#### Scenario: Volume prune with force flag

GIVEN unused volumes exist
WHEN the user invokes `maestro volume prune --force`
THEN unused volumes MUST be removed without a confirmation prompt

---

### Requirement: Network Subcommand Group (Beamseeker)

The `network` subcommand group MUST provide the following commands for network management.

Commands:

- `create <name>` -- create a network
- `ls` -- list networks
- `inspect <network>` -- display detailed network information
- `rm <network>...` -- remove one or more networks
- `connect <network> <container>` -- connect a container to a network
- `disconnect <network> <container>` -- disconnect a container from a network
- `prune` -- remove networks not used by any container

#### Scenario: Network create with custom subnet

GIVEN the user invokes `maestro network create --subnet 10.200.0.0/24 my-network`
WHEN the network is created
THEN it MUST use the subnet `10.200.0.0/24`

#### Scenario: Network rm of default network fails

GIVEN the default bridge network `beam0` exists
WHEN the user invokes `maestro network rm beam0`
THEN an error MUST be returned indicating that the default network cannot be removed

#### Scenario: Network connect at runtime

GIVEN a running container `abc123` exists on `beam0`
AND a network `my-network` exists
WHEN the user invokes `maestro network connect my-network abc123`
THEN the container MUST gain a new network interface on `my-network`

#### Scenario: Network disconnect at runtime

GIVEN a running container `abc123` is connected to `my-network`
WHEN the user invokes `maestro network disconnect my-network abc123`
THEN the container's interface on `my-network` MUST be removed

---

### Requirement: Artifact Subcommand Group (Collector)

The `artifact` subcommand group MUST provide the following commands for OCI artifact management.

Commands:

- `push <reference> <file>` -- push an artifact to a registry
- `pull <reference> -o <file>` -- pull an artifact from a registry
- `attach <image> <file>` -- attach an artifact to an existing image
- `ls <image>` -- list artifacts attached to an image (via Referrers API)
- `inspect <reference>` -- display artifact metadata

#### Scenario: Artifact push with artifact type

GIVEN the user invokes `maestro artifact push registry.io/repo:v1 ./chart.tgz --artifact-type application/vnd.cncf.helm.chart.content.v1.tar+gzip`
WHEN the push completes
THEN the artifact MUST be pushed to the registry with the specified media type

#### Scenario: Artifact ls shows attached artifacts

GIVEN an image `registry.io/app:v1` has an SBOM and a signature attached
WHEN the user invokes `maestro artifact ls registry.io/app:v1`
THEN both the SBOM and signature MUST be listed with their artifact types and digests

#### Scenario: Artifact pull downloads to file

GIVEN an artifact exists at `registry.io/repo:v1`
WHEN the user invokes `maestro artifact pull registry.io/repo:v1 -o ./downloaded.tgz`
THEN the artifact content MUST be saved to `./downloaded.tgz`

---

### Requirement: System Subcommand Group (An-tet)

The `system` subcommand group MUST provide the following commands for system-wide operations.

Commands:

- `info` -- display system information
- `df` -- display disk usage for images, containers, and volumes
- `prune` -- remove all unused resources (stopped containers, dangling images, unused volumes, unused networks)
- `events` -- stream real-time system events
- `check` -- run system diagnostics

#### Scenario: System info output

GIVEN the user invokes `maestro system info`
WHEN the command executes
THEN the output MUST include: operating system, kernel version, detected runtime, storage driver, network mode, rootless status, Waystation root path, number of containers, number of images

#### Scenario: System df output

GIVEN images, containers, and volumes consume disk space
WHEN the user invokes `maestro system df`
THEN the output MUST show a table with TYPE, TOTAL, ACTIVE, SIZE, RECLAIMABLE columns
AND types MUST include Images, Containers, Volumes, Build Cache (if applicable)

#### Scenario: System prune with confirmation

GIVEN unused resources exist
WHEN the user invokes `maestro system prune`
THEN a warning MUST be displayed listing the types of resources that will be removed
AND a confirmation prompt MUST be displayed
AND if confirmed, all unused resources MUST be removed
AND a summary of reclaimed space MUST be displayed

#### Scenario: System prune with all flag

GIVEN the user invokes `maestro system prune --all`
WHEN confirmed and executed
THEN all stopped containers, all unused images (not just dangling), all unused volumes, and all unused networks MUST be removed

#### Scenario: System check reports environment health

GIVEN the user invokes `maestro system check`
WHEN the command executes
THEN a checklist MUST be displayed covering: subuid/subgid configuration, kernel version, available OCI runtimes, storage driver compatibility, networking tools (pasta/slirp4netns), fuse device availability
AND each item MUST show a pass/fail indicator
AND failing items MUST include a suggestion for remediation

---

### Requirement: Service Subcommand Group (Positronics)

The `service` subcommand group MUST provide the following commands for managing the optional Positronics API server.

Commands:

- `start` -- start the Positronics socket mode API server
- `stop` -- stop the Positronics server
- `status` -- display the current status of the Positronics server

#### Scenario: Service start

GIVEN the Positronics server is not running
WHEN the user invokes `maestro service start`
THEN the Positronics server MUST start as a background process
AND the socket path MUST be reported to the user

#### Scenario: Service stop

GIVEN the Positronics server is running
WHEN the user invokes `maestro service stop`
THEN the Positronics server MUST be stopped gracefully

#### Scenario: Service status when running

GIVEN the Positronics server is running with PID 12345
WHEN the user invokes `maestro service status`
THEN the output MUST indicate the server is running
AND the output MUST include the PID and socket path

#### Scenario: Service status when not running

GIVEN the Positronics server is not running
WHEN the user invokes `maestro service status`
THEN the output MUST indicate the server is not running

---

### Requirement: Generate Subcommand Group

The `generate` subcommand group MUST provide commands for generating derived artifacts.

Commands:

- `systemd <container>` -- generate systemd unit files for a container
- `completion <shell>` -- generate shell completion scripts

#### Scenario: Generate systemd unit file

GIVEN a container `web` exists
WHEN the user invokes `maestro generate systemd web`
THEN a valid systemd unit file MUST be written to standard output
AND the unit file MUST contain `[Unit]`, `[Service]`, and `[Install]` sections

#### Scenario: Generate systemd with new flag

GIVEN the user invokes `maestro generate systemd --new web`
WHEN the unit is generated
THEN the `ExecStart` MUST use `maestro run` to create the container from scratch on each start

---

### Requirement: Config Subcommand Group

The `config` subcommand group MUST provide commands for managing the Ka-tet configuration file.

Commands:

- `show` -- display the effective configuration
- `edit` -- open the configuration file in the user's editor

#### Scenario: Config show displays effective config

GIVEN a config file exists with custom settings
WHEN the user invokes `maestro config show`
THEN the effective configuration MUST be displayed in TOML format
AND the output MUST reflect the merged result of defaults and config file values

#### Scenario: Config edit opens editor

GIVEN the environment variable `EDITOR` is set to `vim`
WHEN the user invokes `maestro config edit`
THEN `vim` MUST be invoked with the path to `katet.toml`

#### Scenario: Config edit with no editor set

GIVEN neither `EDITOR` nor `VISUAL` environment variables are set
WHEN the user invokes `maestro config edit`
THEN an error MUST be returned indicating that no editor is configured
AND the error SHOULD suggest setting the `EDITOR` environment variable

---

### Requirement: Login and Logout Commands

Dinh MUST provide `maestro login` and `maestro logout` commands for registry authentication management.

#### Scenario: Interactive login

GIVEN the user invokes `maestro login docker.io`
WHEN the command executes
THEN the user MUST be prompted for a username
AND the user MUST be prompted for a password (input MUST NOT be echoed)
AND upon successful authentication, credentials MUST be stored in the auth file with permissions `0600`

#### Scenario: Login with flags

GIVEN the user invokes `maestro login --username user --password-stdin docker.io`
AND the password is provided via standard input
WHEN the command executes
THEN authentication MUST be attempted with the provided credentials
AND no interactive prompts MUST be displayed

#### Scenario: Logout removes credentials

GIVEN the user has previously logged in to `docker.io`
WHEN the user invokes `maestro logout docker.io`
THEN the stored credentials for `docker.io` MUST be removed from the auth file

#### Scenario: Logout from server with no stored credentials

GIVEN no credentials are stored for `ghcr.io`
WHEN the user invokes `maestro logout ghcr.io`
THEN a message MUST be displayed indicating that no credentials were found for `ghcr.io`
AND the exit code MUST be 0

---

### Requirement: Run Command Integration

The `maestro run` shortcut MUST combine container creation, starting, and optional attachment into a single command. It MUST support both foreground (attached) and background (detached) modes.

#### Scenario: Run in detached mode

GIVEN the user invokes `maestro run -d --name web nginx:latest`
WHEN the command executes
THEN a container MUST be created and started in the background
AND only the container ID MUST be printed to standard output
AND the CLI process MUST exit immediately

#### Scenario: Run in foreground mode

GIVEN the user invokes `maestro run --rm nginx:latest echo hello`
WHEN the command executes
THEN the container MUST be created, started, and the CLI MUST attach to its output
AND `hello` MUST be printed to standard output
AND the container MUST be automatically removed after it exits (due to `--rm`)

#### Scenario: Run with port mapping

GIVEN the user invokes `maestro run -d -p 8080:80 nginx:latest`
WHEN the command executes
THEN port 8080 on the host MUST be mapped to port 80 in the container

#### Scenario: Run with volume mount

GIVEN the user invokes `maestro run -v my-data:/app/data nginx:latest`
WHEN the command executes
THEN the volume `my-data` MUST be mounted at `/app/data` inside the container

#### Scenario: Run with environment variables

GIVEN the user invokes `maestro run -e FOO=bar -e BAZ=qux nginx:latest`
WHEN the command executes
THEN the environment variables `FOO=bar` and `BAZ=qux` MUST be set inside the container

#### Scenario: Run with resource limits

GIVEN the user invokes `maestro run --memory 512m --cpus 2 nginx:latest`
WHEN the command executes
THEN the container MUST be created with a 512MB memory limit and 2 CPU quota

#### Scenario: Run with network selection

GIVEN a network `my-network` exists
WHEN the user invokes `maestro run --network my-network nginx:latest`
THEN the container MUST be connected to `my-network` instead of the default `beam0`

#### Scenario: Run with no network

GIVEN the user invokes `maestro run --network none nginx:latest`
WHEN the command executes
THEN the container MUST have no network connectivity

---

### Requirement: Consistent Argument Handling

Dinh MUST handle command arguments consistently across all commands. Container and image references MUST be resolved by ID prefix match, full ID, or name.

#### Scenario: Container resolved by ID prefix

GIVEN a container with ID `abc123def456` exists and no other container ID starts with `abc`
WHEN the user invokes `maestro container stop abc`
THEN the container `abc123def456` MUST be stopped

#### Scenario: Ambiguous ID prefix

GIVEN containers `abc111` and `abc222` both exist
WHEN the user invokes `maestro container stop abc`
THEN an error MUST be returned indicating that the reference `abc` is ambiguous
AND the error MUST list the matching container IDs

#### Scenario: Container resolved by name

GIVEN a container named `web-server` exists
WHEN the user invokes `maestro container stop web-server`
THEN the container MUST be stopped

#### Scenario: Image resolved by tag

GIVEN an image `nginx:latest` exists locally
WHEN the user invokes `maestro image inspect nginx:latest`
THEN the image metadata MUST be displayed

#### Scenario: Image resolved by digest prefix

GIVEN an image with digest `sha256:abc123def456` exists locally
WHEN the user invokes `maestro image inspect sha256:abc123`
THEN the image metadata MUST be displayed
