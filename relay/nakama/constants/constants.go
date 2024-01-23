package constants

const (
	CardinalCollection = "cardinalCollection"
	PersonaTagKey      = "personaTag"

	TransactionEndpointPrefix = "/tx"

	EnvCardinalAddr      = "CARDINAL_ADDR"
	EnvCardinalNamespace = "CARDINAL_NAMESPACE"
)

var (
	ListEndpoints               = "query/http/endpoints"
	TransactionReceiptsEndpoint = "query/receipts/list"
	EventEndpoint               = "events"

	GlobalCardinalAddress string
	GlobalNamespace       string
)
