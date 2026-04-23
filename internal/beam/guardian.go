package beam

import (
	"context"

	"github.com/containernetworking/cni/libcni"
	"github.com/containernetworking/cni/pkg/types"
)

// Guardian is the CNI plugin executor.
// It discovers plugins and invokes them using the standard CNI protocol.
type Guardian struct {
	pluginPaths []string
	cni         libcni.CNI
}

// NewGuardian creates a new Guardian instance to invoke CNI networks.
func NewGuardian(paths []string) *Guardian {
	if len(paths) == 0 {
		paths = []string{"/opt/cni/bin"}
	}
	return &Guardian{
		pluginPaths: paths,
		cni:         &libcni.CNIConfig{Path: paths},
	}
}

// InvokeADD executes a CNI plugin chain for the ADD command.
func (g *Guardian) InvokeADD(ctx context.Context, netConfList *libcni.NetworkConfigList,
	containerID, netnsPath, ifName string, portMappings []PortMapping) (types.Result, error) {
	rtConf := &libcni.RuntimeConf{
		ContainerID: containerID,
		NetNS:       netnsPath,
		IfName:      ifName,
		CapabilityArgs: map[string]any{
			"portMappings": portMappings,
		},
	}
	return g.cni.AddNetworkList(ctx, netConfList, rtConf)
}

// InvokeDEL executes a CNI plugin chain for the DEL command.
func (g *Guardian) InvokeDEL(ctx context.Context, netConfList *libcni.NetworkConfigList,
	containerID, netnsPath, ifName string, portMappings []PortMapping) error {
	rtConf := &libcni.RuntimeConf{
		ContainerID: containerID,
		NetNS:       netnsPath,
		IfName:      ifName,
		CapabilityArgs: map[string]any{
			"portMappings": portMappings,
		},
	}
	return g.cni.DelNetworkList(ctx, netConfList, rtConf)
}

// InvokeCHECK executes a CNI plugin chain for the CHECK command.
func (g *Guardian) InvokeCHECK(ctx context.Context, netConfList *libcni.NetworkConfigList,
	containerID, netnsPath, ifName string) error {
	rtConf := &libcni.RuntimeConf{
		ContainerID: containerID,
		NetNS:       netnsPath,
		IfName:      ifName,
	}
	return g.cni.CheckNetworkList(ctx, netConfList, rtConf)
}

// LoadConfigList parses raw JSON bytes into a valid CNI NetworkConfigList.
func (g *Guardian) LoadConfigList(confBytes []byte) (*libcni.NetworkConfigList, error) {
	return libcni.ConfListFromBytes(confBytes)
}
