// Package sign allows for the cryptographic signing and verification an arbitrary payload.
package sign

import (
	"bytes"
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"
	"reflect"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/mitchellh/mapstructure"
)

var (
	// ErrSignatureValidationFailed is returned when a signature is not valid.
	ErrSignatureValidationFailed = errors.New("signature validation failed")
	ErrCannotSignEmptyBody       = errors.New("cannot sign empty body")
	ErrInvalidPersonaTag         = errors.New("invalid persona tag")
	ErrInvalidNamespace          = errors.New("invalid namespace")

	zeroHashHex = common.Hash{}.Hex()
)

// SystemPersonaTag is a reserved persona tag for transaction. It is used in transactions when a PersonaTag
// does not actually exist (e.g. during the PersonaTag creation process).
const SystemPersonaTag = "SystemPersonaTag"

type SignedPayload struct {
	PersonaTag string          `json:"personaTag"`
	Namespace  string          `json:"namespace"`
	Nonce      uint64          `json:"nonce"`
	Signature  string          `json:"signature"` // hex encoded string
	Hash       common.Hash     `json:"hash,omitempty"`
	Body       json.RawMessage `json:"body"` // json string
}

func UnmarshalSignedPayload(bz []byte) (*SignedPayload, error) {
	s := new(SignedPayload)
	dec := json.NewDecoder(bytes.NewBuffer(bz))
	dec.DisallowUnknownFields()

	if err := dec.Decode(s); err != nil {
		return nil, fmt.Errorf("error decoding SignedPayload: %w", err)
	}

	// ensure that all fields are present. we could do this via reflection, but checking directly is faster than
	// using reflection package.
	if s.PersonaTag == "" {
		return nil, errors.New("SignerPayload must contain PersonaTag field")
	}
	if s.Namespace == "" {
		return nil, errors.New("SignerPayload must contain Namespace field")
	}
	if s.Signature == "" {
		return nil, errors.New("SignerPayload must contain Signature field")
	}
	if len(s.Body) == 0 {
		return nil, errors.New("SignerPayload must contain Body field")
	}
	s.populateHash()
	return s, nil
}

// MappedSignedPayload Identical to UnmarshalSignedPayload but takes a payload in the form of map[string]any.
func MappedSignedPayload(payload map[string]interface{}) (*SignedPayload, error) {
	s := new(SignedPayload)
	signedPayloadKeys := map[string]bool{
		"personaTag": true,
		"namespace":  true,
		"signature":  true,
		"nonce":      true,
		"body":       true,
		"hash":       true,
	}
	for key := range payload {
		if !signedPayloadKeys[key] {
			return nil, fmt.Errorf("invalid field: %s in body", key)
		}
	}
	// json.Marshal will encode an empty body to "null", so verify the body exists before attempting to Marshal it.
	if _, ok := payload["body"]; !ok {
		return nil, errors.New("SignerPayload must contain Body field")
	}
	serializedBody, err := json.Marshal(payload["body"])
	if err != nil {
		return nil, err
	}
	delete(payload, "hash")
	delete(payload, "body")
	err = mapstructure.Decode(payload, s)
	if err != nil {
		return nil, err
	}
	s.Body = serializedBody
	// ensure that all fields are present. we could do this via reflection, but checking directly is faster than
	// using reflection package.
	if s.PersonaTag == "" {
		return nil, errors.New("SignerPayload must contain PersonaTag field")
	}
	if s.Namespace == "" {
		return nil, errors.New("SignerPayload must contain Namespace field")
	}
	if s.Signature == "" {
		return nil, errors.New("SignerPayload must contain Signature field")
	}
	if len(s.Body) == 0 {
		return nil, errors.New("SignerPayload must contain Body field")
	}
	s.populateHash()
	return s, nil
}

// normalizeJSON marshals the given data object. If data is a string or bytes, the json format is verified
// and any extraneous spaces are removed. Otherwise, the given data is run through json.Marshal.
func normalizeJSON(data any) ([]byte, error) {
	var asBuf []byte
	if v, ok := data.(string); ok {
		asBuf = []byte(v)
	} else if v, ok2 := data.([]byte); ok2 {
		asBuf = v
	}
	if asBuf == nil {
		// The given data was neither a string nor a []byte. Just json.Marshal it.
		return json.Marshal(data)
	}

	if !json.Valid(asBuf) {
		return nil, fmt.Errorf("data %q is not valid json", string(asBuf))
	}

	dst := &bytes.Buffer{}

	// JSON strings need to be compacted (insignificant whitespace removed).
	// This is required because when the signed payload is serialized/deserialized those spaces will also
	// be lost. If they are not removed beforehand, the hashes of the message before serialization and after
	// will be different.
	if err := json.Compact(dst, asBuf); err != nil {
		return nil, err
	}
	return dst.Bytes(), nil
}

// newSignedAny uses the given private key to sign the personaTag, namespace, nonce, and data.
func newSignedAny(pk *ecdsa.PrivateKey, personaTag, namespace string, nonce uint64, data any) (*SignedPayload, error) {
	if data == nil || reflect.ValueOf(data).IsZero() {
		return nil, ErrCannotSignEmptyBody
	}
	if len(namespace) == 0 {
		return nil, ErrInvalidNamespace
	}
	bz, err := normalizeJSON(data)
	if err != nil {
		return nil, err
	}
	if len(bz) == 0 {
		return nil, ErrCannotSignEmptyBody
	}
	sp := &SignedPayload{
		PersonaTag: personaTag,
		Namespace:  namespace,
		Nonce:      nonce,
		Body:       bz,
	}
	sp.populateHash()
	buf, err := crypto.Sign(sp.Hash.Bytes(), pk)
	if err != nil {
		return nil, err
	}
	sp.Signature = common.Bytes2Hex(buf)
	return sp, nil
}

// NewSystemSignedPayload signs a given body, and nonce with the given private key using the SystemPersonaTag.
func NewSystemSignedPayload(pk *ecdsa.PrivateKey, namespace string, nonce uint64, data any) (*SignedPayload, error) {
	return newSignedAny(pk, SystemPersonaTag, namespace, nonce, data)
}

// NewSignedPayload signs a given body, tag, and nonce with the given private key.
func NewSignedPayload(pk *ecdsa.PrivateKey,
	personaTag,
	namespace string,
	nonce uint64,
	data any,
) (*SignedPayload, error) {
	if len(personaTag) == 0 || personaTag == SystemPersonaTag {
		return nil, ErrInvalidPersonaTag
	}
	return newSignedAny(pk, personaTag, namespace, nonce, data)
}

func (s *SignedPayload) IsSystemPayload() bool {
	return s.PersonaTag == SystemPersonaTag
}

// Marshal serializes this SignedPayload to bytes, which can then be passed in to Unmarshal.
func (s *SignedPayload) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

func isZeroHash(hash common.Hash) bool {
	return hash.Hex() == zeroHashHex
}

// HashHex return a hex encoded hash of the signature.
func (s *SignedPayload) HashHex() string {
	if isZeroHash(s.Hash) {
		s.populateHash()
	}
	return s.Hash.Hex()
}

// Verify verifies this SignedPayload has a valid signature. If nil is returned, the signature is valid.
// Signature verification follows the pattern in crypto.TestSign:
// https://github.com/ethereum/go-ethereum/blob/master/crypto/crypto_test.go#L94
// TODO: Review this signature verification, and compare it to geth's sig verification
func (s *SignedPayload) Verify(hexAddress string) error {
	addr := common.HexToAddress(hexAddress)

	if isZeroHash(s.Hash) {
		s.populateHash()
	}
	signerPubKey, err := crypto.SigToPub(s.Hash.Bytes(), common.Hex2Bytes(s.Signature))
	if err != nil {
		return err
	}
	signerAddr := crypto.PubkeyToAddress(*signerPubKey)
	if signerAddr != addr {
		return ErrSignatureValidationFailed
	}
	return nil
}

func (s *SignedPayload) populateHash() {
	s.Hash = crypto.Keccak256Hash(
		[]byte(s.PersonaTag),
		[]byte(s.Namespace),
		[]byte(fmt.Sprintf("%d", s.Nonce)),
		s.Body,
	)
}
