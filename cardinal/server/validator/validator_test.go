package validator

import (
	"crypto/ecdsa"
	"net/http"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/suite"
	"pkg.world.dev/world-engine/cardinal/persona"
	"pkg.world.dev/world-engine/sign"
)

const goodRequestBody = `{"msg": "this is a request body"}`
const badRequestBody = `{{"junk"{{`
const emptyRequestBody = ""

const goodPersona = "good_persona"
const badPersona = "bad_persona"

const goodNamespace = "good_namespace"
const badNamespace = "bad_namespace"

const badSignature = "bad_signature"
const badSignerAddress = "bad_signer_address"

var emptyHash = common.Hash{}
var badHash = crypto.Keccak256Hash([]byte("complete_garbage"))

var veryOldTimestamp = sign.TimestampAt(time.Now().Add(time.Hour * -1000))
var futureTimestamp = sign.TimestampAt(time.Now().Add(time.Hour * 1000))

type ValidatorTestSuite struct {
	suite.Suite

	privateKey *ecdsa.PrivateKey
	signerAddr string
	namespace  string
	provider   SignerAddressProvider
}

type ProviderFixture struct {
	vts *ValidatorTestSuite
}

func (pf *ProviderFixture) GetSignerAddressForPersonaTag(personaTag string) (addr string, err error) {
	if personaTag == badPersona {
		// emulates what would happen if the personal lookup provider couldn't find a signer for the persona
		return "", persona.ErrPersonaTagHasNoSigner
	}
	return pf.vts.signerAddr, nil
}

func TestServerValidator(t *testing.T) {
	suite.Run(t, new(ValidatorTestSuite))
}

// SetupTest runs before each test in the suite.
func (s *ValidatorTestSuite) SetupTest() {
	var err error
	s.privateKey, err = crypto.GenerateKey()
	s.Require().NoError(err)
	s.signerAddr = crypto.PubkeyToAddress(s.privateKey.PublicKey).Hex()
	s.namespace = goodNamespace
	s.provider = &ProviderFixture{
		vts: s,
	}
}

// TearDownTest runs after each test in the suite.
func (s *ValidatorTestSuite) TearDownTest() {
}

func (s *ValidatorTestSuite) createDisabledValidator() *SignatureValidator {
	return NewSignatureValidator(true, 0, 0, s.namespace, s.provider)
}

func (s *ValidatorTestSuite) createValidatorWithTTL(ttl int) *SignatureValidator {
	return NewSignatureValidator(false, ttl, 200, s.namespace, s.provider)
}

// TestCanValidateSignedTxWithVerificationDisabled tests that you can validate a full signed tx when
// sig verification is disabled (no actual validation is done)
func (s *ValidatorTestSuite) TestCanValidateSignedTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx, err := sign.NewTransaction(s.privateKey, goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(err)
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, s.signerAddr)
	s.Require().NoError(err)
}

// TestCanValidateUnsignedTxWithVerificationDisabled tests that you can submit a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateUnsignedTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, s.signerAddr)
	s.Require().NoError(err)
}

// TestCanValidateBadSignatureTxWithVerificationDisabled tests that you can submit a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateBadSignatureTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Signature: badSignature, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)
}

// TestCanValidateBadNamespaceTxWithVerificationDisabled tests that you can submit a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateBadNamespaceTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Namespace: badNamespace, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, s.signerAddr)
	s.Require().NoError(err)
}

// TestCanValidateBadSignatureTxWithVerificationDisabled tests that you can submit a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateBadTimestampsTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Timestamp: veryOldTimestamp, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)

	tx = &sign.Transaction{PersonaTag: goodPersona, Timestamp: futureTimestamp, Body: []byte(goodRequestBody)}
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)
}

// TestIgnoresTxHashWithVerificationDisabled tests that you can submit a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestIgnoresTxHashWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Hash: emptyHash, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)

	tx = &sign.Transaction{PersonaTag: goodPersona, Hash: badHash, Body: []byte(goodRequestBody)}
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)
}

// TestValidationIgnoresBody tests that you can submit a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestValidationIgnoresBodyWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Body: []byte(badRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)

	tx = &sign.Transaction{PersonaTag: goodPersona, Body: []byte(emptyRequestBody)}
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)

	tx = &sign.Transaction{PersonaTag: goodPersona, Body: nil}
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)
}

// TestCanValidateSignedTx tests that you can validate a full signed tx when
// sig verification is enabled.
func (s *ValidatorTestSuite) TestCanValidateSignedTx() {
	validator := s.createValidatorWithTTL(10)
	tx, err := sign.NewTransaction(s.privateKey, goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(err)
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, s.signerAddr)
	s.Require().NoError(err)
}

// TestRejectsUnsignedTx tests that a validation tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestRejectsUnsignedTx() {
	validator := s.createValidatorWithTTL(10)
	tx := &sign.Transaction{PersonaTag: goodPersona, Timestamp: sign.TimestampNow(), Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, s.signerAddr)
	s.Require().Error(err)
	s.Require().Equal(err.GetStatusCode(), http.StatusUnauthorized)
	s.Require().Equal(err.Error(), "Unauthorized - "+ErrInvalidSignature.Error())
}
