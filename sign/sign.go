// Package sign allows for the cryptographic signing and verification an arbitrary payload.
package sign

import (
	"crypto/ecdsa"
	"encoding/json"
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	// ErrorSignatureValidationFailed is returned when a signature is not valid.
	ErrorSignatureValidationFailed = errors.New("signature validation failed")
)

type SignedPayload struct {
	PersonaTag string
	Namespace  string
	Nonce      uint64
	Signature  []byte
	Body       []byte
}

// Unmarshal attempts to unmarshal the given buf into a SignedPayload. SignedPayload.Verify must still
// be called to verify this signature.
func Unmarshal(buf []byte) (*SignedPayload, error) {
	sp := &SignedPayload{}
	if err := json.Unmarshal(buf, sp); err != nil {
		return nil, err
	}
	return sp, nil
}

func NewSignedString(pk *ecdsa.PrivateKey, personaTag, namespace string, nonce uint64, str string) (*SignedPayload, error) {
	sp := &SignedPayload{
		PersonaTag: personaTag,
		Namespace:  namespace,
		Nonce:      nonce,
		Body:       []byte(str),
	}
	hash, err := sp.hash()
	if err != nil {
		return nil, err
	}
	buf, err := crypto.Sign(hash, pk)
	if err != nil {
		return nil, err
	}
	sp.Signature = buf
	return sp, nil

}

// NewSignedPayload signs a given body, tag, and nonce with the given private key.
func NewSignedPayload(pk *ecdsa.PrivateKey, personaTag, namespace string, nonce uint64, data any) (*SignedPayload, error) {
	bz, err := json.Marshal(data)
	if err != nil {
		return nil, err
	}
	return NewSignedString(pk, personaTag, namespace, nonce, string(bz))
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
	signerPubKey, err := crypto.SigToPub(hash, s.Signature)
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
	if _, err := hash.Write(s.Body); err != nil {
		return nil, err
	}
	return hash.Sum(nil), nil
}
