// Package sign allows for the cryptographic signing and verification an arbitrary payload.
package sign

import (
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/json"
	"errors"
	"fmt"
	"hash/fnv"
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

// NewSignedPayload signs a given body, tag, and nonce with the given private key.
func NewSignedPayload(body []byte, personaTag, namespace string, nonce uint64, pk *ecdsa.PrivateKey) (*SignedPayload, error) {
	sp := &SignedPayload{
		PersonaTag: personaTag,
		Namespace:  namespace,
		Nonce:      nonce,
		Body:       body,
	}
	hash, err := sp.hash()
	if err != nil {
		return nil, err
	}
	buf, err := ecdsa.SignASN1(rand.Reader, pk, hash)
	if err != nil {
		return nil, err
	}
	sp.Signature = buf
	return sp, nil
}

// Marshal serializes this SignedPayload to bytes, which can then be passed in to Unmarshal.
func (s *SignedPayload) Marshal() ([]byte, error) {
	return json.Marshal(s)
}

// Verify verifies this SignedPayload has a valid signature. If nil is returned, the signature is valid.
// TODO: Review this signature verification, and compare it to geth's sig verification
func (s *SignedPayload) Verify(key ecdsa.PublicKey) error {
	hash, err := s.hash()
	if err != nil {
		return err
	}
	if !ecdsa.VerifyASN1(&key, hash, s.Signature) {
		return ErrorSignatureValidationFailed
	}
	return nil
}

func (s *SignedPayload) hash() ([]byte, error) {
	hash := fnv.New128()
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
