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
)

var (
	// ErrorSignatureValidationFailed is returned when a signature is not valid.
	ErrorSignatureValidationFailed = errors.New("signature validation failed")
	ErrorCannotSignEmptyBody       = errors.New("cannot sign empty body")
	ErrorInvalidPersonaTag         = errors.New("invalid persona tag")
	ErrorInvalidNamespace          = errors.New("invalid namespace")
)

const SystemPersonaTag = "SystemPersonaTag"

type SignedPayload struct {
	PersonaTag string
	Namespace  string
	Nonce      uint64
	Signature  string          // hex encoded string
	Body       json.RawMessage // json string
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
	return s, nil
}

// newSignedAny uses the given private key to sign the personaTag, namespace, nonce, and data.
func newSignedAny(pk *ecdsa.PrivateKey, personaTag, namespace string, nonce uint64, data any) (*SignedPayload, error) {
	if data == nil || reflect.ValueOf(data).IsZero() {
		return nil, ErrorCannotSignEmptyBody
	}
	if len(namespace) == 0 {
		return nil, ErrorInvalidNamespace
	}

	bz, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	if len(bz) == 0 {
		return nil, ErrorCannotSignEmptyBody
	}
	sp := &SignedPayload{
		PersonaTag: personaTag,
		Namespace:  namespace,
		Nonce:      nonce,
		Body:       bz,
	}
	hash, err := sp.hash()
	if err != nil {
		return nil, err
	}
	buf, err := crypto.Sign(hash, pk)
	if err != nil {
		return nil, err
	}
	sp.Signature = common.Bytes2Hex(buf)
	return sp, nil

}

// NewSystemSignedPayload signs a given body, and nonce with the given private key using the SystemPersonaTag
func NewSystemSignedPayload(pk *ecdsa.PrivateKey, namespace string, nonce uint64, data any) (*SignedPayload, error) {
	return newSignedAny(pk, SystemPersonaTag, namespace, nonce, data)
}

// NewSignedPayload signs a given body, tag, and nonce with the given private key.
func NewSignedPayload(pk *ecdsa.PrivateKey, personaTag, namespace string, nonce uint64, data any) (*SignedPayload, error) {
	if len(personaTag) == 0 || personaTag == SystemPersonaTag {
		return nil, ErrorInvalidPersonaTag
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

// Verify verifies this SignedPayload has a valid signature. If nil is returned, the signature is valid.
// Signature verification follows the pattern in crypto.TestSign:
// https://github.com/ethereum/go-ethereum/blob/master/crypto/crypto_test.go#L94
// TODO: Review this signature verification, and compare it to geth's sig verification
func (s *SignedPayload) Verify(hexAddress string) error {
	addr := common.HexToAddress(hexAddress)

	hash, err := s.hash()
	if err != nil {
		return err
	}
	signerPubKey, err := crypto.SigToPub(hash, common.Hex2Bytes(s.Signature))
	if err != nil {
		return err
	}
	signerAddr := crypto.PubkeyToAddress(*signerPubKey)
	if signerAddr != addr {
		return ErrorSignatureValidationFailed
	}
	return nil
}

func (s *SignedPayload) hash() ([]byte, error) {
	hash := crypto.NewKeccakState()
	if _, err := hash.Write([]byte(s.PersonaTag)); err != nil {
		return nil, err
	}
	if _, err := hash.Write([]byte(s.Namespace)); err != nil {
		return nil, err
	}
	if _, err := hash.Write([]byte(fmt.Sprintf("%d", s.Nonce))); err != nil {
		return nil, err
	}
	if _, err := hash.Write([]byte(s.Body)); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}
