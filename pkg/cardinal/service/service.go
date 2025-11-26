package service

import (
	"context"
	"encoding/hex"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/argus-labs/world-engine/pkg/cardinal/ecs"
	"github.com/argus-labs/world-engine/pkg/cardinal/protoutil"
	"github.com/argus-labs/world-engine/pkg/micro"
	"github.com/argus-labs/world-engine/pkg/sign"
	"github.com/argus-labs/world-engine/pkg/telemetry"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/goccy/go-json"
	"github.com/nats-io/nats.go"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// ShardService extends micro.Service with Cardinal specific functionality.
type ShardService struct {
	*micro.Service

	client    *micro.Client        // NATS client
	world     *ecs.World           // Reference to the ECS world
	tel       *telemetry.Telemetry // Telemetry for logging and tracing
	signer    sign.Signer          // Inter-shard command signer
	queryPool sync.Pool            // Pool for query objects
	personaID string               // Registered persona ID from registry
}

// NewShardService creates a new shard service.
func NewShardService(opts ShardServiceOptions) (*ShardService, error) {
	if err := opts.Validate(); err != nil {
		return nil, eris.Wrap(err, "invalid options passed")
	}

	s := &ShardService{
		world:  opts.World,
		client: opts.Client,
		tel:    opts.Telemetry,
		queryPool: sync.Pool{
			New: func() any {
				return &Query{
					// Pre-allocate space for 8 components which should cover most cases.
					Find: make([]string, 0, 8),
				}
			},
		},
	}

	// TODO: make seed configurable
	signer, err := sign.NewSigner(opts.PrivateKey, 42)
	if err != nil {
		return nil, eris.Wrap(err, "failed to create signer from private key")
	}
	s.signer = signer

	service, err := micro.NewService(opts.Client, opts.Address, opts.Telemetry)
	if err != nil {
		return s, eris.Wrap(err, "failed to create micro service")
	}

	s.Service = service

	if err := s.registerEndpoints(); err != nil {
		return s, eris.Wrap(err, "failed to register endpoints")
	}

	// Try to load persisted persona ID first to avoid re-registration errors.
	if personaID, err := loadPersistedPersonaID(); err == nil && personaID != "" {
		s.personaID = personaID
		s.tel.Logger.Info().Str("personaID", personaID).Msg("loaded persisted persona ID")
	} else {
		if err != nil {
			s.tel.Logger.Debug().Err(err).Msg("failed reading persona file")
		}
		// No persisted persona ID found, register with the registry service.
		if err := s.registerShard(opts.Address, signer.GetSignerAddress(), opts.DisablePersona); err != nil {
			return s, eris.Wrap(err, "failed register shard with registry")
		}
	}

	return s, nil
}

// RegisterEndpoints registers the service endpoints for handling requests.
func (s *ShardService) registerEndpoints() error {
	err := s.AddEndpoint("query", s.handleQuery)
	if err != nil {
		return eris.Wrap(err, "failed to register query handler")
	}
	return nil
}

// registerShard registers this shard persona with the registry service using commands with
// exponential backoff retry.
func (s *ShardService) registerShard(address *micro.ServiceAddress, signerAddress []byte, disablePersona bool) error {
	if disablePersona {
		s.personaID = strings.Repeat("0", 64) // Dummy persona
		return nil
	}

	registryAddress := micro.GetAddress("us-west-2", micro.RealmInternal, "argus", "platform", "registry")

	commandPayload := RegisterPersonaCommand{
		SignerAddress: signerAddress,
		SenderAddress: address,
	}
	commandBody, err := protoutil.MarshalCommand(commandPayload, registryAddress, "0") // persona isn't needed here
	if err != nil {
		return eris.Wrap(err, "failed to marshal register persona command")
	}

	command, err := s.signer.SignCommand(commandBody, iscv1.AuthInfo_AUTH_MODE_DIRECT)
	if err != nil {
		return eris.Wrap(err, "failed to sign register persona command")
	}

	signatureHex := hex.EncodeToString(command.GetSignature())
	subject := fmt.Sprintf("%s.reply.%s", micro.String(address), signatureHex)

	sub, err := s.client.SubscribeSync(subject)
	if err != nil {
		return eris.Wrap(err, "failed to subscribe to register persona receipt")
	}

	// We expect only 1 response, so we can automatically unsubscribe after receiving it.
	if err := sub.AutoUnsubscribe(1); err != nil {
		return eris.Wrap(err, "failed to auto unsubscribe to subject")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	registerPersonaEndpoint := "command." + commandPayload.Name()

	_, err = s.client.Request(ctx, registryAddress, registerPersonaEndpoint, command)
	if err != nil {
		return eris.Wrap(err, "failed to send register-persona command to registry")
	}

	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		return eris.Wrap(err, "failed to receive register-persona response")
	}

	personaID, err := unmarshalResponse(msg)
	if err != nil {
		return eris.Wrap(err, "failed to unmarshal response")
	}

	// Store persona ID in service.
	s.personaID = personaID

	return savePersistedPersonaID(personaID)
}

func unmarshalResponse(msg *nats.Msg) (string, error) {
	var result RegisterPersonaEvent

	// Unmarshal as iscv1.Event.
	var event iscv1.Event
	if err := proto.Unmarshal(msg.Data, &event); err != nil {
		return "", eris.Wrap(err, "failed to unmarshal event")
	}

	// Extract payload from the event and convert to JSON
	payload := event.GetPayload()
	if payload == nil {
		return "", eris.New("event payload is nil")
	}

	jsonBytes, err := payload.MarshalJSON()
	if err != nil {
		return "", eris.Wrap(err, "failed to marshal payload to json")
	}

	// Unmarshal JSON to RegisterPersonaResponse.
	if err := json.Unmarshal(jsonBytes, &result); err != nil {
		return "", eris.Wrap(err, "failed to unmarshal to RegisterPersonaResponse")
	}

	if result.Error != "" {
		return "", eris.New(result.Error)
	}

	return result.PersonaID, nil
}

const personaFilePath = "/etc/cardinal/persona"

// loadPersistedPersonaID loads the persona ID from the file if it exists.
func loadPersistedPersonaID() (string, error) {
	data, err := os.ReadFile(personaFilePath)
	if err != nil {
		return "", err
	}

	return strings.TrimSpace(string(data)), nil
}

// savePersistedPersonaID saves the persona ID to the file atomically.
func savePersistedPersonaID(personaID string) error {
	// Create directory if it doesn't exist.
	if err := os.MkdirAll(filepath.Dir(personaFilePath), 0755); err != nil {
		return eris.Wrap(err, "failed to create persona directory")
	}

	tempPath := personaFilePath + ".tmp"
	if err := os.WriteFile(tempPath, []byte(personaID), 0600); err != nil {
		return eris.Wrap(err, "failed to write temp persona file")
	}

	if err := os.Rename(tempPath, personaFilePath); err != nil {
		return eris.Wrap(err, "failed to rename persona file")
	}

	return nil
}

// -------------------------------------------------------------------------------------------------
// Options
// -------------------------------------------------------------------------------------------------

// ShardServiceOptions contains the configuration for creating a new ShardService.
type ShardServiceOptions struct {
	Client         *micro.Client         // NATS client for inter-service communication
	Address        *micro.ServiceAddress // This Cardinal shard's service address
	World          *ecs.World            // Reference to the ECS world
	Telemetry      *telemetry.Telemetry  // Telemetry for logging and tracing
	PrivateKey     string                // Private key for signing inter-shard commands
	DisablePersona bool
}

// Validate checks that all required fields in ShardServiceOptions are not nil.
func (opts ShardServiceOptions) Validate() error {
	if opts.Client == nil {
		return eris.New("client cannot be nil")
	}
	if opts.Address == nil {
		return eris.New("address cannot be nil")
	}
	if opts.World == nil {
		return eris.New("world cannot be nil")
	}
	if opts.Telemetry == nil {
		return eris.New("telemetry cannot be nil")
	}
	if opts.PrivateKey == "" {
		return eris.New("private key cannot be an empty string")
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Registry types
// -------------------------------------------------------------------------------------------------

// TODO: use the actual type from the registry's package once that is open-sourced so we don't have
// to duplicate this code which can lead to inconsistencies.
const registerPersona = "register-persona"

// RegisterPersonaCommand represents a command to register a new persona in the registry.
type RegisterPersonaCommand struct {
	SignerAddress []byte                // Address of the authorized signer for this persona
	SenderAddress *micro.ServiceAddress // Service address that initiated the registration request
}

func (RegisterPersonaCommand) Name() string {
	return registerPersona
}

// RegisterPersonaEvent represents the result of a persona registration attempt.
type RegisterPersonaEvent struct {
	PersonaID string // Unique identifier of the registered persona
	Error     string // Error message if registration failed
}

// Name returns the event name for routing.
func (RegisterPersonaEvent) Name() string {
	return registerPersona
}
