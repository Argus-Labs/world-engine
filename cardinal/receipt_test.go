package cardinal

import (
	"github.com/golang/mock/gomock"
	"pkg.world.dev/world-engine/cardinal/types/engine/mocks"
	"testing"
)

func TestReceiptLogic(t *testing.T) {
	ctrl := gomock.NewController(t)
	ctx := mocks.NewMockContext(ctrl)

	res, err := queryReceipts(ctx, &ListTxReceiptsRequest{})
}
