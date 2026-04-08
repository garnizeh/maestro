# Tower Engine Specification

## Purpose

The Tower is the core engine that orchestrates all Maestro subsystems. It initializes components, loads configuration from katet.toml, manages the directory structure, coordinates subsystem interactions, provides structured logging, and enforces consistent error handling. Every CLI command and API call flows through the Tower.

The Tower is the nexus upon which all components depend. If it fails, nothing works.

---

## 1. Initialization

### Requirement: Subsystem Initialization

The system MUST initialize all subsystems in a deterministic order during startup. Each subsystem MUST be initialized only when its dependencies are satisfied. Initialization failures in any required subsystem MUST prevent the system from proceeding.

#### Scenario: Successful initialization sequence

GIVEN a valid katet.toml configuration exists
AND all required subsystems are available
WHEN the system starts
THEN the Tower MUST initialize subsystems in dependency order
AND all subsystems MUST report successful initialization before accepting commands

#### Scenario: Required subsystem fails to initialize

GIVEN a required subsystem (e.g., Waystation state store) cannot be initialized
WHEN the system starts
THEN the Tower MUST return an error describing which subsystem failed and why
AND no commands MUST be accepted
AND the exit code MUST be non-zero

#### Scenario: Optional subsystem fails gracefully

GIVEN an optional subsystem (e.g., Positronics API server) is unavailable
WHEN the system starts
THEN the Tower MUST log a warning about the unavailable subsystem
AND the system MUST continue operating with reduced functionality

---

### Requirement: Directory Structure Creation

The system MUST create the required directory structure on first run if it does not already exist. All directories MUST be created with permissions 0700 to restrict access to the invoking user.

#### Scenario: First-run directory creation

GIVEN the Waystation root directory does not exist
WHEN the system starts for the first time
THEN the system MUST create the directory tree including: containers, maturin (with blobs/sha256 and manifests subdirectories), dogan, beam, and thinnies directories
AND all created directories MUST have permissions 0700

#### Scenario: Existing directory structure is preserved

GIVEN the directory structure already exists with content
WHEN the system starts
THEN the system MUST NOT overwrite or remove any existing directories or files
AND the system MUST verify the structure is intact

#### Scenario: Insufficient permissions for directory creation

GIVEN the parent directory does not allow write access by the invoking user
WHEN the system starts for the first time
THEN the system MUST return an error indicating it cannot create the directory structure
AND the error MUST include the path that could not be created

---

## 2. Configuration Loading

### Requirement: Configuration Precedence

The system MUST load configuration from multiple sources with the following precedence (highest to lowest): CLI flags, environment variables, katet.toml file, built-in defaults. A value from a higher-precedence source MUST override the same value from a lower-precedence source.

#### Scenario: CLI flag overrides config file

GIVEN katet.toml contains runtime.default = "runc"
AND the CLI flag --runtime crun is specified
WHEN the system loads configuration
THEN the effective runtime MUST be "crun"

#### Scenario: Environment variable overrides config file

GIVEN katet.toml contains runtime.default = "runc"
AND the environment variable MAESTRO_RUNTIME is set to "youki"
AND no --runtime CLI flag is specified
WHEN the system loads configuration
THEN the effective runtime MUST be "youki"

#### Scenario: Config file value used when no override exists

GIVEN katet.toml contains storage.driver = "btrfs"
AND no CLI flag or environment variable overrides the storage driver
WHEN the system loads configuration
THEN the effective storage driver MUST be "btrfs"

#### Scenario: Built-in default used when no source provides a value

GIVEN katet.toml does not contain a log.driver entry
AND no CLI flag or environment variable specifies the log driver
WHEN the system loads configuration
THEN the effective log driver MUST be the built-in default value

---

### Requirement: Configuration File Format

The system MUST use TOML as the configuration file format. The configuration file MUST be named katet.toml. The system MUST support the following top-level sections: runtime, storage, network, security, log, and registry.mirrors.

#### Scenario: Valid TOML configuration loaded

GIVEN a katet.toml file exists with valid TOML syntax
AND the file contains sections for runtime, storage, network, security, and log
WHEN the system loads the configuration
THEN all values from each section MUST be parsed and available to the corresponding subsystem

#### Scenario: Invalid TOML syntax rejected

GIVEN a katet.toml file exists with invalid TOML syntax
WHEN the system attempts to load the configuration
THEN the system MUST return an error indicating the configuration file is malformed
AND the error MUST include the line number and nature of the syntax error

#### Scenario: Unknown configuration keys are ignored

GIVEN a katet.toml file contains a key not recognized by the system
WHEN the system loads the configuration
THEN the system SHOULD log a warning about the unrecognized key
AND the system MUST continue loading all other valid configuration

---

### Requirement: Configuration File Location

The system MUST look for katet.toml at the default path (~/.config/maestro/katet.toml) unless overridden by the --config CLI flag. The system MUST support the XDG_CONFIG_HOME environment variable for determining the default path.

#### Scenario: Default config path

GIVEN no --config flag is specified
AND XDG_CONFIG_HOME is not set
WHEN the system looks for configuration
THEN the system MUST look for katet.toml at ~/.config/maestro/katet.toml

#### Scenario: XDG_CONFIG_HOME respected

GIVEN XDG_CONFIG_HOME is set to /custom/config
AND no --config flag is specified
WHEN the system looks for configuration
THEN the system MUST look for katet.toml at /custom/config/maestro/katet.toml

#### Scenario: CLI flag overrides default path

GIVEN the --config /path/to/custom.toml flag is specified
WHEN the system looks for configuration
THEN the system MUST load configuration from /path/to/custom.toml
AND the default path MUST NOT be consulted

#### Scenario: Specified config file does not exist

GIVEN the --config /nonexistent/config.toml flag is specified
AND the file does not exist
WHEN the system starts
THEN the system MUST return an error indicating the specified configuration file was not found

---

## 3. First-Run Experience

### Requirement: Missing Configuration Detection

The system MUST detect when no configuration file exists on first run. Upon detection, the system MUST create a configuration file with sensible defaults and display a welcome message informing the user.

#### Scenario: First-run with no config creates default

GIVEN no katet.toml file exists at the default location
AND no --config flag is specified
WHEN the system starts for the first time
THEN the system MUST create a katet.toml file at the default location
AND the file MUST contain sensible defaults for all sections
AND the defaults MUST include rootless mode enabled

#### Scenario: Welcome message displayed on first run

GIVEN no katet.toml file exists at the default location
WHEN the system starts for the first time
THEN the system MUST display a welcome message on stderr
AND the message MUST inform the user that a configuration file has been created
AND the message MUST include the path to the created configuration file

#### Scenario: Subsequent runs skip welcome message

GIVEN a katet.toml file already exists at the default location
WHEN the system starts
THEN no welcome message MUST be displayed

---

## 4. Subsystem Coordination

### Requirement: Command Routing

The system MUST route each CLI command to the appropriate subsystem for execution. The Tower MUST act as the coordinator that delegates work, not as the executor of domain-specific logic.

#### Scenario: Container command routed to Gan

GIVEN the user invokes a container lifecycle command (create, start, stop, kill, rm)
WHEN the Tower receives the command
THEN the Tower MUST delegate execution to the Gan (container lifecycle) subsystem
AND the Tower MUST pass through the parsed configuration and flags

#### Scenario: Image command routed to Maturin

GIVEN the user invokes an image management command (pull, push, ls, rm, inspect)
WHEN the Tower receives the command
THEN the Tower MUST delegate execution to the Maturin (image management) subsystem

#### Scenario: Network command routed to Beam

GIVEN the user invokes a network management command (create, ls, rm, connect, disconnect)
WHEN the Tower receives the command
THEN the Tower MUST delegate execution to the Beam (network management) subsystem

#### Scenario: Security configuration provided to subsystems

GIVEN a container creation command is received
WHEN the Tower coordinates the creation
THEN the Tower MUST request security configuration from White (security subsystem)
AND the Tower MUST pass the security configuration to the OCI spec generator

---

## 5. Logging

### Requirement: Structured Logging

The system MUST use structured logging with key-value pairs. Log entries MUST include at minimum: timestamp, level, message, and component name. The system MUST support configurable log levels: debug, info, warn, and error.

#### Scenario: Log level filtering

GIVEN the log level is configured to "warn"
WHEN a debug-level log event occurs
THEN the event MUST NOT be written to the log output
AND when a warn-level or error-level log event occurs
THEN the event MUST be written to the log output

#### Scenario: Default log level is warn

GIVEN no log level is specified in configuration, environment, or CLI flags
WHEN the system starts
THEN the effective log level MUST be "warn"
AND debug and info messages MUST NOT appear in output

#### Scenario: Debug level shows verbose output

GIVEN the log level is configured to "debug" via --log-level debug
WHEN operations are performed
THEN debug, info, warn, and error messages MUST all be written to the log output

---

### Requirement: Output Format Adaptation

The system MUST adapt log output format based on the output destination. When the output is a TTY (interactive terminal), the system MUST use a human-readable format with colors. When the output is piped or redirected, the system MUST use structured JSON format.

#### Scenario: Human-readable output for TTY

GIVEN stderr is connected to a TTY
WHEN a log event is emitted
THEN the output MUST use a human-readable format
AND the output SHOULD include color-coded log levels

#### Scenario: JSON output for pipe

GIVEN stderr is piped to another process or redirected to a file
WHEN a log event is emitted
THEN the output MUST be valid JSON
AND each line MUST be a complete JSON object containing at minimum: timestamp, level, and message fields

#### Scenario: Logs written to stderr

GIVEN the system is running
WHEN log events are emitted
THEN all log output MUST be written to stderr
AND stdout MUST be reserved for command output

---

## 6. Error Handling

### Requirement: Consistent Error Format

The system MUST use a consistent error format across all subsystems. Errors MUST include a human-readable message, the originating component name, and sufficient context to diagnose the issue.

#### Scenario: Error includes component context

GIVEN an error occurs in the Beam (network) subsystem during container creation
WHEN the error is reported to the user
THEN the error message MUST identify the Beam subsystem as the source
AND the error message MUST describe what operation failed
AND the error message MUST include actionable context (e.g., which network, which container)

#### Scenario: Nested errors preserve chain

GIVEN a low-level system call error occurs during a Prim (storage) operation
WHEN the error propagates to the user
THEN the error MUST include the original system error
AND the error MUST include the higher-level context of what Maestro was trying to do

---

### Requirement: Exit Codes

The system MUST use meaningful exit codes to indicate the outcome of CLI commands. Exit code 0 MUST indicate success. Non-zero exit codes MUST indicate failure, with distinct codes for different failure categories.

#### Scenario: Successful command returns zero

GIVEN a command completes successfully
WHEN the process exits
THEN the exit code MUST be 0

#### Scenario: General error returns non-zero

GIVEN a command fails due to an error
WHEN the process exits
THEN the exit code MUST be non-zero

#### Scenario: Resource not found returns distinct code

GIVEN a command references a container, image, or network that does not exist
WHEN the process exits
THEN the exit code MUST be a distinct non-zero value reserved for "not found" errors
AND the exit code MUST be different from general errors

---

## 7. Configuration Display and Edit

### Requirement: Show Effective Configuration

The system MUST provide a command to display the effective configuration after all precedence rules have been applied. The output MUST be in valid TOML format.

#### Scenario: Show merged configuration

GIVEN katet.toml sets runtime.default = "runc"
AND the environment variable MAESTRO_RUNTIME is set to "crun"
WHEN the user invokes the config show command
THEN the output MUST display runtime.default = "crun" (reflecting the override)
AND the output MUST be valid TOML that could be saved as a new katet.toml

#### Scenario: Show configuration in alternative formats

GIVEN the --format json flag is specified
WHEN the user invokes the config show command
THEN the output MUST be valid JSON representing the effective configuration

---

### Requirement: Edit Configuration

The system MUST provide a command that opens the katet.toml file in the user's preferred editor. The editor MUST be determined by the EDITOR environment variable.

#### Scenario: Open config in editor

GIVEN the EDITOR environment variable is set
WHEN the user invokes the config edit command
THEN the system MUST open katet.toml in the specified editor
AND the system MUST wait for the editor to exit before returning

#### Scenario: No EDITOR variable set

GIVEN the EDITOR environment variable is not set
AND VISUAL is also not set
WHEN the user invokes the config edit command
THEN the system MUST return an error indicating that no editor is configured
AND the error MUST suggest setting the EDITOR environment variable
