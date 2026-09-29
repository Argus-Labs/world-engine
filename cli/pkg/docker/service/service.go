package service

import (
	"fmt"
	"strconv"

	"github.com/moby/moby/api/types/container"
	"github.com/moby/moby/api/types/network"
	ocispec "github.com/opencontainers/image-spec/specs-go/v1"
)

type Builder func(cfg *Config) Service

// ServiceOrder controls how shared service builders are ordered for consumers.
type ServiceOrder int

const (
	// CardinalShardsFirst lists shard services first, then game services and NATS.
	CardinalShardsFirst ServiceOrder = iota
	// InfrastructureFirst lists NATS first, then game services and shard services.
	InfrastructureFirst
)

// Service represents a Docker service configuration.
// It contains the name of the container and a function to get the container and host config.
type Service struct {
	container.Config
	container.HostConfig
	network.NetworkingConfig
	ocispec.Platform

	Name string
	// Dependencies are other services that need to be pull before this service
	Dependencies []Service
	// Dockerfile is the content of the Dockerfile
	Dockerfile string
	// BuildTarget is the target build of the Dockerfile e.g. builder or runtime
	BuildTarget string
	// BuildArgs are ARGs passed to the Docker build (e.g., SOURCE_PATH)
	BuildArgs map[string]string
}

func getExposedPorts(ports []int) network.PortSet {
	exposedPorts := make(network.PortSet)
	for _, port := range ports {
		if port < 1 || port > 65535 {
			panic(fmt.Sprintf("invalid port %d: must be between 1 and 65535", port))
		}
		tcpPort := network.MustParsePort(strconv.Itoa(port) + "/tcp")
		exposedPorts[tcpPort] = struct{}{}
	}
	return exposedPorts
}

// GetServices returns Cardinal shard builders plus game services and NATS.
// Game services keep their world.toml declaration order (it drives start order);
// the auto-provisioned project database (when db = true is used) rides in from
// BuildGameServices, prepended so it comes up first.
func GetServices(cfg *Config, order ServiceOrder) []Builder {
	shards := BuildCardinalShards(cfg)
	gameServices := BuildGameServices(cfg)
	out := make([]Builder, 0, len(shards)+len(gameServices)+1)
	switch order {
	case CardinalShardsFirst:
		out = append(out, shards...)
		out = append(out, gameServices...)
		out = append(out, NATS)
	case InfrastructureFirst:
		out = append(out, NATS)
		out = append(out, gameServices...)
		out = append(out, shards...)
	default:
		panic(fmt.Sprintf("unknown ServiceOrder %d", order))
	}
	return out
}

func newPortMap(ports []int) network.PortMap {
	portMap := make(network.PortMap)
	for _, port := range ports {
		if port < 1 || port > 65535 {
			panic(fmt.Sprintf("invalid port %d: must be between 1 and 65535", port))
		}
		portStr := strconv.Itoa(port)
		tcpPort := network.MustParsePort(portStr + "/tcp")
		portMap[tcpPort] = []network.PortBinding{{HostPort: portStr}}
	}
	return portMap
}
