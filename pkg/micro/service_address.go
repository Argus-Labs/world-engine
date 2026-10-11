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
// <region>.<organization>.<project>.<service_id>.<endpoint>
//
// <region> identifies the deployment region (e.g., "us-west1").
//
// <organization> represents the entity that owns the project (e.g., "argus").
//
// <project> is an arbitrary token to related services together (e.g., "platform", "game-rampage").
//
// <service_id> is a unique identifier for the service instance. It must be unique within the project.
//
// <endpoint> is an arbitrary token that identifies specific functionality within a service.
// An endpoint can contain . as a delimiter to leverage NATS routing.
//
// Examples:
// - us-west1.argus.rampage.gameplay-3.ping
// - us-west1.argus.rampage.gameplay-3.isc.plat_result.platform

// ServiceID represents a unique identifier for a service.
type ServiceID = string

var (
	ErrInvalidAddress = eris.New("invalid service address format")
)

// addressTokens is the number of subject tokens in a service address.
const addressTokens = 4

// ServiceAddress is an alias to the protobuf-generated ServiceAddress type.
type ServiceAddress = microv1.ServiceAddress

// String returns the string representation of a ServiceAddress without the endpoint.
func String(s *ServiceAddress) string {
	return fmt.Sprintf("%s.%s.%s.%s", s.GetRegion(), s.GetOrganization(), s.GetProject(), s.GetServiceId())
}

// Endpoint returns the full service address including the endpoint.
func Endpoint(s *ServiceAddress, endpoint string) string {
	return fmt.Sprintf("%s.%s", String(s), endpoint)
}

// ParseAddress parses a string into a ServiceAddress.
// The format should be "<region>.<organization>.<project>.<service_id>".
func ParseAddress(address string) (*ServiceAddress, error) {
	parts := strings.Split(address, ".")
	if len(parts) != addressTokens {
		return nil, eris.Wrapf(ErrInvalidAddress, "address must have %d parts but got %d", addressTokens, len(parts))
	}
	return &ServiceAddress{
		Region:       parts[0],
		Organization: parts[1],
		Project:      parts[2],
		ServiceId:    parts[3],
	}, nil
}

// GetAddress creates a new ServiceAddress with the given region, organization, project, and service ID.
func GetAddress(region, organization, project string, serviceID ServiceID) *ServiceAddress {
	return &ServiceAddress{
		Region:       region,
		Organization: organization,
		Project:      project,
		ServiceId:    serviceID,
	}
}
