package beam

import (
	"context"
	"fmt"
	"path/filepath"

	"github.com/containernetworking/cni/libcni"
	"github.com/rs/zerolog/log"
)

const (
	// DefaultCNIBinDir is the standard location for CNI plugin binaries.
	DefaultCNIBinDir = "/opt/cni/bin"

	dirPerm  = 0750
	filePerm = 0600
)

// DefaultCNIConfig specifies the fallback embedded network configuration for beam0.
const DefaultCNIConfig = `{
  "cniVersion": "1.1.0",
  "name": "beam0",
  "plugins": [
    {
      "type": "bridge",
      "bridge": "beam0",
      "isGateway": true,
      "ipMasq": true,
      "hairpinMode": true,
      "ipam": {
        "type": "host-local",
        "subnet": "10.99.0.0/16",
        "routes": [
          { "dst": "0.0.0.0/0" }
        ]
      }
    },
    {
      "type": "firewall"
    },
    {
      "type": "portmap",
      "capabilities": {"portMappings": true}
    }
  ]
}`

// Beam manages the container networks and bridges Todash (Namespaces) and Guardian (CNI).
type Beam struct {
	guardian   cniExecutor
	todash     namespaceManager
	mejis      *Mejis
	confDir    string
	binDir     string
	fs         FS
	downloader PluginManager
	rootless   bool
}

// NewBeam creates a new networking engine initialized with CNI plugin paths.
func NewBeam(confDir, binDir, netnsDir string) *Beam {
	if binDir == "" {
		binDir = DefaultCNIBinDir
	}
	return &Beam{
		guardian:   NewGuardian([]string{binDir}),
		todash:     NewTodash(netnsDir),
		mejis:      NewMejis(filepath.Join(confDir, "mejis")),
		confDir:    confDir,
		binDir:     binDir,
		fs:         RealFS{},
		downloader: defaultDownloader,
	}
}

// WithFS sets a custom filesystem implementation.
func (b *Beam) WithFS(fs FS) *Beam {
	b.fs = fs
	return b
}

// WithDownloader sets a custom CNI downloader implementation.
func (b *Beam) WithDownloader(d PluginManager) *Beam {
	b.downloader = d
	return b
}

// WithGuardian sets a custom CNI executor implementation.
func (b *Beam) WithGuardian(g cniExecutor) *Beam {
	b.guardian = g
	return b
}

// WithTodash sets a custom namespace manager implementation.
func (b *Beam) WithTodash(t namespaceManager) *Beam {
	b.todash = t
	return b
}

// WithMejis sets a custom rootless networking driver.
func (b *Beam) WithMejis(m *Mejis) *Beam {
	b.mejis = m
	return b
}

// WithRootless enables or disables rootless networking mode.
func (b *Beam) WithRootless(rootless bool) *Beam {
	b.rootless = rootless
	b.todash.WithRootless(rootless)
	return b
}

// LoadDefaultConfig retrieves the CNI config list, creating it from the embedded default if missing.
func (b *Beam) LoadDefaultConfig() (*libcni.NetworkConfigList, error) {
	confPath := filepath.Join(b.confDir, "cni-beam0.conflist")

	confBytes, err := b.fs.ReadFile(confPath)
	if err == nil {
		return b.guardian.LoadConfigList(confBytes)
	}
	if !b.fs.IsNotExist(err) {
		return nil, fmt.Errorf("failed to read network config %s: %w", confPath, err)
	}

	if errMk := b.fs.MkdirAll(b.confDir, dirPerm); errMk != nil {
		return nil, fmt.Errorf("failed to create network config directory: %w", errMk)
	}
	if errWr := b.fs.WriteFile(confPath, []byte(DefaultCNIConfig), filePerm); errWr != nil {
		return nil, fmt.Errorf("failed to write default CNI config: %w", errWr)
	}

	return b.guardian.LoadConfigList([]byte(DefaultCNIConfig))
}

// Attach allocates a network namespace and connects it to the container network.
func (b *Beam) Attach(
	ctx context.Context,
	containerID string,
	mount *MountRequest,
	portMappings []PortMapping,
) (*AttachResult, error) {
	if b.rootless {
		// 1. Create the network namespace (and perform rootless mount if requested)
		nsPath, launcherPath, err := b.todash.NewNS(containerID, mount)
		if err != nil {
			return nil, fmt.Errorf("failed to create network namespace (rootless): %w", err)
		}

		if attachErr := b.mejis.Attach(ctx, containerID, nsPath, launcherPath, portMappings); attachErr != nil {
			if deleteErr := b.todash.DeleteNS(containerID); deleteErr != nil {
				log.Warn().Err(deleteErr).Str("containerID", containerID).
					Msg("failed to cleanup namespace after attach failure")
			}
			return nil, fmt.Errorf("failed to attach rootless network: %w", attachErr)
		}
		log.Debug().Str("id", containerID).Str("ns", nsPath).Msg("beam: attached rootless network")
		return &AttachResult{
			NetNSPath:    nsPath,
			LauncherPath: launcherPath,
		}, nil
	}

	log.Debug().Str("id", containerID).Msg("beam: attaching rootful network")
	// Rootful CNI logic
	if err := b.downloader.DownloadCNIPlugins(ctx, b.binDir); err != nil {
		return nil, fmt.Errorf("failed to ensure CNI plugins: %w", err)
	}

	configList, err := b.LoadDefaultConfig()
	if err != nil {
		return nil, fmt.Errorf("failed to load CNI config list: %w", err)
	}

	nsPath, _, err := b.todash.NewNS(containerID, mount)
	if err != nil {
		return nil, fmt.Errorf("failed to create network namespace: %w", err)
	}

	res, invokeErr := b.guardian.InvokeADD(
		ctx,
		configList,
		containerID,
		nsPath,
		"eth0",
		portMappings,
	)
	if invokeErr != nil {
		// Rollback namespace on failure
		if deleteErr := b.todash.DeleteNS(containerID); deleteErr != nil {
			log.Warn().Err(deleteErr).Str("containerID", containerID).
				Msg("failed to cleanup namespace after network attach failure")
		}
		return nil, fmt.Errorf("failed to attach network: %w", invokeErr)
	}

	log.Debug().Str("id", containerID).Str("ns", nsPath).Msg("beam: attached network via CNI")

	return &AttachResult{
		NetNSPath: nsPath,
		Result:    res,
	}, nil
}

// Detach disconnects the container from the network and cleans up its namespace.
func (b *Beam) Detach(ctx context.Context, containerID string, portMappings []PortMapping) error {
	if b.rootless {
		if err := b.mejis.Detach(ctx, containerID); err != nil {
			log.Warn().
				Err(err).
				Str("containerID", containerID).
				Msg("failed to detach rootless network driver")
		}
		return b.todash.DeleteNS(containerID)
	}

	nsPath := b.todash.NSPath(containerID)

	configList, err := b.LoadDefaultConfig()
	if err != nil {
		return fmt.Errorf("failed to load CNI config when detaching: %w", err)
	}

	// Invoke DEL, ignore errors if it partially fails (cleanup intent)
	if delErr := b.guardian.InvokeDEL(ctx, configList, containerID, nsPath, "eth0", portMappings); delErr != nil {
		log.Warn().
			Err(delErr).
			Str("containerID", containerID).
			Msg("failed to detach network via CNI")
	}

	log.Debug().Str("id", containerID).Msg("beam: detached network and cleaned up namespace")

	return b.todash.DeleteNS(containerID)
}
