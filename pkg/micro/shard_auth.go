package micro

import (
	"bytes"
	"context"
	"slices"
	"sync"
	"time"

	"buf.build/go/protovalidate"
	"github.com/argus-labs/world-engine/pkg/sign"
	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	registryv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/registry/v1"
	"github.com/coocood/freecache"
	ics23 "github.com/cosmos/ics23/go"
	"github.com/goccy/go-json"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
)

// maxCommandTTL is the duration after which a command is considered expired.
const maxCommandTTL = 120 * time.Second

// clockDriftTolerance is the maximum allowed clock drift in the future.
const clockDriftTolerance = 2 * time.Second

// cacheRetentionExtra is the extra time to keep a command in the replay cache after it has expired.
const cacheRetentionExtra = 10 * time.Second

// commandVerifier handles verification of commands including signature validation, TTL checks, and replay protection.
type commandVerifier struct {
	cache   *freecache.Cache // Cache for replay attack protection (leader mode only)
	shard   *Shard           // Reference to the shard
	address *ServiceAddress  // This shard's address

	// Persona mode specific fields.
	client   *Client                           // NATS client
	personas map[string]personaWithMerkleProof // Cached personas with their merkle proofs
	mu       sync.RWMutex                      // Mutex for protecting persona cache access
}

// newCommandVerifer returns a new command verifier.
func newCommandVerifer(shard *Shard, cacheSize int, address *ServiceAddress, client *Client) *commandVerifier {
	var cache *freecache.Cache
	if shard.Mode() == ModeLeader {
		cache = freecache.NewCache(cacheSize)
	}

	return &commandVerifier{
		cache:    cache,
		shard:    shard,
		address:  address,
		client:   client,
		personas: make(map[string]personaWithMerkleProof),
	}
}

// VerifyCommand verifies a command. It expects you to already protovalidate the command.
func (c *commandVerifier) VerifyCommand(command *iscv1.Command) error {
	commandRaw := &iscv1.CommandRaw{}
	if err := proto.Unmarshal(command.GetCommandBytes(), commandRaw); err != nil {
		return eris.Wrap(err, "failed to unmarshal command bytes")
	}
	if err := protovalidate.Validate(commandRaw); err != nil {
		return eris.Wrap(err, "failed to validate raw command")
	}

	if String(c.address) != String(commandRaw.GetBody().GetAddress()) {
		return eris.New("command address doesn't match shard address")
	}

	if err := c.validateTTL(command, commandRaw); err != nil {
		return eris.Wrap(err, "failed to validate command TTL")
	}

	switch command.GetAuthInfo().GetMode() {
	case iscv1.AuthInfo_AUTH_MODE_DIRECT:
		if err := c.verifyDirect(command); err != nil {
			return eris.Wrap(err, "failed to verify direct command")
		}
	case iscv1.AuthInfo_AUTH_MODE_PERSONA:
		if err := c.verifyPersona(command, commandRaw); err != nil {
			return eris.Wrap(err, "failed to verify persona command")
		}
	case iscv1.AuthInfo_AUTH_MODE_UNSPECIFIED:
		return eris.New("unspecified command auth mode")
	}

	// Add command to replay cache after successful verification (leader mode only).
	if c.shard.Mode() == ModeLeader {
		expirySeconds := int((maxCommandTTL + cacheRetentionExtra).Seconds())
		if err := c.cache.Set(command.GetSignature(), []byte{}, expirySeconds); err != nil {
			return eris.Wrap(err, "failed to set command in replay cache")
		}
	}

	return nil
}

// validateTTL validates command expiration and checks for replay attacks.
func (c *commandVerifier) validateTTL(command *iscv1.Command, commandRaw *iscv1.CommandRaw) error {
	if c.shard.Mode() == ModeFollower {
		return nil
	}

	now := time.Now()
	timestamp := commandRaw.GetTimestamp().AsTime()

	if now.After(timestamp.Add(maxCommandTTL)) {
		return eris.New("command has expired")
	}

	if timestamp.After(now.Add(clockDriftTolerance)) {
		return eris.Errorf("command timestamp is more than %s in the future", clockDriftTolerance)
	}

	// Check for replay attack (cache should not be nil in leader mode).
	if _, err := c.cache.Get(command.GetSignature()); err == nil {
		return eris.New("replay attack detected")
	}

	return nil
}

// -------------------------------------------------------------------------------------------------
// Direct mode verification
// -------------------------------------------------------------------------------------------------

// verifyDirect verifies commands using direct signature verification.
func (c *commandVerifier) verifyDirect(command *iscv1.Command) error {
	if !sign.VerifyCommandSignature(command) {
		return eris.New("invalid signature")
	}
	return nil
}

// -------------------------------------------------------------------------------------------------
// Persona mode verification
// -------------------------------------------------------------------------------------------------

// PersonaDetails contains the details of a persona including its authorized signers.
type PersonaDetails struct {
	Signers [][]byte // List of authorized signer addresses for this persona
}

// Marshal marshals PersonaDetails to JSON bytes.
func (p *PersonaDetails) Marshal() ([]byte, error) {
	return json.Marshal(p)
}

// Unmarshal unmarshals JSON bytes into PersonaDetails.
func (p *PersonaDetails) Unmarshal(data []byte) error {
	return json.Unmarshal(data, p)
}

// personaWithMerkleProof represents a cached persona entry with its merkle proof for verification.
type personaWithMerkleProof struct {
	ID      string                 // Unique identifier of the persona
	Signers [][]byte               // List of authorized signer addresses for this persona
	TTL     time.Time              // Time until when this registry entry can be trusted
	Version int64                  // Registry version when this persona was retrieved
	Proof   *ics23.CommitmentProof // Merkle proof for verifying persona authenticity
	Root    []byte                 // Merkle root hash used for proof verification
}

// verifyPersona verifies commands using persona-based authentication with merkle proofs.
func (c *commandVerifier) verifyPersona(command *iscv1.Command, commandRaw *iscv1.CommandRaw) error {
	if c.shard.IsDisablePersona() {
		return nil
	}

	if !sign.VerifyCommandSignature(command) {
		return eris.New("invalid signature")
	}

	personaWithProof, err := c.getPersonaWithProof(commandRaw)
	if err != nil {
		return eris.Wrap(err, "failed to get persona")
	}

	valid, err := c.verifyMerkleProof(personaWithProof)
	if err != nil {
		return eris.Wrap(err, "failed to verify merkle proof")
	}
	if !valid {
		return eris.New("invalid merkle proof")
	}

	// Add valid persona to the cache.
	c.cachePersona(personaWithProof)

	signer := command.GetAuthInfo().GetSignerAddress()
	if !slices.ContainsFunc(
		personaWithProof.Signers,
		func(authorizedSigner []byte) bool { return bytes.Equal(signer, authorizedSigner) },
	) {
		personaID := commandRaw.GetBody().GetPersona().GetId()
		return eris.Errorf("%x is not an authorized signer for persona %s", signer, personaID)
	}
	return nil
}

// getPersonaWithProof retrieves persona details with merkle proof from cache or registry.
func (c *commandVerifier) getPersonaWithProof(commandRaw *iscv1.CommandRaw) (personaWithMerkleProof, error) {
	persona := commandRaw.GetBody().GetPersona()

	c.mu.RLock()
	cachedPersona, exists := c.personas[persona.GetId()]
	c.mu.RUnlock()

	if exists && time.Now().Before(cachedPersona.TTL) {
		return cachedPersona, nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	registryAddress := GetAddress("us-west-2", RealmInternal, "argus", "platform", "registry")
	request := &registryv1.QueryPersonaRequest{
		Persona:   persona,
		Timestamp: commandRaw.GetTimestamp(),
	}

	response, err := c.client.Request(ctx, registryAddress, "query.persona", request)
	if err != nil {
		return personaWithMerkleProof{}, eris.Wrap(err, "failed to fetch persona from registry")
	}

	var queryResponse registryv1.QueryPersonaResponse
	if err := response.GetPayload().UnmarshalTo(&queryResponse); err != nil {
		return personaWithMerkleProof{}, eris.Wrap(err, "failed to unmarshal query persona response")
	}
	if err := protovalidate.Validate(&queryResponse); err != nil {
		return personaWithMerkleProof{}, eris.Wrap(err, "failed to validate query persona response")
	}

	personaWithProof := personaWithMerkleProof{
		ID:      persona.GetId(),
		Signers: queryResponse.GetDetails().GetSigners(),
		TTL:     queryResponse.GetExpiresAt().AsTime(),
		Version: queryResponse.GetVersion(),
		Proof:   queryResponse.GetProof(),
		Root:    queryResponse.GetRoot(),
	}
	return personaWithProof, nil
}

// verifyMerkleProof verifies the merkle proof for persona authenticity.
func (c *commandVerifier) verifyMerkleProof(personaWithProof personaWithMerkleProof) (bool, error) {
	details := PersonaDetails{Signers: personaWithProof.Signers}
	detailsBytes, err := details.Marshal()
	if err != nil {
		return false, eris.Wrap(err, "failed to marshal persona details")
	}

	return ics23.VerifyMembership(
		ics23.IavlSpec,
		personaWithProof.Root,
		personaWithProof.Proof,
		[]byte(personaWithProof.ID),
		detailsBytes,
	), nil
}

// cachePersona stores the persona with proof in the local cache.
func (c *commandVerifier) cachePersona(personaWithProof personaWithMerkleProof) {
	c.mu.Lock()
	defer c.mu.Unlock()

	c.personas[personaWithProof.ID] = personaWithProof
}
