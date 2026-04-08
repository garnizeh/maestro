# Beam Network Specification

## Purpose

Beam manages container networking for Maestro. It encompasses Todash (network namespace lifecycle), Guardian (CNI plugin integration), Callahan (embedded DNS resolution), Doorway (port mapping), and Mejis (rootless networking via pasta/slirp4netns). Beam provides both the default bridge network (beam0) and user-defined custom networks, with full IPv4/IPv6 dual-stack support.

---

## Requirements

### Requirement: Default Bridge Network (beam0)

The system MUST provide a default bridge network named `beam0` that is automatically created on first use. This network serves as the default connectivity path for containers that do not specify a custom network.

#### Scenario: beam0 auto-creation on first container start

GIVEN no networks have been created yet
WHEN a container is started without specifying a network
THEN the system MUST automatically create the `beam0` network with subnet `10.99.0.0/16`
AND the container MUST receive an IP address from the `10.99.0.0/16` range
AND the container MUST have a default route via the bridge gateway

#### Scenario: beam0 uses required plugin chain

GIVEN the `beam0` network is being created
WHEN the network configuration is generated
THEN the configuration MUST include the bridge plugin with gateway enabled and IP masquerading enabled
AND the configuration MUST include the firewall plugin
AND the configuration MUST include the portmap plugin
AND the CNI version MUST be `1.1.0`

#### Scenario: beam0 is idempotent on repeated use

GIVEN the `beam0` network already exists
WHEN a second container is started without specifying a network
THEN the system MUST reuse the existing `beam0` network
AND the second container MUST receive a different IP address from the same subnet
AND the first container's network MUST NOT be disrupted

#### Scenario: beam0 bridge configuration

GIVEN the `beam0` network is created
WHEN the bridge interface is configured
THEN the bridge device MUST be named `beam0`
AND the bridge MUST have `isGateway` set to true
AND the bridge MUST have `ipMasq` set to true
AND the bridge MUST have `hairpinMode` set to true
AND the IPAM type MUST be `host-local`

---

### Requirement: Custom Network Creation

The system MUST allow users to create custom networks with configurable parameters including driver type, subnet, gateway, IPv6 support, and internal-only mode.

#### Scenario: Create bridge network with default settings

GIVEN no network with the name "my-net" exists
WHEN a user creates a network named "my-net" with no additional options
THEN the system MUST create a bridge-type network
AND the system MUST auto-assign a subnet that does not overlap with existing networks
AND the system MUST auto-assign a gateway address (first usable IP in the subnet)
AND the network MUST appear in the network list

#### Scenario: Create bridge network with explicit subnet

GIVEN no network with the name "custom-net" exists
WHEN a user creates a network named "custom-net" with subnet `172.20.0.0/24`
THEN the system MUST create the network with the specified subnet
AND the IPAM configuration MUST use the subnet `172.20.0.0/24`

#### Scenario: Create bridge network with explicit gateway

GIVEN no network with the name "gw-net" exists
WHEN a user creates a network named "gw-net" with subnet `172.20.0.0/24` and gateway `172.20.0.254`
THEN the system MUST configure the gateway at `172.20.0.254`
AND containers on this network MUST use `172.20.0.254` as their default gateway

#### Scenario: Create macvlan network

GIVEN no network with the name "mv-net" exists
WHEN a user creates a network named "mv-net" with driver `macvlan` and a parent interface specified
THEN the system MUST create a macvlan-type network
AND containers on this network MUST receive unique MAC addresses
AND the network MUST use the specified parent interface

#### Scenario: Create network with IPv6 enabled

GIVEN no network with the name "v6-net" exists
WHEN a user creates a network named "v6-net" with the IPv6 flag enabled and an IPv6 subnet specified
THEN the system MUST create a dual-stack network
AND the IPAM configuration MUST include both IPv4 and IPv6 address ranges
AND containers on this network MUST receive both an IPv4 and an IPv6 address

#### Scenario: Create internal network

GIVEN no network with the name "internal-net" exists
WHEN a user creates a network named "internal-net" with the internal flag set
THEN the system MUST create a network without outbound connectivity
AND containers on this network MUST be able to communicate with each other
AND containers on this network MUST NOT have a route to external networks
AND IP masquerading MUST be disabled for this network

#### Scenario: Reject duplicate network name

GIVEN a network named "existing-net" already exists
WHEN a user attempts to create another network named "existing-net"
THEN the system MUST return an error indicating the name is already in use

#### Scenario: Reject overlapping subnet

GIVEN a network with subnet `172.20.0.0/24` already exists
WHEN a user attempts to create a network with subnet `172.20.0.0/24`
THEN the system MUST return an error indicating the subnet overlaps with an existing network

---

### Requirement: Network CRUD Operations

The system MUST provide full lifecycle management for networks: create, list, inspect, remove, and prune.

#### Scenario: List networks

GIVEN the `beam0` network exists and two custom networks have been created
WHEN a user lists all networks
THEN the output MUST include `beam0` and both custom networks
AND each entry MUST display the network name, ID, driver, and subnet

#### Scenario: Inspect a network

GIVEN a network named "my-net" exists with two containers connected
WHEN a user inspects the network "my-net"
THEN the output MUST include the network name, ID, driver, subnet, gateway, IPv6 status, internal flag, creation timestamp, and labels
AND the output MUST list all connected containers with their IP addresses

#### Scenario: Remove a network with no connected containers

GIVEN a network named "unused-net" exists with no connected containers
WHEN a user removes the network "unused-net"
THEN the network MUST be deleted
AND the network MUST no longer appear in the network list
AND any associated bridge interface MUST be removed from the host

#### Scenario: Reject removal of network with connected containers

GIVEN a network named "active-net" has one or more connected containers
WHEN a user attempts to remove the network "active-net"
THEN the system MUST return an error indicating the network has active endpoints
AND the network MUST NOT be removed

#### Scenario: Never remove beam0 on prune

GIVEN the `beam0` network exists and has no connected containers
AND two custom networks exist with no connected containers
WHEN a user runs network prune
THEN the two custom networks MUST be removed
AND the `beam0` network MUST NOT be removed

#### Scenario: Prune only removes empty networks

GIVEN three custom networks exist: one with connected containers and two without
WHEN a user runs network prune
THEN only the two networks without connected containers MUST be removed
AND the network with connected containers MUST be preserved

#### Scenario: Remove network by ID prefix

GIVEN a network with ID "abc123def456" exists
WHEN a user removes the network by the prefix "abc123"
THEN the system MUST resolve the prefix to the full ID and remove the network

#### Scenario: Ambiguous network ID prefix

GIVEN two networks exist with IDs starting with "abc"
WHEN a user attempts to remove a network by the prefix "abc"
THEN the system MUST return an error indicating the prefix is ambiguous

---

### Requirement: Todash (Network Namespace Lifecycle)

The system MUST manage network namespace creation and destruction as part of the container lifecycle. Network namespaces provide isolated network stacks for each container.

#### Scenario: Create network namespace for new container

GIVEN a container is being created
WHEN the network setup phase begins
THEN the system MUST create a new network namespace
AND the namespace MUST be bind-mounted at a persistent path
AND the persistent path MUST survive the exit of the creating process

#### Scenario: Rootful namespace creation

GIVEN the system is running in rootful mode
WHEN a network namespace is created
THEN the system MUST use `CLONE_NEWNET` to create an isolated network namespace
AND the namespace path MUST be accessible by the runtime

#### Scenario: Rootless namespace creation

GIVEN the system is running in rootless mode
WHEN a network namespace is created
THEN the system MUST create a user namespace first
AND the network namespace MUST be created inside the user namespace
AND the rootless networking tool (pasta or slirp4netns) MUST be started within this namespace

#### Scenario: Namespace cleanup on container removal

GIVEN a container with ID "ctr-1" has a network namespace at a known path
WHEN the container is removed
THEN the bind mount for the network namespace MUST be removed
AND the network namespace MUST be destroyed
AND no stale namespace files MUST remain on the filesystem

#### Scenario: Namespace persists across CLI invocations

GIVEN a container is running with a bind-mounted network namespace
WHEN the CLI process that started the container exits
THEN the network namespace MUST remain accessible
AND the container's network connectivity MUST NOT be interrupted

---

### Requirement: Guardian (CNI Plugin Integration)

The system MUST integrate with CNI (Container Network Interface) plugins to configure container networking. Guardian discovers, loads, and invokes CNI plugins with the correct protocol.

#### Scenario: Plugin discovery in configured directories

GIVEN the configuration specifies CNI plugin directories
WHEN Guardian searches for a plugin of type "bridge"
THEN the system MUST search each configured directory in order
AND the system MUST use the first matching executable found
AND the system MUST return an error if no matching plugin is found in any directory

#### Scenario: Invoke ADD operation

GIVEN a valid CNI conflist and a container's network namespace path
WHEN Guardian invokes the ADD operation
THEN the system MUST set the environment variable `CNI_COMMAND` to `ADD`
AND the system MUST set `CNI_CONTAINERID` to the container ID
AND the system MUST set `CNI_NETNS` to the namespace path
AND the system MUST set `CNI_IFNAME` to the interface name
AND the system MUST set `CNI_PATH` to the configured plugin directories
AND the system MUST pass the network configuration JSON on stdin
AND the system MUST capture the result JSON from stdout

#### Scenario: Invoke DEL operation

GIVEN a container that was previously configured with ADD
WHEN Guardian invokes the DEL operation
THEN the system MUST set `CNI_COMMAND` to `DEL`
AND the system MUST pass the same container ID, namespace, and interface name used during ADD
AND the system MUST NOT fail if the namespace no longer exists

#### Scenario: Invoke CHECK operation

GIVEN a container with an active network configuration
WHEN Guardian invokes the CHECK operation
THEN the system MUST set `CNI_COMMAND` to `CHECK`
AND the system MUST verify the network configuration is still valid
AND a successful CHECK MUST return exit code 0

#### Scenario: Invoke GC operation

GIVEN CNI plugin version 1.1.0 or later is in use
WHEN Guardian invokes the GC operation
THEN the system MUST pass the list of valid attached containers
AND the plugin MUST clean up any stale IPAM reservations or interfaces not in the valid list

#### Scenario: Plugin chaining

GIVEN a conflist with three plugins (bridge, firewall, portmap)
WHEN Guardian processes the ADD operation
THEN the system MUST invoke each plugin in order
AND the result of each plugin MUST be passed as `prevResult` to the next plugin in the chain
AND the final result MUST reflect the cumulative configuration

#### Scenario: Plugin execution failure

GIVEN a conflist with three chained plugins
WHEN the second plugin in the chain returns an error during ADD
THEN the system MUST invoke DEL on the first plugin to roll back
AND the system MUST return the error from the failing plugin to the caller

---

### Requirement: Callahan (Embedded DNS)

The system MUST provide an embedded DNS resolver per network that enables container name resolution within the same network and forwards external queries to host DNS servers.

#### Scenario: Resolve container name within same network

GIVEN two containers "web" and "api" are connected to the same network
WHEN container "web" performs a DNS lookup for "api"
THEN the DNS resolver MUST return the IP address of container "api"

#### Scenario: Resolve container by ID

GIVEN a container with ID "abc123def456" is connected to a network
WHEN another container on the same network performs a DNS lookup for "abc123def456"
THEN the DNS resolver MUST return the IP address of that container

#### Scenario: Forward external DNS queries

GIVEN a container connected to a network with Callahan enabled
WHEN the container performs a DNS lookup for "example.com"
THEN the DNS resolver MUST forward the query to the host's configured DNS servers
AND the response from the host DNS MUST be returned to the container

#### Scenario: Cross-network resolution for multi-homed containers

GIVEN container "app" is connected to both "frontend-net" and "backend-net"
AND container "db" is connected only to "backend-net"
AND container "proxy" is connected only to "frontend-net"
WHEN container "app" performs a DNS lookup for "db"
THEN the resolver MUST return the IP address of "db" on "backend-net"
WHEN container "app" performs a DNS lookup for "proxy"
THEN the resolver MUST return the IP address of "proxy" on "frontend-net"

#### Scenario: Container not resolvable from disconnected network

GIVEN container "secure" is connected only to "internal-net"
AND container "external" is connected only to "public-net"
WHEN container "external" performs a DNS lookup for "secure"
THEN the DNS resolver MUST return NXDOMAIN (name not found)

#### Scenario: Custom DNS configuration via flags

GIVEN a container is started with custom DNS server `8.8.8.8`, DNS search domain `example.com`, and DNS option `ndots:3`
WHEN the container's DNS configuration is set up
THEN the container's resolver configuration MUST list `8.8.8.8` as a nameserver
AND the search domain MUST include `example.com`
AND the option `ndots:3` MUST be set

#### Scenario: DNS resolver listens on loopback inside container

GIVEN a container is started on a network with Callahan enabled
WHEN the container's network namespace is configured
THEN the DNS resolver MUST listen on a loopback address inside the container's network namespace
AND the container's resolver configuration MUST point to that loopback address

#### Scenario: DNS updates on container connect/disconnect

GIVEN container "svc" is connected to "net-a"
WHEN container "svc" is also connected to "net-b"
THEN containers on "net-b" MUST be able to resolve "svc" by name
WHEN container "svc" is disconnected from "net-b"
THEN containers on "net-b" MUST no longer be able to resolve "svc" by name

---

### Requirement: Doorway (Port Mapping)

The system MUST support mapping host ports to container ports, enabling external access to container services. Port mapping specifications MUST follow Docker-compatible syntax.

#### Scenario: Simple host-to-container port mapping

GIVEN a container is started with port specification `-p 8080:80`
WHEN the port mapping is configured
THEN host port 8080 MUST forward to container port 80
AND the protocol MUST default to TCP
AND the mapping MUST listen on all host interfaces (0.0.0.0)

#### Scenario: Port mapping with specific host IP

GIVEN a container is started with port specification `-p 127.0.0.1:8080:80/tcp`
WHEN the port mapping is configured
THEN host port 8080 MUST forward to container port 80
AND the mapping MUST listen only on the loopback interface (127.0.0.1)
AND the protocol MUST be TCP

#### Scenario: Port mapping with UDP protocol

GIVEN a container is started with port specification `-p 5353:53/udp`
WHEN the port mapping is configured
THEN host port 5353 MUST forward to container port 53
AND the protocol MUST be UDP

#### Scenario: Port range mapping

GIVEN a container is started with port specification `-p 8080-8090:80-90`
WHEN the port mapping is configured
THEN each host port in the range 8080-8090 MUST map to the corresponding container port in the range 80-90
AND host port 8080 MUST map to container port 80
AND host port 8090 MUST map to container port 90
AND the total number of mappings MUST be 11

#### Scenario: Random host port assignment

GIVEN a container is started with port specification `-p 80` (no host port specified)
WHEN the port mapping is configured
THEN the system MUST assign a random available host port
AND the assigned port MUST be in the ephemeral port range
AND the mapping MUST forward to container port 80

#### Scenario: Port mapping integration with CNI portmap plugin

GIVEN a container has port mappings configured
WHEN the CNI plugin chain is invoked
THEN the portmap plugin MUST receive the port mapping capabilities
AND the portmap plugin MUST create the appropriate forwarding rules

#### Scenario: List active port mappings

GIVEN a container is running with port mappings `-p 8080:80` and `-p 443:443/tcp`
WHEN a user queries the port mappings for the container
THEN the output MUST show `80/tcp -> 0.0.0.0:8080`
AND the output MUST show `443/tcp -> 0.0.0.0:443`

#### Scenario: Invalid port specification rejected

GIVEN a user specifies port mapping `-p abc:80`
WHEN the port specification is parsed
THEN the system MUST return an error indicating the port specification is invalid

#### Scenario: Port range mismatch rejected

GIVEN a user specifies port mapping `-p 8080-8085:80-90`
WHEN the port specification is parsed
THEN the system MUST return an error indicating the host and container port ranges have different lengths

#### Scenario: Duplicate host port binding rejected

GIVEN a container is running with port mapping `-p 8080:80`
WHEN another container attempts to start with port mapping `-p 8080:3000`
THEN the system MUST return an error indicating host port 8080 is already in use

---

### Requirement: Mejis (Rootless Networking)

The system MUST support container networking in rootless mode using userspace networking tools. Pasta MUST be the default tool, with slirp4netns as a fallback.

#### Scenario: Use pasta as default rootless networking tool

GIVEN the system is running in rootless mode
AND the `pasta` binary is available on the system
WHEN a container's network is being set up
THEN the system MUST use pasta for network connectivity
AND the container MUST have functional outbound network connectivity

#### Scenario: Fall back to slirp4netns when pasta is unavailable

GIVEN the system is running in rootless mode
AND the `pasta` binary is NOT available on the system
AND the `slirp4netns` binary IS available on the system
WHEN a container's network is being set up
THEN the system MUST use slirp4netns as a fallback
AND the container MUST have functional outbound network connectivity

#### Scenario: Error when no rootless networking tool is available

GIVEN the system is running in rootless mode
AND neither `pasta` nor `slirp4netns` is available
WHEN a container's network is being set up
THEN the system MUST return an error with a clear message indicating which tools are required
AND the error message MUST suggest how to install the required tools

#### Scenario: Rootless port mapping with pasta

GIVEN the system is running in rootless mode with pasta
AND a container is started with port mapping `-p 8080:80`
WHEN the port mapping is configured
THEN pasta MUST forward host port 8080 to container port 80
AND the mapping MUST function without iptables or nftables rules

#### Scenario: Privileged port restriction in rootless mode

GIVEN the system is running in rootless mode
AND the kernel parameter `net.ipv4.ip_unprivileged_port_start` is at its default value (1024)
WHEN a container is started with port mapping `-p 80:80`
THEN the system MUST return an error indicating that binding to ports below 1024 requires elevated privileges
AND the error message MUST suggest the sysctl workaround: `sysctl net.ipv4.ip_unprivileged_port_start=0`

#### Scenario: Privileged port allowed when sysctl is configured

GIVEN the system is running in rootless mode
AND the kernel parameter `net.ipv4.ip_unprivileged_port_start` is set to `0`
WHEN a container is started with port mapping `-p 80:80`
THEN the port mapping MUST succeed
AND host port 80 MUST forward to container port 80

#### Scenario: Rootless port mapping with slirp4netns

GIVEN the system is running in rootless mode with slirp4netns (pasta unavailable)
AND a container is started with port mapping `-p 8080:80`
WHEN the port mapping is configured
THEN slirp4netns MUST handle the port forwarding
AND the mapping MUST function correctly despite the userspace TCP/IP overhead

---

### Requirement: Network Connect and Disconnect

The system MUST support attaching and detaching containers to/from networks at runtime without stopping the container.

#### Scenario: Connect running container to additional network

GIVEN a running container "app" is connected to "beam0"
AND a network "backend-net" exists
WHEN a user connects container "app" to "backend-net"
THEN the container MUST receive a new network interface
AND the container MUST receive an IP address from "backend-net"
AND the container's existing connection to "beam0" MUST NOT be disrupted

#### Scenario: Disconnect container from network

GIVEN a running container "app" is connected to both "beam0" and "backend-net"
WHEN a user disconnects container "app" from "backend-net"
THEN the network interface for "backend-net" MUST be removed from the container
AND the container's IP address on "backend-net" MUST be released
AND the container's connection to "beam0" MUST NOT be disrupted

#### Scenario: Reject disconnect from last network

GIVEN a running container "app" is connected only to "beam0"
WHEN a user attempts to disconnect container "app" from "beam0"
THEN the system MUST return an error indicating a container must remain on at least one network
AND the container's connectivity MUST NOT be changed

#### Scenario: Connect to non-existent network

GIVEN a running container "app" exists
AND no network named "ghost-net" exists
WHEN a user attempts to connect container "app" to "ghost-net"
THEN the system MUST return an error indicating the network does not exist

#### Scenario: Connect already-connected container

GIVEN a running container "app" is already connected to "backend-net"
WHEN a user attempts to connect container "app" to "backend-net" again
THEN the system MUST return an error indicating the container is already connected to that network

---

### Requirement: Network Flag on Container Run

The system MUST support the `--network` flag when running containers to specify which network the container should join at startup.

#### Scenario: Run container on specific network

GIVEN a network named "custom-net" exists
WHEN a container is started with `--network custom-net`
THEN the container MUST be connected to "custom-net" instead of `beam0`
AND the container MUST receive an IP address from "custom-net"

#### Scenario: Run container with no networking

GIVEN the system is operational
WHEN a container is started with `--network none`
THEN the container MUST NOT have any network interfaces except loopback
AND the container MUST NOT be connected to any network
AND the loopback interface MUST be up and functional

#### Scenario: Run container on beam0 by default

GIVEN the system is operational
AND no `--network` flag is specified
WHEN a container is started
THEN the container MUST be connected to `beam0`

#### Scenario: Run on non-existent network

GIVEN no network named "phantom-net" exists
WHEN a container is started with `--network phantom-net`
THEN the system MUST return an error indicating the network does not exist
AND the container MUST NOT be created

---

### Requirement: IPv4/IPv6 Dual-Stack Support

The system MUST support IPv4/IPv6 dual-stack networking on custom networks. Both address families MUST be configured simultaneously through the bridge plugin.

#### Scenario: Dual-stack network creation

GIVEN a user creates a network with IPv6 enabled and specifies both an IPv4 subnet and an IPv6 subnet
WHEN the network is created
THEN the IPAM configuration MUST contain address ranges for both IPv4 and IPv6
AND the network configuration MUST include routes for both `0.0.0.0/0` and `::/0`

#### Scenario: Dual-stack address assignment

GIVEN a dual-stack network exists with subnets `10.100.0.0/24` and `fd00:dead:beef::/48`
WHEN a container is connected to this network
THEN the container MUST receive an IPv4 address from `10.100.0.0/24`
AND the container MUST receive an IPv6 address from `fd00:dead:beef::/48`
AND the container MUST be reachable via both addresses from other containers on the same network

#### Scenario: IPv4-only network by default

GIVEN a user creates a network without the IPv6 flag
WHEN the network is created
THEN the network MUST only have IPv4 addressing
AND containers on this network MUST NOT receive an IPv6 address

---

### Requirement: Network Configuration Schema

The system MUST persist network configuration in a well-defined schema that includes all metadata needed to reconstruct the network.

#### Scenario: Network configuration includes required fields

GIVEN a network is created
WHEN the network configuration is persisted
THEN the stored configuration MUST include the network name, unique ID, driver type, the full Guardian (CNI) conflist, IPv6 enabled flag, internal flag, Callahan (DNS) enabled flag, labels, creation timestamp, and connected container list

#### Scenario: Network configuration survives system restart

GIVEN a network "my-net" was created with specific configuration
WHEN the system restarts and loads the network
THEN the loaded configuration MUST match the original configuration exactly
AND containers that reference "my-net" MUST be able to use it

---

### Requirement: Network Cleanup on Container Removal

The system MUST perform complete network cleanup when a container is removed, including CNI DEL invocation, IPAM reservation release, and interface removal.

#### Scenario: Full cleanup on container removal

GIVEN a container is connected to network "my-net" with an assigned IP address
WHEN the container is removed
THEN the system MUST invoke the CNI DEL operation for the container
AND the container's IP address MUST be released back to the IPAM pool
AND the container's network interface (veth pair) MUST be removed
AND the container MUST be removed from the network's connected container list

#### Scenario: Cleanup on container connected to multiple networks

GIVEN a container is connected to both "beam0" and "custom-net"
WHEN the container is removed
THEN the system MUST invoke CNI DEL for each network
AND IP addresses MUST be released on both networks
AND interfaces for both networks MUST be removed

#### Scenario: Cleanup tolerates missing namespace

GIVEN a container's network namespace has already been destroyed (crash scenario)
WHEN the container removal cleanup runs
THEN the CNI DEL operation MUST NOT fail due to the missing namespace
AND the IPAM reservation MUST still be released
AND the system MUST log a warning about the missing namespace

#### Scenario: Cleanup on forced container removal

GIVEN a running container is force-removed
WHEN the cleanup runs
THEN all network resources MUST be cleaned up as in normal removal
AND the cleanup MUST proceed even if the container process has not exited

---

### Requirement: Port Specification Parsing

The system MUST parse all Docker-compatible port specification formats correctly.

#### Scenario: Parse ip:hostPort:containerPort format

GIVEN a port specification `192.168.1.100:8080:80`
WHEN the specification is parsed
THEN the host IP MUST be `192.168.1.100`
AND the host port MUST be `8080`
AND the container port MUST be `80`
AND the protocol MUST default to `tcp`

#### Scenario: Parse ip:hostPort:containerPort/protocol format

GIVEN a port specification `127.0.0.1:5353:53/udp`
WHEN the specification is parsed
THEN the host IP MUST be `127.0.0.1`
AND the host port MUST be `5353`
AND the container port MUST be `53`
AND the protocol MUST be `udp`

#### Scenario: Parse hostPort:containerPort format

GIVEN a port specification `3000:3000`
WHEN the specification is parsed
THEN the host IP MUST default to `0.0.0.0`
AND the host port MUST be `3000`
AND the container port MUST be `3000`

#### Scenario: Parse containerPort only format

GIVEN a port specification `80`
WHEN the specification is parsed
THEN the host IP MUST default to `0.0.0.0`
AND the host port MUST be randomly assigned
AND the container port MUST be `80`

#### Scenario: Parse ip::containerPort format

GIVEN a port specification `127.0.0.1::80`
WHEN the specification is parsed
THEN the host IP MUST be `127.0.0.1`
AND the host port MUST be randomly assigned
AND the container port MUST be `80`

#### Scenario: Parse port range

GIVEN a port specification `8000-8010:9000-9010`
WHEN the specification is parsed
THEN the system MUST produce 11 individual port mappings
AND the first mapping MUST be host 8000 to container 9000
AND the last mapping MUST be host 8010 to container 9010

#### Scenario: Reject zero port

GIVEN a port specification `0:80`
WHEN the specification is parsed
THEN the system MUST return an error indicating port 0 is not valid

#### Scenario: Reject port above 65535

GIVEN a port specification `70000:80`
WHEN the specification is parsed
THEN the system MUST return an error indicating the port number is out of range

---

### Requirement: Multiple Port Mappings

The system MUST support multiple port mapping specifications on a single container.

#### Scenario: Multiple port flags

GIVEN a container is started with `-p 8080:80 -p 8443:443`
WHEN the port mappings are configured
THEN host port 8080 MUST forward to container port 80
AND host port 8443 MUST forward to container port 443
AND both mappings MUST be active simultaneously

#### Scenario: Mixed protocols on same container

GIVEN a container is started with `-p 53:53/tcp -p 53:53/udp`
WHEN the port mappings are configured
THEN host port 53/tcp MUST forward to container port 53/tcp
AND host port 53/udp MUST forward to container port 53/udp

---

### Requirement: Firewall Rules for Bridge Networks

The system MUST configure firewall rules for bridge networks to control traffic flow between containers and between containers and external networks.

#### Scenario: Inter-container traffic on same bridge

GIVEN two containers are connected to the same bridge network
WHEN one container sends traffic to the other's IP address
THEN the firewall MUST allow the traffic

#### Scenario: Outbound masquerading

GIVEN a container on a bridge network with IP masquerading enabled
WHEN the container sends traffic to an external destination
THEN the traffic MUST be masqueraded (NAT) through the host's IP
AND return traffic MUST be correctly routed back to the container

#### Scenario: Internal network blocks outbound

GIVEN a container on an internal bridge network
WHEN the container attempts to send traffic to an external destination
THEN the firewall MUST block the traffic
AND the container MUST NOT have outbound connectivity

---

### Requirement: IPAM Integration

The system MUST integrate with the host-local IPAM plugin for IP address management, ensuring addresses are properly allocated and released.

#### Scenario: Unique IP allocation per container

GIVEN a network with subnet `10.99.0.0/16`
WHEN 10 containers are connected to the network
THEN each container MUST receive a unique IP address from the subnet
AND no two containers MUST share the same IP address

#### Scenario: IP address reuse after container removal

GIVEN a container was assigned IP `10.99.0.5` and then removed
WHEN a new container is connected to the same network
THEN the IP `10.99.0.5` MUST be available for reassignment

#### Scenario: Subnet exhaustion

GIVEN a network with subnet `10.99.0.0/30` (only 2 usable addresses)
AND both usable addresses are already allocated
WHEN a new container attempts to connect to the network
THEN the system MUST return an error indicating the subnet is exhausted
