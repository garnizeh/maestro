# Tet Compose Specification

## Purpose

Tet manages **multi-container applications** defined in a declarative `compose.yaml` file. It reads service definitions, creates containers via Gan, provisions networks via Beam, and mounts volumes via Dogan — orchestrating an entire stack from a single file with a single command. Tet introduces no new container runtime; it adds coordination on top of every existing Maestro component.

> *"The Tet Corporation was a business, but it was also a cause — a group of people, bound by a common purpose, working in concert across different worlds to protect the Rose and ensure the Tower would stand."*

---

## 1. File Discovery and Loading

### Requirement: Compose File Lookup

The system MUST search for a compose file in the current working directory using the following priority order when no explicit file is given:

1. `compose.yaml`
2. `compose.yml`
3. `docker-compose.yaml`
4. `docker-compose.yml`

If none of these files exists, the system MUST return an error indicating that no compose file was found and listing the searched paths.

#### Scenario: Default file found in working directory

GIVEN a file named `compose.yaml` exists in the current directory
WHEN the user invokes `maestro compose up`
THEN the system MUST load `compose.yaml` without requiring an explicit `-f` flag

#### Scenario: Fallback to docker-compose.yml for compatibility

GIVEN no `compose.yaml` or `compose.yml` exists in the current directory
AND a file named `docker-compose.yml` exists
WHEN the user invokes any `maestro compose` subcommand
THEN the system MUST load `docker-compose.yml`
AND a deprecation notice SHOULD be displayed on standard error

#### Scenario: No compose file found

GIVEN no supported compose file exists in the current directory
WHEN the user invokes `maestro compose up`
THEN the system MUST return a non-zero exit code
AND the error message MUST list every searched filename and directory

#### Scenario: Help text available without compose file

GIVEN no compose file exists in the current directory
WHEN the user invokes `maestro compose --help`
THEN help text MUST be displayed regardless of missing compose file

---

### Requirement: Explicit File with -f Flag

The system MUST accept one or more `-f` / `--file` flags to specify compose file paths explicitly. When multiple `-f` flags are given, the system MUST merge the files in order, with later files overriding earlier ones. At least one file MUST be a valid compose document.

#### Scenario: Single explicit file

GIVEN a file at `/home/user/projects/myapp/app.yaml`
WHEN the user invokes `maestro compose -f /home/user/projects/myapp/app.yaml up`
THEN the system MUST load the specified file
AND ignore any compose files in the current directory

#### Scenario: Multiple files merged in order

GIVEN `base.yaml` defines service `web` with image `nginx:latest`
AND `override.yaml` redefines service `web` image as `nginx:alpine`
WHEN the user invokes `maestro compose -f base.yaml -f override.yaml up`
THEN the `web` service MUST use the image `nginx:alpine`
AND all other fields from `base.yaml` not overridden MUST be preserved

#### Scenario: Merge adds new services

GIVEN `base.yaml` defines service `web`
AND `extra.yaml` defines service `worker`
WHEN the user invokes `maestro compose -f base.yaml -f extra.yaml ps`
THEN both `web` and `worker` MUST be listed as known services

#### Scenario: Invalid file path

GIVEN the user invokes `maestro compose -f /nonexistent/path.yaml up`
WHEN the command executes
THEN the system MUST return a non-zero exit code
AND the error MUST include the file path that was not found

---

### Requirement: Compose Override File

When a `compose.override.yaml` or `compose.override.yml` file exists in the same directory as the primary compose file, the system MUST automatically merge it on top of the primary file as if it were appended with an additional `-f` flag, without requiring the user to specify it.

#### Scenario: Override file auto-applied

GIVEN `compose.yaml` defines service `api` with a single replica
AND `compose.override.yaml` adds environment variable `DEBUG=true` to service `api`
WHEN the user invokes `maestro compose up` with no explicit `-f` flags
THEN the `api` service containers MUST have `DEBUG=true` in their environment
AND the override is applied automatically

---

## 2. Project Identity and Isolation

### Requirement: Project Name

Every compose stack MUST be associated with a project name. The project name is determined in the following priority order:

1. `--project-name` / `-p` flag on the CLI
2. `name` field at the top level of the compose file
3. `COMPOSE_PROJECT_NAME` environment variable
4. The basename of the directory containing the compose file (with non-alphanumeric characters replaced by hyphens, lowercased)

The project name MUST be used to prefix or label all resources (containers, networks, volumes) created by Tet to ensure complete isolation between projects.

#### Scenario: Project name from compose file top-level field

GIVEN a `compose.yaml` containing `name: myapp` at the top level
WHEN the user invokes `maestro compose up`
THEN all created containers MUST be labeled with `tet.project=myapp`

#### Scenario: Project name from directory

GIVEN a `compose.yaml` that does not define a `name` field
AND the compose file resides in directory `/home/user/my-project`
WHEN the user invokes `maestro compose up`
THEN the project name MUST be `my-project`
AND all created containers MUST be labeled with `tet.project=my-project`

#### Scenario: CLI flag overrides file

GIVEN a `compose.yaml` containing `name: base`
WHEN the user invokes `maestro compose -p override up`
THEN all created resources MUST be labeled with `tet.project=override`

#### Scenario: Project name normalization

GIVEN the compose directory is named `My_App 2`
WHEN the project name is derived from the directory name
THEN the project name MUST be `my-app-2` (lowercased, non-alphanumeric replaced by hyphens)

---

### Requirement: Resource Labeling

All containers, networks, and volumes created by Tet MUST carry the following labels to enable full project lifecycle management. The system MUST NOT create a resource that cannot be associated back to its project.

- `tet.project=<name>` — the project this resource belongs to
- `tet.service=<name>` — the service this resource implements (containers only)
- `tet.version=<compose-spec-version>` — the compose spec version used

#### Scenario: Container labeled with project and service

GIVEN a compose file with project name `app` defining service `web`
WHEN `maestro compose up` creates the web container
THEN the container MUST have label `tet.project=app`
AND the container MUST have label `tet.service=web`

#### Scenario: Network labeled with project

GIVEN a compose file with project name `app`
WHEN `maestro compose up` creates the default project network
THEN the network MUST have label `tet.project=app`

#### Scenario: Volume labeled with project

GIVEN a compose file with project name `app` defining a named volume `data`
WHEN `maestro compose up` creates the volume
THEN the volume MUST have label `tet.project=app`

---

## 3. Variable Substitution

### Requirement: Environment Variable Interpolation

The system MUST support variable interpolation in compose file values using `${VARIABLE}` and `$VARIABLE` syntax. Variables MUST be resolved from the following sources in priority order:

1. Shell environment variables at the time `maestro compose` is invoked
2. Variables defined in the `.env` file co-located with the compose file

The system MUST support the following interpolation forms:

- `${VARIABLE}` — substitute value; error if not set
- `${VARIABLE:-default}` — substitute value, or `default` if unset or empty
- `${VARIABLE-default}` — substitute value, or `default` if unset (not if empty)
- `${VARIABLE:?error message}` — error with message if unset or empty
- `${VARIABLE?error message}` — error with message if unset

#### Scenario: Variable from shell environment

GIVEN the shell has `TAG=1.2.3` set
AND the compose file contains `image: myapp:${TAG}`
WHEN `maestro compose up` executes
THEN the container MUST be created from image `myapp:1.2.3`

#### Scenario: Variable from .env file

GIVEN a `.env` file in the compose directory containing `TAG=alpine`
AND no `TAG` variable in the shell environment
AND the compose file contains `image: nginx:${TAG}`
WHEN `maestro compose up` executes
THEN the container MUST be created from image `nginx:alpine`

#### Scenario: Shell environment overrides .env file

GIVEN a `.env` file containing `PORT=8080`
AND the shell environment has `PORT=9090`
AND the compose file contains `ports: - "${PORT}:80"`
WHEN `maestro compose up` executes
THEN the port mapping MUST use `9090:80`

#### Scenario: Default value applied when variable unset

GIVEN no `TAG` variable defined anywhere
AND the compose file contains `image: nginx:${TAG:-latest}`
WHEN `maestro compose up` executes
THEN the container MUST use image `nginx:latest`

#### Scenario: Required variable missing

GIVEN no `DB_PASSWORD` variable defined anywhere
AND the compose file contains an entry using `${DB_PASSWORD:?DB_PASSWORD is required}`
WHEN `maestro compose up` executes
THEN the system MUST return a non-zero exit code
AND the error MUST include the message `DB_PASSWORD is required`

---

### Requirement: .env File Loading

The system MUST automatically load a `.env` file from the same directory as the compose file. The `.env` file MUST follow the format:

- One `KEY=VALUE` pair per line
- Lines starting with `#` are comments
- Blank lines are ignored
- Values MAY be quoted with single or double quotes

An explicit `--env-file` flag MUST override the default `.env` file location. If the specified file does not exist, the system MUST return an error.

#### Scenario: .env file loaded automatically

GIVEN a `.env` file containing `APP_ENV=production`
AND the compose file contains a service with `environment: - APP_ENV`
WHEN `maestro compose up` executes
THEN the service container MUST have `APP_ENV=production` in its environment

#### Scenario: .env file with comments and blank lines

GIVEN a `.env` file with comment lines (`# comment`) and blank lines
WHEN the `.env` file is parsed
THEN comment lines and blank lines MUST be silently ignored

#### Scenario: Explicit env-file override

GIVEN a file `/config/prod.env` containing `LOG_LEVEL=info`
WHEN the user invokes `maestro compose --env-file /config/prod.env up`
THEN variables from `/config/prod.env` MUST be used
AND the default `.env` file MUST NOT be loaded

#### Scenario: Missing explicit env-file

GIVEN the user invokes `maestro compose --env-file /nonexistent.env up`
WHEN the command executes
THEN the system MUST return a non-zero exit code with an error indicating the file was not found

---

## 4. Service Configuration

### Requirement: Image Reference

Each service MUST specify either an `image` field or a `build` field. If only a `build` field is present and no built image exists locally, the system MUST return an error indicating that `maestro compose build` must be run first. If both `image` and `build` are specified, `image` defines the tag to apply to the built image.

#### Scenario: Service with image pulls if missing

GIVEN a service defines `image: nginx:latest`
AND `nginx:latest` is not present in the local Maturin store
WHEN `maestro compose up` is invoked
THEN the system MUST automatically pull `nginx:latest` before creating the container
AND progress MUST be displayed consistent with `maestro pull` behavior

#### Scenario: Service with image uses local image

GIVEN a service defines `image: myapp:v1`
AND `myapp:v1` exists in the local Maturin store
WHEN `maestro compose up` is invoked
THEN the system MUST use the local image without making any network request

#### Scenario: Service with build only and no local image errors

GIVEN a service defines only `build: ./app` with no `image` field
AND no local image matching the default tag exists
WHEN `maestro compose up` is invoked without `--build`
THEN the system MUST return a non-zero exit code
AND the error MUST instruct the user to run `maestro compose build` first

---

### Requirement: Port Mapping

Services MAY define port mappings using the `ports` list. Each port mapping MUST be accepted in the following formats (following the Beam port spec):

- `"<host-port>:<container-port>"`
- `"<host-ip>:<host-port>:<container-port>"`
- `"<container-port>"` — assigns a random host port
- `"<host-port>:<container-port>/<protocol>"` where protocol is `tcp` or `udp`

#### Scenario: Simple host-to-container port mapping

GIVEN a service defines `ports: ["8080:80"]`
WHEN the container is created by `maestro compose up`
THEN host port 8080 MUST be mapped to container port 80

#### Scenario: Random host port assignment

GIVEN a service defines `ports: ["80"]`
WHEN the container is created by `maestro compose up`
THEN a random available host port MUST be assigned for container port 80
AND `maestro compose port <service> 80` MUST return the assigned host port

#### Scenario: Multiple port mappings

GIVEN a service defines `ports: ["8080:80", "8443:443"]`
WHEN the container is created
THEN both port mappings MUST be active

---

### Requirement: Volume Mounts

Services MAY define volumes in the `volumes` list. Each entry MAY reference a top-level named volume, a bind-mount path, or a `tmpfs` definition.

#### Scenario: Named volume mount

GIVEN a top-level `volumes:` section defines `pgdata:`
AND a service mounts it as `- pgdata:/var/lib/postgresql/data`
WHEN `maestro compose up` runs
THEN the named Dogan volume `pgdata` MUST be created (if not already existing)
AND mounted at `/var/lib/postgresql/data` in the container

#### Scenario: Bind mount

GIVEN a service defines `volumes: - ./html:/usr/share/nginx/html:ro`
WHEN the container is created
THEN the host directory `./html` (resolved relative to the compose file location) MUST be bind-mounted at `/usr/share/nginx/html` as read-only

#### Scenario: Tmpfs mount via volumes

GIVEN a service defines `volumes: - type: tmpfs\n  target: /tmp\n  tmpfs:\n    size: 64m`
WHEN the container is created
THEN a tmpfs filesystem of 64 MiB MUST be mounted at `/tmp`

---

### Requirement: Environment Variables

Services MAY define environment variables via `environment` (map or list) and/or `env_file` (one or more file paths). When both are present, `environment` values take precedence over `env_file` values.

#### Scenario: Environment map syntax

GIVEN a service defines:

```
environment:
  APP_ENV: production
  LOG_LEVEL: warn
```

WHEN the container is created
THEN both `APP_ENV=production` and `LOG_LEVEL=warn` MUST be present in the container environment

#### Scenario: Environment list syntax

GIVEN a service defines:

```
environment:
  - APP_ENV=production
  - DB_HOST
```

AND the shell has `DB_HOST=db.internal`
WHEN the container is created
THEN `APP_ENV=production` and `DB_HOST=db.internal` MUST be present in the container environment

#### Scenario: env_file loaded into service

GIVEN a service defines `env_file: ./config/app.env`
AND `./config/app.env` contains `WORKERS=4`
WHEN the container is created
THEN `WORKERS=4` MUST be present in the container environment

#### Scenario: environment overrides env_file

GIVEN `env_file` sets `LOG_LEVEL=debug`
AND `environment` sets `LOG_LEVEL=warn`
WHEN the container is created
THEN the container MUST have `LOG_LEVEL=warn`

---

### Requirement: Resource Limits

Services MAY define resource constraints under `deploy.resources`. The system MUST translate these into Gan container resource limits.

```
deploy:
  resources:
    limits:
      cpus: "0.5"
      memory: 512m
    reservations:
      memory: 256m
```

#### Scenario: CPU limit applied

GIVEN a service defines `deploy.resources.limits.cpus: "0.5"`
WHEN the container is created
THEN the container MUST have a CPU quota equivalent to 50% of one CPU

#### Scenario: Memory limit applied

GIVEN a service defines `deploy.resources.limits.memory: 512m`
WHEN the container is created
THEN the container MUST be limited to 512 MiB of memory
AND exceeding the limit MUST trigger an OOM kill

---

### Requirement: Restart Policy

Services MAY define a `restart` policy. The policy MUST map to Gan's restart policy as follows:

| Compose value | Gan restart policy |
|---|---|
| `no` | `no` |
| `always` | `always` |
| `on-failure` | `on-failure` |
| `unless-stopped` | `unless-stopped` |

The default restart policy when not specified MUST be `no`.

#### Scenario: Restart always policy applied

GIVEN a service defines `restart: always`
WHEN the container exits for any reason
THEN Gan MUST restart the container automatically

#### Scenario: Default no-restart when omitted

GIVEN a service does not define a `restart` field
WHEN the container exits
THEN the system MUST NOT automatically restart it

---

### Requirement: Health Check

Services MAY define a `healthcheck` block. The system MUST pass the health check configuration to Gan when creating the container.

```
healthcheck:
  test: ["CMD", "curl", "-f", "http://localhost/health"]
  interval: 30s
  timeout: 10s
  retries: 3
  start_period: 10s
```

The `test` field MUST accept `["CMD", ...]`, `["CMD-SHELL", "shell command"]`, and `["NONE"]` (to disable an inherited health check).

#### Scenario: Health check configured on container

GIVEN a service defines a healthcheck
WHEN `maestro compose up` creates the container
THEN the container MUST be created with the health check specification passed to Gan
AND the container's health status MUST be reflected in `maestro compose ps`

#### Scenario: NONE disables inherited health check

GIVEN a service defines `healthcheck.test: ["NONE"]`
WHEN the container is created from an image that defines a health check
THEN the inherited health check MUST be disabled

---

### Requirement: Profiles

Services MAY declare one or more profiles. A service with profiles defined MUST only be started when one of its profiles is explicitly activated via `--profile` CLI flag or `COMPOSE_PROFILES` environment variable. Services with no `profiles` field MUST always start.

#### Scenario: Service without profile always starts

GIVEN a service has no `profiles` field
WHEN `maestro compose up` is invoked without any `--profile` flag
THEN the service MUST start

#### Scenario: Profiled service does not start by default

GIVEN a service defines `profiles: [debug]`
WHEN `maestro compose up` is invoked without `--profile debug`
THEN the profiled service MUST NOT start

#### Scenario: Profiled service starts when profile activated

GIVEN a service defines `profiles: [debug]`
WHEN the user invokes `maestro compose --profile debug up`
THEN the profiled service MUST start along with all non-profiled services

#### Scenario: Multiple profiles

GIVEN a service defines `profiles: [debug, testing]`
WHEN the user invokes `maestro compose --profile testing up`
THEN the service MUST start
WHEN the user invokes `maestro compose up` with no profile
THEN the service MUST NOT start

---

### Requirement: Service Replicas

Services MAY define `deploy.replicas` to run multiple identical container instances. The default is `1`. Each replica MUST be a distinct Gan container with a unique name derived from the pattern `<project>-<service>-<index>` (1-based).

#### Scenario: Multiple replicas created

GIVEN a service defines `deploy.replicas: 3`
WHEN `maestro compose up` executes
THEN exactly 3 containers MUST be created with names `<project>-<service>-1`, `<project>-<service>-2`, `<project>-<service>-3`

#### Scenario: Default single replica

GIVEN a service does not define `deploy.replicas`
WHEN `maestro compose up` executes
THEN exactly 1 container MUST be created with name `<project>-<service>-1`

---

### Requirement: Additional Service Fields

Services MUST support the following fields, which MUST map to the corresponding Gan/Specgen parameters:

- `command` — override image CMD
- `entrypoint` — override image ENTRYPOINT
- `user` — run container as this user (maps to `--user`)
- `working_dir` — override image WORKDIR
- `hostname` — set container hostname
- `domainname` — set container domainname
- `labels` — add labels to the container (merged with Tet labels)
- `read_only` — mount rootfs as read-only
- `init` — inject init process as PID 1
- `privileged` — run in privileged mode (SHOULD emit security warning)
- `cap_add` / `cap_drop` — Linux capability management
- `security_opt` — pass security options (e.g., `seccomp=unconfined`)
- `sysctls` — set kernel parameters
- `ulimits` — set resource limits
- `extra_hosts` — add entries to `/etc/hosts`
- `dns` / `dns_search` / `dns_opt` — DNS configuration
- `stop_signal` — signal sent when stopping the container (default `SIGTERM`)
- `stop_grace_period` — time to wait before sending SIGKILL (default 10s)
- `tty` — allocate a pseudo-TTY
- `stdin_open` — keep stdin open

#### Scenario: stop_grace_period applied on compose stop

GIVEN a service defines `stop_grace_period: 30s`
WHEN `maestro compose stop` is invoked
THEN the system MUST wait up to 30 seconds for graceful termination before sending SIGKILL

#### Scenario: extra_hosts added to /etc/hosts

GIVEN a service defines `extra_hosts: ["db.local:192.168.1.10"]`
WHEN the container is created
THEN `/etc/hosts` inside the container MUST contain an entry for `db.local` pointing to `192.168.1.10`

---

## 5. Network Configuration

### Requirement: Default Project Network

When `maestro compose up` runs and no network is explicitly attached to a service, the system MUST create a default network named `<project>_default`. All services not specifying a `networks` key MUST be automatically attached to this default network. Services on the same default network MUST be reachable by service name via the Callahan DNS resolver.

#### Scenario: Default network created automatically

GIVEN a compose file with no top-level `networks` section and no service `networks` keys
WHEN `maestro compose up` executes
THEN a Beam network named `<project>_default` MUST be created
AND all service containers MUST be attached to it

#### Scenario: Services on default network reach each other by name

GIVEN two services `web` and `api` are both on the default network
WHEN a process in `web` resolves the hostname `api`
THEN Callahan MUST return the IP address of the `api` container

#### Scenario: Default network removed on compose down

GIVEN `maestro compose up` has created a `<project>_default` network
WHEN `maestro compose down` executes
THEN the `<project>_default` network MUST be removed

---

### Requirement: Custom Networks

The top-level `networks` section MAY define named project networks. Each service MAY attach to one or more named networks. Services on different non-shared networks MUST NOT reach each other by name.

#### Scenario: Custom network created

GIVEN a compose file defines a network `backend:`
WHEN `maestro compose up` executes
THEN a Beam network named `<project>_backend` MUST be created

#### Scenario: Service with multiple networks

GIVEN service `api` declares networks `[frontend, backend]`
WHEN `maestro compose up` creates the `api` container
THEN the container MUST be attached to both `<project>_frontend` and `<project>_backend`

#### Scenario: Network isolation between projects

GIVEN project `app1` and project `app2` both have a service named `web`
WHEN both projects are running
THEN the `web` container of `app1` MUST NOT be reachable by name from `app2`

#### Scenario: Custom network driver

GIVEN the top-level network defines `driver: bridge` with a custom subnet
WHEN `maestro compose up` creates the network
THEN Beam MUST create the network using the specified driver and subnet

---

### Requirement: External Networks

A network MAY be declared as `external: true`. External networks MUST already exist when `maestro compose up` is invoked; the system MUST NOT create or remove them. If the external network does not exist, `maestro compose up` MUST return an error.

#### Scenario: External network used but not created

GIVEN a network is declared as `external: true` and the named Beam network exists
WHEN `maestro compose up` executes
THEN the service containers MUST be attached to the existing network
AND the network MUST NOT be created

#### Scenario: External network missing

GIVEN a network is declared as `external: true` but the named Beam network does not exist
WHEN `maestro compose up` executes
THEN the system MUST return a non-zero exit code
AND the error MUST identify the missing external network by name

#### Scenario: External network not removed on compose down

GIVEN a compose project uses an external network
WHEN `maestro compose down` executes
THEN the external network MUST remain intact

---

## 6. Volume Configuration

### Requirement: Named Volumes

Named volumes defined in the top-level `volumes` section MUST be created by Dogan (Prim) if they do not already exist. Named volumes MUST persist between `compose down` and `compose up` cycles unless explicitly removed with `--volumes`.

#### Scenario: Named volume created on first up

GIVEN the top-level `volumes` section defines `pgdata:` and the volume does not exist
WHEN `maestro compose up` executes
THEN Dogan MUST create the volume `<project>_pgdata`

#### Scenario: Named volume reused on subsequent up

GIVEN a named volume `<project>_pgdata` exists from a previous run
WHEN `maestro compose up` executes again
THEN the existing volume MUST be reused without re-creation
AND any data in the volume MUST be preserved

#### Scenario: Named volume not removed by default on down

GIVEN a compose project is running with a named volume `<project>_data`
WHEN `maestro compose down` executes without `--volumes`
THEN the volume `<project>_data` MUST remain

#### Scenario: Named volume removed with --volumes

GIVEN a compose project is running with named volume `<project>_data`
WHEN `maestro compose down --volumes` executes
THEN the volume `<project>_data` MUST be removed

---

### Requirement: External Volumes

A volume MAY be declared as `external: true`. External volumes MUST already exist when `maestro compose up` is invoked; the system MUST NOT create or remove them. If the external volume does not exist, `maestro compose up` MUST return an error.

#### Scenario: External volume used without creation

GIVEN a volume is declared as `external: true` and the named Dogan volume exists
WHEN `maestro compose up` executes
THEN the volume MUST be mounted in the service container as-is

#### Scenario: External volume missing

GIVEN a volume is declared as `external: true` but does not exist
WHEN `maestro compose up` executes
THEN the system MUST return a non-zero exit code
AND the error MUST identify the missing volume name

---

## 7. Dependency Management

### Requirement: Service Start Ordering via depends_on

Services MAY declare dependencies on other services via `depends_on`. The system MUST start services in an order that satisfies all declared dependencies. Circular dependencies MUST be detected before any container is created, and the system MUST return an error listing the cycle.

The `depends_on` syntax MAY use the short form (list of service names) or the long form with a `condition` field:

- `condition: service_started` — wait until the dependency container is in the Running state (default)
- `condition: service_healthy` — wait until the dependency container is Healthy (requires `healthcheck`)
- `condition: service_completed_successfully` — wait until the dependency container has exited with code 0

#### Scenario: Service started after dependency

GIVEN service `api` declares `depends_on: [db]`
WHEN `maestro compose up` executes
THEN `db` MUST transition to Running before `api` container creation begins

#### Scenario: Health-based dependency wait

GIVEN service `api` declares `depends_on: {db: {condition: service_healthy}}`
AND service `db` has a healthcheck
WHEN `maestro compose up` executes
THEN `api` MUST NOT start until `db` reports a Healthy status
AND the system MUST poll `db` health status at the interval defined in its healthcheck

#### Scenario: Health-based dependency times out

GIVEN service `db` never becomes Healthy within the configured timeout
WHEN `maestro compose up` is waiting for `service_healthy`
THEN the system MUST return a non-zero exit code
AND the error MUST identify which service failed to become healthy

#### Scenario: Completed dependency

GIVEN service `migrate` declares no depends_on and is depended on by `api` with `condition: service_completed_successfully`
WHEN `maestro compose up` executes
THEN `migrate` MUST run to completion with exit code 0 before `api` starts
AND if `migrate` exits with a non-zero code, `api` MUST NOT start

#### Scenario: Circular dependency detected

GIVEN service `a` depends on `b` and service `b` depends on `a`
WHEN `maestro compose up` is invoked
THEN the system MUST return a non-zero exit code before creating any container
AND the error MUST identify the cycle: `a -> b -> a`

---

## 8. Up Operation

### Requirement: Create and Start All Services

The `maestro compose up` command MUST create and start all defined services (respecting active profiles and dependency order). When a service has multiple replicas, all replicas MUST be created. By default, output from all services MUST be aggregated and streamed to the terminal (multiplexed by service name prefix). The `-d` / `--detach` flag MUST suppress output and run all services in the background.

#### Scenario: All services started in dependency order

GIVEN a compose file defining services `db`, `api` (depends on db), and `web` (depends on api)
WHEN `maestro compose up` executes
THEN services MUST start in order: `db` → `api` → `web`
AND all containers MUST be Running when the command completes (or exits with non-zero on failure)

#### Scenario: Detached mode returns immediately

GIVEN `maestro compose up -d` is invoked
WHEN all containers have been created and started
THEN the command MUST return exit code 0 without waiting for container output
AND all containers MUST be running in the background

#### Scenario: Incremental update for unchanged services

GIVEN `maestro compose up` was previously run and all services are running
WHEN `maestro compose up` is invoked again with an unchanged compose file
THEN services whose configuration has not changed MUST NOT be recreated
AND a message MUST indicate which services are already up-to-date

#### Scenario: Recreate on configuration change

GIVEN service `web` is running with image `nginx:latest`
AND the compose file is updated to use `nginx:alpine`
WHEN `maestro compose up` is invoked
THEN the existing `web` container MUST be stopped and removed
AND a new `web` container MUST be created from `nginx:alpine`

#### Scenario: --no-deps flag skips dependency start

GIVEN service `api` depends on `db`
WHEN the user invokes `maestro compose up --no-deps api`
THEN only the `api` service MUST be started
AND `db` MUST NOT be started even if it is not running

#### Scenario: --force-recreate always recreates

GIVEN all services are running without configuration changes
WHEN the user invokes `maestro compose up --force-recreate`
THEN all containers MUST be stopped, removed, and recreated

#### Scenario: --pull before up

GIVEN a service defines `image: myapp:latest`
WHEN the user invokes `maestro compose up --pull always`
THEN `myapp:latest` MUST be pulled before the container is started regardless of local availability

#### Scenario: Up targets specific services

GIVEN a compose file with services `web`, `api`, and `db`
WHEN the user invokes `maestro compose up web api`
THEN only `web` and `api` (and their transitive dependencies) MUST be started

---

## 9. Down Operation

### Requirement: Stop and Remove Services

The `maestro compose down` command MUST stop all running service containers of the project, remove them, and remove the project-owned networks. By default, named volumes MUST be preserved. The `--volumes` flag MUST also remove named volumes. The `--remove-orphans` flag MUST remove containers that are no longer defined in the compose file.

#### Scenario: All services stopped and removed

GIVEN a compose project with running services `web`, `api`, and `db`
WHEN `maestro compose down` executes
THEN all three containers MUST be stopped and removed from Waystation
AND the project network MUST be removed

#### Scenario: Stop grace period respected

GIVEN a compose project in which service `api` has `stop_grace_period: 20s`
WHEN `maestro compose down` executes
THEN the `api` container MUST receive SIGTERM and be given 20 seconds before SIGKILL

#### Scenario: Volumes preserved by default

GIVEN a compose project with a named volume `data`
WHEN `maestro compose down` executes without `--volumes`
THEN the volume `data` MUST NOT be removed

#### Scenario: Volumes removed with --volumes

GIVEN a compose project with a named volume `data`
WHEN `maestro compose down --volumes` executes
THEN the `data` volume MUST be removed

#### Scenario: Orphan containers removed

GIVEN a compose project previously defined service `legacy` which created a container
AND the `legacy` service has since been removed from the compose file
WHEN `maestro compose down --remove-orphans` executes
THEN the `legacy` container MUST be removed

#### Scenario: External resources not removed

GIVEN the compose project uses an external network and an external volume
WHEN `maestro compose down --volumes` executes
THEN the external network and volume MUST remain intact

---

## 10. Status (ps)

### Requirement: Project Container Listing

The `maestro compose ps` command MUST list all containers belonging to the current compose project, showing their service name, container name, state, and port mappings. By default, only running containers MUST be shown. The `--all` / `-a` flag MUST show containers in all states.

#### Scenario: Running containers listed

GIVEN a compose project with running services `web` and `api`
WHEN the user invokes `maestro compose ps`
THEN both containers MUST appear in the output
AND the output MUST include: SERVICE, CONTAINER NAME, STATE, PORTS columns

#### Scenario: Stopped containers hidden by default

GIVEN a stopped container from a compose project
WHEN the user invokes `maestro compose ps` without `--all`
THEN the stopped container MUST NOT appear in the output

#### Scenario: --all shows all states

GIVEN containers in states Running, Stopped, and Created
WHEN the user invokes `maestro compose ps --all`
THEN all containers from the project MUST appear regardless of state

#### Scenario: --format json output

GIVEN a running compose project
WHEN the user invokes `maestro compose ps --format json`
THEN the output MUST be a valid JSON array with the same data

#### Scenario: --quiet shows only container names

GIVEN a running compose project with containers `myapp-web-1` and `myapp-api-1`
WHEN the user invokes `maestro compose ps -q`
THEN only the container names MUST be printed, one per line

---

## 11. Logs

### Requirement: Aggregate Log Access

The `maestro compose logs` command MUST aggregate and display log output from all service containers of the project. Each log line MUST be prefixed with the service name (and replica index if multiple replicas). The `--follow` / `-f` flag MUST stream logs continuously. The `--tail N` flag MUST show only the last N lines per container. The `--timestamps` flag MUST prepend timestamps to each line. A service name argument MUST filter output to that specific service.

#### Scenario: Logs from all services merged

GIVEN services `web` and `api` are running and producing output
WHEN the user invokes `maestro compose logs`
THEN log lines from both services MUST appear
AND each line MUST be prefixed with the service name (e.g., `web-1  |` or `api-1  |`)

#### Scenario: Follow mode streams in real time

GIVEN a running service producing periodic output
WHEN the user invokes `maestro compose logs -f`
THEN new log lines MUST appear in the terminal as they are produced
AND the command MUST not exit until interrupted with Ctrl+C

#### Scenario: Filter by service name

GIVEN services `web`, `api`, and `db` are running
WHEN the user invokes `maestro compose logs api`
THEN only logs from the `api` service containers MUST be displayed

#### Scenario: Tail limits per service

GIVEN service `web` has 1000 log lines
WHEN the user invokes `maestro compose logs --tail 20 web`
THEN at most 20 lines from `web` MUST be displayed

#### Scenario: Timestamps prepended

GIVEN a running service
WHEN the user invokes `maestro compose logs --timestamps`
THEN each log line MUST begin with an RFC 3339 timestamp

---

## 12. Exec and Run

### Requirement: Service Exec

The `maestro compose exec` command MUST execute a command inside a running service container, delegating to Gan's Touch (exec) implementation. If the service has multiple replicas, the `--index` flag MUST specify the target replica (default: 1).

```
maestro compose exec [options] <service> <command> [args...]
```

Options MUST include:

- `-it` — allocate a pseudo-TTY and keep stdin open (default for interactive shells)
- `--user <user>` — run as a specific user
- `--workdir <dir>` — working directory inside the container
- `--env KEY=VALUE` — additional environment variables
- `--index <n>` — replica index (default 1)

#### Scenario: Execute command in service container

GIVEN service `api` is running as project `myapp`
WHEN the user invokes `maestro compose exec api env`
THEN the `env` command MUST execute inside the `myapp-api-1` container
AND the output MUST be printed to standard output

#### Scenario: Interactive shell via exec

GIVEN service `web` is running
WHEN the user invokes `maestro compose exec -it web bash`
THEN an interactive bash session MUST open inside the `web` container

#### Scenario: Target replica with --index

GIVEN service `worker` has 3 replicas
WHEN the user invokes `maestro compose exec --index 2 worker ps`
THEN the `ps` command MUST execute inside `<project>-worker-2`

#### Scenario: Exec fails if service not running

GIVEN service `db` is stopped
WHEN the user invokes `maestro compose exec db psql`
THEN the system MUST return a non-zero exit code
AND the error MUST indicate the service is not running

---

### Requirement: One-off Run

The `maestro compose run` command MUST create and start a one-off container for the specified service, overriding the default command. The container MUST be attached to the same networks as the service's configured containers. Service dependencies (depends_on) MUST be started automatically unless `--no-deps` is passed.

By default, a one-off container MUST be removed after it exits. The `--rm=false` flag MUST prevent automatic removal.

#### Scenario: One-off container runs command and exits

GIVEN a compose file defining service `worker` with image `alpine`
WHEN the user invokes `maestro compose run worker echo hello`
THEN a new container MUST be created and started
AND the output `hello` MUST be printed
AND the container MUST be removed after completion

#### Scenario: One-off container on same networks

GIVEN a compose project with a `backend` network
AND service `worker` is attached to `backend`
WHEN `maestro compose run worker` creates a one-off container
THEN the one-off container MUST also be attached to `backend`
AND it MUST be able to reach other services on that network

#### Scenario: Dependencies started before one-off run

GIVEN service `worker` depends on `db` with `condition: service_started`
WHEN the user invokes `maestro compose run worker ./run-task.sh`
THEN `db` MUST be started (if not already running) before the one-off container

#### Scenario: --no-deps skips dependency start

GIVEN `db` is not running
WHEN the user invokes `maestro compose run --no-deps worker ./run-task.sh`
THEN `db` MUST NOT be started
AND the one-off container MUST start regardless

#### Scenario: --rm false preserves completed container

GIVEN the user invokes `maestro compose run --rm=false worker echo done`
WHEN the container completes
THEN the container MUST NOT be automatically removed
AND it MUST appear in `maestro compose ps --all`

---

## 13. Scale

### Requirement: Adjust Replica Count

The `maestro compose scale` command MUST adjust the number of running replicas for one or more services without recreating unchanged containers.

```
maestro compose scale <service>=<count> [<service2>=<count2>...]
```

When scaling up, new containers MUST be created following the same configuration. When scaling down, excess containers MUST be stopped and removed, starting with the highest-index replica.

#### Scenario: Scale up creates new replicas

GIVEN service `worker` is running with 1 replica
WHEN the user invokes `maestro compose scale worker=3`
THEN containers `<project>-worker-2` and `<project>-worker-3` MUST be created and started
AND `<project>-worker-1` MUST remain running unchanged

#### Scenario: Scale down removes excess replicas

GIVEN service `worker` is running with 3 replicas
WHEN the user invokes `maestro compose scale worker=1`
THEN containers `<project>-worker-2` and `<project>-worker-3` MUST be stopped and removed
AND `<project>-worker-1` MUST remain running

#### Scenario: Scale to zero equivalent to stop

GIVEN service `worker` is running with 2 replicas
WHEN the user invokes `maestro compose scale worker=0`
THEN all `worker` containers MUST be stopped and removed

#### Scenario: Scale multiple services simultaneously

GIVEN services `web` and `worker` are running
WHEN the user invokes `maestro compose scale web=2 worker=4`
THEN `web` MUST have 2 replicas and `worker` MUST have 4 replicas after the command completes

---

## 14. Start, Stop, and Restart

### Requirement: Start, Stop, Restart Existing Service Containers

- `maestro compose start [service...]` MUST start all stopped containers of the project (or specified services) without recreating them.
- `maestro compose stop [service...]` MUST stop all running containers of the project (or specified services) without removing them.
- `maestro compose restart [service...]` MUST stop then start the service containers.

These commands differ from `up`/`down`: they operate on existing containers only and do not create or remove resources.

#### Scenario: Stop running services

GIVEN services `web` and `api` are running
WHEN the user invokes `maestro compose stop`
THEN all containers MUST transition to Stopped
AND they MUST NOT be removed from Waystation

#### Scenario: Start previously stopped services

GIVEN services `web` and `api` are Stopped
WHEN the user invokes `maestro compose start`
THEN all containers MUST transition to Running

#### Scenario: Restart specific service

GIVEN services `web`, `api`, and `db` are running
WHEN the user invokes `maestro compose restart api`
THEN only the `api` container MUST be restarted
AND `web` and `db` MUST remain running and uninterrupted

#### Scenario: Stop timeout

GIVEN a service defines `stop_grace_period: 5s`
WHEN `maestro compose stop` is invoked
THEN the container MUST receive SIGTERM and the system MUST wait up to 5s before SIGKILL
AND the `--timeout` CLI flag MUST override the service's `stop_grace_period` for that invocation

---

## 15. Pause and Unpause

### Requirement: Freeze and Thaw Service Containers

`maestro compose pause [service...]` MUST freeze all containers of the project (or specified services) via the cgroups freezer. `maestro compose unpause [service...]` MUST resume them. Paused containers MUST appear with state `Paused` in `maestro compose ps`.

#### Scenario: Pause freezes all services

GIVEN services `web` and `api` are running
WHEN the user invokes `maestro compose pause`
THEN both containers MUST transition to Paused
AND `maestro compose ps` MUST show state `Paused` for both

#### Scenario: Unpause resumes paused services

GIVEN services `web` and `api` are Paused
WHEN the user invokes `maestro compose unpause`
THEN both containers MUST resume execution
AND `maestro compose ps` MUST show state `Running`

---

## 16. Kill

### Requirement: Force Stop Signal Delivery

`maestro compose kill [service...]` MUST send a signal to all running containers of the project (or specified services). The default signal MUST be `SIGKILL`. The `--signal` flag MUST allow sending any valid POSIX signal.

#### Scenario: Kill with default SIGKILL

GIVEN service `web` is running
WHEN the user invokes `maestro compose kill web`
THEN the `web` container MUST receive SIGKILL and transition to Stopped

#### Scenario: Kill with custom signal

GIVEN service `worker` is running
WHEN the user invokes `maestro compose kill --signal SIGUSR1 worker`
THEN the `worker` container MUST receive SIGUSR1 while remaining in the Running state (if it does not exit on SIGUSR1)

---

## 17. Remove (rm)

### Requirement: Remove Stopped Service Containers

`maestro compose rm [service...]` MUST remove all stopped containers of the project (or specified services). Running containers MUST be rejected unless the `--force` flag is given. The `--stop` / `-s` flag MUST stop running containers before removing them.

#### Scenario: Remove stopped containers

GIVEN service containers `web` and `api` are Stopped
WHEN the user invokes `maestro compose rm`
THEN both containers MUST be removed from Waystation

#### Scenario: Running container rejected without --force

GIVEN service `web` is Running
WHEN the user invokes `maestro compose rm web` without `--force`
THEN the system MUST return a non-zero exit code
AND the error MUST indicate that `web` is still running

#### Scenario: --stop flag stops before remove

GIVEN service `web` is Running
WHEN the user invokes `maestro compose rm --stop web`
THEN `web` MUST be stopped and then removed

---

## 18. Pull

### Requirement: Pull Service Images

`maestro compose pull [service...]` MUST pull the latest version of images for all services (or specified services) from their respective registries. It MUST NOT start or recreate containers.

#### Scenario: Pull all service images

GIVEN a compose file with services using `nginx:latest`, `alpine:3.19`, and `postgres:16`
WHEN the user invokes `maestro compose pull`
THEN all three images MUST be pulled from their registries in parallel

#### Scenario: Pull specific service

GIVEN a compose file with multiple services
WHEN the user invokes `maestro compose pull web`
THEN only the image for `web` MUST be pulled

#### Scenario: Pull respects registry credentials

GIVEN a service uses a private image at `registry.example.com/myapp:latest`
AND credentials for `registry.example.com` exist in the Sigul credential chain
WHEN `maestro compose pull` executes
THEN authentication MUST be used automatically

#### Scenario: --ignore-pull-failures continues on error

GIVEN one service image pull fails due to a network error
WHEN the user invokes `maestro compose pull --ignore-pull-failures`
THEN the system MUST continue pulling remaining images
AND the exit code MUST be non-zero to indicate partial failure

---

## 19. Config Validation

### Requirement: Validate and Render Compose File

`maestro compose config` MUST parse, validate, and render the fully-interpolated compose file after merging all `-f` files and applying variable substitution. The output MUST be valid YAML. Any syntax error, unknown field, or missing required field MUST be reported with its location in the source file.

#### Scenario: Valid compose file prints normalized YAML

GIVEN a valid `compose.yaml` with variable substitution
WHEN the user invokes `maestro compose config`
THEN the fully-resolved compose definition MUST be printed as canonical YAML to standard output

#### Scenario: Syntax error reported

GIVEN a `compose.yaml` with a YAML syntax error on line 12
WHEN the user invokes `maestro compose config`
THEN the system MUST return a non-zero exit code
AND the error MUST reference line 12 of the source file

#### Scenario: undefined required field reported

GIVEN a service in `compose.yaml` has neither an `image` nor a `build` field
WHEN the user invokes `maestro compose config`
THEN the system MUST return a non-zero exit code
AND the error MUST identify the service name and the missing required field

#### Scenario: --quiet flag suppresses output, error code only

GIVEN a valid compose file
WHEN the user invokes `maestro compose config --quiet`
THEN no output MUST be produced
AND the exit code MUST be 0

#### Scenario: --services lists service names

GIVEN a compose file with services `web`, `api`, `db`
WHEN the user invokes `maestro compose config --services`
THEN the output MUST be three lines, each containing one service name

#### Scenario: --volumes lists volume names

GIVEN a compose file with volumes `data` and `pgdata`
WHEN the user invokes `maestro compose config --volumes`
THEN the output MUST list `<project>_data` and `<project>_pgdata`

---

## 20. Images

### Requirement: List Images Used by Services

`maestro compose images [service...]` MUST list the images used by all running service containers of the project (or specified services), showing the container name, repository, tag, and size.

#### Scenario: Images listed for running project

GIVEN a compose project with running services using `nginx:alpine` and `postgres:16`
WHEN the user invokes `maestro compose images`
THEN both images MUST be listed with CONTAINER, REPOSITORY, TAG, IMAGE ID, SIZE columns

#### Scenario: --format json output

GIVEN a running compose project
WHEN the user invokes `maestro compose images --format json`
THEN the output MUST be a valid JSON array with the image data

---

## 21. Top

### Requirement: Process Listing per Service

`maestro compose top [service...]` MUST display the running processes for each service container of the project (or specified services), delegating to Gan's top implementation.

#### Scenario: Processes listed per service

GIVEN services `web` and `api` are running
WHEN the user invokes `maestro compose top`
THEN the process table MUST be shown for each running container
AND each section MUST be headed by the service name

---

## 22. Port

### Requirement: Display Public Port for Service

`maestro compose port <service> <private-port>` MUST display the host IP and port mapped to the private port of the running service container (replica 1 by default). The `--index` flag MUST specify a different replica. The `--protocol` flag MUST specify `tcp` (default) or `udp`.

#### Scenario: Display mapped port

GIVEN service `web` maps host port 32768 to container port 80
WHEN the user invokes `maestro compose port web 80`
THEN the output MUST be `0.0.0.0:32768`

#### Scenario: Port for specific replica

GIVEN service `web` has 2 replicas with different host ports
WHEN the user invokes `maestro compose port --index 2 web 80`
THEN the host port for the second replica MUST be returned

---

## 23. Events

### Requirement: Real-time Event Streaming

`maestro compose events [service...]` MUST stream real-time events for all containers of the project (or specified services), formatted as `<timestamp> <service>  | <TYPE> <ACTION>`. The command MUST run until interrupted with Ctrl+C. The `--json` flag MUST produce one JSON object per line.

#### Scenario: Events from all services

GIVEN a compose project with running services
WHEN the user invokes `maestro compose events` and a container dies
THEN an event line MUST be printed to standard output indicating the container die event

#### Scenario: Filter events by service

GIVEN multiple services are running
WHEN the user invokes `maestro compose events web`
THEN only events from `web` service containers MUST be displayed

#### Scenario: JSON event output

GIVEN the user invokes `maestro compose events --json`
WHEN a container lifecycle event occurs
THEN each event MUST be written as a JSON object with at minimum: `time`, `service`, `action`, `container` fields

---

## 24. Project Listing (ls)

### Requirement: List Active Compose Projects

`maestro compose ls` MUST list all compose projects that have containers present in the Waystation state store, showing the project name, status, and path to the compose file. The status MUST be one of: `running`, `exited`, `partially running`.

#### Scenario: Active projects listed

GIVEN two compose projects `app1` and `app2` are running
WHEN the user invokes `maestro compose ls`
THEN both projects MUST appear with their NAME, STATUS, CONFIG FILE columns

#### Scenario: Project with mixed states

GIVEN project `app1` has service `web` running and service `db` stopped
WHEN `maestro compose ls` is invoked
THEN `app1` MUST show status `partially running`

#### Scenario: --format json output

GIVEN active compose projects exist
WHEN the user invokes `maestro compose ls --format json`
THEN the output MUST be a valid JSON array

#### Scenario: --all includes projects with no running containers

GIVEN a project `app1` has all containers stopped
WHEN the user invokes `maestro compose ls --all`
THEN `app1` MUST appear with status `exited`
WHEN the user invokes `maestro compose ls` without `--all`
THEN `app1` MUST NOT appear

---

## 25. Build

### Requirement: Build Service Images (Future)

`maestro compose build [service...]` is reserved for a future milestone when `maestro build` is implemented. When invoked in the current release, the system MUST return an error indicating that the build feature is not yet available and instructing the user to build images manually and push them to a registry or use a locally available image.

#### Scenario: Build returns not-implemented error

GIVEN the user invokes `maestro compose build`
THEN the system MUST return a non-zero exit code
AND the error MUST state that the build command is not yet implemented
AND the error MUST include guidance for using pre-built images

---

## 26. Global Compose Flags

### Requirement: Compose-level Flags

All `maestro compose` subcommands MUST support the following compose-level flags (in addition to all root-level global flags):

- `-f` / `--file <path>` — compose file(s) to use (repeatable)
- `-p` / `--project-name <name>` — project name override
- `--profile <profile>` — activate service profile (repeatable)
- `--env-file <path>` — path to an env file
- `--project-directory <path>` — root directory for relative paths (default: compose file location)

#### Scenario: --project-directory scopes relative paths

GIVEN a `compose.yaml` with bind mount `./data:/app/data`
WHEN the user invokes `maestro compose --project-directory /opt/app up`
THEN the bind mount source MUST resolve to `/opt/app/data`

#### Scenario: Compose flags before or after subcommand

GIVEN the user invokes `maestro compose up --project-name myapp`
THEN the project name `myapp` MUST be used
AND `maestro compose --project-name myapp up` MUST be equivalent
