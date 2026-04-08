# Gan Lifecycle Specification

## Purpose

Gan manages the container lifecycle: creation, start (Roland), stop, kill, exec (Touch), logs (Glass), and state machine (Ka). It orchestrates Eld (runtime), Prim (storage), Beam (network), and White (security) to bring containers into existence.

---

## 1. Container Creation

### Requirement: Unique Container Identity

The system MUST generate a unique identifier for each container at creation time. The identifier MUST be a hex-encoded string of at least 64 characters derived from a cryptographically secure random source. The system MUST accept a truncated prefix of the identifier (minimum 12 characters) as a valid reference in all commands, provided the prefix is unambiguous.

#### Scenario: Container receives unique identifier on creation

GIVEN a valid image reference exists locally
WHEN a container is created from that image
THEN the system MUST assign a unique 64-character hex identifier to the container
AND the identifier MUST be persisted in the Waystation state store

#### Scenario: Short identifier prefix resolves to full identifier

GIVEN a container exists with identifier "abc123def456789..."
WHEN the user references the container by the prefix "abc123def456"
THEN the system MUST resolve the prefix to the full identifier

#### Scenario: Ambiguous short identifier is rejected

GIVEN two containers exist whose identifiers share the prefix "abc123"
WHEN the user references a container by the prefix "abc123"
THEN the system MUST return an error indicating the prefix is ambiguous
AND the error message MUST list the matching container identifiers

### Requirement: Bundle Directory Preparation

The system MUST prepare an OCI-compliant bundle directory for each container at creation time. The bundle directory MUST contain a `rootfs` directory with the container's root filesystem and a `config.json` file conforming to the OCI Runtime Specification.

#### Scenario: Bundle directory created with rootfs and config.json

GIVEN a valid image exists in the local Maturin store
WHEN a container is created from that image
THEN the system MUST create a bundle directory under the Waystation containers path
AND the bundle MUST contain a `rootfs` directory with the image filesystem extracted via Prim
AND the bundle MUST contain a `config.json` file that is valid per the OCI Runtime Spec

#### Scenario: Bundle preparation fails when image is missing

GIVEN the specified image does not exist in the local Maturin store
WHEN a container creation is attempted with that image reference
THEN the system MUST return an error indicating the image was not found

### Requirement: Rootfs Extraction via Prim

The system MUST extract the container's root filesystem using the Prim snapshotter. The extraction MUST apply all image layers in order and provide a writable upper layer for the container.

#### Scenario: Layers applied in correct order

GIVEN an image with three layers (base, middle, top)
WHEN a container is created from that image
THEN Prim MUST mount the layers as a unified filesystem with the top layer taking precedence
AND the container MUST have a writable layer on top of the image layers

#### Scenario: Rootfs is writable by default

GIVEN a container is created without the `--read-only` flag
WHEN a process writes a file to the root filesystem
THEN the write MUST succeed and the file MUST appear in the container's writable layer

#### Scenario: Rootfs is read-only when requested

GIVEN a container is created with the `--read-only` flag
WHEN a process attempts to write a file to the root filesystem
THEN the write MUST fail with a read-only filesystem error

### Requirement: OCI Runtime Config Generation

The system MUST generate a valid OCI runtime `config.json` by merging the image configuration, user-specified flags, security defaults, and network configuration. The generated configuration MUST be accepted by any compliant OCI runtime.

#### Scenario: Image CMD and ENTRYPOINT applied to config.json

GIVEN an image with ENTRYPOINT ["/entrypoint.sh"] and CMD ["--flag"]
WHEN a container is created from that image without overriding the command
THEN the `config.json` process args MUST be ["/entrypoint.sh", "--flag"]

#### Scenario: User command overrides image CMD

GIVEN an image with ENTRYPOINT ["/entrypoint.sh"] and CMD ["default"]
WHEN a container is created with the command "echo hello"
THEN the `config.json` process args MUST be ["/entrypoint.sh", "echo", "hello"]

#### Scenario: User entrypoint override replaces both ENTRYPOINT and CMD

GIVEN an image with ENTRYPOINT ["/entrypoint.sh"] and CMD ["default"]
WHEN a container is created with `--entrypoint /bin/sh` and command "-c ls"
THEN the `config.json` process args MUST be ["/bin/sh", "-c", "ls"]

#### Scenario: Image environment variables merged into config.json

GIVEN an image with ENV PATH=/usr/bin and ENV APP_ENV=production
WHEN a container is created from that image
THEN the `config.json` process env MUST include both PATH=/usr/bin and APP_ENV=production

#### Scenario: Image working directory applied to config.json

GIVEN an image with WORKDIR /app
WHEN a container is created from that image without overriding the working directory
THEN the `config.json` process cwd MUST be "/app"

#### Scenario: Image exposed ports recorded but not published

GIVEN an image with EXPOSE 80/tcp and EXPOSE 443/tcp
WHEN a container is created from that image without port flags
THEN the container metadata MUST record the exposed ports
AND no host port mappings MUST be created

---

## 2. Ka (State Machine)

### Requirement: Container State Transitions

The system MUST enforce a strict state machine (Ka) governing the lifecycle of every container. The valid states are: Created, Running, Paused, Stopped, and Deleted. Only the transitions defined below are permitted. Any attempt to perform an invalid transition MUST be rejected with an error.

Valid transitions:

- Created -> Running (start)
- Created -> Deleted (remove)
- Running -> Paused (pause)
- Running -> Stopped (stop, kill, or process exit)
- Paused -> Running (unpause)
- Stopped -> Running (start/restart)
- Stopped -> Deleted (remove)

#### Scenario: Container transitions from Created to Running

GIVEN a container in the Created state
WHEN the container is started
THEN the container state MUST transition to Running
AND the state file in the Waystation MUST reflect the Running status

#### Scenario: Container transitions from Running to Stopped on stop

GIVEN a container in the Running state
WHEN the container is stopped
THEN the container state MUST transition to Stopped
AND the exit code MUST be recorded in the Waystation

#### Scenario: Container transitions from Running to Stopped on process exit

GIVEN a container in the Running state
WHEN the container's main process exits on its own
THEN the container state MUST transition to Stopped
AND the exit code of the main process MUST be recorded

#### Scenario: Container transitions from Running to Paused

GIVEN a container in the Running state
WHEN the container is paused
THEN the container state MUST transition to Paused
AND the container's processes MUST be frozen via the cgroups freezer

#### Scenario: Container transitions from Paused to Running

GIVEN a container in the Paused state
WHEN the container is unpaused
THEN the container state MUST transition to Running
AND the container's processes MUST resume execution

#### Scenario: Container transitions from Stopped to Running on restart

GIVEN a container in the Stopped state
WHEN the container is started again
THEN the container state MUST transition to Running

#### Scenario: Container transitions from Created to Deleted

GIVEN a container in the Created state (never started)
WHEN the container is removed
THEN the container MUST be deleted and all associated resources released

#### Scenario: Container transitions from Stopped to Deleted

GIVEN a container in the Stopped state
WHEN the container is removed
THEN the container MUST be deleted and all associated resources released

### Requirement: Invalid State Transition Rejection

The system MUST reject any operation that would cause an invalid state transition and MUST return a descriptive error message indicating the current state and the attempted operation.

#### Scenario: Stop rejected for container in Created state

GIVEN a container in the Created state
WHEN the user attempts to stop the container
THEN the system MUST return an error indicating the container is not running

#### Scenario: Pause rejected for container in Stopped state

GIVEN a container in the Stopped state
WHEN the user attempts to pause the container
THEN the system MUST return an error indicating the container is not running

#### Scenario: Start rejected for container in Paused state

GIVEN a container in the Paused state
WHEN the user attempts to start the container
THEN the system MUST return an error indicating the container must be unpaused first

#### Scenario: Remove rejected for container in Running state

GIVEN a container in the Running state
WHEN the user attempts to remove the container without the force flag
THEN the system MUST return an error indicating the container must be stopped first

#### Scenario: Force remove succeeds for container in Running state

GIVEN a container in the Running state
WHEN the user attempts to remove the container with the `--force` flag
THEN the system MUST stop the container (kill) and then remove it

#### Scenario: Unpause rejected for container not in Paused state

GIVEN a container in the Running state
WHEN the user attempts to unpause the container
THEN the system MUST return an error indicating the container is not paused

### Requirement: Atomic State Persistence

All state transitions MUST be persisted atomically to the Waystation. If the system crashes during a state transition, the state MUST remain in the last successfully committed state.

#### Scenario: State survives process crash during transition

GIVEN a container in the Running state
WHEN a stop operation begins and the system crashes before completion
THEN the container state MUST be either Running or Stopped when the system recovers, never in an intermediate state

#### Scenario: Concurrent state reads during transition

GIVEN a container undergoing a state transition
WHEN another process reads the container's state concurrently
THEN the reader MUST see either the old state or the new state, never a partial or corrupted state

---

## 3. Roland (Start/Stop/Kill)

### Requirement: Container Start

Starting a container MUST fork a Cort (conmon-rs) monitor process, which in turn MUST invoke the Eld (OCI runtime) to create and start the container. The Cort process MUST survive the exit of the CLI process so that the container continues running independently.

#### Scenario: Container start forks Cort and invokes Eld

GIVEN a container in the Created state
WHEN the container is started
THEN the system MUST fork a Cort process
AND Cort MUST invoke Eld to create the container from the bundle
AND Cort MUST invoke Eld to start the container
AND the container's main process MUST be running

#### Scenario: Container survives CLI exit

GIVEN a container started in detached mode
WHEN the CLI process exits
THEN the Cort process MUST continue running
AND the container's main process MUST continue running

#### Scenario: Start of already running container is rejected

GIVEN a container in the Running state
WHEN the user attempts to start the container again
THEN the system MUST return an error indicating the container is already running

### Requirement: Container Stop

Stopping a container MUST send SIGTERM to the container's main process, wait for a configurable timeout period (default 10 seconds), and then send SIGKILL if the process has not exited.

#### Scenario: Graceful stop with SIGTERM

GIVEN a container in the Running state whose process handles SIGTERM
WHEN the container is stopped with the default timeout
THEN the system MUST send SIGTERM to the container's main process
AND the system MUST wait up to 10 seconds for the process to exit
AND if the process exits within the timeout, the exit code MUST be recorded

#### Scenario: Forceful stop with SIGKILL after timeout

GIVEN a container in the Running state whose process ignores SIGTERM
WHEN the container is stopped with the default timeout
THEN the system MUST send SIGTERM first
AND after 10 seconds the system MUST send SIGKILL
AND the container MUST transition to the Stopped state

#### Scenario: Custom stop timeout

GIVEN a container in the Running state
WHEN the container is stopped with `--time 5`
THEN the system MUST wait 5 seconds between SIGTERM and SIGKILL

#### Scenario: Stop timeout of zero sends immediate SIGKILL

GIVEN a container in the Running state
WHEN the container is stopped with `--time 0`
THEN the system MUST send SIGKILL immediately without sending SIGTERM

#### Scenario: Custom stop signal from image configuration

GIVEN an image with a StopSignal of SIGQUIT
AND a container created from that image
WHEN the container is stopped
THEN the system MUST send SIGQUIT instead of SIGTERM as the initial signal

### Requirement: Container Kill

Killing a container MUST send the specified signal to the container's main process immediately. The default signal MUST be SIGKILL.

#### Scenario: Kill with default signal

GIVEN a container in the Running state
WHEN the container is killed without specifying a signal
THEN the system MUST send SIGKILL to the container's main process

#### Scenario: Kill with specific signal

GIVEN a container in the Running state
WHEN the container is killed with `--signal SIGINT`
THEN the system MUST send SIGINT to the container's main process

#### Scenario: Kill with numeric signal

GIVEN a container in the Running state
WHEN the container is killed with `--signal 15`
THEN the system MUST send signal 15 (SIGTERM) to the container's main process

#### Scenario: Kill with invalid signal is rejected

GIVEN a container in the Running state
WHEN the container is killed with `--signal INVALID`
THEN the system MUST return an error indicating the signal is not valid

#### Scenario: Kill of a stopped container is rejected

GIVEN a container in the Stopped state
WHEN the user attempts to kill the container
THEN the system MUST return an error indicating the container is not running

---

## 4. Container Naming

### Requirement: Explicit Container Name

The system MUST accept a `--name` flag during container creation to assign a human-readable name. Container names MUST be unique across all containers (including stopped containers).

#### Scenario: Container created with explicit name

GIVEN no container named "web-server" exists
WHEN a container is created with `--name web-server`
THEN the container MUST be accessible by the name "web-server"

#### Scenario: Duplicate name is rejected

GIVEN a container named "web-server" already exists
WHEN the user attempts to create another container with `--name web-server`
THEN the system MUST return an error indicating the name is already in use

#### Scenario: Name freed after container removal

GIVEN a container named "web-server" exists
WHEN the container is removed
THEN a new container MAY be created with the name "web-server"

### Requirement: Auto-Generated Container Names

When no `--name` flag is provided, the system MUST automatically generate a random, human-readable name in adjective_noun style (e.g., "happy_turing", "brave_hopper"). The generated name MUST be unique.

#### Scenario: Container receives auto-generated name

GIVEN no `--name` flag is provided
WHEN a container is created
THEN the system MUST assign a random adjective_noun style name
AND the name MUST be unique among all existing containers

#### Scenario: Auto-generated name collision is resolved

GIVEN an auto-generated name collides with an existing container name
WHEN the system detects the collision during creation
THEN the system MUST regenerate a different name until a unique one is found

### Requirement: Container Name as Reference

Containers MUST be referenceable by name in all commands that accept a container identifier. Both the name and the ID (or ID prefix) MUST be accepted.

#### Scenario: Container referenced by name in stop command

GIVEN a running container named "web-server"
WHEN the user executes stop with the argument "web-server"
THEN the system MUST resolve "web-server" to the correct container and stop it

#### Scenario: Container referenced by name in logs command

GIVEN a container named "web-server" with recorded logs
WHEN the user executes logs with the argument "web-server"
THEN the system MUST display the logs for the correct container

---

## 5. Container Removal

### Requirement: Container Removal Cleanup

Removing a container MUST clean up all associated resources: the rootfs snapshot via Prim, the state files in the Waystation, and any network resources allocated by Beam. The container MUST be in the Stopped or Created state before removal, unless the `--force` flag is used.

#### Scenario: Remove stopped container cleans up all resources

GIVEN a stopped container with rootfs, state files, and network resources
WHEN the container is removed
THEN the Prim snapshot MUST be released
AND the state files in the Waystation MUST be deleted
AND network resources allocated by Beam MUST be released
AND the container MUST no longer appear in the container list

#### Scenario: Remove created (never started) container

GIVEN a container in the Created state
WHEN the container is removed
THEN the bundle directory MUST be deleted
AND the state files MUST be deleted
AND the container MUST no longer appear in the container list

#### Scenario: Remove running container without force is rejected

GIVEN a container in the Running state
WHEN the user attempts to remove it without `--force`
THEN the system MUST return an error indicating the container must be stopped first

#### Scenario: Force remove running container

GIVEN a container in the Running state
WHEN the user removes it with `--force`
THEN the system MUST kill the container first
AND then perform all cleanup steps as for a stopped container removal

#### Scenario: Remove container with mounted volume does not remove the volume

GIVEN a stopped container that was using a named volume
WHEN the container is removed
THEN the named volume MUST NOT be removed
AND the volume MUST remain available for use by other containers

### Requirement: Bulk Container Removal

The system MUST support removing multiple containers in a single command and MUST continue processing remaining containers if one removal fails.

#### Scenario: Remove multiple containers

GIVEN three stopped containers with IDs "aaa", "bbb", "ccc"
WHEN the user issues a remove command with all three IDs
THEN all three containers MUST be removed

#### Scenario: Partial failure during bulk removal

GIVEN containers "aaa" (stopped) and "bbb" (running) and "ccc" (stopped)
WHEN the user issues a remove command with all three IDs without `--force`
THEN "aaa" and "ccc" MUST be removed
AND the system MUST report an error for "bbb"

### Requirement: Container Prune

The system MUST support a prune operation that removes all containers in the Stopped state. The prune operation MUST support filtering.

#### Scenario: Prune removes all stopped containers

GIVEN two stopped containers and one running container
WHEN the user executes container prune
THEN both stopped containers MUST be removed
AND the running container MUST NOT be affected

#### Scenario: Prune with label filter

GIVEN a stopped container with label "env=test" and a stopped container with label "env=prod"
WHEN the user prunes with `--filter label=env=test`
THEN only the container with label "env=test" MUST be removed

---

## 6. Touch (Exec)

### Requirement: Execute Command in Running Container

The system MUST allow executing a command inside a running container's namespaces (pid, net, mount, ipc, uts). The command MUST execute in the context of the container's root filesystem and network namespace.

#### Scenario: Execute simple command in running container

GIVEN a running container
WHEN the user executes `exec <container> ls /`
THEN the system MUST execute `ls /` inside the container's namespaces
AND the output MUST reflect the container's root filesystem

#### Scenario: Exec rejected for non-running container

GIVEN a container in the Stopped state
WHEN the user attempts to exec a command
THEN the system MUST return an error indicating the container is not running

#### Scenario: Exec rejected for paused container

GIVEN a container in the Paused state
WHEN the user attempts to exec a command
THEN the system MUST return an error indicating the container is paused

### Requirement: TTY Allocation for Exec

The system MUST support allocating a pseudo-terminal for interactive exec sessions via the `-it` flags. When a TTY is allocated, the system MUST handle terminal resizing and raw mode.

#### Scenario: Interactive exec with TTY

GIVEN a running container with /bin/sh available
WHEN the user executes `exec -it <container> /bin/sh`
THEN the system MUST allocate a pseudo-terminal
AND the user MUST receive an interactive shell prompt
AND terminal input and output MUST be forwarded bidirectionally

#### Scenario: Terminal resize is propagated

GIVEN an active interactive exec session with TTY
WHEN the user's terminal is resized
THEN the system MUST propagate the new terminal dimensions to the container's pseudo-terminal

### Requirement: Exec Environment Injection

The system MUST support injecting additional environment variables into the exec session via `--env KEY=VALUE` and `--env-file` flags. These variables MUST be available in addition to the container's existing environment.

#### Scenario: Env variable injected into exec

GIVEN a running container
WHEN the user executes `exec --env MY_VAR=hello <container> env`
THEN the output MUST include "MY_VAR=hello"

#### Scenario: Env file injected into exec

GIVEN a running container
AND a file containing "VAR1=one" and "VAR2=two" (one per line)
WHEN the user executes `exec --env-file <path> <container> env`
THEN the output MUST include "VAR1=one" and "VAR2=two"

#### Scenario: Exec env overrides container env for same key

GIVEN a running container with environment variable APP_ENV=production
WHEN the user executes `exec --env APP_ENV=debug <container> env`
THEN the output MUST include "APP_ENV=debug" (not "APP_ENV=production")

### Requirement: Exec User Override

The system MUST support executing a command as a different user inside the container via the `--user` flag.

#### Scenario: Exec as specific user

GIVEN a running container with user "nobody" existing
WHEN the user executes `exec --user nobody <container> whoami`
THEN the output MUST be "nobody"

#### Scenario: Exec as specific UID:GID

GIVEN a running container
WHEN the user executes `exec --user 1000:1000 <container> id`
THEN the output MUST show uid=1000 and gid=1000

### Requirement: Exec Working Directory Override

The system MUST support overriding the working directory for an exec session via the `--workdir` flag.

#### Scenario: Exec with custom working directory

GIVEN a running container with directory /tmp existing
WHEN the user executes `exec --workdir /tmp <container> pwd`
THEN the output MUST be "/tmp"

#### Scenario: Exec with non-existent working directory is rejected

GIVEN a running container
WHEN the user executes `exec --workdir /nonexistent <container> pwd`
THEN the system MUST return an error indicating the directory does not exist

---

## 7. Glass (Logs)

### Requirement: JSON-File Log Driver

The system MUST record container stdout and stderr output using a json-file log driver by default. Each log entry MUST include the output stream identifier (stdout or stderr), a timestamp in RFC 3339 format, and the log message.

#### Scenario: Stdout captured in log file

GIVEN a running container that writes "hello world" to stdout
WHEN the log file is examined
THEN it MUST contain an entry with stream "stdout", a valid RFC 3339 timestamp, and log message "hello world"

#### Scenario: Stderr captured separately from stdout

GIVEN a running container that writes "error occurred" to stderr
WHEN the log file is examined
THEN it MUST contain an entry with stream "stderr" and log message "error occurred"

#### Scenario: Log entries are ordered by timestamp

GIVEN a container that produces multiple log entries
WHEN the logs are retrieved
THEN the entries MUST be in chronological order

### Requirement: Log Display

The system MUST display container logs via the `logs` command. By default, the system MUST display all available logs from the container.

#### Scenario: Display all logs for a container

GIVEN a container that has produced 100 log entries
WHEN the user executes `logs <container>`
THEN all 100 entries MUST be displayed

#### Scenario: Display logs with timestamps

GIVEN a container with log entries
WHEN the user executes `logs --timestamps <container>`
THEN each log line MUST be prefixed with its RFC 3339 timestamp

### Requirement: Log Follow (Streaming)

The system MUST support streaming new log entries in real time via the `--follow` flag. The stream MUST continue until the user interrupts or the container stops.

#### Scenario: Follow streams new log entries

GIVEN a running container
WHEN the user executes `logs --follow <container>`
AND the container subsequently writes "new message" to stdout
THEN "new message" MUST appear in the output stream

#### Scenario: Follow terminates when container stops

GIVEN a running container being followed
WHEN the container stops
THEN the follow stream MUST terminate

### Requirement: Log Tail

The system MUST support displaying only the last N log entries via the `--tail N` flag.

#### Scenario: Tail displays last N entries

GIVEN a container with 100 log entries
WHEN the user executes `logs --tail 10 <container>`
THEN only the last 10 entries MUST be displayed

#### Scenario: Tail with value exceeding total entries

GIVEN a container with 5 log entries
WHEN the user executes `logs --tail 100 <container>`
THEN all 5 entries MUST be displayed

#### Scenario: Tail zero displays no entries

GIVEN a container with log entries
WHEN the user executes `logs --tail 0 <container>`
THEN no log entries MUST be displayed (useful with `--follow` to see only new entries)

### Requirement: Log Time Filtering

The system MUST support filtering logs by timestamp via `--since` and `--until` flags. The flags MUST accept RFC 3339 timestamps and relative durations (e.g., "5m", "1h", "2h30m").

#### Scenario: Filter logs since a relative duration

GIVEN a container with logs spanning the last 2 hours
WHEN the user executes `logs --since 30m <container>`
THEN only entries from the last 30 minutes MUST be displayed

#### Scenario: Filter logs since an absolute timestamp

GIVEN a container with logs including entries before and after "2026-03-27T10:00:00Z"
WHEN the user executes `logs --since 2026-03-27T10:00:00Z <container>`
THEN only entries at or after that timestamp MUST be displayed

#### Scenario: Filter logs until a timestamp

GIVEN a container with logs spanning the last 2 hours
WHEN the user executes `logs --until 1h <container>`
THEN only entries older than 1 hour MUST be displayed

#### Scenario: Combined since and until filtering

GIVEN a container with logs spanning the last 3 hours
WHEN the user executes `logs --since 2h --until 1h <container>`
THEN only entries between 2 hours ago and 1 hour ago MUST be displayed

### Requirement: Log Rotation

The system MUST support automatic log rotation based on configurable maximum file size and maximum number of retained files. When the active log file exceeds the configured maximum size, the system MUST rotate the file and remove the oldest files beyond the retention limit.

#### Scenario: Log file rotated when exceeding max size

GIVEN a log configuration with max_size of 10MB and max_files of 3
AND a container whose log file reaches 10MB
WHEN additional log output is written
THEN the system MUST rotate the current log file
AND create a new active log file

#### Scenario: Oldest log files removed beyond retention

GIVEN a log configuration with max_files of 3
AND a container with 3 rotated log files
WHEN a fourth rotation occurs
THEN the oldest log file MUST be removed
AND only 3 log files (including the active one) MUST remain

#### Scenario: Default log rotation settings from configuration

GIVEN the system configuration specifies log max_size of 10MB and max_files of 3
WHEN a container is created without overriding log settings
THEN the container MUST use max_size of 10MB and max_files of 3

---

## 8. Container Inspection

### Requirement: Full Container Inspection

The system MUST support displaying a complete JSON representation of a container's state, configuration, network settings, mounts, resource limits, and runtime information.

#### Scenario: Inspect returns complete container state

GIVEN a running container with name, image, network, volumes, and resource limits configured
WHEN the user executes `inspect <container>`
THEN the output MUST be valid JSON
AND it MUST include the container ID, name, creation timestamp, and current state (Ka)
AND it MUST include the image reference and digest
AND it MUST include the network configuration (IP address, gateway, port mappings)
AND it MUST include mount information (volumes, bind mounts)
AND it MUST include resource limits (memory, CPU, pids)
AND it MUST include the runtime (Eld) name and path
AND it MUST include the Cort (conmon-rs) PID
AND it MUST include the log file path

#### Scenario: Inspect a stopped container includes exit information

GIVEN a stopped container
WHEN the user inspects the container
THEN the output MUST include the exit code, the time the container finished, and whether it was OOM-killed

#### Scenario: Inspect non-existent container returns error

GIVEN no container with ID "nonexistent" exists
WHEN the user executes `inspect nonexistent`
THEN the system MUST return an error indicating the container was not found

---

## 9. Container Listing (ps)

### Requirement: List Running Containers

The system MUST list running containers by default when the `ps` command is invoked. The output MUST include: container ID (truncated), image, command, creation time, status, ports, and names.

#### Scenario: List running containers only by default

GIVEN two running containers and one stopped container
WHEN the user executes `ps`
THEN only the two running containers MUST be listed

#### Scenario: List output includes required columns

GIVEN a running container created from "nginx:latest" with name "web" and port 8080->80
WHEN the user executes `ps`
THEN the output MUST show the container ID (truncated to 12 characters), image "nginx:latest", the container command, the creation time as a relative time, the status "Running", the port mapping, and the name "web"

### Requirement: List All Containers

The system MUST support listing all containers regardless of state when the `--all` flag is used.

#### Scenario: List all containers with --all

GIVEN containers in Created, Running, Paused, and Stopped states
WHEN the user executes `ps --all`
THEN all containers MUST be listed with their current state

### Requirement: Container List Filtering

The system MUST support filtering the container list by various criteria using `--filter` flags.

#### Scenario: Filter by status

GIVEN running and stopped containers
WHEN the user executes `ps --all --filter status=stopped`
THEN only stopped containers MUST be listed

#### Scenario: Filter by name

GIVEN containers named "web", "api", and "db"
WHEN the user executes `ps --all --filter name=web`
THEN only the container named "web" MUST be listed

#### Scenario: Filter by label

GIVEN containers with labels "env=prod" and "env=test"
WHEN the user executes `ps --all --filter label=env=prod`
THEN only containers with label "env=prod" MUST be listed

#### Scenario: Filter by ancestor image

GIVEN containers created from "nginx:latest" and "redis:latest"
WHEN the user executes `ps --all --filter ancestor=nginx:latest`
THEN only containers created from "nginx:latest" MUST be listed

### Requirement: Container List Output Formatting

The system MUST support multiple output formats for the container list: table (default), JSON, and Go template.

#### Scenario: Table format output

GIVEN running containers
WHEN the user executes `ps`
THEN the output MUST be a formatted table with column headers

#### Scenario: JSON format output

GIVEN running containers
WHEN the user executes `ps --format json`
THEN the output MUST be a valid JSON array of container objects

#### Scenario: Go template format output

GIVEN a running container with ID "abc123" and name "web"
WHEN the user executes `ps --format '{{.ID}}\t{{.Names}}'`
THEN the output MUST be "abc123\tweb" (with template values substituted)

#### Scenario: Quiet mode outputs only IDs

GIVEN running containers
WHEN the user executes `ps --quiet`
THEN the output MUST contain only container IDs, one per line

---

## 10. Attach

### Requirement: Attach to Container Main Process

The system MUST support attaching to the stdin, stdout, and stderr of a container's main process. The attach MUST connect the user's terminal to the container's primary I/O streams.

#### Scenario: Attach to running container receives stdout

GIVEN a running container producing stdout output
WHEN the user attaches to the container
THEN the user MUST receive the container's stdout output

#### Scenario: Attach sends stdin to container

GIVEN a running container that reads from stdin
WHEN the user attaches and types input
THEN the input MUST be forwarded to the container's stdin

#### Scenario: Attach to non-running container is rejected

GIVEN a container in the Stopped state
WHEN the user attempts to attach
THEN the system MUST return an error indicating the container is not running

### Requirement: Detach Sequence

The system MUST support a detach key sequence (default: Ctrl+P Ctrl+Q) that disconnects the user's terminal from the container without stopping the container.

#### Scenario: Detach with default key sequence

GIVEN an active attach session
WHEN the user enters the Ctrl+P Ctrl+Q key sequence
THEN the attach session MUST end
AND the container MUST continue running

#### Scenario: Container continues after detach

GIVEN a user attached to a running container
WHEN the user detaches using the detach sequence
THEN the container's main process MUST continue running uninterrupted

---

## 11. Copy (cp)

### Requirement: Bidirectional File Copy

The system MUST support copying files and directories between the host filesystem and a container's filesystem in both directions. The container MAY be running or stopped.

#### Scenario: Copy file from host to container

GIVEN a file "/tmp/test.txt" on the host
AND a running container
WHEN the user executes `cp /tmp/test.txt <container>:/app/`
THEN the file MUST appear at `/app/test.txt` inside the container

#### Scenario: Copy file from container to host

GIVEN a running container with a file at `/etc/hostname`
WHEN the user executes `cp <container>:/etc/hostname /tmp/`
THEN the file MUST appear at `/tmp/hostname` on the host

#### Scenario: Copy directory from host to container

GIVEN a directory "/tmp/mydir/" with files on the host
WHEN the user executes `cp /tmp/mydir <container>:/app/`
THEN the entire directory tree MUST appear at `/app/mydir/` inside the container

#### Scenario: Copy from stopped container

GIVEN a stopped container with files in its filesystem
WHEN the user executes `cp <container>:/etc/hostname /tmp/`
THEN the file MUST be copied successfully from the stopped container's filesystem

#### Scenario: Copy to non-existent destination path in container

GIVEN a running container without directory `/nonexistent/`
WHEN the user executes `cp /tmp/test.txt <container>:/nonexistent/test.txt`
THEN the system MUST return an error indicating the destination path does not exist

---

## 12. Stats

### Requirement: Live Container Resource Statistics

The system MUST support displaying live resource usage statistics for running containers, including CPU usage, memory usage and limit, network I/O, and block I/O. The data MUST be sourced from cgroups.

#### Scenario: Stats displays live resource usage

GIVEN a running container
WHEN the user executes `stats`
THEN the output MUST display the container ID, name, CPU percentage, memory usage and limit, memory percentage, network I/O, block I/O, and PID count
AND the output MUST refresh continuously

#### Scenario: Stats for specific container

GIVEN a running container named "web"
WHEN the user executes `stats web`
THEN the output MUST display resource usage only for the "web" container

#### Scenario: Stats no-stream for single snapshot

GIVEN a running container
WHEN the user executes `stats --no-stream`
THEN the output MUST display a single snapshot of resource usage and exit

#### Scenario: Stats for stopped container shows no data

GIVEN a stopped container
WHEN the user executes `stats --no-stream <container>`
THEN the output MUST show zero or dashes for all resource metrics

---

## 13. Pause/Unpause

### Requirement: Container Pause via Cgroups Freezer

The system MUST support pausing a running container by freezing all its processes using the cgroups freezer. Paused containers MUST NOT consume CPU time.

#### Scenario: Pause freezes all container processes

GIVEN a running container consuming CPU
WHEN the container is paused
THEN the container's CPU usage MUST drop to zero
AND all processes in the container MUST be frozen

#### Scenario: Unpause resumes container processes

GIVEN a paused container
WHEN the container is unpaused
THEN all processes MUST resume execution
AND the container MUST transition to the Running state

#### Scenario: Pause of non-running container is rejected

GIVEN a container in the Stopped state
WHEN the user attempts to pause the container
THEN the system MUST return an error indicating the container is not running

---

## 14. Diff

### Requirement: Filesystem Change Detection

The system MUST support showing filesystem changes made inside a container compared to the original image layers. Changes MUST be categorized as Added (A), Changed (C), or Deleted (D).

#### Scenario: Detect added file

GIVEN a running container that has created a new file "/tmp/newfile.txt"
WHEN the user executes `diff <container>`
THEN the output MUST include an entry "A /tmp/newfile.txt"

#### Scenario: Detect changed file

GIVEN a running container that has modified "/etc/hostname"
WHEN the user executes `diff <container>`
THEN the output MUST include an entry "C /etc/hostname"

#### Scenario: Detect deleted file

GIVEN a running container that has deleted "/etc/motd"
WHEN the user executes `diff <container>`
THEN the output MUST include an entry "D /etc/motd"

#### Scenario: No changes detected

GIVEN a running container that has made no filesystem modifications
WHEN the user executes `diff <container>`
THEN the output MUST be empty

---

## 15. Wait

### Requirement: Block Until Container Stops

The system MUST support blocking until a container exits, then returning the container's exit code. The wait operation MUST work for both running and already-stopped containers.

#### Scenario: Wait blocks until container exits

GIVEN a running container that will exit with code 0 after 5 seconds
WHEN the user executes `wait <container>`
THEN the command MUST block for approximately 5 seconds
AND then return exit code 0

#### Scenario: Wait on already stopped container returns immediately

GIVEN a container that has already stopped with exit code 137
WHEN the user executes `wait <container>`
THEN the command MUST return immediately with exit code 137

#### Scenario: Wait on multiple containers

GIVEN two running containers that will exit at different times
WHEN the user executes `wait <container1> <container2>`
THEN the command MUST block until both containers have exited
AND the exit code for each container MUST be reported

#### Scenario: Wait on non-existent container returns error

GIVEN no container with ID "nonexistent" exists
WHEN the user executes `wait nonexistent`
THEN the system MUST return an error indicating the container was not found

---

## 16. Restart Policies

### Requirement: Container Restart Policy

The system MUST support restart policies that control whether a container is automatically restarted when it exits. The following policies MUST be supported: `no`, `on-failure`, `always`, and `unless-stopped`. The restart policy MUST be enforced by the Cort (conmon-rs) monitor process.

#### Scenario: Policy "no" does not restart

GIVEN a container created with `--restart no`
WHEN the container's main process exits
THEN the container MUST NOT be restarted
AND the container MUST transition to the Stopped state

#### Scenario: Policy "on-failure" restarts on non-zero exit

GIVEN a container created with `--restart on-failure`
WHEN the container's main process exits with code 1
THEN the container MUST be automatically restarted

#### Scenario: Policy "on-failure" does not restart on success

GIVEN a container created with `--restart on-failure`
WHEN the container's main process exits with code 0
THEN the container MUST NOT be restarted

#### Scenario: Policy "on-failure" with max retries

GIVEN a container created with `--restart on-failure:3`
WHEN the container's main process fails and restarts 3 times
AND the process fails a fourth time
THEN the container MUST NOT be restarted after the third failure
AND the container MUST transition to the Stopped state

#### Scenario: Policy "always" restarts regardless of exit code

GIVEN a container created with `--restart always`
WHEN the container's main process exits with code 0
THEN the container MUST be automatically restarted

#### Scenario: Policy "always" restarts after explicit stop and system restart

GIVEN a container created with `--restart always` that was explicitly stopped by the user
WHEN the system (Cort/Positronics) restarts or the machine reboots
THEN the container MUST be automatically restarted

#### Scenario: Policy "unless-stopped" restarts on exit

GIVEN a container created with `--restart unless-stopped`
WHEN the container's main process exits with any code
THEN the container MUST be automatically restarted

#### Scenario: Policy "unless-stopped" does not restart after explicit stop

GIVEN a container created with `--restart unless-stopped`
WHEN the user explicitly stops the container
AND the system (Cort/Positronics) restarts or the machine reboots
THEN the container MUST NOT be automatically restarted

#### Scenario: Restart with increasing backoff delay

GIVEN a container with a restart policy that triggers restarts
WHEN the container fails repeatedly
THEN the system SHOULD introduce an increasing delay between restart attempts to avoid tight restart loops
AND the delay SHOULD start at 100ms and double each time up to a maximum of 1 minute

---

## 17. Resource Limits

### Requirement: Memory Limits

The system MUST support setting memory limits for containers via the `--memory` flag. The limit MUST be enforced via cgroups. When a container exceeds its memory limit, the kernel MUST OOM-kill the container's process.

#### Scenario: Memory limit applied via cgroups

GIVEN a container created with `--memory 256m`
WHEN the container attempts to allocate 512MB of memory
THEN the kernel MUST OOM-kill the container's process
AND the container state MUST record that it was OOM-killed

#### Scenario: Memory limit in different units

GIVEN containers created with `--memory 512m`, `--memory 1g`, and `--memory 1073741824`
WHEN the containers are inspected
THEN the memory limits MUST be 536870912, 1073741824, and 1073741824 bytes respectively

#### Scenario: Invalid memory limit is rejected

GIVEN a user specifies `--memory 0`
WHEN a container creation is attempted
THEN the system MUST return an error indicating the memory limit is invalid

### Requirement: CPU Limits

The system MUST support setting CPU limits for containers via the `--cpus` flag. The limit MUST be enforced via cgroups CPU quota and period.

#### Scenario: CPU limit applied via cgroups

GIVEN a container created with `--cpus 0.5`
WHEN the container attempts to use 100% of a CPU core
THEN the container MUST be throttled to approximately 50% of a single core

#### Scenario: CPU limit of 2.0 allows two full cores

GIVEN a container created with `--cpus 2.0`
WHEN the container attempts to use multiple cores
THEN the container MUST be allowed up to the equivalent of 2 full CPU cores

### Requirement: PID Limits

The system MUST support setting a maximum number of processes (PIDs) for containers via the `--pids-limit` flag. The limit MUST be enforced via cgroups.

#### Scenario: PID limit prevents fork bomb

GIVEN a container created with `--pids-limit 100`
WHEN the container attempts to create more than 100 processes
THEN process creation MUST fail with a resource limit error

#### Scenario: PID limit of -1 means unlimited

GIVEN a container created with `--pids-limit -1`
WHEN the container creates processes
THEN there MUST be no PID limit imposed by cgroups

---

## 18. Environment Variables

### Requirement: Environment Variable Injection

The system MUST support injecting environment variables into a container at creation time via `--env KEY=VALUE` and loading from a file via `--env-file`.

#### Scenario: Single environment variable injected

GIVEN a container created with `--env MY_VAR=hello`
WHEN a process inside the container reads the MY_VAR variable
THEN the value MUST be "hello"

#### Scenario: Multiple environment variables injected

GIVEN a container created with `--env VAR1=one --env VAR2=two`
WHEN a process inside the container reads both variables
THEN VAR1 MUST be "one" and VAR2 MUST be "two"

#### Scenario: Environment variable from file

GIVEN a file containing "DB_HOST=localhost" and "DB_PORT=5432" (one per line)
AND a container created with `--env-file <path>`
WHEN a process inside the container reads the variables
THEN DB_HOST MUST be "localhost" and DB_PORT MUST be "5432"

#### Scenario: Env flag overrides image environment variable

GIVEN an image with ENV APP_ENV=production
AND a container created with `--env APP_ENV=development`
WHEN a process inside the container reads APP_ENV
THEN the value MUST be "development"

#### Scenario: Env variable with empty value

GIVEN a container created with `--env MY_VAR=`
WHEN a process inside the container reads MY_VAR
THEN the variable MUST exist with an empty string value

#### Scenario: Env-file with comments and blank lines

GIVEN an env-file containing lines: "# This is a comment", "VAR1=one", a blank line, "VAR2=two", and "# Another comment"
WHEN a container is created with `--env-file <path>`
THEN VAR1 MUST be "one" and VAR2 MUST be "two"
AND comment lines and blank lines MUST be ignored

#### Scenario: Non-existent env-file is rejected

GIVEN a path to a file that does not exist
WHEN a container creation is attempted with `--env-file <path>`
THEN the system MUST return an error indicating the file was not found

---

## 19. Labels

### Requirement: Container Labels

The system MUST support attaching key-value labels to containers at creation time via `--label key=value` and loading from a file via `--label-file`. Labels MUST be persisted in the Waystation and MUST be filterable.

#### Scenario: Label attached to container

GIVEN a container created with `--label app=web`
WHEN the container is inspected
THEN the labels MUST include "app" with value "web"

#### Scenario: Multiple labels attached

GIVEN a container created with `--label app=web --label env=prod`
WHEN the container is inspected
THEN both labels MUST be present

#### Scenario: Labels from file

GIVEN a label file containing "tier=frontend" and "version=1.0" (one per line)
AND a container created with `--label-file <path>`
WHEN the container is inspected
THEN both labels MUST be present

#### Scenario: Labels filterable in container list

GIVEN a container with label "app=web" and a container with label "app=api"
WHEN the user executes `ps --all --filter label=app=web`
THEN only the container with label "app=web" MUST be listed

#### Scenario: Image labels inherited by container

GIVEN an image with label "maintainer=admin at example.com"
WHEN a container is created from that image
THEN the container MUST inherit the image's labels
AND user-specified labels MUST override image labels with the same key

---

## 20. Run Shortcut

### Requirement: Run as Create + Start Workflow

The `run` command MUST be a shortcut that performs container creation and start in a single operation. In foreground mode (default), it MUST also attach to the container's I/O streams. In detached mode (`-d`), it MUST return the container ID immediately without attaching.

#### Scenario: Foreground run with auto-attach

GIVEN a valid image reference
WHEN the user executes `run <image> echo hello`
THEN the system MUST create and start a container
AND attach to its stdout
AND display "hello"
AND the command MUST exit when the container's process exits

#### Scenario: Detached run returns container ID

GIVEN a valid image reference
WHEN the user executes `run -d <image>`
THEN the system MUST create and start a container in the background
AND the command MUST immediately print the full container ID and exit

#### Scenario: Run with --rm auto-removes container

GIVEN a valid image reference
WHEN the user executes `run --rm <image> echo hello`
THEN the system MUST create, start, and attach to the container
AND after the container's process exits, the container MUST be automatically removed
AND the container MUST NOT appear in `ps --all`

#### Scenario: Run with --rm and detached mode

GIVEN a valid image reference
WHEN the user executes `run -d --rm <image>`
THEN the system MUST create and start the container in the background
AND when the container's process eventually exits, the container MUST be automatically removed

#### Scenario: Run with all flags combined

GIVEN a valid image reference
WHEN the user executes `run -d --name web --memory 256m --cpus 0.5 -p 8080:80 --env APP=test --label tier=frontend --restart always <image>`
THEN the container MUST be created with the name "web"
AND memory limit of 256MB
AND CPU limit of 0.5
AND port mapping 8080->80
AND environment variable APP=test
AND label tier=frontend
AND restart policy "always"
AND the container MUST start in the background

#### Scenario: Run pulls image if not present locally

GIVEN an image reference that is NOT present in the local Maturin store
AND the image IS available in a remote registry
WHEN the user executes `run <image>`
THEN the system MUST automatically pull the image first
AND then create and start the container

#### Scenario: Run fails if image not found anywhere

GIVEN an image reference that is NOT present locally and NOT available in any registry
WHEN the user executes `run <image>`
THEN the system MUST return an error indicating the image was not found

### Requirement: Container Restart Command

The `restart` command MUST stop and then start a container. It MUST accept the same `--time` flag as stop for configuring the grace period.

#### Scenario: Restart a running container

GIVEN a running container
WHEN the user executes `restart <container>`
THEN the system MUST stop the container with the default timeout
AND then start the container again
AND the container MUST be in the Running state

#### Scenario: Restart with custom timeout

GIVEN a running container
WHEN the user executes `restart --time 5 <container>`
THEN the system MUST stop the container with a 5-second timeout before SIGKILL
AND then start the container again

#### Scenario: Restart a stopped container

GIVEN a stopped container
WHEN the user executes `restart <container>`
THEN the system MUST start the container
AND the container MUST be in the Running state

---

## 21. Container Rename

### Requirement: Rename a Container

The system MUST support renaming a container. The new name MUST be unique. The rename MUST update all state references atomically.

#### Scenario: Rename a container

GIVEN a container named "old-name"
WHEN the user executes `rename old-name new-name`
THEN the container MUST be accessible by the name "new-name"
AND the name "old-name" MUST no longer resolve to any container

#### Scenario: Rename to existing name is rejected

GIVEN a container named "web" and another container named "api"
WHEN the user attempts to rename "api" to "web"
THEN the system MUST return an error indicating the name "web" is already in use

#### Scenario: Rename preserves all other container properties

GIVEN a running container with specific configuration
WHEN the container is renamed
THEN all properties except the name MUST remain unchanged

---

## 22. Container Top

### Requirement: List Processes Inside Container

The system MUST support listing the processes running inside a container, showing at minimum the PID, user, time, and command for each process.

#### Scenario: Top displays container processes

GIVEN a running container with multiple processes
WHEN the user executes `top <container>`
THEN the output MUST list all processes inside the container
AND each entry MUST include at minimum PID, USER, TIME, and COMMAND

#### Scenario: Top of non-running container is rejected

GIVEN a stopped container
WHEN the user executes `top <container>`
THEN the system MUST return an error indicating the container is not running
