package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"
	"github.com/rotisserie/eris"

	personaMsg "pkg.world.dev/world-engine/cardinal/persona/msg"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"
	"pkg.world.dev/world-engine/cardinal/server/validator"
	"pkg.world.dev/world-engine/cardinal/types"
	"pkg.world.dev/world-engine/sign"
)

// PostTransactionResponse is the HTTP response for a successful transaction submission
type PostTransactionResponse struct {
	TxHash string
	Tick   uint64
}

// PostTransaction godoc
//
//	@Summary      Submits a transaction
//	@Description  Submits a transaction
//	@Accept       application/json
//	@Produce      application/json
//	@Param        txGroup  path      string                   true  "Message group"
//	@Param        txName   path      string                   true  "Name of a registered message"
//	@Param        txBody   body      sign.Transaction         true  "Transaction details & message to be submitted"
//	@Success      200      {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400      {string}  string                   "Invalid request parameter"
//	@Failure      403      {string}  string                   "Forbidden"
//	@Failure      408      {string}  string                   "Request Timeout - message expired"
//	@Router       /tx/{txGroup}/{txName} [post]
func PostTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, validator *validator.SignatureValidator,
) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		msgType, ok := msgs[ctx.Params("group")][ctx.Params("name")]
		if !ok {
			log.Errorf("Unknown msg type: %s", ctx.Params("name"))
			return fiber.NewError(fiber.StatusNotFound, "Not Found - bad msg type")
		}

		// extract the transaction from the fiber context
		tx, err := extractTx(ctx, validator)
		if err != nil {
			return err
		}

		// make sure the transaction hasn't expired
		if err = validator.ValidateTransactionTTL(tx); err != nil {
			return httpResultFromError(err, false)
		}

		// Decode the message from the transaction
		msg, err := msgType.Decode(tx.Body)
		if err != nil {
			log.Error("message %s Decode failed: %v", tx.Hash.String(), err)
			return fiber.NewError(fiber.StatusBadRequest, "Bad Request - failed to decode tx message")
		}

		// there's a special case for the CreatePersona message
		var signerAddress string
		if msgType.Name() == personaMsg.CreatePersonaMessageName {
			createPersonaMsg, ok := msg.(personaMsg.CreatePersona)
			if !ok {
				return fiber.NewError(fiber.StatusInternalServerError, "Internal Server Error - bad message type")
			}
			signerAddress = createPersonaMsg.SignerAddress
		}

		// Validate the transaction's signature
		if err = validator.ValidateTransactionSignature(tx, signerAddress); err != nil {
			return httpResultFromError(err, true)
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
//	@Param        txBody  body      sign.Transaction         true  "Transaction details & message to be submitted"
//	@Success      200     {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400     {string}  string                   "Invalid request parameter"
//	@Failure      403     {string}  string                   "Forbidden"
//	@Failure      408     {string}  string                   "Request Timeout - message expired"
//	@Router       /tx/game/{txName} [post]
func PostGameTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, validator *validator.SignatureValidator,
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
//	@Param        txBody  body      sign.Transaction         true  "Transaction details & message to be submitted"
//	@Success      200     {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400     {string}  string                   "Invalid request parameter"
//	@Failure      401     {string}  string                   "Unauthorized - signature was invalid"
//	@Failure      403     {string}  string                   "Forbidden"
//	@Failure      408     {string}  string                   "Request Timeout - message expired"
//	@Failure      500     {string}  string                   "Internal Server Error - unexpected cache errors"
//	@Router       /tx/persona/create-persona [post]
func PostPersonaTransaction(
	world servertypes.ProviderWorld, msgs map[string]map[string]types.Message, validator *validator.SignatureValidator,
) func(*fiber.Ctx) error {
	return PostTransaction(world, msgs, validator)
}

func extractTx(ctx *fiber.Ctx, validator *validator.SignatureValidator) (*sign.Transaction, error) {
	var tx *sign.Transaction
	var err error
	// Parse the request body into a sign.Transaction struct tx := new(Transaction)
	// this also calculates the hash
	if validator != nil && !validator.IsDisabled {
		// we are doing signature verification, so use sign's Unmarshal which does extra checks
		tx, err = sign.UnmarshalTransaction(ctx.Body())
	} else {
		// we aren't doing signature verification, so just use the generic body parser with is more forgiving
		tx = new(sign.Transaction)
		err = ctx.BodyParser(tx)
	}
	if err != nil {
		log.Errorf("body parse failed: %v", err)
		return nil, eris.Wrap(err, "Bad Request - unparseable body")
	}
	return tx, nil
}

// turns the various errors into an appropriate HTTP result
func httpResultFromError(err error, isSignatureValidation bool) error {
	log.Error(err) // log the private internal details
	if eris.Is(err, validator.ErrDuplicateMessage) {
		return fiber.NewError(fiber.StatusForbidden, "Forbidden - duplicate message")
	}
	if eris.Is(err, validator.ErrMessageExpired) {
		return fiber.NewError(fiber.StatusRequestTimeout, "Request Timeout - message expired")
	}
	if eris.Is(err, validator.ErrBadTimestamp) {
		return fiber.NewError(fiber.StatusBadRequest, "Bad Request - bad timestamp")
	}
	if eris.Is(err, validator.ErrNoPersonaTag) {
		return fiber.NewError(fiber.StatusBadRequest, "Bad Request - no persona tag")
	}
	if eris.Is(err, validator.ErrInvalidSignature) {
		return fiber.NewError(fiber.StatusUnauthorized, "Unauthorized - signature validation failed")
	}
	if isSignatureValidation {
		return fiber.NewError(fiber.StatusInternalServerError, "Internal Server Error - signature validation failed")
	}
	return fiber.NewError(fiber.StatusInternalServerError, "Internal Server Error - ttl validation failed")
}
