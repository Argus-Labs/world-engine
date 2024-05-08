package engine

type GetWorldResponse struct {
	Namespace  string        `json:"namespace"`
	Components []FieldDetail `json:"components"` // list of component names
	Messages   []FieldDetail `json:"messages"`
	Queries    []FieldDetail `json:"queries"`
}

type FieldDetail struct {
	Name   string         `json:"name"`   // name of the message or query
	Fields map[string]any `json:"fields"` // variable name and type
	URL    string         `json:"url,omitempty"`
}
