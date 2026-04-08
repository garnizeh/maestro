# Eld Runtime Specification

## Purpose

Eld is the runtime abstraction layer. All OCI runtimes (runc, crun, youki, gVisor, Kata) descend from the Eld interface. Includes Pathfinder (runtime discovery), Cort (conmon-rs container monitor), Specgen (OCI config generation), and the hook system.

---

## 1. Runtime Interface

### Requirement: Eld Interface Operations

The Eld interface MUST define a common contract for all OCI-compatible container runtimes. The following operations MUST be supported: Create, Start, Kill, Delete, State, and Features. All implementations MUST adhere to this contract regardless of the underlying runtime binary.

#### Scenario: Create operation prepares container from bundle

GIVEN a valid OCI bundle directory with rootfs and config.json
WHEN the Create operation is invoked with a container ID and bundle path
THEN the runtime MUST create the container environment per the config.json
AND the container MUST be in the "created" state as reported by the runtime
AND the runtime MUST NOT start the user-specified process

#### Scenario: Start operation begins the user process

GIVEN a container in the "created" state
WHEN the Start operation is invoked with the container ID
THEN the runtime MUST start the user-specified process from config.json
AND the container MUST transition to the "running" state

#### Scenario: Kill operation sends signal to container process

GIVEN a container in the "running" state
WHEN the Kill operation is invoked with the container ID and a signal (e.g., SIGTERM)
THEN the runtime MUST deliver the specified signal to the container's init process

#### Scenario: Delete operation removes container resources

GIVEN a container in the "stopped" state
WHEN the Delete operation is invoked with the container ID
THEN the runtime MUST release all resources associated with the container (namespaces, cgroups)
AND the container MUST no longer appear in the runtime's state queries

#### Scenario: Delete with force for stuck containers

GIVEN a container that has exited but whose resources were not fully cleaned up
WHEN the Delete operation is invoked with the force option
THEN the runtime MUST forcibly remove all container resources

#### Scenario: State operation returns current container state

GIVEN a running container
WHEN the State operation is invoked with the container ID
THEN the runtime MUST return a state object containing at minimum: the OCI version, the container ID, the status (created/running/stopped), the PID of the init process, and the bundle path

#### Scenario: State of non-existent container returns error

GIVEN no container with the specified ID exists in the runtime
WHEN the State operation is invoked
THEN the runtime MUST return an error indicating the container does not exist

#### Scenario: Features operation reports runtime capabilities

WHEN the Features operation is invoked
THEN the runtime MUST return the set of supported features
AND the response MUST indicate which namespaces, cgroup controllers, and other capabilities the runtime supports

---

## 2. OCI Runtime Spec Compliance

### Requirement: Valid config.json Generation

The system MUST generate a config.json file that conforms to the OCI Runtime Specification. The generated configuration MUST be accepted without error by all supported OCI runtimes (runc, crun, youki).

#### Scenario: Generated config.json includes required fields

WHEN a config.json is generated for a new container
THEN the file MUST include the `ociVersion` field set to a supported OCI Runtime Spec version
AND the file MUST include a `root` object with a `path` pointing to the rootfs directory
AND the file MUST include a `process` object when the container is intended to be started

#### Scenario: Generated config.json passes runtime validation

GIVEN a generated config.json
WHEN the OCI runtime's spec validation is invoked against it
THEN the validation MUST pass without errors

#### Scenario: ociVersion field matches supported spec version

WHEN a config.json is generated
THEN the `ociVersion` field MUST be set to a version compatible with the detected runtime's supported OCI spec version

### Requirement: Default Linux Namespaces

The generated config.json MUST configure Linux namespaces to provide process isolation. By default, the system MUST create new pid, network, mount, ipc, uts, and cgroup namespaces.

#### Scenario: Default namespace configuration

WHEN a container config.json is generated without namespace overrides
THEN the linux.namespaces array MUST include entries for: pid, network, mount, ipc, uts, and cgroup
AND each namespace entry MUST have the type field set appropriately

#### Scenario: Network namespace uses provided path

GIVEN a pre-created network namespace at a specific path
WHEN a config.json is generated with the network namespace path
THEN the network namespace entry MUST include the path to the pre-created namespace instead of creating a new one

#### Scenario: User namespace included in rootless mode

WHEN a config.json is generated in rootless (Calla) mode
THEN the linux.namespaces array MUST additionally include a user namespace entry
AND UID/GID mappings MUST be configured in the linux section

### Requirement: Default Masked and Readonly Paths

The generated config.json MUST include security-hardening paths. Sensitive kernel interfaces MUST be masked and security-relevant paths MUST be mounted read-only.

#### Scenario: Default masked paths configured

WHEN a config.json is generated with default security settings
THEN linux.maskedPaths MUST include at minimum: /proc/acpi, /proc/asound, /proc/kcore, /proc/keys, /proc/latency_stats, /proc/timer_list, /proc/timer_stats, /proc/sched_debug, /sys/firmware, /proc/scsi

#### Scenario: Default readonly paths configured

WHEN a config.json is generated with default security settings
THEN linux.readonlyPaths MUST include at minimum: /proc/bus, /proc/fs, /proc/irq, /proc/sys, /proc/sysrq-trigger

---

## 3. Pathfinder (Runtime Discovery)

### Requirement: Automatic Runtime Detection

The Pathfinder MUST automatically discover available OCI runtimes by searching the system PATH in the following priority order: crun, runc, youki. The first runtime found MUST be used as the default.

#### Scenario: Crun detected as highest priority

GIVEN crun and runc are both available in the system PATH
WHEN Pathfinder performs runtime discovery
THEN crun MUST be selected as the default runtime

#### Scenario: Runc selected when crun is absent

GIVEN runc is available in the system PATH but crun is not
WHEN Pathfinder performs runtime discovery
THEN runc MUST be selected as the default runtime

#### Scenario: Youki selected as last resort

GIVEN youki is the only OCI runtime available in the system PATH
WHEN Pathfinder performs runtime discovery
THEN youki MUST be selected as the default runtime

#### Scenario: No runtime found produces clear error

GIVEN no OCI runtimes (crun, runc, youki) are available in the system PATH
AND no runtime is configured in the system configuration
WHEN Pathfinder performs runtime discovery
THEN the system MUST return an error clearly indicating that no OCI runtime was found
AND the error message MUST suggest how to install a supported runtime

### Requirement: Configuration Override for Runtime Path

The system MUST allow overriding the runtime binary path via the system configuration file. A configured runtime path MUST take precedence over PATH discovery.

#### Scenario: Configured runtime path takes precedence

GIVEN the system configuration specifies runtime.path as "/opt/custom/crun"
AND runc is available in the system PATH
WHEN Pathfinder performs runtime discovery
THEN the runtime at "/opt/custom/crun" MUST be used

#### Scenario: Configured runtime path that does not exist produces error

GIVEN the system configuration specifies runtime.path as "/nonexistent/runtime"
WHEN Pathfinder performs runtime discovery
THEN the system MUST return an error indicating the configured runtime binary was not found at the specified path

### Requirement: Runtime Version Validation

The Pathfinder MUST validate the detected runtime by querying its version. The runtime MUST report a version that supports the OCI Runtime Specification version required by the system.

#### Scenario: Valid runtime version accepted

GIVEN a detected runtime binary
WHEN Pathfinder queries the runtime's version
THEN the system MUST verify the runtime responds successfully
AND the version information MUST be recorded for later inspection

#### Scenario: Runtime binary that fails version check is rejected

GIVEN a binary at the expected runtime path that is not a valid OCI runtime
WHEN Pathfinder queries its version
THEN the system MUST return an error indicating the binary is not a valid OCI runtime

### Requirement: Runtime Feature Detection

The Pathfinder MUST query the runtime's supported features (when available) to determine capabilities such as cgroup version support, seccomp support, and available namespace types.

#### Scenario: Features detected from runtime

GIVEN a runtime that supports the "features" command
WHEN Pathfinder queries the runtime's features
THEN the system MUST record the supported features
AND the features MUST be available for later queries

#### Scenario: Features unavailable handled gracefully

GIVEN a runtime that does not support the "features" command
WHEN Pathfinder queries the runtime's features
THEN the system MUST NOT return an error
AND the system SHOULD assume a default set of features based on the runtime type and version

---

## 4. Runtime Selection

### Requirement: Per-Container Runtime Selection

The system MUST support selecting a specific OCI runtime for each container via the `--runtime` flag. This MUST override the default runtime determined by Pathfinder or configuration.

#### Scenario: Container created with specific runtime

GIVEN crun is the default runtime
AND runc is also available
WHEN the user creates a container with `--runtime runc`
THEN the container MUST be created and managed using runc instead of crun

#### Scenario: Requested runtime not found

GIVEN the user specifies `--runtime nonexistent`
AND no runtime named "nonexistent" is available
WHEN the user attempts to create a container
THEN the system MUST return an error indicating the specified runtime was not found

#### Scenario: Runtime recorded in container state

GIVEN a container created with `--runtime crun`
WHEN the container's state is inspected
THEN the state MUST include the runtime name "crun" and the runtime binary path

### Requirement: Default Runtime from Configuration

The system MUST support setting a default runtime preference in the system configuration file. This default MUST be used when no `--runtime` flag is specified.

#### Scenario: Configured default runtime used

GIVEN the system configuration specifies runtime.default as "runc"
AND both crun and runc are available
WHEN a container is created without the `--runtime` flag
THEN runc MUST be used as the runtime

#### Scenario: CLI flag overrides configured default

GIVEN the system configuration specifies runtime.default as "runc"
WHEN a container is created with `--runtime crun`
THEN crun MUST be used as the runtime

### Requirement: Runtime-Specific CLI Adaptations

The system MUST handle differences in CLI invocation patterns between runtimes. Different runtimes MAY accept different flags or require different argument ordering for the same operation.

#### Scenario: Standard OCI runtime invocation

GIVEN a standard OCI runtime (runc, crun, or youki)
WHEN the system invokes the Create operation
THEN the system MUST invoke the runtime with the standard arguments: `<runtime> create --bundle <path> <container-id>`

#### Scenario: Runtime with custom arguments

GIVEN a runtime that requires additional flags (e.g., gVisor with --rootless)
WHEN the system invokes the Create operation
THEN the system MUST include the runtime-specific additional flags in the invocation

---

## 5. Cort (conmon-rs Integration)

### Requirement: Container Monitor Process

The system MUST fork a Cort (conmon-rs) monitor process for each container that is started. Cort MUST outlive both the CLI process and the OCI runtime process, maintaining supervision of the container for its entire lifetime.

#### Scenario: Cort process forked on container start

GIVEN a container in the Created state
WHEN the container is started
THEN a Cort process MUST be forked
AND Cort MUST be a separate process from the CLI
AND the Cort process PID MUST be recorded in the container state

#### Scenario: Cort survives CLI exit

GIVEN a container started in detached mode
WHEN the CLI process exits
THEN the Cort process MUST continue running
AND the container's main process MUST continue running under Cort's supervision

#### Scenario: Cort survives OCI runtime exit

GIVEN a running container
WHEN the OCI runtime process exits (as is normal after starting the container)
THEN Cort MUST continue monitoring the container
AND the container's main process MUST continue running

### Requirement: Stdio Pipe Configuration

Cort MUST configure standard I/O pipes for the container's main process. Cort MUST capture stdout and stderr and forward them to the configured log driver. Cort MUST optionally accept stdin forwarding for interactive containers.

#### Scenario: Stdout and stderr captured by Cort

GIVEN a running container that writes to both stdout and stderr
WHEN the log file is examined
THEN both stdout and stderr output MUST be present
AND each line MUST be attributed to the correct stream

#### Scenario: Stdin forwarded for interactive containers

GIVEN a container started with interactive mode (-i)
WHEN data is written to the stdin pipe
THEN the data MUST be forwarded to the container's main process stdin

#### Scenario: Stdio pipes configured for detached containers

GIVEN a container started in detached mode
WHEN the container writes to stdout and stderr
THEN Cort MUST capture the output and write it to the log file
AND no output MUST be sent to the CLI's terminal

### Requirement: PID File Management

Cort MUST write the container's init process PID to a PID file. The PID file MUST be atomically written and MUST be removed when the container is fully cleaned up.

#### Scenario: PID file created on container start

GIVEN a container that is started
WHEN Cort begins monitoring the container
THEN a PID file MUST be created containing the container's init process PID
AND the PID in the file MUST correspond to the actual running process

#### Scenario: PID file readable by other processes

GIVEN a running container with a PID file
WHEN another process reads the PID file
THEN it MUST contain a valid PID of the container's init process

### Requirement: Exit Code Collection

Cort MUST collect the exit code of the container's main process when it terminates. The exit code MUST be written to an exit file and persisted in the Waystation.

#### Scenario: Normal exit code collected

GIVEN a running container whose main process exits with code 0
WHEN Cort detects the exit
THEN Cort MUST write the exit code 0 to an exit file
AND the exit code MUST be persisted in the Waystation state

#### Scenario: Non-zero exit code collected

GIVEN a running container whose main process exits with code 137
WHEN Cort detects the exit
THEN the exit code 137 MUST be recorded

#### Scenario: Signal-killed process exit code collected

GIVEN a running container whose main process is killed by SIGKILL
WHEN Cort detects the termination
THEN the exit code MUST reflect the signal (128 + signal number = 137 for SIGKILL)

### Requirement: Log Forwarding

Cort MUST forward container stdout and stderr to the configured log driver. For the json-file driver, Cort MUST write each log line as a JSON object with stream identifier, timestamp, and log message.

#### Scenario: JSON-file log format

GIVEN a container configured with the json-file log driver
WHEN the container writes "hello world" to stdout
THEN Cort MUST write a JSON object to the log file containing:

- A "stream" field with value "stdout"
- A "time" field with an RFC 3339 timestamp
- A "log" field with value "hello world\n"

#### Scenario: Stderr logged separately from stdout

GIVEN a container that writes to both stdout and stderr
WHEN the logs are examined
THEN stdout entries MUST have stream "stdout" and stderr entries MUST have stream "stderr"

---

## 6. OCI Lifecycle Hooks

### Requirement: OCI Standard Hooks

The system MUST support all OCI Runtime Specification lifecycle hooks: createRuntime, createContainer, startContainer, poststart, and poststop. Each hook MUST be executed at the correct phase of the container lifecycle. Hooks MUST have configurable timeouts.

#### Scenario: createRuntime hook executed after namespaces created

GIVEN a container with a createRuntime hook configured
WHEN the container is created
THEN the createRuntime hook MUST be executed after runtime environment setup (namespaces created) but before pivot_root

#### Scenario: createContainer hook executed after createRuntime

GIVEN a container with a createContainer hook configured
WHEN the container is created
THEN the createContainer hook MUST be executed after createRuntime hooks complete but before pivot_root

#### Scenario: startContainer hook executed before user process

GIVEN a container with a startContainer hook configured
WHEN the container is started
THEN the startContainer hook MUST be executed after pivot_root but before the user-specified process begins

#### Scenario: poststart hook executed after user process begins

GIVEN a container with a poststart hook configured
WHEN the container is started and the user process is running
THEN the poststart hook MUST be executed after the user process has started

#### Scenario: poststop hook executed after container stops

GIVEN a container with a poststop hook configured
WHEN the container stops (regardless of exit reason)
THEN the poststop hook MUST be executed after the container process has terminated

#### Scenario: Hook timeout enforced

GIVEN a container with a hook configured with a timeout of 5 seconds
WHEN the hook process does not exit within 5 seconds
THEN the system MUST terminate the hook process
AND the system MUST report a hook timeout error

#### Scenario: Hook failure during create aborts container creation

GIVEN a container with a createRuntime hook that fails (exits non-zero)
WHEN the container creation is attempted
THEN the creation MUST fail
AND the error MUST indicate which hook failed and its exit code

#### Scenario: Poststart hook failure does not stop the container

GIVEN a container with a poststart hook that fails (exits non-zero)
WHEN the container is started
THEN the container MUST continue running despite the hook failure
AND the system SHOULD log the hook failure as a warning

### Requirement: Maestro-Specific Hooks

The system MUST support two additional hooks beyond the OCI standard: `maestro.prerun` (executed before container creation) and `maestro.postrun` (executed after container is fully removed).

#### Scenario: Prerun hook executed before creation

GIVEN a container with a maestro.prerun hook configured
WHEN the user initiates a container run
THEN the prerun hook MUST be executed before the container creation begins

#### Scenario: Prerun hook failure prevents container creation

GIVEN a container with a maestro.prerun hook that fails
WHEN the user initiates a container run
THEN the container MUST NOT be created
AND the system MUST report the prerun hook failure

#### Scenario: Postrun hook executed after full removal

GIVEN a container with a maestro.postrun hook configured
WHEN the container is stopped and removed
THEN the postrun hook MUST be executed after all container resources have been released

#### Scenario: Postrun hook failure does not affect cleanup

GIVEN a container with a maestro.postrun hook that fails
WHEN the container is removed
THEN the container MUST still be fully cleaned up
AND the system SHOULD log the postrun hook failure as a warning

---

## 7. Specgen (Configuration Generator)

### Requirement: Image Config Merging

The Specgen MUST merge the OCI image configuration into the generated config.json. Fields from the image config that MUST be merged include: Env, Cmd, Entrypoint, WorkingDir, ExposedPorts, User, Labels, and Volumes.

#### Scenario: Image Env merged into process env

GIVEN an image with Env ["PATH=/usr/bin", "LANG=C.UTF-8"]
WHEN a config.json is generated
THEN the process env MUST include "PATH=/usr/bin" and "LANG=C.UTF-8"

#### Scenario: Image Cmd used as default process args

GIVEN an image with Cmd ["nginx", "-g", "daemon off;"]
AND no Entrypoint defined
AND no user command override
WHEN a config.json is generated
THEN the process args MUST be ["nginx", "-g", "daemon off;"]

#### Scenario: Image Entrypoint prepended to Cmd

GIVEN an image with Entrypoint ["/docker-entrypoint.sh"] and Cmd ["nginx"]
AND no user command override
WHEN a config.json is generated
THEN the process args MUST be ["/docker-entrypoint.sh", "nginx"]

#### Scenario: Image WorkingDir applied to process cwd

GIVEN an image with WorkingDir "/app"
AND no `--workdir` override
WHEN a config.json is generated
THEN the process cwd MUST be "/app"

#### Scenario: No WorkingDir defaults to root

GIVEN an image with no WorkingDir set
AND no `--workdir` override
WHEN a config.json is generated
THEN the process cwd MUST default to "/"

#### Scenario: Image User applied to process user

GIVEN an image with User "1000:1000"
AND no `--user` override
WHEN a config.json is generated
THEN the process user UID MUST be 1000 and GID MUST be 1000

#### Scenario: Image Labels stored in container annotations

GIVEN an image with Labels {"maintainer": "admin at example.com"}
WHEN a config.json is generated
THEN the labels MUST be preserved in the container's annotations or metadata

#### Scenario: Image Volumes create mount points

GIVEN an image with Volumes ["/data", "/config"]
WHEN a config.json is generated without corresponding volume flags
THEN anonymous volumes SHOULD be created for /data and /config
AND they MUST be mounted in the container

### Requirement: User Flag Overrides

User-specified flags MUST override the corresponding image configuration values. The following flags MUST be supported: `--env`, `--user`, `--workdir`, `--hostname`, `--domainname`, `--entrypoint`, and custom commands.

#### Scenario: User env overrides image env

GIVEN an image with Env ["APP_MODE=production"]
AND the user specifies `--env APP_MODE=development`
WHEN a config.json is generated
THEN the process env MUST contain "APP_MODE=development"
AND MUST NOT contain "APP_MODE=production"

#### Scenario: User env adds to image env

GIVEN an image with Env ["APP_MODE=production"]
AND the user specifies `--env NEW_VAR=hello`
WHEN a config.json is generated
THEN the process env MUST contain both "APP_MODE=production" and "NEW_VAR=hello"

#### Scenario: User workdir overrides image workdir

GIVEN an image with WorkingDir "/app"
AND the user specifies `--workdir /tmp`
WHEN a config.json is generated
THEN the process cwd MUST be "/tmp"

#### Scenario: User hostname sets container hostname

GIVEN the user specifies `--hostname myhost`
WHEN a config.json is generated
THEN the hostname field MUST be "myhost"

#### Scenario: User domainname sets container domainname

GIVEN the user specifies `--domainname example.com`
WHEN a config.json is generated
THEN the domainname field MUST be "example.com"

#### Scenario: Default hostname is container name

GIVEN a container created with `--name web-server` and no `--hostname` flag
WHEN a config.json is generated
THEN the hostname SHOULD default to "web-server"

#### Scenario: User command overrides image CMD

GIVEN an image with CMD ["default-cmd"]
AND the user specifies the command "custom-cmd --flag"
WHEN a config.json is generated
THEN the process args MUST use the user's command instead of the image CMD

#### Scenario: User entrypoint overrides image ENTRYPOINT

GIVEN an image with Entrypoint ["/old-entrypoint.sh"]
AND the user specifies `--entrypoint /new-entrypoint.sh`
WHEN a config.json is generated
THEN the process args MUST use "/new-entrypoint.sh" as the entrypoint

#### Scenario: User override with UID string

GIVEN the user specifies `--user nobody`
WHEN a config.json is generated
THEN the system MUST resolve "nobody" to the appropriate UID using the container's /etc/passwd
AND set the process user accordingly

### Requirement: Security Config Merging

The Specgen MUST merge security configuration from the White subsystem into the generated config.json. This includes seccomp profiles, capability sets, and the no_new_privileges flag.

#### Scenario: Default seccomp profile applied

WHEN a config.json is generated without seccomp overrides
THEN the linux.seccomp section MUST contain the default seccomp profile
AND the profile MUST restrict dangerous syscalls

#### Scenario: Seccomp disabled with unconfined

GIVEN the user specifies `--security-opt seccomp=unconfined`
WHEN a config.json is generated
THEN the linux.seccomp section MUST NOT be present or MUST be set to allow all syscalls

#### Scenario: Custom seccomp profile applied

GIVEN the user specifies `--security-opt seccomp=<path-to-profile>`
WHEN a config.json is generated
THEN the linux.seccomp section MUST contain the custom profile loaded from the specified path

#### Scenario: Default capability set applied

WHEN a config.json is generated without capability overrides
THEN the process.capabilities MUST include a minimal default set of capabilities
AND dangerous capabilities (e.g., CAP_SYS_ADMIN, CAP_NET_RAW) MUST NOT be in the default set

#### Scenario: Capabilities added with --cap-add

GIVEN the user specifies `--cap-add NET_ADMIN`
WHEN a config.json is generated
THEN the process.capabilities MUST include CAP_NET_ADMIN in addition to the default set

#### Scenario: Capabilities dropped with --cap-drop

GIVEN the user specifies `--cap-drop NET_RAW`
WHEN a config.json is generated
THEN the process.capabilities MUST NOT include CAP_NET_RAW

#### Scenario: All capabilities dropped

GIVEN the user specifies `--cap-drop ALL`
WHEN a config.json is generated
THEN the process.capabilities MUST be empty (no capabilities granted)

#### Scenario: Cap-add after cap-drop ALL

GIVEN the user specifies `--cap-drop ALL --cap-add NET_BIND_SERVICE`
WHEN a config.json is generated
THEN the process.capabilities MUST contain only CAP_NET_BIND_SERVICE

#### Scenario: no_new_privileges enabled by default

WHEN a config.json is generated without security overrides
THEN the process.noNewPrivileges MUST be set to true

### Requirement: Network Config Merging

The Specgen MUST incorporate network configuration into the generated config.json. This includes setting the network namespace path to the pre-created namespace from Beam.

#### Scenario: Network namespace path set from Beam

GIVEN Beam has created a network namespace at path "/run/user/1000/netns/abc123"
WHEN a config.json is generated for a container using that namespace
THEN the linux.namespaces entry for "network" MUST include the path "/run/user/1000/netns/abc123"

#### Scenario: No network mode

GIVEN the user specifies `--network none`
WHEN a config.json is generated
THEN the container MUST have a new, isolated network namespace with no external connectivity

#### Scenario: Host network mode

GIVEN the user specifies `--network host`
WHEN a config.json is generated
THEN the network namespace entry MUST NOT be present (container shares the host network namespace)

### Requirement: Mount Config Merging

The Specgen MUST merge mount configuration from user flags into the generated config.json. Supported mount types include: named volumes, bind mounts, and tmpfs mounts.

#### Scenario: Named volume mount added to config

GIVEN the user specifies `-v myvolume:/data`
WHEN a config.json is generated
THEN the mounts array MUST include an entry with destination "/data" and source pointing to the named volume's data directory

#### Scenario: Bind mount added to config

GIVEN the user specifies `-v /host/path:/container/path`
WHEN a config.json is generated
THEN the mounts array MUST include an entry with destination "/container/path" and source "/host/path"

#### Scenario: Read-only bind mount

GIVEN the user specifies `-v /host/path:/container/path:ro`
WHEN a config.json is generated
THEN the mount entry MUST include "ro" in its options

#### Scenario: Tmpfs mount added to config

GIVEN the user specifies `--tmpfs /tmp:size=100m`
WHEN a config.json is generated
THEN the mounts array MUST include a tmpfs entry at "/tmp" with size option "100m"

#### Scenario: Multiple mounts combined

GIVEN the user specifies `-v vol1:/data -v /host:/host:ro --tmpfs /tmp`
WHEN a config.json is generated
THEN all three mounts MUST be present in the mounts array

#### Scenario: Default mounts always present

WHEN a config.json is generated for any container
THEN the mounts array MUST include standard system mounts for /proc, /dev, /dev/pts, /dev/shm, and /sys

---

## 8. Runtime-Specific Adaptations

### Requirement: gVisor (runsc) Support

The system MUST support gVisor (runsc) as an alternative runtime. The system MUST adapt its invocation to account for gVisor-specific differences in CLI flags and lifecycle behavior.

#### Scenario: Container created with gVisor runtime

GIVEN runsc is available on the system
WHEN a container is created with `--runtime runsc`
THEN the system MUST invoke runsc with the appropriate flags
AND the container MUST run inside gVisor's sandboxed kernel

#### Scenario: gVisor rootless mode flag

GIVEN runsc is used in rootless mode
WHEN the system invokes runsc
THEN the system MUST include the `--rootless` flag in the invocation

#### Scenario: gVisor feature limitations handled

GIVEN runsc is the active runtime
WHEN the system queries features
THEN the system MUST report any features that are not supported by gVisor (e.g., certain cgroup controllers or device access)

### Requirement: Kata Containers Support

The system MUST support Kata Containers as an alternative runtime. The system MUST adapt its invocation to account for Kata-specific differences including VM-based isolation.

#### Scenario: Container created with Kata runtime

GIVEN kata-runtime is available on the system
WHEN a container is created with `--runtime kata`
THEN the system MUST invoke kata-runtime
AND the container MUST run inside a lightweight virtual machine

#### Scenario: Kata runtime resource implications reported

GIVEN kata-runtime is the active runtime
WHEN the system reports runtime information
THEN the output SHOULD indicate that VM-based isolation is in effect
AND SHOULD note the additional resource overhead compared to namespace-based runtimes

---

## 9. Error Handling

### Requirement: Runtime Not Found Error

When the specified or auto-detected runtime binary is not found, the system MUST return a clear, actionable error message.

#### Scenario: Named runtime not found

GIVEN the user specifies `--runtime crun` but crun is not installed
WHEN container creation is attempted
THEN the system MUST return an error indicating "crun" was not found
AND the error MUST suggest installing the runtime or specifying a different one

#### Scenario: No runtime available at all

GIVEN no OCI runtime is installed and none is configured
WHEN any container operation is attempted
THEN the system MUST return an error indicating no OCI runtime is available
AND the error MUST list the supported runtimes (crun, runc, youki)

### Requirement: Runtime Crash Handling

The system MUST handle unexpected runtime process crashes gracefully. If the runtime process terminates abnormally during an operation, the system MUST report the failure and attempt to clean up.

#### Scenario: Runtime crashes during create

GIVEN a runtime that crashes (exits with non-zero code or signal) during the create operation
WHEN the crash is detected
THEN the system MUST return an error indicating the runtime failed during creation
AND the error MUST include the runtime's stderr output if available
AND the system MUST clean up any partially created resources

#### Scenario: Runtime crashes during start

GIVEN a runtime that crashes during the start operation
WHEN the crash is detected
THEN the system MUST return an error indicating the runtime failed during start
AND the container state MUST be updated to reflect the failure
AND the system MUST NOT leave the container in an inconsistent state

#### Scenario: Runtime stderr included in error message

GIVEN a runtime that fails and writes diagnostic information to stderr
WHEN the error is reported to the user
THEN the error message MUST include the runtime's stderr output to aid in diagnosis

### Requirement: Container Creation Failure Cleanup

When container creation fails at any step, the system MUST clean up all resources that were allocated during the partial creation attempt.

#### Scenario: Rootfs cleanup after config generation failure

GIVEN Prim has prepared a rootfs snapshot
AND config.json generation subsequently fails
WHEN the failure is detected
THEN the system MUST release the Prim snapshot
AND the system MUST remove the bundle directory

#### Scenario: Network cleanup after runtime creation failure

GIVEN Beam has allocated network resources (namespace, IP address)
AND the OCI runtime create subsequently fails
WHEN the failure is detected
THEN the system MUST release all Beam resources (invoke CNI DEL, destroy namespace)
AND the system MUST release the Prim snapshot
AND the system MUST remove the bundle directory

#### Scenario: Full cleanup after any creation step failure

GIVEN a container creation that fails at any step (Prim, Beam, White, config generation, or runtime invocation)
WHEN the failure is detected
THEN all resources allocated in preceding steps MUST be released
AND the container MUST NOT appear in the container list
AND no orphaned resources (snapshots, namespaces, state files) MUST remain

### Requirement: Start Failure Handling

When a container fails to start after being created, the system MUST update the container state appropriately and provide diagnostic information.

#### Scenario: Start failure leaves container in stopped state

GIVEN a container in the Created state
WHEN the start operation fails (e.g., the entrypoint binary does not exist)
THEN the container state MUST transition to Stopped (not remain in Created)
AND the exit code or error information MUST be recorded in the state
AND the error message MUST include relevant diagnostic information from the runtime

#### Scenario: Start failure with missing entrypoint

GIVEN a container whose config.json specifies a process path that does not exist in the rootfs
WHEN the container is started
THEN the system MUST return an error indicating the entrypoint was not found
AND the error MUST include the path that was not found

#### Scenario: Start failure due to resource limits

GIVEN a container configured with resource limits that the system cannot satisfy
WHEN the container is started
THEN the system MUST return an error indicating the resource limit could not be applied
AND the error MUST specify which resource limit failed
