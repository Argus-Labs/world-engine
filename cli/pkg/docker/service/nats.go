package service

import (
	"fmt"
	"time"

	"github.com/moby/moby/api/types/container"

	"github.com/argus-labs/world-engine/cli/pkg/version"
)

const (
	// DefaultNatsClientPort is the default NATS client port.
	DefaultNatsClientPort = 4222
	// DefaultNatsMonitorPort is the default NATS monitoring port.
	DefaultNatsMonitorPort = 8222
	// DefaultNatsClusterPort is the default NATS cluster port.
	DefaultNatsClusterPort = 6222
	// DefaultNatsWebsocketPort is the default NATS websocket port.
	DefaultNatsWebsocketPort = 4443

	jetstreamStoreDir = "/data/jetstream"

	// DefaultNatsImage is the default Docker image for the NATS service.
	DefaultNatsImage = "nats:"

	// DefaultNatsContainerName is the default container name for the NATS service.
	DefaultNatsContainerName = "world-engine-nats"
)

// NATS creates a NATS service with JetStream support.
func NATS(_ *Config) Service {
	// Enable JetStream via CLI flags (no config file needed)
	cmd := []string{"-js", "-sd", jetstreamStoreDir}

	return Service{
		Name:         DefaultNatsContainerName,
		Image:        DefaultNatsImage + version.Nats,
		Cmd:          cmd,
		Env:          []string{},
		ExposedPorts: getExposedPorts(getNATSPorts()),
		Healthcheck: &container.HealthConfig{
			Test:     []string{"CMD", "curl", "-f", "http://localhost:8222/healthz"},
			Interval: 5 * time.Second,
			Timeout:  3 * time.Second,
			Retries:  5,
		},
		PortBindings:  newPortMap(getNATSPorts()),
		RestartPolicy: container.RestartPolicy{Name: "unless-stopped"},
		NetworkMode:   DefaultNetworkMode,
		Binds:         []string{fmt.Sprintf("%s:%s", DefaultNatsContainerName, jetstreamStoreDir)},
	}
}

// getNATSPorts returns the list of NATS ports to expose.
func getNATSPorts() []int {
	return []int{
		DefaultNatsClientPort,
		DefaultNatsMonitorPort,
		DefaultNatsClusterPort,
		DefaultNatsWebsocketPort,
	}
}
