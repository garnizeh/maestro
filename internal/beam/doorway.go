package beam

import (
	"errors"
	"fmt"
	"net"
	"strconv"
	"strings"
)

const (
	partsWithHostPort      = 2
	partsWithHostIPAndPort = 3

	// ProtoTCP is the name of the Transmission Control Protocol.
	ProtoTCP = "tcp"
	// ProtoUDP is the name of the User Datagram Protocol.
	ProtoUDP = "udp"
)

// ParsePortMapping parses a Docker-style port spec string (-p).
// Formats supported:
// - "8080:80"
// - "8080:80/tcp"
// - "127.0.0.1:8080:80"
// - "127.0.0.1:8080:80/udp"
// Note: Ranges like "8080-8090:80-90" are not fully implemented in this baseline but could be expanded.
func ParsePortMapping(spec string) ([]PortMapping, error) {
	protocol := ProtoTCP
	if idx := strings.LastIndex(spec, "/"); idx != -1 {
		protocol = strings.ToLower(spec[idx+1:])
		if protocol != ProtoTCP && protocol != ProtoUDP && protocol != "sctp" {
			return nil, fmt.Errorf("invalid protocol '%s' in port spec '%s'", protocol, spec)
		}
		spec = spec[:idx]
	}

	parts := strings.Split(spec, ":")
	var hostIP string
	var hostPortStr, containerPortStr string

	switch len(parts) {
	case partsWithHostPort:
		// hostPort:containerPort
		hostPortStr = parts[0]
		containerPortStr = parts[1]
	case partsWithHostIPAndPort:
		// hostIP:hostPort:containerPort
		hostIP = parts[0]
		hostPortStr = parts[1]
		containerPortStr = parts[2]
		if net.ParseIP(hostIP) == nil {
			return nil, fmt.Errorf("invalid IP address '%s' in port spec '%s'", hostIP, spec)
		}
	default:
		return nil, fmt.Errorf("invalid port specification format '%s'", spec)
	}

	// Handle ranges if needed (basic implementation without ranges first)
	hostPorts, err := parsePortRange(hostPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid host port '%s': %w", hostPortStr, err)
	}

	containerPorts, err := parsePortRange(containerPortStr)
	if err != nil {
		return nil, fmt.Errorf("invalid container port '%s': %w", containerPortStr, err)
	}

	if len(hostPorts) != len(containerPorts) && len(containerPorts) != 1 {
		return nil, errors.New("invalid port range: host and container " +
			"ranges must be equal length or container must be a single port")
	}

	mappings := make([]PortMapping, 0, len(hostPorts))
	for i, h := range hostPorts {
		c := containerPorts[0]
		if len(containerPorts) > 1 {
			c = containerPorts[i]
		}
		mappings = append(mappings, PortMapping{
			HostPort:      h,
			ContainerPort: c,
			Protocol:      protocol,
			HostIP:        hostIP,
		})
	}

	return mappings, nil
}

func parsePortRange(portStr string) ([]int, error) {
	if portStr == "" {
		return nil, errors.New("empty port string")
	}

	if strings.Contains(portStr, "-") {
		parts := strings.Split(portStr, "-")
		if len(parts) != partsWithHostPort {
			return nil, errors.New("invalid port range len")
		}
		start, err1 := strconv.Atoi(parts[0])
		end, err2 := strconv.Atoi(parts[1])
		if err1 != nil || err2 != nil || start > end || start <= 0 || end > 65535 {
			return nil, fmt.Errorf("invalid port range %s", portStr)
		}
		var ports []int
		for i := start; i <= end; i++ {
			ports = append(ports, i)
		}
		return ports, nil
	}

	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 || port > 65535 {
		return nil, fmt.Errorf("invalid port %s", portStr)
	}
	return []int{port}, nil
}
