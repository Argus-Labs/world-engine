// Package httpsign allows for the cryptographic signing and verification of http requests.
//
// This package is based on https://datatracker.ietf.org/doc/draft-ietf-httpbis-message-signatures/, although
// it does not fully implement the specification. It uses ECDS-SECP256k1 for signing and verifying.
//
// This package augments http.Requests by adding "Signature-Input" and "Signature" headers. The "Signature-Input" header
// specifies what fields are to be used to generate and verify the signature. The "Signature" header specifies the
// actual cryptographic signature.
//
// The values in the "Signature-Input" header are hashed together, along with the message body to create a hash
// of the HTTP request. The hash is then signed according th ECDSA. The resulting signature is then base64 encoded.
package httpsign

import (
	"bytes"
	"crypto/ecdsa"
	"crypto/rand"
	"encoding/base64"
	"errors"
	"fmt"
	"hash/fnv"
	"io"
	"net/http"
	"strconv"
	"strings"
)

var (
	// ErrorSignatureValidationFailed is returned when a signature is not valid.
	ErrorSignatureValidationFailed = errors.New("signature validation failed")
)

type SigDetails struct {
	Tag   string
	Nonce uint64
}

const (
	signatureInputHeader = "Signature-Input"
	signatureHeader      = "Signature"
	keyTag               = "tag"
	keyNonce             = "nonce"
)

// Sign signs the given http request. The request body, as well as the given tag and nonce are used to generate
// the signature.
func Sign(req *http.Request, tag string, nonce uint64, pk *ecdsa.PrivateKey) error {
	req.Header.Add(signatureInputHeader, fmt.Sprintf("%s=%s", keyTag, tag))
	req.Header.Add(signatureInputHeader, fmt.Sprintf("%s=%d", keyNonce, nonce))
	hash, err := hashRequest(req)
	if err != nil {
		return err
	}
	buf, err := ecdsa.SignASN1(rand.Reader, pk, hash)
	if err != nil {
		return err
	}
	req.Header.Add(signatureHeader, base64.StdEncoding.EncodeToString(buf))
	return nil
}

// GetSigDetails returns the tag and nonce that were used to generate the signature of this http request.
func GetSigDetails(req *http.Request) (SigDetails, error) {
	sd := SigDetails{}
	foundNonce := false
	for _, val := range req.Header.Values(signatureInputHeader) {
		parts := strings.Split(val, "=")
		if parts[0] == keyTag {
			sd.Tag = parts[1]
		} else if parts[0] == keyNonce {
			var err error
			if sd.Nonce, err = strconv.ParseUint(parts[1], 10, 64); err != nil {
				return sd, fmt.Errorf("cannot parse nonce: %w", err)
			}
			foundNonce = true
		}
	}
	if sd.Tag == "" {
		return sd, errors.New("tag not found")
	}
	if !foundNonce {
		return sd, errors.New("nonce not found")
	}

	return sd, nil
}

// Verify verifies that the given http request was actually signed by the private key that is associated with the
// given public key. A returned nil error means the http request is correctly signed. The ErrorIncorrectSignature error
// is returned if the signature is invalid. Other non-nil errors can be returned if some other problem is found (e.g.
// the "Signature-Input" field is missing.
func Verify(req *http.Request, key ecdsa.PublicKey) error {
	hash, err := hashRequest(req)
	if err != nil {
		return err
	}
	sig := req.Header.Get(signatureHeader)
	if len(sig) == 0 {
		return fmt.Errorf("signature required")
	}

	buf, err := base64.StdEncoding.DecodeString(sig)
	if err != nil {
		return fmt.Errorf("can't decode signautre: %w", err)
	}

	if !ecdsa.VerifyASN1(&key, hash, buf) {
		return ErrorSignatureValidationFailed
	}
	return nil
}

// hashRequest hashes the values in the "Signature-Input" header along with the request body.
func hashRequest(req *http.Request) ([]byte, error) {
	hash := fnv.New128()
	for _, input := range req.Header.Values(signatureInputHeader) {
		parts := strings.Split(input, "=")
		if len(parts) < 2 {
			return nil, fmt.Errorf("expect a value for key %q", input)
		}
		if _, err := hash.Write([]byte(parts[1])); err != nil {
			return nil, fmt.Errorf("hashing %q failed: %w", parts[1], err)
		}
	}
	buf, err := extractBody(req)
	if err != nil {
		return nil, fmt.Errorf("can't get request body: %w", err)
	}
	if _, err := hash.Write(buf); err != nil {
		return nil, fmt.Errorf("can't hash request body: %w", err)
	}
	return hash.Sum(nil), nil
}

// extractBody returns the body of the http request without actually consuming the body.
func extractBody(r *http.Request) ([]byte, error) {
	buf, err := io.ReadAll(r.Body)
	if err != nil {
		return nil, err
	}
	r.Body = io.NopCloser(bytes.NewReader(buf))
	return buf, nil
}
