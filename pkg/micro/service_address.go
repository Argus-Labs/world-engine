package micro

import (
	"fmt"
	"strings"

	microv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/micro/v1"
	"github.com/rotisserie/eris"
)

// This file contains the convention for naming service addresses for all Worldbase related services.
//
// The ServiceAddress convention is as follows:
// <project>.<service_id>.<endpoint>
//
// <project> groups related services (e.g., "rampage").
//
// <service_id> is a unique identifier for the service instance. It must be unique within the project.
// A deployment that runs a service in several places puts the place in the ID (e.g., "gameplay-usw1-3").
//
// <endpoint> is an arbitrary token that identifies specific functionality within a service.
// An endpoint can contain . as a delimiter to leverage NATS routing.
//
// Examples:
// - rampage.gameplay-usw1-3.ping
// - rampage.gameplay-usw1-3.isc.plat_result.rampage.platform

// ServiceID represents a unique identifier for a service.
type ServiceID = string

var (
	ErrInvalidAddress = eris.New("invalid service address format")
)

// addressTokens is the number of subject tokens in a service address.
const addressTokens = 2

// ServiceAddress is an alias to the protobuf-generated ServiceAddress type.
type ServiceAddress = microv1.ServiceAddress

// String returns the string representation of a ServiceAddress without the endpoint.
func String(s *ServiceAddress) string {
	return fmt.Sprintf("%s.%s", s.GetProject(), s.GetServiceId())
}

// Endpoint returns the full service address including the endpoint.
func Endpoint(s *ServiceAddress, endpoint string) string {
	return fmt.Sprintf("%s.%s", String(s), endpoint)
}

// ParseAddress parses a string into a ServiceAddress.
// The format should be "<project>.<service_id>".
func ParseAddress(address string) (*ServiceAddress, error) {
	parts := strings.Split(address, ".")
	if len(parts) != addressTokens {
		return nil, eris.Wrapf(ErrInvalidAddress, "address must have %d parts but got %d", addressTokens, len(parts))
	}
	return &ServiceAddress{
		Project:   parts[0],
		ServiceId: parts[1],
	}, nil
}

// GetAddress creates a new ServiceAddress with the given project and service ID.
func GetAddress(project string, serviceID ServiceID) *ServiceAddress {
	return &ServiceAddress{
		Project:   project,
		ServiceId: serviceID,
	}
}
