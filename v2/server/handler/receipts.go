package handler

import (
	"github.com/ethereum/go-ethereum/common"
	"github.com/goccy/go-json"
	"github.com/gofiber/fiber/v2"

	"pkg.world.dev/world-engine/cardinal/v2/world"
)

type GetReceiptsRequest struct {
	TxHashes []common.Hash `json:"txHashes"`
}

// GetReceiptsResponse returns the transaction receipts for the given range of ticks. The interval is closed on
// StartTick and open on EndTick: i.e. [StartTick, EndTick)
// Meaning StartTick is included and EndTick is not. To iterate over all ticks in the future, use the returned
// EndTick as the StartTick in the next request. If StartTick == EndTick, the receipts list will be empty.
type GetReceiptsResponse struct {
	Receipts map[common.Hash]json.RawMessage `json:"receipts"`
}

// GetReceipts godoc
//
//	@Summary      Retrieves all transaction receipts
//	@Description  Retrieves all transaction receipts
//	@Accept       application/json
//	@Produce      application/json
//	@Param        GetReceiptsRequest  body      GetReceiptsRequest  true  "Query body"
//	@Success      200                    {object}  GetReceiptsResponse "List of receipts"
//	@Failure      400                    {string}  string                 "Invalid request body"
//	@Router       /query/receipts/list [post]
func GetReceipts(w *world.World) func(*fiber.Ctx) error {
	return func(ctx *fiber.Ctx) error {
		req := new(GetReceiptsRequest)
		if err := ctx.BodyParser(req); err != nil {
			return err
		}

		receipts, err := w.GetReceiptsBytes(req.TxHashes)
		if err != nil {
			return fiber.NewError(fiber.StatusBadRequest, "failed to get receipts: "+err.Error())
		}

		return ctx.JSON(&GetReceiptsResponse{Receipts: receipts})
	}
}
