package handler

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/v2/types/message"
	"pkg.world.dev/world-engine/cardinal/v2/world"
	"pkg.world.dev/world-engine/sign"
)

// PostTransactionResponse is the HTTP response for a successful transaction submission
type PostTransactionResponse struct {
	TxHash common.Hash `json:"txHash"`
}

// PostTransaction godoc
//
//	@Summary      Submits a transaction
//	@Description  Submits a transaction
//	@Accept       application/json
//	@Produce      application/json
//	@Param        group  path      string                   true  "Message group"
//	@Param        name   path      string                   true  "Name of a registered message"
//	@Param        body   body      sign.Transaction              true  "Transaction details & message to be submitted"
//	@Success      200      {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400      {string}  string                   "Invalid request parameter"
//	@Router       /tx/{group}/{name} [post]
func PostTransaction(w *world.World) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		tx := new(sign.Transaction)
		if err := ctx.BodyParser(tx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to parse request body: "+err.Error())
		}

		group := ctx.Params("group")
		name := ctx.Params("name")

		var msgName string
		if group == message.DefaultGroup {
			msgName = name
		} else {
			msgName = group + "." + name
		}

		txHash, err := w.AddTransaction(msgName, tx)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to submit transaction: "+err.Error())
		}

		return ctx.JSON(PostTransactionResponse{
			TxHash: txHash,
		})
	}
}

// -----------------------------------------------------------------------------
// For Swagger Docs
// -----------------------------------------------------------------------------

// PostGameTransaction godoc
//
//	@Summary      Submits a transaction
//	@Description  Submits a transaction
//	@Accept       application/json
//	@Produce      application/json
//	@Param        txName  path      string                   true  "Name of a registered message"
//	@Param        txBody  body      sign.Transaction              true  "Transaction details & message to be submitted"
//	@Success      200     {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400     {string}  string                   "Invalid request parameter"
//	@Router       /tx/game/{txName} [post]
func PostGameTransaction(w *world.World) func(*fiber.Ctx) error {
	return PostTransaction(w)
}

// PostPersonaTransaction godoc
//
//	@Summary      Creates a persona
//	@Description  Creates a persona
//	@Accept       application/json
//	@Produce      application/json
//	@Param        txBody  body      sign.Transaction              true  "Transaction details & message to be submitted"
//	@Success      200     {object}  PostTransactionResponse  "Transaction hash and tick"
//	@Failure      400     {string}  string                   "Invalid request parameter"
//	@Router       /tx/persona/create-persona [post]
func PostPersonaTransaction(w *world.World) func(*fiber.Ctx) error {
	return PostTransaction(w)
}
