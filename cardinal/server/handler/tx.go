package handler

import (
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/common"
	"time"

	"github.com/coocood/freecache"
	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"

	personaMsg "pkg.world.dev/world-engine/cardinal/persona/msg"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

const cacheRetentionExtraSeconds = 10 // this is how many seconds past normal expiration a hash is left in the cache.
// we want to ensure it's long enough that any message that's not expired but
// still has its hash in the cache for replay protection. Setting it too long
// would cause the cache to be bigger than necessary

var (
	ErrNoPersonaTag               = errors.New("persona tag is required")
	ErrWrongNamespace             = errors.New("incorrect namespace")
	ErrSystemTransactionRequired  = errors.New("system transaction required")
	ErrSystemTransactionForbidden = errors.New("system transaction forbidden")
)

// PostTransactionResponse is the HTTP response for a successful transaction submission
type PostTransactionResponse struct {
	TxHash string
	Tick   uint64
}

type SignatureVerification struct {
	IsDisabled               bool
	MessageExpirationSeconds int
	HashCacheSizeKB          int
	Cache                    *freecache.Cache
}

type Transaction = sign.Transaction

// PostTransaction godoc
//
//	@Summary      Submits a transaction
//	@Description  Submits a transaction
//	@Accept       application/json
//	@Produce      application/json
//	@Param        txGroup  path      string                   true  "Message group"
//	@Param        txName   path      string                   true  "Name of a registered message"
//	@Param        txBody   body      Transaction              true  "Transaction details & message to be submitted"
//	@Success      200      {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400      {string}  string                   "Invalid request parameter"
//	@Failure      403      {string}  string                   "Forbidden"
//	@Failure      408      {string}  string                   "Request Timeout - message expired"
//	@Router       /tx/{txGroup}/{txName} [post]
//
//nolint:gocognit
func PostTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, verify SignatureVerification,
) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		msgType, ok := msgs[ctx.Params("group")][ctx.Params("name")]
		if !ok {
			return fiber.NewError(fiber.StatusNotFound, "message type not found")
		}

		// Parse the request body into a sign.Transaction struct
		tx := new(Transaction)
		if err := ctx.BodyParser(tx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to parse request body: "+err.Error())
		}
		if !verify.IsDisabled { //nolint: nestif // I'm okay with this - EdZ
			txEarliestValidCreateTime := time.Now().Add(-(time.Duration(verify.MessageExpirationSeconds) * time.Second))
			txCreated := time.UnixMicro(tx.Created)
			// before we even create the hash or validate the signature, check to see if the message has expired
			if txCreated.Before(txEarliestValidCreateTime) {
				return fiber.NewError(fiber.StatusRequestTimeout, fmt.Sprintf(
					"message more than %d seconds old", verify.MessageExpirationSeconds))
			}

			// if the hash was sent with the message, check that it isn't already in the cache
			// this saves us the cost of calculating the hash if there's an early lookup
			hashReceived := false
			if !sign.IsZeroHash(tx.Hash) {
				if found, err := isHashInCache(tx.Hash, verify.Cache); found {
					// if found in the cache, the message hash has already been used, so reject it
					return fiber.NewError(fiber.StatusForbidden, fmt.Sprintf(
						"already handled message %s", tx.Hash.String()))
				} else {
					return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf(
						"unexpected error %s from cache fetch for %s", err.Error(), tx.Hash.String()))
				}
				hashReceived = true
			}
			// generate the hash and check it
			receivedHashValue := tx.Hash
			tx.PopulateHash()
			if hashReceived {
				// we got a hash with the message, check that the generated one hasn't changed
				if tx.Hash != receivedHashValue {
					return fiber.NewError(fiber.StatusForbidden, fmt.Sprintf(
						"sent hash does not match %s", tx.Hash.String()))
				}
				// at this point we know the generated hash matches the received one, and is not in the cache,
				// so this message is not a replay
			} else {
				// we didn't receive a hash, so check to see if our generated hash is in the cache
				if found, err := isHashInCache(tx.Hash, verify.Cache); found {
					// if found in the cache, the message hash has already been used, so reject it
					return fiber.NewError(fiber.StatusForbidden, fmt.Sprintf(
						"already handled message %s", tx.Hash.String()))
				} else {
					return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf(
						"unexpected error %s from cache fetch for %s", err.Error(), tx.Hash.String()))
				}
				// at this point we know that the generated hash is not in the cache, so this message is not a replay
			}
		}

		// Validate the transaction
		if err := validateTx(tx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid transaction payload: "+err.Error())
		}

		// Decode the message from the transaction
		msg, err := msgType.Decode(tx.Body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to decode message from transaction")
		}

		if !verify.IsDisabled {
			var signerAddress string
			if msgType.Name() == personaMsg.CreatePersonaMessageName {
				// don't need to check the cast bc we already validated this above
				createPersonaMsg, _ := msg.(personaMsg.CreatePersona)
				signerAddress = createPersonaMsg.SignerAddress
			}

			if err = lookupSignerAndValidateSignature(world, signerAddress, tx); err != nil {
				return fiber.NewError(fiber.StatusUnauthorized, fmt.Sprintf(
					"Signature validation failed for message %s. %s", tx.Hash.String(), err.Error()))
			}

			// the message was valid, so add its hash to the cache
			// we don't do this until we have verified the signature to prevent an attack where someone sends
			// large numbers of hashes with unsigned/invalid messages and thus blocks legit messages from
			// being handled
			err = verify.Cache.Set(tx.Hash.Bytes(), nil, verify.MessageExpirationSeconds+cacheRetentionExtraSeconds)
			if err != nil {
				// if we couldn't store the hash in the cache, don't process the transaction, since that
				// would open us up to replay attacks
				return fiber.NewError(fiber.StatusInternalServerError, fmt.Sprintf(
					"unexpected error %s from cache store for %s", err.Error(), tx.Hash.String()))
			}
		}

		// Add the transaction to the engine
		// TODO(scott): this should just deal with txpool instead of having to go through engine
		tick, hash := world.AddTransaction(msgType.ID(), msg, tx)

		return ctx.JSON(&PostTransactionResponse{
			TxHash: string(hash),
			Tick:   tick,
		})
	}
}

// NOTE: duplication for cleaner swagger docs
// PostTransaction godoc
//
//	@Summary      Submits a transaction
//	@Description  Submits a transaction
//	@Accept       application/json
//	@Produce      application/json
//	@Param        txName  path      string                   true  "Name of a registered message"
//	@Param        txBody  body      Transaction              true  "Transaction details & message to be submitted"
//	@Success      200     {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400     {string}  string                   "Invalid request parameter"
//	@Failure      403     {string}  string                   "Forbidden"
//	@Failure      408     {string}  string                   "Request Timeout - message expired"
//	@Router       /tx/game/{txName} [post]
func PostGameTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, verify SignatureVerification,
) func(*fiber.Ctx) error {
	return PostTransaction(world, msgs, verify)
}

// NOTE: duplication for cleaner swagger docs
// PostTransaction godoc
//
//	@Summary      Creates a persona
//	@Description  Creates a persona
//	@Accept       application/json
//	@Produce      application/json
//	@Param        txBody  body      Transaction              true  "Transaction details & message to be submitted"
//	@Success      200     {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400     {string}  string                   "Invalid request parameter"
//	@Failure      401     {string}  string                   "Unauthorized - signature was invalid"
//	@Failure      403     {string}  string                   "Forbidden"
//	@Failure      408     {string}  string                   "Request Timeout - message expired"
//	@Failure      500     {string}  string                   "Internal Server Error - unexpected cache errors"
//	@Router       /tx/persona/create-persona [post]
func PostPersonaTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, verify SignatureVerification,
) func(*fiber.Ctx) error {
	return PostTransaction(world, msgs, verify)
}

func lookupSignerAndValidateSignature(world servertypes.ProviderWorld, signerAddress string, tx *Transaction) error {
	var err error
	if signerAddress == "" {
		signerAddress, err = world.GetSignerForPersonaTag(tx.PersonaTag, 0)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "could not get signer for persona: "+err.Error())
		}
	}
	if err = validateSignature(tx, signerAddress, world.Namespace(),
		tx.IsSystemTransaction()); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "failed to validate transaction: "+err.Error())
	}
	return nil
}

func isHashInCache(hash common.Hash, cache *freecache.Cache) (bool, error) {
	if _, err := cache.Get(hash.Bytes()); err == nil {
		// Hash is in the cache
		return true, nil
	} else if !errors.Is(err, freecache.ErrNotFound) {
		// Unexpected error
		return false, err
	}
	// Hash is not in the cache
	return false, nil
}

// validateTx validates the transaction payload
func validateTx(tx *Transaction) error {
	// TODO(scott): we should use the validator package here
	if tx.PersonaTag == "" {
		return ErrNoPersonaTag
	}
	return nil
}

// validateSignature validates that the signature of transaction is valid
func validateSignature(tx *Transaction, signerAddr string, namespace string, systemTx bool) error {
	if tx.Namespace != namespace {
		return eris.Wrap(ErrWrongNamespace, fmt.Sprintf("expected %q got %q", namespace, tx.Namespace))
	}
	if systemTx && !tx.IsSystemTransaction() {
		return eris.Wrap(ErrSystemTransactionRequired, "")
	}
	if !systemTx && tx.IsSystemTransaction() {
		return eris.Wrap(ErrSystemTransactionForbidden, "")
	}
	return eris.Wrap(tx.Verify(signerAddr), "")
}
