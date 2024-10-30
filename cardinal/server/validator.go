package server

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coocood/freecache"
	"github.com/ethereum/go-ethereum/common" // for hash
	"github.com/rotisserie/eris"
	personaMsg "pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

type SignerAddressProvider interface {
	GetSignerAddressForPersonaTag(personaTag string) (addr string, err error)
}

const cacheRetentionExtraSeconds = 10 // this is how many seconds past normal expiration a hash is left in the cache.
// we want to ensure it's long enough that any message that's not expired but
// still has its hash in the cache for replay protection. Setting it too long
// would cause the cache to be bigger than necessary

var (
	ErrNoPersonaTag     = errors.New("persona tag is required")
	ErrWrongNamespace   = errors.New("incorrect namespace")
	ErrMessageExpired   = errors.New("signature too old")
	ErrCacheReadFailed  = errors.New("cache read failed")
	ErrCacheWriteFailed = errors.New("cache store failed")
	ErrDuplicateMessage = errors.New("duplicate message")
	ErrInvalidSignature = errors.New("invalid signature")
)

type SignatureValidator struct {
	IsDisabled               bool
	MessageExpirationSeconds int
	HashCacheSizeKB          int
	cache                    *freecache.Cache
	signerAddressProvider    SignerAddressProvider
}

type ValidationError struct {
	StatusCode  int
	Err         error  // public
	InternalMsg string // internal, for logging only
}

func (e *ValidationError) Error() string {
	return http.StatusText(e.StatusCode) + " - " + e.Err.Error()
}

func NewSignatureValidator(disabled bool, msgExpirationSec int, hashCacheSizeKB int, provider SignerAddressProvider,
) *SignatureValidator {
	validator := SignatureValidator{
		IsDisabled:               disabled,
		MessageExpirationSeconds: msgExpirationSec,
		HashCacheSizeKB:          hashCacheSizeKB,
		cache:                    nil,
		signerAddressProvider:    provider,
	}
	if !disabled {
		validator.cache = freecache.NewCache(validator.HashCacheSizeKB)
	}
	return &validator
}

type Transaction = sign.Transaction

func (validator *SignatureValidator) ValidateTransactionTTL(tx *Transaction) *ValidationError {
	if !validator.IsDisabled {
		txEarliestValidTimestamp := sign.TimestampAt(
			time.Now().Add(-(time.Duration(validator.MessageExpirationSeconds) * time.Second)))
		// before we even create the hash or validator the signature, check to see if the message has expired
		if tx.Timestamp < txEarliestValidTimestamp {
			return &ValidationError{http.StatusRequestTimeout, ErrMessageExpired,
				fmt.Sprintf("message older than %d seconds. Got timestamp: %d, current timestamp: %d ",
					validator.MessageExpirationSeconds, tx.Timestamp, sign.TimestampNow())}
		}
		// check for duplicate message via hash cache
		if found, err := validator.isHashInCache(tx.Hash); err != nil {
			return &ValidationError{http.StatusInternalServerError, ErrCacheReadFailed,
				fmt.Sprintf("unexpected cache error %v. message %s ignored", err, tx.Hash.String())}
		} else if found {
			// if found in the cache, the message hash has already been used, so reject it
			return &ValidationError{http.StatusForbidden, ErrDuplicateMessage,
				fmt.Sprintf("message %s already handled", tx.Hash.String())}
		}
	}
	return nil
}

func (validator *SignatureValidator) ValidateTransactionSignature(tx *Transaction,
	msgType types.Message, msg any, namespace string) *ValidationError {
	// this is the only validation we do when signature validation is disabled
	if tx.PersonaTag == "" {
		return &ValidationError{http.StatusBadRequest, ErrNoPersonaTag,
			fmt.Sprintf("Missing persona tag for message %s", tx.Hash.String())}
	}
	if validator.IsDisabled {
		return nil
	}

	// check the signature
	// FIXME: this seems messy, with signature validation having a special case for a particular type of message
	// especially since this is the only reason we need msg or msgType as parameters.
	var signerAddress string
	if msgType.Name() == personaMsg.CreatePersonaMessageName {
		// don't need to check the cast bc we already validated this above
		createPersonaMsg, _ := msg.(personaMsg.CreatePersona)
		signerAddress = createPersonaMsg.SignerAddress
	}

	var err error
	if signerAddress == "" {
		signerAddress, err = validator.signerAddressProvider.GetSignerAddressForPersonaTag(tx.PersonaTag)
		if err != nil {
			return &ValidationError{http.StatusUnauthorized, ErrInvalidSignature,
				fmt.Sprintf("could not get signer for persona %s: %w", tx.PersonaTag, err)}
		}
	}

	if err = validator.validateSignature(tx, signerAddress, namespace); err != nil {
		return &ValidationError{http.StatusUnauthorized, ErrInvalidSignature,
			fmt.Sprintf("Signature validation failed for message %s: %v", tx.Hash.String(), err)}
	}

	// the message was valid, so add its hash to the cache
	// we don't do this until we have verified the signature to prevent an attack where someone sends
	// large numbers of hashes with unsigned/invalid messages and thus blocks legit messages from
	// being handled
	err = validator.cache.Set(tx.Hash.Bytes(), nil, validator.MessageExpirationSeconds+cacheRetentionExtraSeconds)
	if err != nil {
		// if we couldn't store the hash in the cache, don't process the transaction, since that
		// would open us up to replay attacks
		return &ValidationError{http.StatusInternalServerError, ErrCacheWriteFailed,
			fmt.Sprintf("unexpected cache store error %v. message %s ignored", err, tx.Hash.String())}
	}
	return nil
}

func (validator *SignatureValidator) isHashInCache(hash common.Hash) (bool, error) {
	_, err := validator.cache.Get(hash.Bytes())
	if err == nil {
		// found it
		return true, nil
	}
	if errors.Is(err, freecache.ErrNotFound) {
		// ignore ErrNotFound, just return false
		return false, nil
	}
	// return all other errors
	return false, err
}

// validateSignature validates that the signature of transaction is valid
func (validator *SignatureValidator) validateSignature(tx *Transaction, signerAddr string, namespace string) error {
	if tx.Namespace != namespace {
		return eris.Wrap(ErrWrongNamespace, fmt.Sprintf("expected %q got %q", namespace, tx.Namespace))
	}
	return eris.Wrap(tx.Verify(signerAddr), "")
}
