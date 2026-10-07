package service

import (
	"strconv"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"

	"github.com/argus-labs/world-engine/cli/pkg/version"
)

const (
	DefaultNatsClientPort  = 4222
	DefaultNatsMonitorPort = 8222

	jetstreamStoreDir = "/data"

	DefaultNatsImage = "nats:"
)

// NatsContainerName is "<project>-nats"; also the in-network host shards dial.
func NatsContainerName(project string) string { return project + "-nats" }

// NatsVolumeName holds the JetStream store; purge removes it.
func NatsVolumeName(project string) string { return NatsContainerName(project) + "-data" }

// NatsURL is the in-network URL shards and services use.
func NatsURL(project string) string {
	return "nats://" + NatsContainerName(project) + ":" + strconv.Itoa(DefaultNatsClientPort)
}

// NATS is the per-project JetStream server, published on 127.0.0.1:4222 (clients,
// reload --purge) and :8222 (monitoring). The image is scratch-based, so it cannot run
// a Docker healthcheck; readiness is the monitoring endpoint's /healthz instead.
func NATS(cfg *Config) Service {
	project := cfg.WorldToml.Project
	ports := []int{DefaultNatsClientPort, DefaultNatsMonitorPort}
	bindings := network.PortMap{}
	for _, p := range ports {
		tcp := network.MustParsePort(strconv.Itoa(p) + "/tcp")
		bindings[tcp] = loopbackBinding(p)
	}
	return Service{
		Name:         NatsContainerName(project),
		Image:        DefaultNatsImage + version.Nats,
		Cmd:          []string{"-js", "-sd", jetstreamStoreDir, "-m", strconv.Itoa(DefaultNatsMonitorPort)},
		ExposedPorts: getExposedPorts(ports),
		Labels:       Labels(project, RoleNATS, "", ""),
		PortBindings: bindings,
		NetworkMode:  container.NetworkMode(NetworkName(project)),
		Binds:        []string{NatsVolumeName(project) + ":" + jetstreamStoreDir},
		ReadyURL:     "http://127.0.0.1:" + strconv.Itoa(DefaultNatsMonitorPort) + "/healthz",
	}
}
