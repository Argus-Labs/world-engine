package handler

import (
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/log"

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
			log.Errorf(validationErr.GetInternalMessage())                              // log the private internal details
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
			log.Errorf(validationErr.GetInternalMessage())                              // log the private internal details
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

func extractTx(ctx *fiber.Ctx, validate *SignatureValidator) (*sign.Transaction, *fiber.Error) {
	var tx *sign.Transaction
	var err error
	// Parse the request body into a sign.Transaction struct tx := new(Transaction)
	// this also calculates the hash
	if !validate.IsDisabled {
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
	return tx, nil
}
