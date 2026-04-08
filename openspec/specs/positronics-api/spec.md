# Positronics API Specification

## Purpose

Positronics is the optional socket-mode API server for Maestro. It provides a gRPC interface over a Unix socket for programmatic access to all Maestro subsystems, Ka-shume (event streaming) for real-time event notification, and Breaker (background garbage collection). Positronics runs as the invoking user, never as root.

North Central Positronics built persistent technological services that serve anyone who speaks their protocol.

---

## Requirements

### Requirement: gRPC Server on Unix Socket

The system MUST support starting a gRPC API server that listens on a Unix domain socket. The default socket path MUST be within the user's XDG_RUNTIME_DIR. The server MUST run as the invoking user and MUST NOT require root privileges.

#### Scenario: Start API server

GIVEN no Positronics server is currently running
WHEN the user invokes service start
THEN a gRPC server MUST begin listening on the Unix socket at the default path
AND the socket file MUST be created with permissions restricting access to the invoking user

#### Scenario: Socket path respects XDG_RUNTIME_DIR

GIVEN XDG_RUNTIME_DIR is set to /run/user/1000
WHEN the Positronics server starts
THEN the socket MUST be created at /run/user/1000/maestro/positronics.sock

---

### Requirement: Service Definitions

The system MUST expose the following gRPC services: GanService (container lifecycle), MaturinService (image management), BeamService (network management), DoganService (volume management), and TowerService (system operations). Each service MUST delegate to the same subsystem logic used by the CLI.

#### Scenario: Container operations via gRPC

GIVEN the Positronics server is running
WHEN a gRPC client invokes GanService.Create followed by GanService.Start
THEN the container MUST be created and started using the same logic as the CLI
AND the container MUST appear in both gRPC List and CLI ps output

#### Scenario: Image pull with streaming progress

GIVEN the Positronics server is running
WHEN a gRPC client invokes MaturinService.Pull
THEN the server MUST stream progress updates to the client as layers are downloaded
AND the final message MUST indicate success or failure

---

### Requirement: Ka-shume (Event Streaming)

The system MUST support streaming real-time events to connected clients via a gRPC server-streaming RPC. Events MUST include container lifecycle changes (create, start, stop, die), image operations (pull, rm), and system events.

#### Scenario: Stream container lifecycle events

GIVEN a gRPC client is subscribed to the event stream
WHEN a container is created, started, and then stopped via any interface (CLI or API)
THEN the event stream MUST deliver events for each state transition in order
AND each event MUST include the event type, resource identifier, and timestamp

#### Scenario: No events when idle

GIVEN a gRPC client is subscribed to the event stream
AND no operations are performed
WHEN the client waits
THEN no events MUST be delivered
AND the connection MUST remain open

---

### Requirement: Breaker (Background Garbage Collection)

The system MUST support a background garbage collection worker that periodically reclaims unused resources (orphaned blobs, dangling images, stopped containers). The interval MUST be configurable via katet.toml.

#### Scenario: Periodic garbage collection

GIVEN the Positronics server is running
AND the gc_interval is configured to a specific duration
WHEN the interval elapses
THEN the Breaker MUST perform garbage collection
AND unreferenced blobs and stopped containers MUST be eligible for removal

---

### Requirement: Service Start/Stop/Status

The system MUST provide CLI commands to start, stop, and query the status of the Positronics server. The status command MUST report whether the server is running, its PID, and uptime.

#### Scenario: Service lifecycle

GIVEN the Positronics server is not running
WHEN the user invokes service start
THEN the server MUST start in the background
AND service status MUST report the server as running with its PID and start time
AND when the user invokes service stop
THEN the server MUST shut down gracefully
AND service status MUST report the server as not running

---

### Requirement: CLI Auto-Detection of Positronics

The system MUST automatically detect whether a Positronics server is running by checking for the existence of the Unix socket. When the server is available, the CLI SHOULD route operations through the API. When unavailable, the CLI MUST execute commands directly without error.

#### Scenario: CLI uses API when server is available

GIVEN the Positronics socket exists and the server is responding
WHEN the user invokes a CLI command (e.g., container ps)
THEN the CLI SHOULD route the request through the gRPC API

#### Scenario: CLI falls back to direct execution

GIVEN no Positronics socket exists
WHEN the user invokes a CLI command
THEN the CLI MUST execute the command directly without attempting API communication
AND the command MUST succeed as if no server mode existed
