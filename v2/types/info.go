package types

type WorldInfo struct {
	Namespace  string
	Components []ComponentInfo
	Messages   []EndpointInfo
	Queries    []EndpointInfo
}

// EndpointInfo provides metadata information about a message or query.
type EndpointInfo struct {
	Name   string         `json:"name"`   // name of the message or query
	Fields map[string]any `json:"fields"` // property name and type
	URL    string         `json:"url,omitempty"`
}

// ComponentInfo provides metadata information about a component.
type ComponentInfo struct {
	Name   string         `json:"name"`   // name of the component
	Fields map[string]any `json:"fields"` // property name and type
}
