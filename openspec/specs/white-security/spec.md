# White Security Specification

## Purpose

White is the security subsystem of Maestro. It governs all security-related concerns: Calla (rootless setup via user namespaces), seccomp profile management, Sandalwood (Linux capabilities), Gunslinger (AppArmor/SELinux integration), no_new_privileges enforcement, masked and readonly paths, read-only rootfs, Eld Mark (image signing and verification), and system check diagnostics.

White embodies the protective force that maintains order. It constrains and protects -- it does not attack.

---

## 1. Calla (Rootless Setup)

### Requirement: User Namespace Creation

The system MUST create a user namespace for each container so that an unprivileged host user is mapped to UID 0 inside the container. This mapping MUST be performed without requiring root privileges on the host.

#### Scenario: Default rootless UID mapping

GIVEN a host user with UID 1000
AND the user has a valid entry in /etc/subuid with at least 65536 subordinate UIDs
AND the user has a valid entry in /etc/subgid with at least 65536 subordinate GIDs
WHEN a container is created in rootless mode
THEN the container process MUST run as UID 0 inside the container
AND the container process MUST run as UID 1000 on the host
AND subordinate UIDs MUST be mapped to container UIDs 1 through 65535

#### Scenario: Container processes see root identity

GIVEN a rootless container is running
WHEN a process inside the container queries its own UID
THEN the result MUST be 0
AND when queried from the host, the process MUST show the invoking user's UID

---

### Requirement: subuid/subgid Reading and Validation

The system MUST read /etc/subuid and /etc/subgid to determine the subordinate UID and GID ranges allocated to the invoking user. The system MUST reject configurations where the allocated range is fewer than 65536 entries.

#### Scenario: Valid subuid/subgid allocation

GIVEN /etc/subuid contains "alice:100000:65536"
AND /etc/subgid contains "alice:100000:65536"
WHEN the user "alice" creates a container
THEN the system MUST use UIDs 100000-165535 for the container's subordinate range
AND container creation MUST succeed

#### Scenario: Insufficient subordinate range

GIVEN /etc/subuid contains "bob:200000:1000"
WHEN the user "bob" attempts to create a container
THEN the system MUST return an error indicating insufficient subordinate UID range
AND the error message MUST specify the minimum required count of 65536
AND the error message MUST suggest how to configure /etc/subuid

#### Scenario: Missing subuid entry

GIVEN /etc/subuid does not contain an entry for user "charlie"
WHEN the user "charlie" attempts to create a container
THEN the system MUST return an error indicating no subordinate UID allocation was found
AND the error message MUST include the username that was looked up

#### Scenario: Overlapping subuid ranges detected

GIVEN /etc/subuid contains overlapping ranges for two different users
WHEN the system validates the subuid allocation
THEN the system SHOULD log a warning about potential cross-user namespace corruption

---

### Requirement: newuidmap/newgidmap Invocation

The system MUST use newuidmap and newgidmap to establish UID and GID mappings within the user namespace. The system MUST verify these binaries are available and have the required SETUID bit before attempting container creation.

#### Scenario: Successful UID/GID mapping

GIVEN newuidmap and newgidmap are installed with SETUID permissions
AND valid subuid/subgid entries exist for the current user
WHEN a container is created
THEN the system MUST invoke newuidmap to configure UID mappings
AND the system MUST invoke newgidmap to configure GID mappings
AND the mappings MUST be established before the container process starts

#### Scenario: Missing newuidmap binary

GIVEN newuidmap is not installed on the system
WHEN a container creation is attempted
THEN the system MUST return an error indicating newuidmap was not found
AND the error MUST suggest an installation method

#### Scenario: newuidmap lacks SETUID bit

GIVEN newuidmap is installed but does not have the SETUID bit set
WHEN a container creation is attempted
THEN the system MUST return an error indicating insufficient permissions on newuidmap
AND the error MUST suggest how to correct the permissions

---

### Requirement: Keep-ID User Namespace Mode

The system MUST support a --userns=keep-id flag that maps the invoking user's UID and GID as themselves inside the container, rather than mapping to root. This is used when file ownership must be preserved between host and container.

#### Scenario: Keep-ID mapping preserves host UID

GIVEN a host user with UID 1000 and GID 1000
WHEN a container is created with --userns=keep-id
THEN the container process MUST run as UID 1000 inside the container
AND files created inside the container MUST be owned by UID 1000 on the host

#### Scenario: Keep-ID with bind mount preserves ownership

GIVEN a host user with UID 1000
AND a bind-mounted directory from the host
WHEN a container is created with --userns=keep-id
THEN the container process MUST be able to read and write files in the bind mount
AND the files MUST retain the host user's ownership

---

## 2. Seccomp

### Requirement: Default Seccomp Profile

The system MUST apply a built-in default seccomp profile to every container unless explicitly overridden. The default profile MUST block approximately 44 dangerous syscalls using an allowlist approach with a default action of SCMP_ACT_ERRNO. The blocked syscalls MUST be consistent with the shared Docker/Podman/CRI-O default profile.

#### Scenario: Default seccomp applied automatically

GIVEN no --security-opt seccomp flag is specified
WHEN a container is created
THEN the OCI runtime configuration MUST include a seccomp section
AND the seccomp default action MUST be SCMP_ACT_ERRNO
AND the profile MUST block dangerous syscalls including but not limited to: kexec_load, mount, umount2, ptrace, reboot, settimeofday, swapon, swapoff

#### Scenario: Container blocked from executing filtered syscall

GIVEN a container is running with the default seccomp profile
WHEN a process inside the container invokes a blocked syscall
THEN the syscall MUST return an error (EPERM)
AND the container MUST continue running

---

### Requirement: Seccomp Disable

The system MUST support disabling seccomp filtering entirely via the --security-opt seccomp=unconfined flag.

#### Scenario: Seccomp disabled with unconfined

GIVEN the flag --security-opt seccomp=unconfined is specified
WHEN a container is created
THEN the OCI runtime configuration MUST NOT include a seccomp section
AND the container process MUST have unrestricted access to all syscalls

---

### Requirement: Custom Seccomp Profile

The system MUST support loading a custom seccomp profile from a file path via --security-opt seccomp=PATH. The system MUST validate that the file exists and contains valid JSON before applying it.

#### Scenario: Custom seccomp profile from file

GIVEN a valid seccomp profile JSON file exists at a specified path
WHEN a container is created with --security-opt seccomp=PATH
THEN the OCI runtime configuration MUST use the custom profile
AND the custom profile's rules MUST be in effect inside the container

#### Scenario: Invalid seccomp profile path

GIVEN the specified seccomp profile path does not exist
WHEN a container is created with --security-opt seccomp=NONEXISTENT_PATH
THEN the system MUST return an error indicating the file was not found
AND container creation MUST NOT proceed

#### Scenario: Malformed seccomp profile

GIVEN a file at the specified path contains invalid JSON
WHEN a container is created with --security-opt seccomp=PATH
THEN the system MUST return an error indicating the profile is malformed
AND container creation MUST NOT proceed

---

## 3. Sandalwood (Capabilities)

### Requirement: Default Minimal Capability Set

The system MUST assign a minimal default set of Linux capabilities to every container. The default set MUST include exactly: CHOWN, DAC_OVERRIDE, FOWNER, FSETID, KILL, NET_BIND_SERVICE, SETGID, SETUID, SETFCAP, SETPCAP, SYS_CHROOT. All other capabilities MUST be dropped.

#### Scenario: Default capabilities applied

GIVEN no --cap-add or --cap-drop flags are specified
WHEN a container is created
THEN the OCI runtime configuration MUST include the effective, permitted, and bounding capability sets
AND each set MUST contain exactly the default minimal capabilities
AND all capabilities not in the default set MUST be absent

#### Scenario: Container cannot perform privileged action

GIVEN a container running with the default capability set
WHEN a process attempts an operation requiring CAP_SYS_ADMIN
THEN the operation MUST fail with a permission error

---

### Requirement: Add Capabilities

The system MUST support the --cap-add flag to grant additional capabilities beyond the default set. Multiple capabilities MAY be added in a single invocation.

#### Scenario: Add single capability

GIVEN the flag --cap-add NET_ADMIN is specified
WHEN a container is created
THEN the OCI runtime configuration MUST include NET_ADMIN in addition to all default capabilities

#### Scenario: Add multiple capabilities

GIVEN the flags --cap-add NET_ADMIN and --cap-add SYS_PTRACE are specified
WHEN a container is created
THEN the OCI runtime configuration MUST include both NET_ADMIN and SYS_PTRACE in addition to all default capabilities

#### Scenario: Add invalid capability name

GIVEN the flag --cap-add NONEXISTENT_CAP is specified
WHEN a container creation is attempted
THEN the system MUST return an error indicating the capability name is not recognized

---

### Requirement: Drop Capabilities

The system MUST support the --cap-drop flag to remove capabilities from the default set. The system MUST support --cap-drop ALL to remove all capabilities.

#### Scenario: Drop single capability

GIVEN the flag --cap-drop CHOWN is specified
WHEN a container is created
THEN the OCI runtime configuration MUST NOT include CHOWN
AND all other default capabilities MUST remain

#### Scenario: Drop all capabilities

GIVEN the flag --cap-drop ALL is specified
WHEN a container is created
THEN the OCI runtime configuration MUST contain an empty capability set
AND the container process MUST have zero capabilities

#### Scenario: Drop all then add specific

GIVEN the flags --cap-drop ALL and --cap-add NET_BIND_SERVICE are specified
WHEN a container is created
THEN the OCI runtime configuration MUST contain only NET_BIND_SERVICE
AND all other capabilities MUST be absent

---

## 4. Gunslinger (AppArmor/SELinux)

### Requirement: LSM Auto-Detection

The system MUST detect which Linux Security Module (AppArmor or SELinux) is available and enforcing on the host. The system MUST apply the appropriate default confinement profile based on the detected LSM.

#### Scenario: AppArmor detected and default profile applied

GIVEN AppArmor is enabled on the host system
AND no --security-opt apparmor flag is specified
WHEN a container is created
THEN the OCI runtime configuration MUST include an AppArmor profile reference
AND the profile MUST be the default maestro confinement profile

#### Scenario: SELinux detected and labels applied

GIVEN SELinux is in enforcing mode on the host system
AND no --security-opt label flag is specified
WHEN a container is created
THEN the OCI runtime configuration MUST include SELinux process labels
AND the container process MUST be labeled with the appropriate container type

#### Scenario: No LSM available

GIVEN neither AppArmor nor SELinux is enabled on the host
WHEN a container is created
THEN the system MUST proceed without LSM confinement
AND the system SHOULD log a warning indicating no LSM is available

---

### Requirement: Custom AppArmor Profile

The system MUST support selecting a specific AppArmor profile via --security-opt apparmor=PROFILE. The system MUST support disabling AppArmor with --security-opt apparmor=unconfined.

#### Scenario: Custom AppArmor profile specified

GIVEN AppArmor is enabled
AND a loaded profile named "my-custom-profile" exists in the kernel
WHEN a container is created with --security-opt apparmor=my-custom-profile
THEN the container MUST run under the "my-custom-profile" AppArmor profile

#### Scenario: Unconfined AppArmor

GIVEN AppArmor is enabled
WHEN a container is created with --security-opt apparmor=unconfined
THEN the container MUST run without any AppArmor confinement

---

### Requirement: SELinux Label Control

The system MUST support disabling SELinux confinement via --security-opt label=disable. The system MUST support setting custom SELinux label components (type, level, user, role).

#### Scenario: Disable SELinux labels

GIVEN SELinux is in enforcing mode
WHEN a container is created with --security-opt label=disable
THEN the container MUST run without SELinux confinement
AND no SELinux labels MUST be applied to the container process

#### Scenario: Custom SELinux type label

GIVEN SELinux is in enforcing mode
WHEN a container is created with --security-opt label=type:my_container_t
THEN the container process MUST be labeled with type my_container_t

---

## 5. no_new_privileges

### Requirement: no_new_privileges Default Enforcement

The system MUST enable the no_new_privileges flag in the OCI runtime configuration for every container by default. This prevents processes from gaining privileges through setuid/setgid binaries or file capabilities.

#### Scenario: no_new_privileges enabled by default

GIVEN no --security-opt no-new-privileges flag is specified
WHEN a container is created
THEN the OCI runtime configuration MUST set noNewPrivileges to true
AND setuid binaries inside the container MUST NOT be able to escalate privileges

#### Scenario: Disable no_new_privileges explicitly

GIVEN the flag --security-opt no-new-privileges=false is specified
WHEN a container is created
THEN the OCI runtime configuration MUST set noNewPrivileges to false
AND setuid binaries inside the container MAY escalate privileges

---

## 6. Masked and Readonly Paths

### Requirement: Default Masked Paths

The system MUST mask sensitive paths inside the container by default to prevent information leakage from the host. The masked paths MUST include at minimum: /proc/asma, /proc/kcore, /proc/keys, /proc/latency_stats, /proc/timer_list, /proc/timer_stats, /proc/sched_debug, /sys/firmware, /proc/scsi.

#### Scenario: Masked paths are inaccessible

GIVEN a container is created with default security settings
WHEN a process inside the container attempts to read /proc/kcore
THEN the read MUST fail or return empty content
AND the container MUST continue running normally

#### Scenario: Masked paths present in runtime config

GIVEN a container is created with default security settings
WHEN the OCI runtime configuration is generated
THEN the linux.maskedPaths array MUST contain the default set of masked paths

---

### Requirement: Default Readonly Paths

The system MUST mount certain paths as read-only inside the container by default. The readonly paths MUST include at minimum: /proc/bus, /proc/fs, /proc/irq, /proc/sys, /proc/sysrq-trigger.

#### Scenario: Readonly paths cannot be written

GIVEN a container is created with default security settings
WHEN a process inside the container attempts to write to /proc/sys
THEN the write MUST fail with a read-only filesystem error
AND the container MUST continue running normally

#### Scenario: Readonly paths present in runtime config

GIVEN a container is created with default security settings
WHEN the OCI runtime configuration is generated
THEN the linux.readonlyPaths array MUST contain the default set of readonly paths

---

## 7. Read-Only Rootfs

### Requirement: Read-Only Root Filesystem

The system MUST support a --read-only flag that mounts the container's root filesystem as read-only. When enabled, the container process MUST NOT be able to write to the root filesystem. This flag MUST be disabled by default.

#### Scenario: Read-only rootfs enabled

GIVEN the flag --read-only is specified
WHEN a container is created
THEN the OCI runtime configuration MUST set root.readonly to true
AND any attempt by the container process to write to the root filesystem MUST fail

#### Scenario: Read-only rootfs with tmpfs exception

GIVEN the flag --read-only is specified
AND the flag --tmpfs /tmp is specified
WHEN a container is created
THEN writes to /tmp MUST succeed (tmpfs is writable)
AND writes to any other path in the root filesystem MUST fail

#### Scenario: Read-only rootfs disabled by default

GIVEN no --read-only flag is specified
WHEN a container is created
THEN the OCI runtime configuration MUST set root.readonly to false
AND the container process MUST be able to write to the root filesystem

---

## 8. Eld Mark (Image Signing and Verification)

### Requirement: Key-Based Image Signing

The system MUST support signing container images using a private key. The signature MUST be stored as an OCI artifact attached to the image via the referrers API.

#### Scenario: Sign image with key file

GIVEN a private key file exists at the specified path
AND a pushed image exists at the target registry
WHEN the user invokes image signing with --key KEY_PATH
THEN the system MUST generate a signature for the image manifest digest
AND the signature MUST be pushed to the same registry as an OCI artifact
AND the artifact MUST reference the image via the subject field

#### Scenario: Sign image with missing key file

GIVEN the specified key file does not exist
WHEN the user invokes image signing with --key NONEXISTENT_PATH
THEN the system MUST return an error indicating the key file was not found
AND no signature MUST be created

---

### Requirement: Keyless Image Signing

The system MUST support keyless signing using an OIDC identity provider via Fulcio (certificate authority) and Rekor (transparency log). This mode is designed for CI/CD environments where managing long-lived keys is impractical.

#### Scenario: Keyless signing in CI environment

GIVEN an OIDC token is available in the environment
AND Fulcio and Rekor endpoints are reachable
WHEN the user invokes image signing with --keyless
THEN the system MUST obtain a short-lived certificate from Fulcio
AND the system MUST record the signing event in the Rekor transparency log
AND the signature MUST be pushed to the registry as an OCI artifact

#### Scenario: Keyless signing without OIDC token

GIVEN no OIDC token is available in the environment
AND the session is non-interactive
WHEN the user invokes image signing with --keyless
THEN the system MUST return an error indicating no OIDC identity was available

---

### Requirement: Image Signature Verification

The system MUST support verifying image signatures. Verification MUST confirm that the signature is valid and was produced by a trusted key or identity.

#### Scenario: Verify signed image with public key

GIVEN an image has been signed with a known private key
AND the corresponding public key is available
WHEN the user invokes image verification with the public key
THEN the verification MUST succeed
AND the system MUST report the verification result

#### Scenario: Verify unsigned image

GIVEN an image has no signatures attached
WHEN the user invokes image verification
THEN the system MUST return an error indicating no signature was found

#### Scenario: Verify image with wrong key

GIVEN an image has been signed with key A
WHEN the user invokes image verification with key B (which did not produce the signature)
THEN the verification MUST fail
AND the error MUST indicate the signature does not match the provided key

---

### Requirement: Signature Verification on Run

The system MUST support a --verify-signature flag on the run command that rejects images without a valid signature before starting a container.

#### Scenario: Run verified signed image

GIVEN an image has a valid signature from a trusted key
WHEN the user runs the image with --verify-signature
THEN the container MUST be created and started normally

#### Scenario: Run unsigned image with verification required

GIVEN an image has no signature
WHEN the user runs the image with --verify-signature
THEN the system MUST reject the image
AND return an error indicating the image is not signed
AND no container MUST be created

---

### Requirement: Policy-Based Signature Enforcement

The system MUST support a configuration-based policy in katet.toml that requires signatures for all image operations. The policy MUST support specifying a list of trusted keys or identity sources.

#### Scenario: Policy requires signatures globally

GIVEN the configuration contains security.policy.require_signature = true
AND security.policy.trusted_keys lists one or more public key paths or Fulcio identity URIs
WHEN the user pulls or runs any image
THEN the system MUST verify the image signature against the trusted keys before proceeding
AND unsigned images MUST be rejected

#### Scenario: Policy with trusted keys verification

GIVEN the configuration contains security.policy.trusted_keys = ["cosign.pub"]
AND an image is signed with the corresponding private key
WHEN the user pulls the image
THEN the verification MUST succeed using the listed trusted key

#### Scenario: Policy disabled by default

GIVEN the configuration does not contain security.policy.require_signature
OR security.policy.require_signature is set to false
WHEN the user pulls or runs an image
THEN no automatic signature verification MUST be performed
AND the user MAY still use --verify-signature explicitly

---

## 9. System Check Diagnostics

### Requirement: Comprehensive System Diagnostics

The system MUST provide a diagnostic command that validates all prerequisites for rootless container operation. The diagnostics MUST check: subuid/subgid allocation, kernel version and features, available OCI runtimes, storage driver compatibility, rootless network tools availability, and LSM status.

#### Scenario: All checks pass on a healthy system

GIVEN a properly configured system with valid subuid/subgid entries, a compatible kernel, at least one OCI runtime, a compatible storage driver, and rootless network tools installed
WHEN the user runs system diagnostics
THEN the output MUST display a success indicator for each validated requirement
AND the overall result MUST indicate the system is ready for rootless operation

#### Scenario: Missing subuid/subgid reports failure

GIVEN /etc/subuid does not contain an entry for the current user
WHEN the user runs system diagnostics
THEN the output MUST display a failure indicator for subuid/subgid validation
AND the output MUST include a suggestion for how to fix the issue

#### Scenario: Missing OCI runtime reports failure

GIVEN no OCI runtime (runc, crun, or youki) is found in the system PATH or configured paths
WHEN the user runs system diagnostics
THEN the output MUST display a failure indicator for runtime availability
AND the output MUST suggest how to install a runtime

#### Scenario: Missing rootless network tools reports failure

GIVEN neither pasta nor slirp4netns is installed
WHEN the user runs system diagnostics
THEN the output MUST display a failure indicator for rootless networking
AND the output MUST suggest how to install the required tool

#### Scenario: Storage driver compatibility warning

GIVEN the kernel version is below 5.13
AND fuse-overlayfs is not installed
WHEN the user runs system diagnostics
THEN the output MUST display a warning for storage driver compatibility
AND the output MUST indicate that VFS fallback will be used
AND the output SHOULD suggest installing fuse-overlayfs for better performance

---

## 10. Security Configuration Integration

### Requirement: Combined Security Defaults in OCI Config

The system MUST integrate all security settings (seccomp, capabilities, no_new_privileges, masked paths, readonly paths, LSM profiles) into the generated OCI runtime configuration for every container. The combined default security posture MUST be equivalent to or stricter than established container runtimes.

#### Scenario: Full security defaults applied to config.json

GIVEN a container is created with no security-related flags
WHEN the OCI runtime configuration is generated
THEN the configuration MUST include a seccomp profile with the default filter
AND the configuration MUST include the minimal default capability set
AND the configuration MUST set noNewPrivileges to true
AND the configuration MUST include the default masked paths
AND the configuration MUST include the default readonly paths
AND the configuration MUST include an LSM profile if one is available on the host

#### Scenario: Multiple security overrides combine correctly

GIVEN the flags --cap-drop ALL --cap-add NET_BIND_SERVICE --security-opt seccomp=unconfined --security-opt no-new-privileges=false are specified
WHEN the OCI runtime configuration is generated
THEN the capabilities MUST contain only NET_BIND_SERVICE
AND no seccomp profile MUST be present
AND noNewPrivileges MUST be false
AND masked paths and readonly paths MUST still be applied (they are independent of the overridden settings)
