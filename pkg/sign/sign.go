package sign

import (
	"crypto/ed25519"
	"encoding/hex"
	"math/rand"

	iscv1 "github.com/argus-labs/world-engine/proto/gen/go/worldengine/isc/v1"
	"github.com/rotisserie/eris"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// Signer handles signing commands using an Ed25519 private key.
type Signer struct {
	privateKey ed25519.PrivateKey
	rng        *rand.Rand
}

// NewSigner creates a new Signer instance from a hex-encoded Ed25519 private key.
//
// The private key seed must be a valid 32-byte seed encoded as a hex string. This is used to derive
// the full Ed25519 private key for signing operations.
// The rng seed is used for deterministic salt generation with math/rand.
func NewSigner(privateKeySeed string, rngSeed int64) (Signer, error) {
	var signer Signer

	key, err := hex.DecodeString(privateKeySeed)
	if err != nil {
		return signer, eris.Wrap(err, "failed to decode hex private key")
	}

	if len(key) != 32 {
		return signer, eris.New("private key must be 32 bytes")
	}

	signer.privateKey = ed25519.NewKeyFromSeed(key)
	signer.rng = rand.New(rand.NewSource(rngSeed)) //nolint:gosec // need deterministim

	return signer, nil
}

// SignCommand signs a command body to produce a signed command that can be verified.
func (s *Signer) SignCommand(commandBody *iscv1.CommandBody, mode iscv1.AuthInfo_AuthMode) (*iscv1.Command, error) {
	if commandBody == nil {
		return nil, eris.New("command body is required")
	}

	commandRaw := &iscv1.CommandRaw{
		Timestamp: timestamppb.Now(),
		Salt:      s.generateSalt(),
		Body:      commandBody,
	}

	commandBytes, err := proto.Marshal(commandRaw)
	if err != nil {
		return nil, eris.Wrap(err, "failed to marshal command")
	}

	signature := ed25519.Sign(s.privateKey, commandBytes)

	return &iscv1.Command{
		Signature: signature,
		AuthInfo: &iscv1.AuthInfo{
			Mode:          mode,
			SignerAddress: s.GetSignerAddress(),
		},
		CommandBytes: commandBytes,
	}, nil
}

// VerifyCommandSignature verifies that a command has a valid signature.
func VerifyCommandSignature(command *iscv1.Command) bool {
	return ed25519.Verify(
		command.GetAuthInfo().GetSignerAddress(),
		command.GetCommandBytes(),
		command.GetSignature(),
	)
}

func (s *Signer) generateSalt() []byte {
	salt := make([]byte, 16)
	s.rng.Read(salt)
	return salt
}

func (s *Signer) GetSignerAddress() ed25519.PublicKey {
	return s.privateKey.Public().(ed25519.PublicKey) //nolint:errcheck // it's fine
}
