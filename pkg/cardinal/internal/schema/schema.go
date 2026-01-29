package schema

import (
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/types/known/structpb"
)

// TODO: should we just encode JSON []byte instead of using proto struct?
// TODO: JSON converts u64s to f64 which loses precision of some types. Consider other serialization
// format or create a custom one.

// Serializable is the interface that all user-defined types (components, commands, events) must implement.
type Serializable interface {
	Name() string
}

// ToProtoStruct converts a Serializable to a protobuf struct.
func ToProtoStruct(s Serializable) (*structpb.Struct, error) {
	bytes, err := json.Marshal(s)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal schema")
	}

	var m map[string]any
	if err := json.Unmarshal(bytes, &m); err != nil {
		return nil, eris.Wrap(err, "failed to unmarshal schema to map[string]any")
	}

	pbStruct, err := structpb.NewStruct(m)
	if err != nil {
		return nil, eris.Wrap(err, "failed to convert map to structpb.Struct")
	}

	return pbStruct, nil
}
