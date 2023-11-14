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
	"github.com/rotisserie/eris"
)

var (
	// ErrSignatureValidationFailed is returned when a signature is not valid.
	ErrSignatureValidationFailed = errors.New("signature validation failed")
	ErrCannotSignEmptyBody       = errors.New("cannot sign empty body")
	ErrInvalidPersonaTag         = errors.New("invalid persona tag")
	ErrInvalidNamespace          = errors.New("invalid namespace")

	ErrNoPersonaTagField = errors.New("transaction must contain personaTag field")
	ErrNoNamespaceField  = errors.New("transaction must contain namespace field")
	ErrNoSignatureField  = errors.New("transaction must contain signature field")
	ErrNoBodyField       = errors.New("transaction must contain body field")
)

// SystemPersonaTag is a reserved persona tag for transaction. It is used in transactions when a PersonaTag
// does not actually exist (e.g. during the PersonaTag creation process).
const SystemPersonaTag = "SystemPersonaTag"

type Transaction struct {
	PersonaTag string          `json:"personaTag"`
	Namespace  string          `json:"namespace"`
	Nonce      uint64          `json:"nonce"`
	Signature  string          `json:"signature"` // hex encoded string
	Hash       common.Hash     `json:"hash,omitempty"`
	Body       json.RawMessage `json:"body"` // json string
}

func UnmarshalTransaction(bz []byte) (*Transaction, error) {
	s := new(Transaction)
	dec := json.NewDecoder(bytes.NewBuffer(bz))
	dec.DisallowUnknownFields()

	if err := dec.Decode(s); err != nil {
		return nil, eris.Wrap(err, "error decoding Transaction")
	}

	if err := s.checkRequiredFields(); err != nil {
		return nil, err
	}
	s.populateHash()
	return s, nil
}

// checkRequiredFields ensures that all fields are present. we could do this via reflection, but checking directly is
// faster than using reflection.
func (s *Transaction) checkRequiredFields() error {
	if s.PersonaTag == "" {
		return eris.Wrap(ErrNoPersonaTagField, "")
	}
	if s.Namespace == "" {
		return eris.Wrap(ErrNoNamespaceField, "")
	}
	if s.Signature == "" {
		return eris.Wrap(ErrNoSignatureField, "")
	}
	if len(s.Body) == 0 {
		return eris.Wrap(ErrNoBodyField, "")
	}
	return nil
}

// MappedTransaction Identical to UnmarshalTransaction but takes a transaction in the form of map[string]any.
func MappedTransaction(tx map[string]interface{}) (*Transaction, error) {
	s := new(Transaction)
	transactionKeys := map[string]bool{
		"personaTag": true,
		"namespace":  true,
		"signature":  true,
		"nonce":      true,
		"body":       true,
		"hash":       true,
	}
	for key := range tx {
		if !transactionKeys[key] {
			return nil, fmt.Errorf("invalid field: %s in body", key)
		}
	}
	// json.Marshal will encode an empty body to "null", so verify the body exists before attempting to Marshal it.
	if _, ok := tx["body"]; !ok {
		return nil, ErrNoBodyField
	}
	serializedBody, err := json.Marshal(tx["body"])
	if err != nil {
		return nil, err
	}
	delete(tx, "hash")
	delete(tx, "body")
	err = mapstructure.Decode(tx, s)
	if err != nil {
		return nil, eris.Wrap(err, "error decoding map structure")
	}
	s.Body = serializedBody
	if err := s.checkRequiredFields(); err != nil {
		return nil, err
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
		res, err := json.Marshal(data)
		return res, eris.Wrap(err, "")
	}

	asMap := map[string]any{}

	// The swagger endpoints end up processing the transaction body as a map[string]any{}. When this map is
	// marshalled, the resulting JSON blob has keys in sorted order. If the original JSON blob did NOT have
	// sorted keys, the resulting hashes will be different and the signature will fail.
	// For this reason, we must Unmarshal/Marshal any pre-built JSON bodies to ensure the resulting hashes during
	// signing match the hash during verification
	if err := json.Unmarshal(asBuf, &asMap); err != nil {
		return nil, eris.Errorf("data %q is not valid json", string(asBuf))
	}

	normalizedBz, err := json.Marshal(asMap)
	if err != nil {
		return nil, eris.Wrapf(err, "failed to generate compact json")
	}
	return normalizedBz, nil
}

// sign uses the given private key to sign the personaTag, namespace, nonce, and data.
func sign(pk *ecdsa.PrivateKey, personaTag, namespace string, nonce uint64, data any) (*Transaction, error) {
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
	sp := &Transaction{
		PersonaTag: personaTag,
		Namespace:  namespace,
		Nonce:      nonce,
		Body:       bz,
	}
	sp.populateHash()
	buf, err := crypto.Sign(sp.Hash.Bytes(), pk)
	if err != nil {
		return nil, eris.Wrap(err, "error signing hash")
	}
	sp.Signature = common.Bytes2Hex(buf)
	return sp, nil
}

// NewSystemTransaction signs a given body, and nonce with the given private key using the SystemPersonaTag.
func NewSystemTransaction(pk *ecdsa.PrivateKey, namespace string, nonce uint64, data any) (*Transaction, error) {
	return sign(pk, SystemPersonaTag, namespace, nonce, data)
}

// NewTransaction signs a given body, tag, and nonce with the given private key.
func NewTransaction(pk *ecdsa.PrivateKey,
	personaTag,
	namespace string,
	nonce uint64,
	data any,
) (*Transaction, error) {
	if len(personaTag) == 0 || personaTag == SystemPersonaTag {
		return nil, ErrInvalidPersonaTag
	}
	return sign(pk, personaTag, namespace, nonce, data)
}

func (s *Transaction) IsSystemTransaction() bool {
	return s.PersonaTag == SystemPersonaTag
}

// Marshal serializes this Transaction to bytes, which can then be passed in to Unmarshal.
func (s *Transaction) Marshal() ([]byte, error) {
	res, err := json.Marshal(s)
	err = eris.Wrap(err, "")
	return res, err
}

func isZeroHash(hash common.Hash) bool {
	return hash == common.Hash{}
}

// HashHex return a hex encoded hash of the signature.
func (s *Transaction) HashHex() string {
	if isZeroHash(s.Hash) {
		s.populateHash()
	}
	return s.Hash.Hex()
}

// Verify verifies this Transaction has a valid signature. If nil is returned, the signature is valid.
// Signature verification follows the pattern in crypto.TestSign:
// https://github.com/ethereum/go-ethereum/blob/master/crypto/crypto_test.go#L94
// TODO: Review this signature verification, and compare it to geth's sig verification
func (s *Transaction) Verify(hexAddress string) error {
	addr := common.HexToAddress(hexAddress)

	if isZeroHash(s.Hash) {
		s.populateHash()
	}

	sig := common.Hex2Bytes(s.Signature)
	if sig[crypto.RecoveryIDOffset] == 27 || sig[crypto.RecoveryIDOffset] == 28 {
		sig[crypto.RecoveryIDOffset] -= 27 // Transform yellow paper V from 27/28 to 0/1
	}

	signerPubKey, err := crypto.SigToPub(s.Hash.Bytes(), sig)
	err = eris.Wrap(err, "")
	if err != nil {
		return err
	}
	signerAddr := crypto.PubkeyToAddress(*signerPubKey)
	if signerAddr != addr {
		return eris.Wrap(ErrSignatureValidationFailed, "")
	}
	return nil
}

func (s *Transaction) populateHash() {
	s.Hash = crypto.Keccak256Hash(
		[]byte(s.PersonaTag),
		[]byte(s.Namespace),
		[]byte(fmt.Sprintf("%d", s.Nonce)),
		s.Body,
	)
}
