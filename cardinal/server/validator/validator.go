package validator

import (
	"errors"
	"fmt"
	"net/http"
	"time"

	"github.com/coocood/freecache"
	"github.com/ethereum/go-ethereum/common" // for hash
	"github.com/rotisserie/eris"

	"pkg.world.dev/world-engine/sign"
)

// we define the particular interface we need here to avoid dragging in the whole world provider interface
// and making independent testing of validator require more complicated interfaces
type SignerAddressProvider interface {
	// tick is used by world provider, but not by the validator package. we include it here
	// to avoid creating an extra method for a very minor bit of abstraction
	GetSignerForPersonaTag(personaTag string, tick uint64) (addr string, err error)
}

const cacheRetentionExtraSeconds = 10 // this is how many seconds past normal expiration a hash is left in the cache.
// we want to ensure it's long enough that any message that's not expired but
// still has its hash in the cache for replay protection. Setting it too long
// would cause the cache to be bigger than necessary

const ttlMaxFutureSeconds = 2 // this is how many seconds in the future a message is allowed to be timestamped
// this allows for some degree of clock drift. It's safe enought to accept a message that's stamped from the near
// future because we will still keep it in the hash cache and prevent it from being a replay attack vector. However,
// we don't want to take messages from an unlimited amount of time into the future since they could cause our hash
// cache to overflow

const bytesPerKb = 1024

var (
	ErrNoPersonaTag     = errors.New("persona tag is required")
	ErrWrongNamespace   = errors.New("incorrect namespace")
	ErrMessageExpired   = errors.New("signature too old")
	ErrBadTimestamp     = errors.New("invalid future timestamp")
	ErrCacheReadFailed  = errors.New("cache read failed")
	ErrCacheWriteFailed = errors.New("cache store failed")
	ErrDuplicateMessage = errors.New("duplicate message")
	ErrInvalidSignature = errors.New("invalid signature")
)

type SignatureValidator struct {
	IsDisabled               bool
	MessageExpirationSeconds uint
	HashCacheSizeKB          uint
	namespace                string
	cache                    *freecache.Cache
	signerAddressProvider    SignerAddressProvider
}

type ValidationError interface {
	error
	GetLogMessage() string
	GetStatusCode() int
}
type validationError struct {
	error
	StatusCode int
	LogMsg     string // internal, for logging only
}

func (e *validationError) Error() string {
	return http.StatusText(e.StatusCode) + " - " + e.error.Error()
}
func (e *validationError) GetStatusCode() int    { return e.StatusCode }
func (e *validationError) GetLogMessage() string { return e.LogMsg }

func NewSignatureValidator(disabled bool, msgExpirationSec uint, hashCacheSizeKB uint, namespace string,
	provider SignerAddressProvider,
) *SignatureValidator {
	validator := SignatureValidator{
		IsDisabled:               disabled,
		MessageExpirationSeconds: msgExpirationSec,
		HashCacheSizeKB:          hashCacheSizeKB,
		namespace:                namespace,
		cache:                    nil,
		signerAddressProvider:    provider,
	}
	if !disabled {
		// freecache enforces its own minimum size of 512K
		validator.cache = freecache.NewCache(int(validator.HashCacheSizeKB * bytesPerKb))
	}
	return &validator
}

// ValidateTransactionTTL checks that the timestamp on the message is valid, the message has not expired,
// and that the message is not previously handled as indicated by it being in the hash cache.
// returns an error (ErrMessageExpired, ErrBadTimestamp, ErrDuplicateMessage, or ErrCacheReadFailed) if
// there was a problem, and nil if everything was ok
// if signature validation is disabled, no checks are done and nil is always returned
func (validator *SignatureValidator) ValidateTransactionTTL(tx *sign.Transaction) ValidationError {
	if !validator.IsDisabled {
		now := time.Now()
		txEarliestValidTimestamp := sign.TimestampAt(
			now.Add(-(time.Duration(validator.MessageExpirationSeconds) * time.Second)))
		txLatestValidTimestamp := sign.TimestampAt(now.Add(time.Duration(ttlMaxFutureSeconds) * time.Second))
		// before we even create the hash or validator the signature, check to see if the message has expired
		if tx.Timestamp < txEarliestValidTimestamp {
			return &validationError{ErrMessageExpired, http.StatusRequestTimeout,
				fmt.Sprintf("message older than %d seconds. Got timestamp: %d, current timestamp: %d ",
					validator.MessageExpirationSeconds, tx.Timestamp, sign.TimestampAt(now))}
		} else if tx.Timestamp > txLatestValidTimestamp {
			return &validationError{ErrBadTimestamp, http.StatusBadRequest,
				fmt.Sprintf(
					"message timestamp more than %d seconds in the future. Got timestamp: %d, current timestamp: %d ",
					ttlMaxFutureSeconds, tx.Timestamp, sign.TimestampAt(now))}
		}
		// check for duplicate message via hash cache
		if found, err := validator.isHashInCache(tx.Hash); err != nil {
			return &validationError{ErrCacheReadFailed, http.StatusInternalServerError,
				fmt.Sprintf("unexpected cache error %v. message %s ignored", err, tx.Hash.String())}
		} else if found {
			// if found in the cache, the message hash has already been used, so reject it
			return &validationError{ErrDuplicateMessage, http.StatusForbidden,
				fmt.Sprintf("message %s already handled", tx.Hash.String())}
		}
	}
	return nil
}

// ValidateTransactionSignature checks that the signature is valid, was signed by the persona (or signer passed in),
// has the correct namespace, and has not been altered. If all checks pass, it is added to the hash cache as a
// known message, and nil is returned. Other possible returns are ErrNoPersonaTag, ErrInvalidSignature, and
// ErrCacheWriteFailed. If signature validation is disabled, we only check for the presence of a persona tag.
func (validator *SignatureValidator) ValidateTransactionSignature(tx *sign.Transaction, signerAddress string,
) ValidationError {
	// this is the only validation we do when signature validation is disabled
	if tx.PersonaTag == "" {
		return &validationError{ErrNoPersonaTag, http.StatusBadRequest,
			fmt.Sprintf("Missing persona tag for message %s", tx.Hash.String())}
	}
	if validator.IsDisabled {
		return nil
	}

	// if they didn't give us a signer address, we will have to look it up with the provider
	var err error
	if signerAddress == "" {
		signerAddress, err = validator.signerAddressProvider.GetSignerForPersonaTag(tx.PersonaTag, 0)
		if err != nil {
			return &validationError{ErrInvalidSignature, http.StatusUnauthorized,
				fmt.Sprintf("could not get signer for persona %s: %v", tx.PersonaTag, err)}
		}
	}

	// check the signature against the address
	if err = validator.validateSignature(tx, signerAddress); err != nil {
		return &validationError{ErrInvalidSignature, http.StatusUnauthorized,
			fmt.Sprintf("Signature validation failed for message %s: %v", tx.Hash.String(), err)}
	}

	// the message was valid, so add its hash to the cache
	// we don't do this until we have verified the signature to prevent an attack where someone sends
	// large numbers of hashes with unsigned/invalid messages and thus blocks legit messages from
	// being handled
	err = validator.cache.Set(tx.Hash.Bytes(), nil,
		int(validator.MessageExpirationSeconds+cacheRetentionExtraSeconds))
	if err != nil {
		// if we couldn't store the hash in the cache, don't process the transaction, since that
		// would open us up to replay attacks
		return &validationError{ErrCacheWriteFailed, http.StatusInternalServerError,
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
func (validator *SignatureValidator) validateSignature(tx *sign.Transaction, signerAddr string) error {
	if tx.Namespace != validator.namespace {
		return eris.Wrap(ErrWrongNamespace, fmt.Sprintf("expected %q got %q", validator.namespace, tx.Namespace))
	}
	return eris.Wrap(tx.Verify(signerAddr), "")
}
