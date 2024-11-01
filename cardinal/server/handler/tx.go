package handler

import (
	"errors"
	"fmt"
	"time"

	"github.com/coocood/freecache"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"

	personaMsg "pkg.world.dev/world-engine/cardinal/persona/msg"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/server/validator"
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
type SignatureValidator = validator.SignatureValidator

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
func PostTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, validator *SignatureValidator,
) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		msgType, ok := msgs[ctx.Params("group")][ctx.Params("name")]
		if !ok {
			log.Errorf("Unknown msg type: %s", ctx.Params("name"))
			return fiber.NewError(fiber.StatusNotFound, "Not Found - bad msg type")
		}

		// extract the transaction from the fiber context
		tx, fiberErr := extractTx(ctx, validator)
		if fiberErr != nil {
			return fiberErr
		}

		// make sure the transaction hasn't expired
		if validationErr := validator.ValidateTransactionTTL(tx); validationErr != nil {
			log.Errorf(validationErr.GetLogMessage())                                   // log the private internal details
			return fiber.NewError(validationErr.GetStatusCode(), validationErr.Error()) // return public error result
		}

		// Decode the message from the transaction
		msg, err := msgType.Decode(tx.Body)
		if err != nil {
			log.Errorf("message %s Decode failed: %v", tx.Hash.String(), err)
			return fiber.NewError(fiber.StatusBadRequest, "Bad Request - failed to decode tx message")
		}

		// there's a special case for the CreatePersona message
		var signerAddress string
		if msgType.Name() == personaMsg.CreatePersonaMessageName {
			// don't need to check the cast bc we already validated this above
			createPersonaMsg, _ := msg.(personaMsg.CreatePersona)
			signerAddress = createPersonaMsg.SignerAddress
		}

		// Validate the transaction's signature
		if validationErr := validator.ValidateTransactionSignature(tx, signerAddress); validationErr != nil {
			log.Errorf(validationErr.GetLogMessage())                                   // log the private internal details
			return fiber.NewError(validationErr.GetStatusCode(), validationErr.Error()) // return public error result
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
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, validator *SignatureValidator,
) func(*fiber.Ctx) error {
	return PostTransaction(world, msgs, validator)
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
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, validator *SignatureValidator,
) func(*fiber.Ctx) error {
	return PostTransaction(world, msgs, validator)
}

func extractTx(ctx *fiber.Ctx, validator *SignatureValidator) (*sign.Transaction, *fiber.Error) {
	var tx *sign.Transaction
	var err error
	// Parse the request body into a sign.Transaction struct tx := new(Transaction)
	// this also calculates the hash
	if !validator.IsDisabled {
		// we are doing signature verification, so use sign's Unmarshal which does extra checks
		tx, err = sign.UnmarshalTransaction(ctx.Body())
	} else {
		// we aren't doing signature verification, so just use the generic body parser with is more forgiving
		tx = new(sign.Transaction)
		err = ctx.BodyParser(tx)
	}
	if err != nil {
		log.Errorf("body parse failed: %v", err)
		return nil, fiber.NewError(fiber.StatusBadRequest, "Bad Request - unparseable body")
	}
	if !verify.IsDisabled {
		txEarliestValidTimestamp := sign.TimestampAt(
			time.Now().Add(-(time.Duration(verify.MessageExpirationSeconds) * time.Second)))
		// before we even create the hash or validate the signature, check to see if the message has expired
		if tx.Timestamp < txEarliestValidTimestamp {
			log.Errorf("message older than %d seconds. Got timestamp: %d, current timestamp: %d ",
				verify.MessageExpirationSeconds, tx.Timestamp, sign.TimestampNow())
			return nil, fiber.NewError(fiber.StatusRequestTimeout, "Request Timeout - signature too old")
		}
		// check for duplicate message via hash cache
		if found, err := isHashInCache(tx.Hash, verify.Cache); err != nil {
			log.Errorf("unexpected cache error %v. message %s ignored", err, tx.Hash.String())
			return nil, fiber.NewError(fiber.StatusInternalServerError, "Internal Server Error - cache failed")
		} else if found {
			// if found in the cache, the message hash has already been used, so reject it
			log.Errorf("message %s already handled", tx.Hash.String())
			return nil, fiber.NewError(fiber.StatusForbidden, "Forbidden - duplicate message")
		}
		// at this point we know that the generated hash is not in the cache, so this message is not a replay
	}
	return tx, nil
}

func lookupSignerAndValidateSignature(world servertypes.ProviderWorld, signerAddress string, tx *Transaction) error {
	var err error
	if signerAddress == "" {
		signerAddress, err = world.GetSignerForPersonaTag(tx.PersonaTag, 0)
		if err != nil {
			return fmt.Errorf("could not get signer for persona %s: %w", tx.PersonaTag, err)
		}
	}
	if err = validateSignature(tx, signerAddress, world.Namespace(),
		tx.IsSystemTransaction()); err != nil {
		return fmt.Errorf("could not validate signature for persona %s: %w", tx.PersonaTag, err)
	}
	return nil
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
