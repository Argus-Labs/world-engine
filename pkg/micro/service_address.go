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
// <realm>.<organization>.<project>.<service_id>.<endpoint>
//
// <realm> is one of the following:
// - internal: Reserved for internal services
// - world: Publicly accessible services that is a part of Worldbase network
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
// - internal.argus.platform.gateway-us-west-2.micro.ping
// - world.argus.rampage.lobby-1.shard.message.player.connect

// Realm represents the access scope of the service.
type Realm = microv1.ServiceAddress_Realm

const (
	RealmUnspecified = microv1.ServiceAddress_REALM_UNSPECIFIED // Should not be used
	RealmInternal    = microv1.ServiceAddress_REALM_INTERNAL    // Reserved for internal services
	RealmWorld       = microv1.ServiceAddress_REALM_WORLD       // Publicly accessible services that are
	// part of Worldbase network.
)

// ServiceID represents a unique identifier for a service.
type ServiceID = string

var (
	ErrInvalidAddress = eris.New("invalid service address format")
)

// ServiceAddress is an alias to the protobuf-generated ServiceAddress type.
type ServiceAddress = microv1.ServiceAddress

// String returns the string representation of a ServiceAddress without the endpoint.
func String(s *ServiceAddress) string {
	return fmt.Sprintf("%s.%s.%s.%s.%s", s.Region, realmToString(s.Realm), s.Organization, s.Project, s.ServiceId)
}

// realmToString converts a Realm (microv1.ServiceAddress_Realm) to its string representation.
func realmToString(realm Realm) string {
	// The protobuf enum name is in the format REALM_X, we want to extract just X and lowercase it
	enumStr := realm.String()
	if strings.HasPrefix(enumStr, "REALM_") {
		// Remove "REALM_" prefix and convert to lowercase
		return strings.ToLower(strings.TrimPrefix(enumStr, "REALM_"))
	}
	return "unspecified"
}

// stringToRealm converts a string to a Realm (microv1.ServiceAddress_Realm).
func stringToRealm(str string) Realm {
	// Convert string to uppercase and add REALM_ prefix to match enum name format
	enumName := "REALM_" + strings.ToUpper(str)

	// Try to find the enum value by name
	enumValues := microv1.ServiceAddress_Realm_value
	if val, ok := enumValues[enumName]; ok {
		return Realm(val)
	}

	return RealmUnspecified
}

// Endpoint returns the full service address including the endpoint.
func Endpoint(s *ServiceAddress, endpoint string) string {
	return fmt.Sprintf("%s.%s", String(s), endpoint)
}

// ParseAddress parses a string into a ServiceAddress.
// The format should be "<region>.<realm>.<organization>.<project>.<service_id>".
func ParseAddress(address string) (*ServiceAddress, error) {
	parts := strings.Split(address, ".")
	if len(parts) != 5 {
		return nil, eris.Wrapf(ErrInvalidAddress, "address must have 5 parts but got %d", len(parts))
	}

	realm := stringToRealm(parts[1])
	if realm == microv1.ServiceAddress_REALM_UNSPECIFIED {
		return nil, eris.Wrapf(ErrInvalidAddress, "unknown realm '%s'", parts[1])
	}

	return &ServiceAddress{
		Region:       parts[0],
		Realm:        realm,
		Organization: parts[2],
		Project:      parts[3],
		ServiceId:    parts[4],
	}, nil
}

// GetAddress creates a new ServiceAddress with the given realm, organization, project, and service ID.
func GetAddress(region string, realm Realm, organization, project string, serviceID ServiceID) *ServiceAddress {
	return &ServiceAddress{
		Region:       region,
		Realm:        realm,
		Organization: organization,
		Project:      project,
		ServiceId:    serviceID,
	}
}
