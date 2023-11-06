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
		return nil, fmt.Errorf("error decoding Transaction: %w", err)
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
		return ErrNoPersonaTagField
	}
	if s.Namespace == "" {
		return ErrNoNamespaceField
	}
	if s.Signature == "" {
		return ErrNoSignatureField
	}
	if len(s.Body) == 0 {
		return ErrNoBodyField
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
	serializedBody, err := json.Marshal(tx["body"])
	if err != nil {
		return nil, err
	}
	delete(tx, "hash")
	delete(tx, "body")
	err = mapstructure.Decode(tx, s)
	if err != nil {
		return nil, err
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
		return json.Marshal(data)
	}

	if !json.Valid(asBuf) {
		return nil, fmt.Errorf("data %q is not valid json", string(asBuf))
	}

	dst := &bytes.Buffer{}

	// JSON strings need to be compacted (insignificant whitespace removed).
	// This is required because when the Transaction is serialized/deserialized those spaces will also
	// be lost. If they are not removed beforehand, the hashes of the message before serialization and after
	// will be different.
	if err := json.Compact(dst, asBuf); err != nil {
		return nil, err
	}
	return dst.Bytes(), nil
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
		return nil, err
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
	return json.Marshal(s)
}

// HashHex return a hex encoded hash of the signature.
func (s *Transaction) HashHex() string {
	if len(s.Hash) == 0 {
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

	if len(s.Hash) == 0 {
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

func (s *Transaction) populateHash() {
	s.Hash = crypto.Keccak256Hash(
		[]byte(s.PersonaTag),
		[]byte(s.Namespace),
		[]byte(fmt.Sprintf("%d", s.Nonce)),
		s.Body,
	)
}
