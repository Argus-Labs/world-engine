package redis

import (
	"os"

	"github.com/redis/go-redis/v9"
	"github.com/rs/zerolog"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/anypb"

	"github.com/argus-labs/world-engine/cardinal/ecs/component"
	"github.com/argus-labs/world-engine/cardinal/ecs/storage"
)

type Storage struct {
	registry               storage.TypeRegistry
	worldID                string
	componentStoragePrefix string
	Client                 *redis.Client
	log                    zerolog.Logger
}

// Options makes DevEx cleaner by proxying the actual redis options struct
// With this, the developer doesn't need to import Redis libraries on their game logic implementation.
type Options struct {
	// host:port address.
	Addr string

	// Use the specified Username to authenticate the current connection
	// with one of the connections defined in the ACL list when connecting
	// to a Redis 6.0 instance, or greater, that is using the Redis ACL system.
	Username string

	// Optional password. Must match the password specified in the
	// requirepass server configuration option (if connecting to a Redis 5.0 instance, or lower),
	// or the User Password when connecting to a Redis 6.0 instance, or greater,
	// that is using the Redis ACL system.
	Password string

	// Database to be selected after connecting to the server.
	DB int
}

func NewStorage(options Options, worldID string) Storage {
	return Storage{
		worldID: worldID,
		Client: redis.NewClient(&redis.Options{
			Addr:     options.Addr,
			Username: options.Username,
			Password: options.Password,
			DB:       options.DB,
		}),
		log: zerolog.New(os.Stdout),
	}
}

// ---------------------------------------------------------------------------
// 								UTILITIES
// ---------------------------------------------------------------------------

func marshalProto(msg proto.Message) ([]byte, error) {
	bz, err := proto.Marshal(msg)
	return bz, err
}

func unmarshalProto(bz []byte, msg proto.Message) error {
	return proto.Unmarshal(bz, msg)
}

// encode encodes the component type to anypb.Any, then proto marshals it to []byte.
func (r *Storage) encode(c component.IComponentType) ([]byte, error) {
	comp := c.ProtoReflect().New()
	a, err := anypb.New(comp.Interface())
	if err != nil {
		return nil, err
	}
	bz, err := proto.Marshal(a)
	return bz, err
}

// decode decodes the bytes into anypb.Any, then will unmarshal the anypb.Any into the underlying component type.
func (r *Storage) decode(bz []byte) (component.IComponentType, error) {
	a := new(anypb.Any)
	err := proto.Unmarshal(bz, a)
	if err != nil {
		return nil, err
	}
	msg, err := anypb.UnmarshalNew(a, r.unmarshalOptions())
	if err != nil {
		return nil, err
	}
	return component.IComponentType(msg), nil
}

// unmarshalOptions returns the unmarshal options.
// NOTE: we are leaving the default value fields visible here in case they need to be changed later.
func (r *Storage) unmarshalOptions() proto.UnmarshalOptions {
	return proto.UnmarshalOptions{
		NoUnkeyedLiterals: struct{}{},
		Merge:             false,
		AllowPartial:      false,
		DiscardUnknown:    false,
		Resolver:          r.registry,
		RecursionLimit:    0,
	}
}
