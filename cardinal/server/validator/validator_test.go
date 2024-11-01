package validator

import (
	"crypto/ecdsa"
	"fmt"
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
const hackedRequestBody = `{"give": "much_gold", "to": "me"}`
const badRequestBody = `{{"junk"{{`
const emptyRequestBody = ""

const goodPersona = "good_persona"
const badPersona = "bad_persona"

const goodNamespace = "good_namespace"
const badNamespace = "bad_namespace"

const badSignature = "bad_signature"
const badSignerAddress = "bad_signer_address"
const lookupSignerAddress = ""

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

// create an enabled validator with a specific ttl
func (s *ValidatorTestSuite) createValidatorWithTTL(ttl int) *SignatureValidator { //nolint: unparam // future use
	return NewSignatureValidator(false, ttl, 200, s.namespace, s.provider)
}

func (s *ValidatorTestSuite) simulateReceivedTransaction(personaTag, namespace string,
	data any, //nolint: unparam // future use
) (*Transaction, error) {
	tx, err := sign.NewTransaction(s.privateKey, personaTag, namespace, data)
	if err == nil {
		// sign puts a hash value into the transaction, but a newly received transaction will not have a hash value
		// because the Unmarshal function used to on the incoming message does not copy it
		tx.Hash = emptyHash
	}
	return tx, err
}

// TestCanValidateSignedTxWithVerificationDisabled tests that you can validate a full signed tx when
// sig verification is disabled (no actual validation is done)
func (s *ValidatorTestSuite) TestCanValidateSignedTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx, err := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(err)
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().NoError(err)
}

// TestCanValidateUnsignedTxWithVerificationDisabled tests that you can validate a tx with just a persona and body when
// sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateUnsignedTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().NoError(err)
}

// TestCanValidateBadSignatureTxWithVerificationDisabled tests that you can validate a tx with an invalid signature
// when sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateBadSignatureTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Signature: badSignature, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, badSignerAddress)
	s.Require().NoError(err)
}

// TestCanValidateBadNamespaceTxWithVerificationDisabled tests that you can validate a tx with the wrong namespace
// when sig verification is disabled.
func (s *ValidatorTestSuite) TestCanValidateBadNamespaceTxWithVerificationDisabled() {
	validator := s.createDisabledValidator()
	tx := &sign.Transaction{PersonaTag: goodPersona, Namespace: badNamespace, Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().NoError(err)
}

// TestCanValidateBadTimestampsTxWithVerificationDisabled tests that you can validate transactions with expired or
// future timestamps when sig verification is disabled.
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

// TestIgnoresTxHashWithVerificationDisabled tests that you can validate a tx with a bogus hash value
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

// TestValidationIgnoresBody tests that you can validate a tx without a valid body when
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
	tx, err := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(err)
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().NoError(err)
}

// TestRejectsMissingPersonaTx tests that transaction without a persona tag is always rejected, regardless
// of whether signature validation is enabled or not
func (s *ValidatorTestSuite) TestAlwaysRejectsMissingPersonaTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)

	tx.PersonaTag = ""
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusBadRequest, err.GetStatusCode())
	s.Require().Equal("Bad Request - "+ErrNoPersonaTag.Error(), err.Error())

	validator = s.createDisabledValidator()
	tx = &sign.Transaction{PersonaTag: "", Body: []byte(goodRequestBody)}

	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusBadRequest, err.GetStatusCode())
	s.Require().Equal("Bad Request - "+ErrNoPersonaTag.Error(), err.Error())
}

// TestRejectsUnsignedTx tests that an unsigned transaction with a valid timestamp is rejected.
func (s *ValidatorTestSuite) TestRejectsUnsignedTx() {
	validator := s.createValidatorWithTTL(10)
	tx := &sign.Transaction{PersonaTag: goodPersona, Timestamp: sign.TimestampNow(), Body: []byte(goodRequestBody)}
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
}

// TestRejectsBadNamespaceTx tests that a signed transaction with the wrong namespace is rejected
func (s *ValidatorTestSuite) TestRejectsBadNamespaceTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(goodPersona, badNamespace, goodRequestBody)
	s.Require().NoError(e)
	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), "incorrect namespace")
}

// TestRejectsInvalidTimestampsTx tests that transactions with invalid timestamps or with a timestamp altered
// after signing are rejected
func (s *ValidatorTestSuite) TestRejectsInvalidTimestampsTx() {
	ttl := 10
	validator := s.createValidatorWithTTL(ttl)
	tx, e := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)
	s.Require().True(sign.IsZeroHash(tx.Hash))

	saved := tx.Timestamp

	tx.Timestamp = veryOldTimestamp
	err := validator.ValidateTransactionTTL(tx)
	s.Require().Error(err)
	s.Require().Equal(http.StatusRequestTimeout, err.GetStatusCode())
	s.Require().Equal("Request Timeout - "+ErrMessageExpired.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), fmt.Sprintf("message older than %d seconds", ttl))

	tx.Timestamp = futureTimestamp
	err = validator.ValidateTransactionTTL(tx)
	s.Require().Error(err)
	s.Require().Equal(http.StatusBadRequest, err.GetStatusCode())
	s.Require().Equal("Bad Request - "+ErrBadTimestamp.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), fmt.Sprintf("message timestamp more than %d seconds in the future",
		ttlMaxFutureSeconds))

	tx.Timestamp = saved - 1
	err = validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)

	// this step actually calculates the hash
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), "Signature validation failed for message")
}

// TestRejectsAlteredHashTx tests that a transaction with a hashes that is altered after signing is rejected
func (s *ValidatorTestSuite) TestRejectsAlteredHashTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)
	s.Require().True(sign.IsZeroHash(tx.Hash))

	tx.Hash = badHash // alter the hash

	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)

	// this step normally calculates the hash, but since it's altered it will use the altered one
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), "Signature validation failed for message")
}

// TestRejectsAlteredSaltTx tests that a transaction with a salt value that is altered after signing is rejected
func (s *ValidatorTestSuite) TestRejectsAlteredSaltTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)
	s.Require().True(sign.IsZeroHash(tx.Hash))

	tx.Salt++ // alter the salt value

	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)

	// this step normally calculates the hash, but since it's altered it will use the altered one
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), "Signature validation failed for message")
}

// TestRejectsAlteredBodyTx tests that a transaction with a body that is altered after signing is rejected
func (s *ValidatorTestSuite) TestRejectsAlteredBodyTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)
	s.Require().True(sign.IsZeroHash(tx.Hash))

	tx.Body = []byte(hackedRequestBody) // replace the body with another valid one

	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)

	// this step normally calculates the hash, but since it's altered it will use the altered one
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), "Signature validation failed for message")
}

// TestRejectsInvalidPersonaTx tests that a transaction with an invalid signature is rejected.
func (s *ValidatorTestSuite) TestRejectsInvalidPersonaTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(badPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)

	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().Error(err)
	s.Require().Equal(http.StatusUnauthorized, err.GetStatusCode())
	s.Require().Equal("Unauthorized - "+ErrInvalidSignature.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), "could not get signer for persona bad_persona")
}

// TestRejectsDuplicateTx tests that a transaction that's previously been validated is rejected if you try to validate
// it again. This prevents replay attacks because each message sent must be uniquely signed.
func (s *ValidatorTestSuite) TestRejectsDuplicateTx() {
	validator := s.createValidatorWithTTL(10)
	tx, e := s.simulateReceivedTransaction(goodPersona, goodNamespace, goodRequestBody)
	s.Require().NoError(e)

	err := validator.ValidateTransactionTTL(tx)
	s.Require().NoError(err)
	err = validator.ValidateTransactionSignature(tx, lookupSignerAddress)
	s.Require().NoError(err)

	// try to validate same transaction again, detect an error
	err = validator.ValidateTransactionTTL(tx)
	s.Require().Error(err)
	s.Require().Equal(http.StatusForbidden, err.GetStatusCode())
	s.Require().Equal("Forbidden - "+ErrDuplicateMessage.Error(), err.Error())
	s.Require().Contains(err.GetLogMessage(), fmt.Sprintf("message %s already handled", tx.Hash))
}
