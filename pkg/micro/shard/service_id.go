package shard

import (
	"fmt"

	"github.com/argus-labs/world-engine/pkg/micro"
)

// ID aliases ServiceID for backward compatibility.
type ID = micro.ServiceID

// Address aliases ServiceAddress for backward compatibility.
type Address = micro.ServiceAddress

// GetID builds a hierarchical service ID from organization, project, and namespace components.
// Format: <organization>.<project>.<namespace>.
func GetID(organization, project, namespace string) ID {
	return fmt.Sprintf("%s.%s.%s", organization, project, namespace)
}

// CreateAddress creates a service address for a shard service with the given components.
func CreateAddress(region, organization, project string, id ID) *Address {
	return micro.GetAddress(region, micro.RealmWorld, organization, project, id)
}

// EventEndpoint returns the full service address for an event with the given name.
func EventEndpoint(address *Address, eventName string) string {
	return micro.Endpoint(address, fmt.Sprintf("event.%s", eventName))
}

// CommandEndpoint returns the full service address for a command with the given name.
func CommandEndpoint(address *Address, commandName string) string {
	return micro.Endpoint(address, fmt.Sprintf("command.%s", commandName))
}
