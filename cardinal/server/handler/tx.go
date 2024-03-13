package handler

import (
	"errors"
	"fmt"
	servertypes "pkg.world.dev/world-engine/cardinal/server/types"

	"github.com/gofiber/fiber/v2"
	"github.com/rotisserie/eris"
	"pkg.world.dev/world-engine/sign"

	personaMsg "pkg.world.dev/world-engine/cardinal/persona/msg"
	"pkg.world.dev/world-engine/cardinal/types"
)

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

type Transaction = sign.Transaction

// PostTransaction godoc
//
//	@Summary		Submit a transaction to Cardinal
//	@Description	Submit a transaction to Cardinal
//	@Accept			application/json
//	@Produce		application/json
//	@Param			txType	path		string	true	"label of the transaction that wants to be submitted"
//	@Success		200		{object}	PostTransactionResponse
//	@Failure		400		{string}	string	"Invalid transaction request"
//	@Router			/tx/game/{txType} [post]
//
// PostTransaction with Persona godoc
//
//	@Summary		Create a Persona transaction to Cardinal
//	@Description	Create a Persona transaction to Cardinal
//	@Accept			application/json
//	@Produce		application/json
//	@Param			txBody	body		Transaction	true	"Transaction details"
//	@Success		200		{object}	PostTransactionResponse
//	@Failure		400		{string}	string	"Invalid transaction request"
//	@Router			/tx/persona/create-persona [post]
func PostTransaction(
	provider servertypes.Provider, msgs map[string]map[string]types.Message, disableSigVerification bool,
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

		// Validate the transaction
		if err := validateTx(tx); err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "invalid transaction payload: "+err.Error())
		}

		// Decode the message from the transaction
		msg, err := msgType.Decode(tx.Body)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to decode message from transaction")
		}

		if !disableSigVerification {
			var signerAddress string
			// TODO(scott): don't hardcode this
			if msgType.Name() == "create-persona" {
				// don't need to check the cast bc we already validated this above
				createPersonaMsg, _ := msg.(personaMsg.CreatePersona)
				signerAddress = createPersonaMsg.SignerAddress
			}

			if err = lookupSignerAndValidateSignature(provider, signerAddress, tx); err != nil {
				return err
			}
		}

		// Add the transaction to the engine
		// TODO(scott): this should just deal with txpool instead of having to go through engine
		tick, hash := provider.AddTransaction(msgType.ID(), msg, tx)

		return ctx.JSON(&PostTransactionResponse{
			TxHash: string(hash),
			Tick:   tick,
		})
	}
}

func lookupSignerAndValidateSignature(provider servertypes.Provider, signerAddress string, tx *Transaction) error {
	var err error
	if signerAddress == "" {
		signerAddress, err = provider.GetSignerForPersonaTag(tx.PersonaTag, 0)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "could not get signer for persona: "+err.Error())
		}
	}
	if err = validateSignature(tx, signerAddress, provider.Namespace(),
		tx.IsSystemTransaction()); err != nil {
		return fiber.NewError(fiber.StatusBadRequest, "failed to validate transaction: "+err.Error())
	}
	// TODO(scott): this should be refactored; it should be the responsibility of the engine tx processor
	//  to mark the nonce as used once it's included in the tick, not the server.
	if err = provider.UseNonce(signerAddress, tx.Nonce); err != nil {
		return fiber.NewError(fiber.StatusInternalServerError, "failed to use nonce: "+err.Error())
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
